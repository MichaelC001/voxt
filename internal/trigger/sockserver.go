package trigger

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hehehai/VoxtMini/internal/ctlbridge"
)

// SocketServer serves ctlbridge.ControlRequest over a local Unix socket
// (0600 in a 0700 directory).
type SocketServer struct {
	svc  *Service
	path string
	ln   net.Listener
	log  func(level, msg string)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Listen starts a SocketServer on ctlbridge.ControlSocketPath(home, cache).
func Listen(home, cache string, svc *Service, log func(level, msg string)) (*SocketServer, error) {
	if log == nil {
		log = func(string, string) {}
	}
	path := ctlbridge.ControlSocketPath(home, cache)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	_ = os.Chmod(dir, 0o700)
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &SocketServer{svc: svc, path: path, ln: ln, log: log, ctx: ctx, cancel: cancel}
	s.wg.Add(1)
	go s.acceptLoop()
	log("info", "control socket listening on "+path)
	return s, nil
}

// Path returns the Unix socket path.
func (s *SocketServer) Path() string { return s.path }

// Close stops accepting connections, cancels in-flight requests, and removes the socket.
func (s *SocketServer) Close() error {
	s.cancel()
	err := s.ln.Close()
	s.wg.Wait()
	_ = os.Remove(s.path)
	return err
}

func (s *SocketServer) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) && s.ctx.Err() == nil {
				s.log("warning", fmt.Sprintf("control socket accept: %v", err))
			}
			return
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer c.Close()
			s.handle(c)
		}(conn)
	}
}

func (s *SocketServer) handle(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 16<<10), 1<<20)
	if !sc.Scan() {
		return
	}
	var req ctlbridge.ControlRequest
	if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
		writeResp(conn, ctlbridge.ControlResponse{OK: false, Error: "invalid request JSON"})
		return
	}
	_ = conn.SetDeadline(time.Time{})

	var resp ctlbridge.ControlResponse
	switch req.Method {
	case "trigger":
		res, err := s.svc.Trigger(s.ctx, Request{
			Feature:        req.Feature,
			Action:         ParseAction(req.Action),
			Device:         req.Device,
			Fallback:       req.Fallback,
			WaitDeviceMS:   req.WaitDeviceMS,
			WaitDeviceSet:  req.WaitDeviceSet,
			TargetLanguage: req.TargetLanguage,
			Deliver:        req.Deliver,
			Source:         req.Source,
			Wait:           req.Wait,
		})
		resp = ctlbridge.ControlResponse{
			OK:        res.OK && err == nil,
			Error:     res.Error,
			SessionID: res.SessionID,
			Feature:   res.Feature,
			State:     res.State,
			Text:      res.Text,
			Delivered: res.Delivered,
			Debounced: res.Debounced,
			Device:    res.Device,
		}
		if err != nil && resp.Error == "" {
			resp.Error = err.Error()
		}
	case "devices":
		devs, err := s.svc.Devices(s.ctx)
		if err != nil {
			resp = ctlbridge.ControlResponse{OK: false, Error: err.Error()}
		} else {
			resp = ctlbridge.ControlResponse{OK: true, Devices: devs}
		}
	case "status":
		if !s.svc.cfg.Settings().Trigger.Enabled {
			resp = ctlbridge.ControlResponse{OK: false, Error: ErrDisabled.Error()}
		} else {
			st := s.svc.Status()
			resp = ctlbridge.ControlResponse{
				OK:        true,
				SessionID: st.SessionID,
				Feature:   st.Feature,
				State:     st.State,
				Text:      st.Text,
				Device:    st.Device,
			}
		}
	default:
		resp = ctlbridge.ControlResponse{OK: false, Error: fmt.Sprintf("unknown method %q", req.Method)}
	}
	writeResp(conn, resp)
}

func writeResp(conn net.Conn, resp ctlbridge.ControlResponse) {
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = conn.Write(append(b, '\n'))
}
