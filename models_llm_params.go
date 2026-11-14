package main

// The local language models' generation parameters (settings.Models'
// LLMParams, the Local models page's Advanced section): per model,
// temperature, top-p, the output budget and the repetition penalty, each
// Automatic (VoxtMini's tuned value per task, internal/llmrt's profiles)
// or a number for every task. A change applies to the next request
// (llmrt.Runtime.SetTuning); a model need not load again.

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/llmrt"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// llmTunings are m's parameters as the runtime's tunings.
func llmTunings(m settings.Models) map[string]llmrt.Tuning {
	out := map[string]llmrt.Tuning{}
	for id, p := range m.LLMParams {
		out[id] = llmrt.Tuning{Temperature: p.Temperature, TopP: p.TopP, MaxTokens: p.MaxTokens, RepetitionPenalty: p.RepetitionPenalty}
	}
	return out
}

// applyLLMParams gives the running runtime m's parameters
// (settingsChanged; llmRuntime sets them on the one it makes).
func (a *app) applyLLMParams(m settings.Models) {
	if l := a.llmMade(); l != nil {
		l.SetTuning(llmTunings(m))
	}
}

// llmParamsForm is the parameters' fields as typed for model; seen is
// the model and saved values they were last set from; bad the field
// that does not read as a number in range ("" for none).
type llmParamsForm struct {
	model                   string
	temp, topP, maxTok, rep string
	seen                    string
	bad                     string
}

// tunable are the installed local language models VoxtMini runs (not
// Apple's, which takes no parameters), by catalogue id, with their names.
func (a *app) tunable() (ids, names []string) {
	if a.local == nil {
		return nil, nil
	}
	for _, m := range a.local.Installed(local.LLM) {
		if _, ok := llmrt.Lookup(m.ID); ok && !m.Remote {
			ids, names = append(ids, m.ID), append(names, m.Name)
		}
	}
	return ids, names
}

func numText(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

// sync sets the fields from the saved parameters of f.model when they
// changed elsewhere (or the model did).
func (f *llmParamsForm) sync(m settings.Models) {
	p := m.LLMParamsOf(f.model)
	key := fmt.Sprintf("%s|%s|%s|%d|%s", f.model, numText(p.Temperature), numText(p.TopP), p.MaxTokens, numText(p.RepetitionPenalty))
	if key == f.seen {
		return
	}
	f.seen, f.bad = key, ""
	f.temp, f.topP, f.rep = numText(p.Temperature), numText(p.TopP), numText(p.RepetitionPenalty)
	f.maxTok = ""
	if p.MaxTokens > 0 {
		f.maxTok = strconv.Itoa(p.MaxTokens)
	}
}

// parse reads the fields: the parameters, or the label of the first
// field out of its range.
func (f *llmParamsForm) parse() (settings.LLMParams, string) {
	var p settings.LLMParams
	num := func(s string, lo, hi float64, label string, dst **float64) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v < lo || v > hi {
			return label
		}
		*dst = &v
		return ""
	}
	for _, e := range []string{
		num(f.temp, 0, settings.MaxTemperature, i18n.T("Temperature"), &p.Temperature),
		num(f.topP, 0.01, 1, i18n.T("Top-p"), &p.TopP),
		num(f.rep, settings.MinRepetitionPenalty, settings.MaxRepetitionPenalty, i18n.T("Repetition penalty"), &p.RepetitionPenalty),
	} {
		if e != "" {
			return p, e
		}
	}
	if s := strings.TrimSpace(f.maxTok); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > settings.MaxLLMTokens {
			return p, i18n.T("Max output tokens")
		}
		p.MaxTokens = n
	}
	return p, ""
}

// llmParamsSection is the parameters' part of Advanced: a model, its
// four fields and Reset. Nothing without a model VoxtMini runs.
func (a *app) llmParamsSection(c *ui.Context) {
	t := c.Theme()
	ids, names := a.tunable()
	if len(ids) == 0 {
		return
	}
	f := &a.llmForm
	if !slices.Contains(ids, f.model) {
		f.model = ids[0]
	}
	f.sync(a.prefs.Models)
	prof, _ := llmrt.Lookup(f.model)
	exact, rewrite, maxTok := prof.Defaults()
	sel := names[slices.Index(ids, f.model)]
	settingRow(c, "llm-params-model", i18n.T("Generation"),
		i18n.T("How the chosen language model writes. Automatic keeps VoxtMini's values, tuned for each model and task; a number applies to every task."), func() {
			if ui.Select(c.Key("llm-params-select"), &sel, names).Label(i18n.T("Model to tune")).Width(px(t, 240)).Shrink(1).MinWidth(px(t, 140)).Changed() {
				f.model = ids[max(slices.Index(names, sel), 0)]
			}
		})
	auto := i18n.T("Automatic")
	both := func(a, b float64) string {
		if a == b {
			return numText(&a)
		}
		return i18n.T("%s for clean-up and translation, %s for Rewrite", numText(&a), numText(&b))
	}
	changed := false
	ui.Row(c).Gap(px(t, sp3)).Wrap().Children(func() {
		for _, fl := range []struct {
			key, label, info string
			v                *string
		}{
			{"llm-temp", i18n.T("Temperature"), i18n.T("0–2: higher writes more freely. Automatic: %s.", both(exact.Temperature, rewrite.Temperature)), &f.temp},
			{"llm-topp", i18n.T("Top-p"), i18n.T("0.01–1: lower keeps to the likeliest words. Automatic: %s.", both(exact.TopP, rewrite.TopP)), &f.topP},
			{"llm-maxtok", i18n.T("Max output tokens"), i18n.T("Automatic: by the input's length, at most %d.", maxTok), &f.maxTok},
			{"llm-rep", i18n.T("Repetition penalty"), i18n.T("0.5–2: above 1 repeats less. Automatic: %s.", both(exact.RepetitionPenalty, rewrite.RepetitionPenalty)), &f.rep},
		} {
			ui.Column(c).Grow(1).Basis(px(t, 220)).Children(func() { // two by two
				fieldInfo(c, fl.label, fl.info, func() {
					if ui.TextInput(c.Key(fl.key), fl.v).Placeholder(auto).Label(fl.label).Changed() {
						changed = true
					}
				})
			})
		}
	})
	if changed {
		p, bad := f.parse()
		if f.bad = bad; bad == "" {
			a.saveLLMParams(f.model, p)
		}
	}
	ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Children(func() {
		if f.bad != "" {
			caption(c, i18n.T("%s is out of range.", f.bad)).TextColor(tok(t).Danger).Grow(1).MinWidth(0)
		} else {
			ui.Box(c).Grow(1)
		}
		set := !a.prefs.Models.LLMParamsOf(f.model).IsZero() || f.temp+f.topP+f.maxTok+f.rep != ""
		if ghostIconButton(c.Key("llm-params-reset"), icons.Refresh, i18n.T("Reset to default")).Disabled(!set).Shrink(0).Clicked() {
			f.temp, f.topP, f.maxTok, f.rep, f.bad = "", "", "", "", ""
			a.saveLLMParams(f.model, settings.LLMParams{})
		}
	})
}

// saveLLMParams saves model id's parameters; the fields keep what was
// typed ("0." stays while its value, 0, is saved).
func (a *app) saveLLMParams(id string, p settings.LLMParams) {
	f := &a.llmForm
	m := a.prefs.Models
	m.LLMParams = m.WithLLMParams(id, p)
	saved := m.LLMParamsOf(id)
	f.seen = fmt.Sprintf("%s|%s|%s|%d|%s", id, numText(saved.Temperature), numText(saved.TopP), saved.MaxTokens, numText(saved.RepetitionPenalty))
	a.updateSettings(func(s *settings.Settings) { s.Models.LLMParams = s.Models.WithLLMParams(id, p) })
}
