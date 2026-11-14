package main

// A language service's API and web search in the service dialog's
// Advanced section (remote_dialog.go; internal/llm's Responses transport
// and search): the API only for a service that offers both, the switch
// only where the service searches over the API chosen.

import (
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/providers"
	"github.com/hehehai/VoxtMini/internal/settings"
)

func (a *app) chatAPI(c *ui.Context, pr providers.Preset) {
	t := c.Theme()
	d := &a.rmDlg
	if pr.Responses {
		labels := []string{i18n.T("Chat Completions"), i18n.T("Responses")}
		sel := labels[0]
		if d.api == llm.APIResponses {
			sel = labels[1]
		}
		fieldInfo(c, i18n.T("API"), i18n.T("Responses is OpenAI's newer API. Some services search the web only through it."), func() {
			if ui.Select(c.Key("rm-api"), &sel, labels).Label(i18n.T("API")).Width(px(t, 320)).Changed() {
				d.api = ""
				if sel == labels[1] {
					d.api = llm.APIResponses
				}
			}
		})
	}
	if !providers.SearchSupported(settings.Provider{ID: d.id, API: d.api, Endpoint: d.endpoint}) {
		return
	}
	settingRow(c, "rm-search", i18n.T("Search the web when rewriting"),
		i18n.T("Rewrite may look up current facts on the web: slower, and it may cost more. Transcribe and Translate never search."), func() {
			ui.Switch(c.Key("rm-search-switch"), &d.search).Label(i18n.T("Search the web when rewriting"))
		})
}
