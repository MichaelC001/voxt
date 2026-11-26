package mcp

// Recording session trigger and status tools (require Policy.AllowTrigger).

import "strings"

func triggerTools() []tool {
	tStatus := def("session_status", "Get current recording session state, active microphone, and latest result.", ScopeTrigger,
		func(c *call, _ struct{}) (any, error) {
			st, err := c.srv.opt.Host.SessionStatus(c.ctx)
			if err != nil {
				return nil, err
			}
			c.count = 1
			return st, nil
		})
	tStatus.readOnly = true

	return []tool{
		def("trigger_session", "Start, stop, toggle, or cancel a VoxtMini voice session, optionally selecting a microphone.", ScopeTrigger,
			func(c *call, in struct {
				Feature        string `json:"feature" jsonschema:"Feature: transcribe, translate, or rewrite"`
				Action         string `json:"action" jsonschema:"Action: start, stop, toggle, or cancel"`
				Device         string `json:"device,omitempty" jsonschema:"Optional microphone UID or name"`
				TargetLanguage string `json:"target_language,omitempty" jsonschema:"Optional target language for translate: en, zh-Hans, ja"`
				Deliver        string `json:"deliver,omitempty" jsonschema:"Optional delivery: cursor, clipboard, or card"`
			}) (any, error) {
				feat := strings.ToLower(strings.TrimSpace(in.Feature))
				if feat != "transcribe" && feat != "translate" && feat != "rewrite" {
					return nil, fail("bad_arguments", "feature must be transcribe, translate, or rewrite")
				}
				act := strings.ToLower(strings.TrimSpace(in.Action))
				if act != "start" && act != "stop" && act != "toggle" && act != "cancel" {
					return nil, fail("bad_arguments", "action must be start, stop, toggle, or cancel")
				}
				deliv := strings.ToLower(strings.TrimSpace(in.Deliver))
				if deliv != "" && deliv != "cursor" && deliv != "clipboard" && deliv != "card" {
					return nil, fail("bad_arguments", "deliver must be cursor, clipboard, or card")
				}
				lang := ""
				if strings.TrimSpace(in.TargetLanguage) != "" {
					var ok bool
					lang, ok = normalizeTargetLang(in.TargetLanguage)
					if !ok {
						return nil, fail("bad_arguments", "target_language must be en, zh, zh-Hans, or ja")
					}
				}
				if err := c.srv.gate.confirm(c, AskTrigger, "Trigger VoxtMini recording session ("+feat+" "+act+")", 1, false); err != nil {
					return nil, err
				}
				res, err := c.srv.opt.Host.TriggerSession(c.ctx, TriggerRequest{
					Feature:        feat,
					Action:         act,
					Device:         strings.TrimSpace(in.Device),
					TargetLanguage: lang,
					Deliver:        deliv,
				})
				if err != nil {
					return nil, err
				}
				c.count = 1
				return res, nil
			}),

		tStatus,
	}
}
