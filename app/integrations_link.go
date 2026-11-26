package main

// Integrations wiring: run/mcp.json status file, the external trigger
// engine (internal/trigger), the control Unix socket server
// (run/control.sock), and voxtmini://trigger/* URL handling without
// bringing the main window to the front.

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/ctlbridge"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/trigger"
)

const triggerSource = "trigger"

// integrationsState is the integrations area's state embedded in app.
type integrationsState struct {
	triggerSvc   *trigger.Service
	ctrlSrv      *trigger.SocketServer
	integTab     int // 0: MCP, 1: External trigger
	cacheDir     string
	resourcesDir string
	mcpState
}

// openIntegrations creates the trigger service, MCP server, and synchronises
// run/mcp.json and the local sockets with the current settings.
func (a *app) openIntegrations() {
	a.triggerSvc = trigger.New(trigger.Config{
		Settings:  a.settings.Get,
		Session:   a.mgr,
		Start:     a.startTriggerSession,
		Devices:   a.triggerDevices,
		Frontmost: a.triggerFrontmost,
		Log:       a.logTrigger,
	})
	a.openMCP()
	a.syncIntegrations()
}

func (a *app) logTrigger(level, msg string) {
	if a.logs == nil {
		return
	}
	a.logs.Append(core.LogEntry{
		Time:    time.Now(),
		Level:   core.Severity(level),
		Source:  triggerSource,
		Message: msg,
	})
}

// startTriggerSession starts a session on the main thread so blockers,
// notices and view state stay consistent with hotkey starts.
func (a *app) startTriggerSession(req session.StartRequest) (string, error) {
	type res struct {
		id  string
		err error
	}
	ch := make(chan res, 1)
	a.post(func() {
		if a.mgr == nil {
			ch <- res{err: errors.New("session manager unavailable")}
			return
		}
		if a.sess.State.Live() {
			ch <- res{err: session.ErrBusy}
			return
		}
		id, notice := a.startSessionID(req)
		if notice != nil {
			ch <- res{err: errors.New(notice.text)}
			return
		}
		if id == "" {
			ch <- res{err: session.ErrBusy}
			return
		}
		ch <- res{id: id}
	})
	r := <-ch
	return r.id, r.err
}

func (a *app) triggerDevices(ctx context.Context) ([]proto.CaptureDevice, error) {
	if a.host != nil && a.host.sup != nil && a.helperReady() {
		return session.HelperAudio{H: a.host.sup}.Devices(ctx)
	}
	ch := make(chan []proto.CaptureDevice, 1)
	a.post(func() { ch <- slices.Clone(a.audio.devices) })
	return <-ch, nil
}

func (a *app) triggerFrontmost(ctx context.Context) (proto.App, error) {
	if a.host != nil && a.host.sup != nil && a.helperReady() {
		ht := session.HelperText{H: a.host.sup}
		if front, err := ht.Frontmost(ctx); err == nil {
			return front, nil
		}
	}
	ch := make(chan proto.App, 1)
	a.post(func() { ch <- a.front })
	return <-ch, nil
}

// syncIntegrations writes run/mcp.json and starts or stops run/control.sock
// according to the current MCP and Trigger settings.
func (a *app) syncIntegrations() {
	home, cache := a.dataHome, a.cacheDir
	mcpOn := a.prefs.MCP.Enabled
	trigOn := a.prefs.Trigger.Enabled
	if home != "" {
		a.async(func() {
			if err := ctlbridge.WriteState(home, ctlbridge.State{MCP: mcpOn, Trigger: trigOn}); err != nil {
				a.logTrigger("warning", "writing run/mcp.json: "+err.Error())
			}
		})
	}
	if trigOn && home != "" && a.triggerSvc != nil {
		if a.ctrlSrv == nil {
			srv, err := trigger.Listen(home, cache, a.triggerSvc, a.logTrigger)
			if err != nil {
				a.logTrigger("warning", "starting control socket: "+err.Error())
			} else {
				a.ctrlSrv = srv
			}
		}
	} else if a.ctrlSrv != nil {
		srv := a.ctrlSrv
		a.ctrlSrv = nil
		a.async(func() { _ = srv.Close() })
	}
	a.syncMCP()
}

// closeIntegrations stops the control and MCP socket servers on app quit.
func (a *app) closeIntegrations() {
	a.closeMCP()
	if a.ctrlSrv != nil {
		_ = a.ctrlSrv.Close()
		a.ctrlSrv = nil
	}
}

// handleTriggerURL processes voxtmini://trigger/* without activating or
// navigating the main window so the frontmost app remains the target.
func (a *app) handleTriggerURL(raw string) {
	if !a.prefs.Trigger.Enabled {
		a.logTrigger("info", "ignored URL trigger (external trigger disabled): "+raw)
		a.toast(i18n.T("External trigger is turned off in Settings."))
		return
	}
	if !a.prefs.Trigger.AllowURLOn() {
		a.logTrigger("info", "ignored URL trigger (URL triggers disabled): "+raw)
		a.toast(i18n.T("URL triggers are turned off in Settings."))
		return
	}
	req, err := trigger.ParseURL(raw)
	if err != nil {
		a.logTrigger("warning", "invalid trigger URL: "+err.Error())
		a.toast(i18n.T("Invalid trigger link: %v", err))
		return
	}
	svc := a.triggerSvc
	if svc == nil {
		return
	}
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := svc.Trigger(ctx, req); err != nil {
			a.logTrigger("warning", "URL trigger failed: "+err.Error())
			a.post(func() { a.toast(err.Error()) })
		}
	})
}
