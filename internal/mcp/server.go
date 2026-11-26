package mcp

// Unix socket listener, bridge handshake, and per-connection MCP session.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// BridgeHelloLine is the first line sent by voxtmini-ctl mcp / SelfTest.
const BridgeHelloLine = `{"app":"voxtmini","v":1}`

// BridgeHello is the handshake line written by the stdio bridge before JSON-RPC.
type BridgeHello struct {
	App      string `json:"app,omitempty"`
	Instance string `json:"instance,omitempty"`
	V        int    `json:"v,omitempty"`
	Bridge   int    `json:"bridge,omitempty"` // compatibility alias
}

// Options configures a Server.
type Options struct {
	Host      Host
	Prompter  Prompter  // asks the user to approve agents/actions
	Inspector Inspector // nil: SystemInspector()
	Audit     Audit     // nil: no audit records
	Version   string
	// Bridges lists executable paths allowed to connect as the stdio bridge.
	// Defaults to os.Executable() and Contents/Resources/ctl/voxtmini-ctl.
	Bridges []string
	// AllowedAudioRoots restricts transcribe_audio to files under these dirs.
	// Defaults to [os.UserHomeDir(), os.TempDir()].
	AllowedAudioRoots []string
	// Now is the clock (nil: time.Now).
	Now func() time.Time
	// AskTimeout bounds user confirmation prompts (0: 60s).
	AskTimeout time.Duration
}

// Server serves MCP agents over a local Unix domain socket.
type Server struct {
	opt  Options
	gate *gate
	lim  *limiter

	mu        sync.Mutex
	ln        net.Listener
	path      string
	conns     map[*conn]struct{}
	wg        sync.WaitGroup
	refusedAt map[string]time.Time
}

// New creates a Server; call Listen to begin accepting connections.
func New(opt Options) *Server {
	if opt.Inspector == nil {
		opt.Inspector = SystemInspector()
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.AskTimeout == 0 {
		opt.AskTimeout = time.Minute
	}
	if opt.Bridges == nil {
		if exe, err := os.Executable(); err == nil {
			contents := filepath.Dir(filepath.Dir(exe))
			opt.Bridges = []string{
				exe,
				filepath.Join(contents, "Resources", "ctl", "voxtmini-ctl"),
			}
		}
	}
	if len(opt.AllowedAudioRoots) == 0 {
		var roots []string
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			roots = append(roots, home)
		}
		if tmp := os.TempDir(); tmp != "" {
			roots = append(roots, tmp)
		}
		opt.AllowedAudioRoots = roots
	}
	s := &Server{
		opt:   opt,
		conns: map[*conn]struct{}{},
	}
	s.gate = newGate(s)
	s.lim = newLimiter(opt.Now)
	return s
}

// Listen binds a Unix socket at path (0600 inside a 0700 directory).
// A stale socket with no active listener is removed before binding.
func (s *Server) Listen(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	if c, err := net.DialTimeout("unix", path, 300*time.Millisecond); err == nil {
		c.Close()
		return fmt.Errorf("another VoxtMini serves MCP at %s", path)
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return err
	}
	s.ln, s.path = ln, path
	s.wg.Add(1)
	go s.accept(ln)
	log.Printf("mcp: listening at %s", path)
	return nil
}

// Close stops accepting connections, closes active sessions, and removes the socket.
func (s *Server) Close() error {
	s.mu.Lock()
	ln := s.ln
	s.ln = nil
	conns := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	path := s.path
	s.mu.Unlock()
	if ln == nil {
		return nil
	}
	_ = ln.Close()
	for _, c := range conns {
		c.close()
	}
	s.wg.Wait()
	_ = os.Remove(path)
	if flusher, ok := s.opt.Audit.(interface{ Flush() }); ok {
		flusher.Flush()
	}
	log.Printf("mcp: stopped")
	return nil
}

// Serving reports whether the server is actively listening.
func (s *Server) Serving() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ln != nil
}

func (s *Server) accept(ln net.Listener) {
	defer s.wg.Done()
	for {
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		c := &conn{srv: s, nc: nc, since: s.opt.Now()}
		s.mu.Lock()
		s.conns[c] = struct{}{}
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			c.serve()
			s.mu.Lock()
			delete(s.conns, c)
			s.mu.Unlock()
			s.notifyChanged()
		}()
	}
}

func (s *Server) notifyChanged() {
	if s.opt.Host != nil {
		s.opt.Host.Changed()
	}
}

// Live returns the currently connected external clients, deduplicated by key.
func (s *Server) Live() []Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	var out []Client
	for c := range s.conns {
		cl := c.who()
		if cl.Key == "" || cl.Self || seen[cl.Key] {
			continue
		}
		seen[cl.Key] = true
		out = append(out, cl)
	}
	return out
}

// Disconnect closes all active connections belonging to clientKey.
func (s *Server) Disconnect(clientKey string) {
	s.mu.Lock()
	var cs []*conn
	for c := range s.conns {
		if c.who().Key == clientKey {
			cs = append(cs, c)
		}
	}
	s.mu.Unlock()
	for _, c := range cs {
		c.close()
	}
}

const maxLine = 1 << 20

type conn struct {
	srv   *Server
	nc    net.Conn
	since time.Time

	mu      sync.Mutex
	client  Client
	sess    *sdk.ServerSession
	closed  bool
	session *sdk.Server
	listed  map[string]bool
}

func (c *conn) who() Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

func (c *conn) close() {
	c.mu.Lock()
	c.closed = true
	sess := c.sess
	c.mu.Unlock()
	if sess != nil {
		_ = sess.Close()
	}
	_ = c.nc.Close()
}

func (c *conn) serve() {
	defer c.nc.Close()
	s := c.srv
	br := bufio.NewReaderSize(c.nc, 64<<10)
	_ = c.nc.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := br.ReadBytes('\n')
	var hello BridgeHello
	if err != nil || json.Unmarshal(line, &hello) != nil || (hello.V < 1 && hello.Bridge < 1) {
		return
	}
	_ = c.nc.SetReadDeadline(time.Time{})

	client, err := identify(s.opt.Inspector, c.nc, s.opt.Bridges, os.Getuid())
	if err == nil && !s.opt.Host.Policy().Enabled && !client.Self {
		err = ErrOff
	}
	if err == nil {
		err = s.opt.Host.Ready()
	}
	if err != nil {
		log.Printf("mcp: refused connection: %v", err)
		s.auditRefused(client, err)
		refuse(br, c.nc, err)
		return
	}

	c.mu.Lock()
	c.client = client
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return
	}

	srv := s.newSession(c)
	sess, err := srv.Connect(context.Background(), &sdk.IOTransport{
		Reader:        readCloser{br, c.nc},
		Writer:        c.nc,
		MaxLineLength: maxLine,
	}, nil)
	if err != nil {
		log.Printf("mcp: session error: %v", err)
		return
	}
	c.mu.Lock()
	c.sess = sess
	c.mu.Unlock()
	s.notifyChanged()
	_ = sess.Wait()
}

func (s *Server) auditRefused(cl Client, err error) {
	if s.opt.Audit == nil || cl.Self {
		return
	}
	name := cl.Display()
	if cl.Key == "" {
		name = "?"
	}
	now := s.opt.Now()
	key := cl.Key + "\x00" + err.Error()
	s.mu.Lock()
	if s.refusedAt == nil {
		s.refusedAt = map[string]time.Time{}
	}
	if at, ok := s.refusedAt[key]; ok && now.Sub(at) < time.Minute {
		s.mu.Unlock()
		return
	}
	s.refusedAt[key] = now
	s.mu.Unlock()
	s.opt.Audit.Record(context.Background(), CallRecord{
		At:         now,
		ClientKey:  cl.Key,
		ClientName: name,
		Tool:       "connect",
		Result:     Refused,
	})
	s.notifyChanged()
}

type readCloser struct {
	*bufio.Reader
	c io.Closer
}

func (r readCloser) Close() error { return r.c.Close() }

// refuse answers the first JSON-RPC request on a rejected connection
// with a standard error response, then closes the connection.
func refuse(br *bufio.Reader, w io.Writer, err error) {
	if nc, ok := w.(net.Conn); ok {
		_ = nc.SetDeadline(time.Now().Add(10 * time.Second))
	}
	line, rerr := br.ReadBytes('\n')
	if rerr != nil && len(line) == 0 {
		return
	}
	var req struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(line, &req) != nil || len(req.ID) == 0 {
		return
	}
	out, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"error":   map[string]any{"code": -32603, "message": err.Error()},
	})
	_, _ = w.Write(append(out, '\n'))
}
