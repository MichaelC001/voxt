package trigger

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hehehai/VoxtMini/internal/session"
)

// ParseURL parses a voxtmini://trigger/<feature>?... URL into a Request.
// Accepted paths:
//   - voxtmini://trigger/transcribe?action=...&device=...&fallback=...&wait_device_ms=...&to=...&deliver=...&source=...
//   - voxtmini://trigger/translate?...
//   - voxtmini://trigger/rewrite?...
//   - voxtmini://trigger/stop[?source=...]
//   - voxtmini://trigger/cancel[?source=...]
func ParseURL(raw string) (Request, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Request{}, fmt.Errorf("invalid trigger URL: %w", err)
	}
	if u.Scheme != "" && !strings.EqualFold(u.Scheme, "voxtmini") {
		return Request{}, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	path := u.Path
	if u.Scheme != "" && u.Host != "" {
		path = "/" + u.Host + u.Path
	}
	path = "/" + strings.Trim(path, "/")
	rest, ok := strings.CutPrefix(path, "/trigger/")
	if !ok || rest == "" {
		return Request{}, fmt.Errorf("invalid trigger URL path %q", path)
	}
	seg := strings.ToLower(strings.Trim(rest, "/"))
	q := u.Query()
	req := Request{
		Device:         strings.TrimSpace(q.Get("device")),
		Fallback:       strings.ToLower(strings.TrimSpace(q.Get("fallback"))),
		TargetLanguage: strings.TrimSpace(q.Get("to")),
		Deliver:        strings.ToLower(strings.TrimSpace(q.Get("deliver"))),
		Source:         trimSource(q.Get("source")),
		ViaURL:         true,
	}
	switch seg {
	case "transcribe", "translate", "rewrite":
		req.Feature = seg
		act := strings.ToLower(strings.TrimSpace(q.Get("action")))
		if act == "" {
			req.Action = ActionToggle
		} else {
			req.Action = Action(act)
		}
	case "stop":
		req.Action = ActionStop
	case "cancel":
		req.Action = ActionCancel
	default:
		return Request{}, fmt.Errorf("unknown trigger target %q", seg)
	}
	if rawWait := strings.TrimSpace(q.Get("wait_device_ms")); rawWait != "" {
		ms, err := strconv.Atoi(rawWait)
		if err != nil {
			return Request{}, fmt.Errorf("invalid wait_device_ms %q", rawWait)
		}
		req.WaitDeviceMS = clampWaitMS(ms)
		req.WaitDeviceSet = true
	}
	if err := req.validate(); err != nil {
		return Request{}, err
	}
	return req, nil
}

func clampWaitMS(ms int) int {
	if ms < 0 {
		return 0
	}
	if ms > session.MaxDeviceWaitMS {
		return session.MaxDeviceWaitMS
	}
	return ms
}

func trimSource(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 64 {
		return s[:64]
	}
	return s
}
