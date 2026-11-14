package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/remote"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// modelsPageShots saves the Models page (TestSnapshots) on each tab and
// filter, with two remote services set up, then with models hidden and
// Show hidden on: models-speech, models-speech-local,
// models-speech-remote, models-language, models-language-more (the
// More models section), models-hidden, models-show-hidden.
func modelsPageShots(a *app, tt *ui.Tester, save func(string)) {
	ring := &memKeyring{keys: map[string]string{"llm.deepseek": "sk-1234567890abcd", "asr.doubao": "tok-1234567890"}}
	a.rkeys = remote.NewKeys(ring)
	old := a.local
	a.local = remote.New(remote.Config{Local: old, Settings: a.lib.settings.Get, Keys: a.rkeys})
	a.updateSettings(func(s *settings.Settings) {
		s.Providers.ASR = []settings.Provider{{ID: "doubao", Enabled: true, AppID: "app-1"}}
		s.Providers.LLM = []settings.Provider{{ID: "deepseek", Enabled: true, Model: "deepseek-chat"}}
	})
	_ = a.rkeys.Load([]string{"llm.deepseek", "asr.doubao"})
	a.navigate(modelsPath + "/speech")
	a.mdFilter = filterAll
	save("models-speech")
	scrollToText(tt, i18n.T("More models"))
	save("models-speech-more")
	a.navigate("/home")
	a.navigate(modelsPath + "/local")
	scrollToText(tt, "Whisper large-v3-turbo")
	save("models-speech-local")
	a.navigate("/home")
	a.navigate(modelsPath + "/remote")
	save("models-speech-remote")
	a.navigate("/home")
	a.navigate(modelsPath + "/language")
	a.mdFilter = filterAll
	save("models-language")
	scrollToText(tt, i18n.T("More models"))
	save("models-language-more")
	// Hidden: a recommended model, two others and a remote service.
	a.updateSettings(func(s *settings.Settings) {
		s.Models.Hidden = []string{models.LLMFast, models.LLMLFM2, models.LLMQwen36_27B, settings.HiddenRemote("llm", "openai")}
	})
	a.navigate("/home")
	a.navigate(modelsPath)
	save("models-hidden")
	a.mdShowHidden = true
	save("models-show-hidden")
	a.navigate("/home")
	a.navigate(modelsPath)
	scrollToText(tt, "Qwen3.6 27B · 4-bit")
	save("models-show-hidden-more")
	a.mdShowHidden, a.mdTab = false, 0
	a.updateSettings(func(s *settings.Settings) {
		s.Providers = settings.Providers{}
		s.Models.Hidden = nil
	})
	a.local = old
}
