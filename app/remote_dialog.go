package main

// The Remote models page's dialog (remote_page.go; Auralink's service
// dialog): a service's key, endpoint and model, its advanced parameters
// (Voxt's: the speech services' language and dictionary hints, the
// language models' temperature, top-p, output budget and extra body),
// with a connection test before saving.

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/lang"
	"github.com/hehehai/VoxtMini/internal/providers"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// providerDialog is the open service dialog's form.
type providerDialog struct {
	open               bool
	kind               providers.Kind
	id                 string
	exists             bool // the service was set up before
	name, key          string
	endpoint, appID    string
	model, customModel string // model: the select's label
	enabled            bool
	// The advanced parameters, as typed ("" the default).
	advanced                 bool // the section is open
	prompt, language         string
	hints                    bool
	temp, topP, maxTok, body string
	err, errMore             string // a save or check error
	saving, removeKey        bool
	test                     providerTest
	fam                      dialogFamily // the family's keys (family_keys.go)
	// A language model's API ("" its usual one, llm.APIResponses) and web
	// search while rewriting.
	api    string
	search bool
}

// otherModel is the model select's last choice: a model id typed in.
func otherModel() string { return i18n.T("Other model") }

// ---- The dialog ----

func (a *app) openProviderDialog(kind providers.Kind, id string) {
	pr, _ := providers.LookupPreset(kind, id)
	p, exists := providers.Setup(a.prefs, kind, id)
	d := providerDialog{open: true, kind: kind, id: id, exists: exists, enabled: !exists || p.Enabled,
		name: p.Name, endpoint: p.Endpoint, appID: p.AppID, prompt: p.Prompt, language: p.Language, hints: !p.NoHints,
		maxTok: intText(p.MaxTokens), temp: floatText(p.Temperature), topP: floatText(p.TopP), body: p.ExtraBody,
		api: p.API, search: p.WebSearch}
	model := p.Model
	if model == "" {
		model = pr.Model
	}
	d.model = model
	if model == "" || !slices.Contains(pr.Models, model) {
		d.model, d.customModel = otherModel(), model
	}
	d.advanced = d.prompt != "" || d.language != "" || !d.hints || d.temp != "" || d.topP != "" || d.maxTok != "" || d.body != "" ||
		d.api != "" || d.search
	acc := providers.Account(kind, id)
	// A new key is shared with the family by default (spec §8.3); a key
	// of its own saved before only when asked.
	d.fam = dialogFamily{ref: p.CredentialRef, share: p.CredentialRef != "" || !a.hasKey(acc)}
	a.rmDlg = d
	a.loadDialogFamily()
	if _, read := a.keyMask(acc); !read {
		a.loadRemoteKeys([]string{acc})
	}
}

func intText(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func floatText(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

// setup is the setup the dialog's form describes, or what is wrong with
// its advanced parameters ("" when they are fine).
func (d *providerDialog) setup() (settings.Provider, string) {
	pr, _ := providers.LookupPreset(d.kind, d.id)
	p := settings.Provider{ID: d.id, Enabled: d.enabled, Endpoint: strings.TrimSpace(d.endpoint), AppID: strings.TrimSpace(d.appID)}
	if pr.Custom {
		p.Name = strings.TrimSpace(d.name)
	}
	model := d.model
	if model == otherModel() {
		model = strings.TrimSpace(d.customModel)
	}
	if model != pr.Model {
		p.Model = model
	}
	if d.kind == providers.ASR {
		p.Prompt, p.Language, p.NoHints = strings.TrimSpace(d.prompt), d.language, !d.hints
		if !pr.Prompt {
			p.Prompt = ""
		}
		return p, ""
	}
	num := func(s string, lo, hi float64) (*float64, bool) {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, true
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v < lo || v > hi {
			return nil, false
		}
		return &v, true
	}
	var ok bool
	if p.Temperature, ok = num(d.temp, 0, 2); !ok {
		return p, i18n.T("Temperature is a number from 0 to 2.")
	}
	if p.TopP, ok = num(d.topP, 0, 1); !ok {
		return p, i18n.T("Top-p is a number from 0 to 1.")
	}
	if s := strings.TrimSpace(d.maxTok); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 16 || n > 200000 {
			return p, i18n.T("Max output tokens is a whole number from 16 to 200000.")
		}
		p.MaxTokens = n
	}
	p.ExtraBody = strings.TrimSpace(d.body)
	if pr.Responses {
		p.API = d.api
	}
	p.WebSearch = d.search && providers.SearchSupported(p)
	if _, err := providers.ExtraBody(p.ExtraBody); err != nil {
		return p, i18n.T("The extra body must be a JSON object, like {\"thinking\": {\"type\": \"disabled\"}}.")
	}
	return p, ""
}

func (a *app) providerDialogView(c *ui.Context) {
	d := &a.rmDlg
	if !d.open {
		if d.fam.joined { // closed unsaved after "Use this key"
			f := d.fam
			d.fam.joined = false
			a.dialogClosed(f)
		}
		return
	}
	t := c.Theme()
	pr, _ := providers.LookupPreset(d.kind, d.id)
	acc := providers.Account(d.kind, d.id)
	name := uitext.PresetName(pr)
	sub := i18n.T("Speech recognition · %s", uitext.ModeLabel(pr))
	if d.kind == providers.LLM {
		sub = i18n.T("Language model · %s", uitext.ModeLabel(pr))
	}
	dialogFrame(c, "rm-dialog", &d.open, name, sub, 560, func() {
		ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Wrap().Children(func() {
			what := i18n.T("When a feature uses it, your recordings are sent to %s to be recognised.", name)
			switch {
			case d.kind == providers.LLM && pr.Custom:
				what = i18n.T("When a feature uses it, recognised and selected text is sent to the server at the endpoint below.")
			case d.kind == providers.LLM:
				what = i18n.T("When a feature uses it, recognised and selected text is sent to %s.", name)
			}
			caption(c, what).Grow(1).Basis(px(t, 240))
			if pr.KeyURL != "" {
				ui.Link(c.Key("rm-keyurl"), i18n.T("Get a key"), pr.KeyURL).FontSize(px(t, fsCaption))
			}
		})
		settingRow(c, "rm-enabled", i18n.T("Use this service"), i18n.T("Off keeps its setup and key."), func() {
			ui.Switch(c.Key("rm-enabled-switch"), &d.enabled).Label(i18n.T("Use this service"))
		})
		if pr.Custom {
			fieldInfo(c, i18n.T("Name"), "", func() {
				ui.TextInput(c.Key("rm-name"), &d.name).Placeholder(name).Label(i18n.T("Name"))
			})
		}
		if pr.AppID {
			fieldInfo(c, i18n.T("App ID"), i18n.T("From the app in the Volcengine speech console, with its Access Token below. A key of the newer console needs no App ID."), func() {
				ui.TextInput(c.Key("rm-appid"), &d.appID).Placeholder(i18n.T("Optional")).Label(i18n.T("App ID"))
			})
		}
		if e := a.rmKeyErr[acc]; e != "" && d.key == "" {
			notice(c, "rm-key-err", noticeWarn, i18n.T("Could not read the key from the Keychain. Check that VoxtMini may use the login Keychain, then open this again."), e, nil, nil)
		}
		mask, _ := a.keyMask(acc)
		if a.offerShown(mask) {
			a.sharedKeyOffer(c, name)
		} else {
			a.keyField(c, name, mask, pr.KeyOptional)
			a.sharedKeyNotes(c, mask)
		}
		fieldInfo(c, i18n.T("Endpoint"), i18n.T("Leave empty for the service's own. Only https:// (wss:// for streaming) addresses, or http:// to this Mac."), func() {
			ph := pr.Endpoint
			if ph == "" {
				ph = "https://…/v1"
			}
			ui.TextInput(c.Key("rm-endpoint"), &d.endpoint).Placeholder(ph).Label(i18n.T("Endpoint"))
		})
		fieldInfo(c, i18n.T("Model"), "", func() {
			opts := append(slices.Clone(pr.Models), otherModel())
			if !slices.Contains(opts, d.model) {
				opts = append([]string{d.model}, opts...)
			}
			ui.Select(c.Key("rm-model"), &d.model, opts).Label(i18n.T("Model")).Width(px(t, 320))
			if d.model == otherModel() {
				ui.TextInput(c.Key("rm-model-custom"), &d.customModel).Placeholder(i18n.T("Model id")).Label(i18n.T("Model id"))
			}
		})
		collapsible(c, i18n.T("Advanced"), &d.advanced, nil, func() {
			if d.kind == providers.ASR {
				a.speechAdvanced(c, pr)
			} else {
				a.chatAdvanced(c, pr)
			}
		})
		if d.err != "" {
			notice(c, "rm-dlg-err", noticeError, d.err, d.errMore, nil, func() { d.err, d.errMore = "", "" })
		}
	}, func() {
		if ui.Button(c.Key("rm-dlg-test"), i18n.T("Test connection")).Disabled(d.test.running || d.saving).Shrink(0).Clicked() {
			a.testDialog()
		}
		// The test's state, beside its button; a long reason is cut, its
		// ⓘ tells the whole of it.
		ui.Row(c.Key("rm-dlg-status")).Gap(px(t, sp1)).AlignItems(ui.Center).Grow(1).Shrink(1).Basis(0).MinWidth(0).Children(func() {
			testStatus(c, d.test, true)
		})
		if ghostButton(c.Key("rm-dlg-cancel"), i18n.T("Cancel")).Shrink(0).Clicked() {
			d.open = false
		}
		if primaryButton(c.Key("rm-dlg-save"), i18n.T("Save")).Disabled(d.saving).Shrink(0).Clicked() {
			a.saveProviderDialog()
		}
	})
}

// speechAdvanced are a speech service's advanced parameters.
func (a *app) speechAdvanced(c *ui.Context, pr providers.Preset) {
	t := c.Theme()
	d := &a.rmDlg
	codes := []string{"", providers.LanguageAuto}
	labels := []string{i18n.T("Main language (as local models)"), i18n.T("Detect automatically")}
	for _, code := range append([]string{a.prefs.General.MainLanguage}, a.prefs.General.OtherLanguages...) {
		if l, ok := lang.Lookup(code); ok && !slices.Contains(codes, l.Code) {
			codes, labels = append(codes, l.Code), append(labels, l.NameIn(i18n.Locale()))
		}
	}
	if !slices.Contains(codes, d.language) { // a language no longer spoken: the main one
		d.language = ""
	}
	sel := labels[slices.Index(codes, d.language)]
	fieldInfo(c, i18n.T("Language"), "", func() {
		if ui.Select(c.Key("rm-lang"), &sel, labels).Label(i18n.T("Language")).Width(px(t, 320)).Changed() {
			d.language = codes[slices.Index(labels, sel)]
		}
	})
	settingRow(c, "rm-hints", i18n.T("Send the dictionary"), i18n.T("Your dictionary's terms go to the service with each recording."), func() {
		ui.Switch(c.Key("rm-hints-switch"), &d.hints).Label(i18n.T("Send the dictionary"))
	})
	if pr.Prompt {
		fieldInfo(c, i18n.T("Recognition prompt"), i18n.T("Topics and names that help recognition. Your dictionary's terms are added."), func() {
			ui.TextArea(c.Key("rm-prompt"), &d.prompt).Placeholder(i18n.T("Optional")).Label(i18n.T("Recognition prompt")).MinHeight(px(t, 64))
		})
	}
}

// chatAdvanced are a language model's advanced parameters.
func (a *app) chatAdvanced(c *ui.Context, pr providers.Preset) {
	t := c.Theme()
	d := &a.rmDlg
	a.chatAPI(c, pr) // remote_dialog_api.go
	def := i18n.T("Default")
	ui.Row(c).Gap(px(t, sp3)).Wrap().Children(func() {
		for _, f := range []struct {
			key, label, info string
			v                *string
		}{
			{"rm-temp", i18n.T("Temperature"), i18n.T("0–2. Default: VoxtMini's, low for clean-up and translation."), &d.temp},
			{"rm-topp", i18n.T("Top-p"), i18n.T("0–1. Default: the service's."), &d.topP},
			{"rm-maxtok", i18n.T("Max output tokens"), i18n.T("Default: the service's."), &d.maxTok},
		} {
			ui.Column(c).Grow(1).Basis(px(t, 140)).Children(func() {
				fieldInfo(c, f.label, f.info, func() {
					ui.TextInput(c.Key(f.key), f.v).Placeholder(def).Label(f.label)
				})
			})
		}
	})
	fieldInfo(c, i18n.T("Extra body (JSON)"), i18n.T("Merged into every request, over the service's defaults: for example {\"thinking\": {\"type\": \"enabled\"}} turns thinking on where VoxtMini turns it off."), func() {
		ui.TextArea(c.Key("rm-body"), &d.body).Placeholder("{}").Label(i18n.T("Extra body (JSON)")).MinHeight(px(t, 56))
	})
}

// testStatus is a connection test's state: a spinner while it runs, a
// green tick and its time once it works, a red mark and a short reason
// when it failed, with the long one and what the service said in a
// tooltip (and an ⓘ, when long is set: the dialog's).
func testStatus(c *ui.Context, tr providerTest, long bool) {
	t := c.Theme()
	k := tok(t)
	switch {
	case tr.running:
		ui.Spinner(c).Size(px(t, 14), px(t, 14)).Shrink(0)
		if long {
			caption(c, i18n.T("Testing…")).SingleLine().Ellipsis("…").MinWidth(0)
		}
	case tr.res.At.IsZero() && tr.res.Err == nil && tr.res.Latency == 0:
	case tr.res.Err == nil:
		text := i18n.T("%d ms", tr.res.Latency.Milliseconds())
		if long {
			text = i18n.T("Connection OK · %d ms", tr.res.Latency.Milliseconds())
		}
		statusMark(c, icons.CheckCircle, k.Success, text).Shrink(1).Tooltip(text)
	default:
		short := uitext.ProviderErrorShort(tr.res.Err)
		msg, _ := uitext.ProviderErrorText(tr.res.Err)
		tip := msg + "\n\n" + uitext.ProviderErrorDetail(tr.res.Err)
		if !long {
			statusMark(c, icons.CloseCircle, k.Danger, i18n.T("Failed")).Shrink(0).Tooltip(short + "\n" + tip)
			return
		}
		statusMark(c, icons.CloseCircle, k.Danger, short).Shrink(1).Tooltip(tip)
		infoTip(c, tip).Shrink(0)
	}
}

// testDialog tests what the dialog's form describes, saved or not: the
// key typed, else the saved one (read in the background).
func (a *app) testDialog() {
	d := &a.rmDlg
	kind := d.kind
	p, bad := d.setup()
	if bad != "" {
		d.err, d.errMore = bad, ""
		return
	}
	p.Enabled = true
	if err := providers.CheckEndpoint(kind, p); err != nil {
		d.err, d.test = "", providerTest{res: providers.TestResult{Err: err, At: time.Now()}}
		return
	}
	d.err, d.test = "", providerTest{running: true}
	acc := providers.Account(kind, p.ID)
	typed, remove, keys, proxy, id, logs := strings.TrimSpace(d.key), d.removeKey, a.rkeys, a.remoteProxy(), d.id, a.logs
	cs, vendor := a.famKeys.Load(), ""
	if d.fam.chosen { // "Use this key", not saved yet: the family's key
		vendor = d.fam.vendor
	}
	a.async(func() {
		k, err := typed, error(nil)
		switch {
		case k != "" || remove:
		case vendor != "" && cs != nil:
			k, err = cs.Secret(context.Background(), vendor)
		default:
			k, err = keys.Get(acc)
		}
		res := providers.TestResult{Err: err, At: time.Now()}
		if err == nil {
			res = runProviderTest(context.Background(), kind, p, providers.Env{Key: k, Proxy: proxy()})
		}
		logTest(logs, kind, p, res, k)
		a.post(func() {
			if a.rmDlg.open && a.rmDlg.id == id && a.rmDlg.kind == kind {
				a.rmDlg.test = providerTest{res: res}
			}
			if a.rmTests == nil {
				a.rmTests = map[string]providerTest{}
			}
			a.rmTests[acc] = providerTest{res: res}
		})
	})
}

// saveProviderDialog checks the form, saves the key (when one was typed
// or removed) and then the setup.
func (a *app) saveProviderDialog() {
	d := &a.rmDlg
	kind := d.kind
	p, bad := d.setup()
	if bad != "" {
		d.err, d.errMore = bad, ""
		return
	}
	if err := providers.CheckEndpoint(kind, p); err != nil {
		d.err, d.errMore = uitext.ProviderErrorText(err)
		return
	}
	acc := providers.Account(kind, p.ID)
	plan := a.dialogKeyPlan(p, "VoxtMini "+uitext.ProviderName(kind, p))
	d.saving, d.err = true, ""
	a.async(func() {
		ref, err := plan.run()
		p.CredentialRef = ref
		a.post(func() {
			a.rmDlg.saving = false
			if err != nil {
				a.rmDlg.err, a.rmDlg.errMore = i18n.T("Could not save the key in the Keychain."), err.Error()
				a.loadRemoteKeys([]string{acc}) // what the Keychain holds now
				return
			}
			delete(a.rmTests, acc)
			a.updateSettings(func(s *settings.Settings) { providers.Put(s, kind, p) })
			a.loadRemoteKeys([]string{acc}) // read back what was saved
			a.rmDlg = providerDialog{}
			a.toast(i18n.T("%s saved", uitext.ProviderName(kind, p)))
		})
	})
}

// keyField is the dialog's key: a password field over the saved key's
// mask, and Remove.
func (a *app) keyField(c *ui.Context, name, mask string, optional bool) {
	t := c.Theme()
	d := &a.rmDlg
	info := i18n.T("Saved only in this Mac's Keychain.")
	if optional {
		info = i18n.T("Optional. Saved only in this Mac's Keychain.")
	}
	fieldInfo(c, uitext.KeyLabel(d.kind, d.id), info, func() {
		ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Children(func() {
			ph := i18n.T("Paste the key")
			if mask != "" && !d.removeKey {
				ph = i18n.T("Saved: %s · paste a new one to replace it", mask)
			}
			ui.TextInput(c.Key("rm-key"), &d.key).Password().Placeholder(ph).Label(i18n.T("Key for %s", name)).Grow(1).MinWidth(0)
			if mask != "" && !d.removeKey && ghostButton(c.Key("rm-key-remove"), i18n.T("Remove")).Label(i18n.T("Remove the saved key")).Clicked() {
				d.removeKey, d.key = true, ""
			}
		})
		if d.removeKey {
			caption(c, i18n.T("The saved key is removed when you save."))
		}
	})
}
