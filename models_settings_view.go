package main

// A local model's Settings dialog (Voxt's per-model configuration
// sheets: CustomLLMGenerationSettingsSheet, MLXASRConfigurationSheet),
// opened from its card's menu: the knobs its family takes
// (models_settings.go), switches first, then the numbers, each empty for
// Automatic with VoxtMini's value in its ⓘ. Changes are saved as typed;
// Reset to defaults clears them all.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/knobs"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// modelSettingsForm is the open dialog: its model, the numbers as typed
// (by knob), the saved values they were last set from (seen) and the
// label of the field that does not read as a number in range.
type modelSettingsForm struct {
	open bool
	id   string
	vals map[knobs.Knob]string
	seen string
	bad  string
}

// openModelSettings opens model id's Settings dialog.
func (a *app) openModelSettings(id string) {
	a.modelSet = modelSettingsForm{open: true, id: id}
}

// paramText is p's value of numeric knob k as the field shows it ("" for
// Automatic).
func paramText(p settings.ModelParams, k knobs.Knob) string {
	num := func(v *float64) string {
		if v == nil {
			return ""
		}
		return strconv.FormatFloat(*v, 'f', -1, 64)
	}
	whole := func(n int) string {
		if n <= 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	switch k {
	case knobs.Temperature:
		return num(p.Temperature)
	case knobs.TopP:
		return num(p.TopP)
	case knobs.MinP:
		return num(p.MinP)
	case knobs.RepetitionPenalty:
		return num(p.RepetitionPenalty)
	case knobs.TopK:
		return whole(p.TopK)
	case knobs.MaxTokens:
		return whole(p.MaxTokens)
	}
	return ""
}

// setParam sets numeric knob k of p to v (nil: Automatic).
func setParam(p *settings.ModelParams, k knobs.Knob, v *float64) {
	n := 0
	if v != nil {
		n = int(*v)
	}
	switch k {
	case knobs.Temperature:
		p.Temperature = v
	case knobs.TopP:
		p.TopP = v
	case knobs.MinP:
		p.MinP = v
	case knobs.RepetitionPenalty:
		p.RepetitionPenalty = v
	case knobs.TopK:
		p.TopK = n
	case knobs.MaxTokens:
		p.MaxTokens = n
	}
}

// paramsKey identifies p's numbers, for sync.
func paramsKey(id string, p settings.ModelParams, fields []knobs.Field) string {
	var b strings.Builder
	b.WriteString(id)
	for _, f := range fields {
		b.WriteString("|" + paramText(p, f.Knob))
	}
	return b.String()
}

// sync sets the fields from the saved settings when they changed
// elsewhere (or the model did).
func (f *modelSettingsForm) sync(m settings.Models, fields []knobs.Field) {
	p := m.ParamsOf(f.id)
	key := paramsKey(f.id, p, fields)
	if key == f.seen {
		return
	}
	f.seen, f.bad = key, ""
	f.vals = map[knobs.Knob]string{}
	for _, fl := range fields {
		f.vals[fl.Knob] = paramText(p, fl.Knob)
	}
}

// parse reads the numbers over saved (whose switches stay): the
// settings, or the label of the first field out of its range.
func (f *modelSettingsForm) parse(saved settings.ModelParams, fields []knobs.Field) (settings.ModelParams, string) {
	p := saved
	for _, fl := range fields {
		if fl.Knob.Switch() {
			continue
		}
		s := strings.TrimSpace(f.vals[fl.Knob])
		if s == "" {
			setParam(&p, fl.Knob, nil)
			continue
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v < fl.Min || v > fl.Max || fl.Knob.Whole() && v != float64(int(v)) {
			return p, knobLabel(fl.Knob)
		}
		setParam(&p, fl.Knob, &v)
	}
	return p, ""
}

// knobLabel is a knob's name.
func knobLabel(k knobs.Knob) string {
	switch k {
	case knobs.Thinking:
		return i18n.T("Thinking")
	case knobs.MainLanguage:
		return i18n.T("Use the main language")
	case knobs.Temperature:
		return i18n.T("Temperature")
	case knobs.TopP:
		return i18n.T("Top-p")
	case knobs.TopK:
		return i18n.T("Top-k")
	case knobs.MinP:
		return i18n.T("Min-p")
	case knobs.RepetitionPenalty:
		return i18n.T("Repetition penalty")
	case knobs.MaxTokens:
		return i18n.T("Max output tokens")
	}
	return string(k)
}

// autoText is a field's Automatic value(s): one, or per task.
func autoText(f knobs.Field) string {
	v := func(x float64) string {
		if x == 0 && (f.Knob == knobs.TopK || f.Knob == knobs.MinP) {
			return i18n.T("off")
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	a := f.Auto
	switch {
	case len(a) == 0:
		return ""
	case len(a) < 3 || a[0] == a[1] && a[1] == a[2]:
		return v(a[0])
	case a[0] == a[1]:
		return i18n.T("%s for clean-up and translation, %s for Rewrite", v(a[0]), v(a[2]))
	}
	return i18n.T("%s for clean-up, %s for translation, %s for Rewrite", v(a[0]), v(a[1]), v(a[2]))
}

// knobInfo is a field's ⓘ: what it does, its range, its Automatic.
func knobInfo(f knobs.Field) string {
	rng := fmt.Sprintf("%s–%s", strconv.FormatFloat(f.Min, 'f', -1, 64), strconv.FormatFloat(f.Max, 'f', -1, 64))
	switch f.Knob {
	case knobs.Thinking:
		return i18n.T("Thinks before it answers: slower, so VoxtMini waits three times as long for it. Off by default.")
	case knobs.MainLanguage:
		return i18n.T("On: the model is told your main language (Whisper only when you speak no other one). Off: it detects the language.")
	case knobs.Temperature:
		return i18n.T("%s: higher writes more freely. Automatic: %s.", rng, autoText(f))
	case knobs.TopP:
		return i18n.T("%s: lower keeps to the likeliest words. Automatic: %s.", rng, autoText(f))
	case knobs.TopK:
		return i18n.T("%s: chooses among this many likeliest words. Automatic: %s.", rng, autoText(f))
	case knobs.MinP:
		return i18n.T("%s: drops words far less likely than the likeliest. Automatic: %s.", rng, autoText(f))
	case knobs.RepetitionPenalty:
		return i18n.T("%s: above 1 repeats less. Automatic: %s.", rng, autoText(f))
	case knobs.MaxTokens:
		return i18n.T("%s. Automatic: by the input's length, at most %s.", rng, autoText(f))
	}
	return ""
}

// switchOn is a switch knob's state in p.
func switchOn(p settings.ModelParams, k knobs.Knob) bool {
	if k == knobs.Thinking {
		return p.Think
	}
	return !p.DetectLanguage
}

// modelSettingsDialog is the open dialog.
func (a *app) modelSettingsDialog(c *ui.Context) {
	f := &a.modelSet
	if !f.open {
		return
	}
	t := c.Theme()
	fields := modelKnobs(f.id)
	f.sync(a.prefs.Models, fields)
	saved := a.prefs.Models.ParamsOf(f.id)
	dialogFrame(c, "model-set", &f.open, uitext.ModelName(f.id), "", 520, func() {
		changed := false
		for _, fl := range fields {
			if !fl.Knob.Switch() {
				continue
			}
			on := switchOn(saved, fl.Knob)
			formRow(c, "model-set-"+string(fl.Knob), knobLabel(fl.Knob), knobInfo(fl), func() {
				if ui.Switch(c.Key("model-set-sw-"+string(fl.Knob)), &on).Label(knobLabel(fl.Knob)).Changed() {
					p := saved
					if fl.Knob == knobs.Thinking {
						p.Think = on
					} else {
						p.DetectLanguage = !on
					}
					a.saveModelParams(f.id, p, fields)
				}
			})
		}
		ui.Row(c).Gap(px(t, sp3)).Wrap().Children(func() {
			for _, fl := range fields {
				if fl.Knob.Switch() {
					continue
				}
				v := f.vals[fl.Knob]
				ui.Column(c).Grow(1).Basis(px(t, 200)).Children(func() { // two by two
					fieldInfo(c, knobLabel(fl.Knob), knobInfo(fl), func() {
						if ui.TextInput(c.Key("model-set-"+string(fl.Knob)), &v).Placeholder(i18n.T("Automatic")).Label(knobLabel(fl.Knob)).Changed() {
							f.vals[fl.Knob] = v
							changed = true
						}
					})
				})
			}
		})
		if changed {
			p, bad := f.parse(saved, fields)
			if f.bad = bad; bad == "" {
				a.saveModelParams(f.id, p, fields)
			}
		}
		if f.bad != "" {
			caption(c.Key("model-set-bad"), i18n.T("%s is out of range.", f.bad)).TextColor(tok(t).Danger)
		}
	}, func() {
		typed := false
		for _, v := range f.vals {
			typed = typed || v != ""
		}
		if ghostButton(c.Key("model-set-reset"), i18n.T("Reset to default")).Disabled(saved.IsZero() && !typed).Clicked() {
			f.vals, f.bad = map[knobs.Knob]string{}, ""
			a.saveModelParams(f.id, settings.ModelParams{}, fields)
		}
		ui.Spacer(c)
		if primaryButton(c.Key("model-set-done"), i18n.T("Done")).Clicked() {
			f.open = false
		}
	})
}

// saveModelParams saves model id's settings; the fields keep what was
// typed ("0." stays while its value, 0, is saved).
func (a *app) saveModelParams(id string, p settings.ModelParams, fields []knobs.Field) {
	m := a.prefs.Models
	m.Params = m.WithParams(id, p)
	a.modelSet.seen = paramsKey(id, m.ParamsOf(id), fields)
	a.updateSettings(func(s *settings.Settings) { s.Models.Params = s.Models.WithParams(id, p) })
}
