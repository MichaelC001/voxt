package llmrt

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/llmrt/wire"
	"github.com/hehehai/VoxtMini/internal/logs"
	"github.com/hehehai/VoxtMini/internal/procio"
)

// proc is one voxtmini-llm process.
type proc struct {
	rt     *Runtime
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	tail   *procio.Tail
	exited chan struct{}
	ids    atomic.Int64

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[string]*call
	closed  bool
	// intentional: stopped on purpose (idle, Stop), not a failure.
	intentional atomic.Bool
	killReason  string
}

type call struct {
	method string
	notify func(wire.Message)
	done   chan struct{}
	resp   wire.Message
	err    error
}

func (c *call) result() (json.RawMessage, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.resp.Error != nil {
		return nil, c.resp.Error
	}
	return c.resp.Result, nil
}

// startProc spawns the process and its readers; onExit runs once it has
// ended and its calls have failed.
func (r *Runtime) startProc() (*proc, error) {
	cmd := exec.Command(r.cfg.Bin)
	cmd.Dir = filepath.Dir(r.cfg.Bin)
	cmd.Env = append(os.Environ(), r.cfg.Env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", r.cfg.Bin, err)
	}
	p := &proc{rt: r, cmd: cmd, stdin: stdin, tail: procio.NewTail(40), exited: make(chan struct{}), pending: map[string]*call{}}
	var logFile *logs.CappedFile
	if r.cfg.LogDir != "" {
		logFile = logs.OpenCapped(filepath.Join(r.cfg.LogDir, "llm.log"), 0)
	}
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			p.tail.Add(line)
			logFile.Println(logs.Redact(line)) // nil: no log file; redacted as the log ring does
			if strings.TrimSpace(line) != "" {
				r.log(procio.StderrLevel(line), line)
			}
		}
	}()
	go func() {
		p.read(stdout)
		<-stderrDone
		err := cmd.Wait()
		logFile.Close()
		p.end(err)
	}()
	return p, nil
}

// read routes stdout: responses to their calls, a job's notifications to
// its call, log lines to the log.
func (p *proc) read(stdout io.Reader) {
	br := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, err := procio.ReadLine(br, wire.MaxLine)
		if len(line) > 0 {
			p.handle(line)
		}
		if err != nil {
			if err != io.EOF {
				p.tail.Add("stdout: " + err.Error())
			}
			io.Copy(io.Discard, br)
			return
		}
	}
}

func (p *proc) handle(line []byte) {
	m, err := wire.Decode(line)
	if err != nil {
		p.tail.Add("stdout: " + string(line))
		return
	}
	switch m.Kind() {
	case wire.KindResponse:
		p.mu.Lock()
		c := p.pending[m.ID]
		delete(p.pending, m.ID)
		p.mu.Unlock()
		if c != nil {
			c.resp = m
			close(c.done)
		}
	case wire.KindNotification:
		var ref struct {
			Job string `json:"job"`
		}
		m.DecodeParams(&ref)
		if m.Method == wire.MethodLog {
			var lp wire.LogParams
			if m.DecodeParams(&lp) == nil {
				p.rt.log(severity(lp.Level), lp.Message)
			}
			return
		}
		if ref.Job == "" {
			return
		}
		p.mu.Lock()
		c := p.pending[ref.Job]
		p.mu.Unlock()
		if c != nil && c.notify != nil {
			c.notify(m)
		}
	}
}

// end fails the process's calls and tells the runtime.
func (p *proc) end(waitErr error) {
	p.mu.Lock()
	p.closed = true
	calls := p.pending
	p.pending = map[string]*call{}
	reason := p.killReason
	p.mu.Unlock()
	msg := "voxtmini-llm exited"
	switch {
	case reason != "":
		msg += ": " + reason
	case waitErr != nil:
		msg += ": " + waitErr.Error()
	}
	close(p.exited)
	// The runtime first, so that a failed call finds it without the
	// process (its status, the model unloaded).
	p.rt.exited(p, msg)
	for _, c := range calls {
		c.err = ErrExited
		close(c.done)
	}
}

func (p *proc) kill(reason string) {
	p.mu.Lock()
	if p.killReason == "" {
		p.killReason = reason
	}
	p.mu.Unlock()
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
}

func (p *proc) write(m wire.Message) error {
	line, err := wire.Encode(m)
	if err != nil {
		return err
	}
	if len(line) > wire.MaxLine {
		return fmt.Errorf("request of %d bytes is over the protocol's limit: %w", len(line), core.ErrInvalid)
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if _, err := p.stdin.Write(line); err != nil {
		return fmt.Errorf("writing to voxtmini-llm: %v: %w", err, ErrExited)
	}
	return nil
}

// call sends a request and waits for its response. notify receives a
// job's notifications on the reader goroutine; it must not block.
// Cancelling ctx sends job.cancel for a job and waits for its answer
// (killing the process after Config.CancelGrace); a control request is
// abandoned.
func (p *proc) call(ctx context.Context, method string, params any, notify func(wire.Message)) (json.RawMessage, error) {
	id := fmt.Sprintf("c%d", p.ids.Add(1))
	req, err := wire.NewRequest(id, method, params)
	if err != nil {
		return nil, fmt.Errorf("%s params: %w", method, err)
	}
	c := &call{method: method, notify: notify, done: make(chan struct{})}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrExited
	}
	p.pending[id] = c
	p.mu.Unlock()
	if err := p.write(req); err != nil {
		p.drop(id)
		return nil, err
	}
	select {
	case <-c.done:
		return c.result()
	case <-ctx.Done():
	}
	if !wire.IsJob(method) {
		p.drop(id)
		return nil, ctx.Err()
	}
	go func() {
		cctx, cancel := context.WithTimeout(context.Background(), p.rt.cfg.CancelGrace)
		defer cancel()
		p.call(cctx, wire.MethodJobCancel, wire.CancelParams{Job: id}, nil)
	}()
	grace := time.NewTimer(p.rt.cfg.CancelGrace)
	defer grace.Stop()
	select {
	case <-c.done:
	case <-grace.C:
		p.kill(fmt.Sprintf("chat %s did not stop within %s of being cancelled", id, p.rt.cfg.CancelGrace))
		<-c.done
	}
	if _, err := c.result(); err != nil {
		return nil, fmt.Errorf("%w (%v)", ctx.Err(), err)
	}
	return nil, ctx.Err()
}

func (p *proc) drop(id string) {
	p.mu.Lock()
	delete(p.pending, id)
	p.mu.Unlock()
}

func (p *proc) busy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pending) > 0
}

func severity(level string) core.Severity {
	switch s := core.Severity(level); s {
	case core.SeverityDebug, core.SeverityInfo, core.SeverityWarning, core.SeverityError:
		return s
	}
	return core.SeverityInfo
}
