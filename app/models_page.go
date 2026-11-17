package main

// The Models page (/models, docs/models-page.md): speech and language
// models, local and remote, in one list per kind (models_view.go). A
// local model's card says what it is good at and which languages it
// knows, its size, whether it is installed or downloading, with Download,
// Cancel and Delete, and, while it is loaded, the memory it uses
// (models_memory.go). The models live in the ActNow family's library, so
// a card also says which other family apps share the model, or download
// it, or deleted it (models_cards.go); deleting follows the family's
// rules (models_delete.go); Network… sets how downloads reach the Hub
// (models_network.go). The remote services' cards are remote_page.go's;
// hiding a model or a service is models_hide.go's.
//
// The models on disk and their downloads are internal/models' (a.models);
// what is installed, as the sessions open it, internal/local's (a.local,
// models_runtimes.go). Views read a.modelStatus, the copy of the
// statuses modelsChanged takes on the main thread, and a.installed
// (memory only). A card in the sidebar says when Transcribe lacks a model
// (checkModels, shell_notices.go).

import (
	"github.com/hehehai/VoxtMini/internal/apple"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/remote"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// modelsPath is the Models page's path.
const modelsPath = "/models"

// modelsState is the local models and the Models page's state.
type modelsState struct {
	// mdTab: 0 speech models, 1 language models; mdFilter: filterAll,
	// filterLocal or filterRemote; mdShowHidden: the hidden models are
	// listed too (models_view.go).
	mdTab        int
	mdFilter     int
	mdShowHidden bool
	local        local.Services
	models       *models.Manager
	// apples are Apple's models under a.local (apple_link.go), appleSt
	// what the helper last said of them, appleDl Apple's download of
	// speech languages.
	apples  *apple.Services
	appleSt apple.Status
	appleDl appleDownload
	// modelStatus is each catalogue model's status, as modelsChanged
	// last read it.
	modelStatus map[string]models.Status
	// modelAsk is the model whose deletion is being confirmed when no
	// other app uses it, modelAskFree what that frees (models_delete.go).
	modelAsk     string
	modelAskFree int64
	// modelShared is the deletion being confirmed of a model other
	// ActNow apps use too (family spec §7.10.3).
	modelShared *sharedDelete
	// modelDeciding is the model whose deletion is being weighed (the
	// family's files read off the main thread).
	modelDeciding string
	// netOpen: the Network dialog is open; netForm is its fields
	// (models_network.go).
	netOpen bool
	netForm networkForm
	// netProbeSeq numbers the Network dialog's connection tests: a
	// result of an older one is dropped.
	netProbeSeq int
	// modelAdvanced: the Advanced section is open; modelKeepSel is its
	// select's label, modelKeepFor the setting and language it shows
	// (models_keep.go).
	modelAdvanced bool
	modelKeepSel  string
	modelKeepFor  string
	// modelSet is the open Settings dialog of a model
	// (models_settings_view.go).
	modelSet modelSettingsForm

	// presetSel is the shortcut presets' select (settings_shortcuts.go).
	presetSel string
}

// openModels reads the models on disk and sets up the local models,
// then the sidebar's card.
func (a *app) openModels() {
	if a.models == nil {
		a.models = models.Open(a.managerOptions(func() { a.post(a.modelsChanged) }))
	}
	if a.local == nil {
		// The local models, then the remote services the user set up
		// (remote_page.go): a feature uses one only when the user picks it.
		// family_keys.go: VoxtMini's items, then the family's.
		a.rkeys = remote.NewKeys(a.keyRing())
		var st func() settings.Settings
		if a.settings != nil {
			st = a.settings.Get
		}
		// Apple's on-device models after the downloaded ones (apple_link.go).
		a.apples = apple.New(apple.Config{Local: a.withDevLLM(newLocalServices(a)), Helper: helperCaller{a.host}, Name: uitext.ModelName}) // llm_link.go: VOXTMINI_DEV_LLM
		a.apples.SetParams(appleParams(a.prefs.Models))                                                                                    // models_settings.go
		a.local = remote.New(remote.Config{
			Local:    a.apples,
			Settings: st,
			Keys:     a.rkeys,
			Proxy:    a.remoteProxy(),
		})
		a.loadRemoteKeys(remoteAccounts(a.prefs))
	}
	a.readModelStatus()
	a.checkModels() // shell_notices.go
}

// readModelStatus copies the statuses (memory only), and says which
// models were installed since the last copy.
func (a *app) readModelStatus() (installed []string) {
	next := map[string]models.Status{}
	for _, m := range models.Catalog() {
		s := a.models.Status(m.ID)
		if s.Installed && a.modelStatus != nil && !a.modelStatus[m.ID].Installed {
			installed = append(installed, m.ID)
		}
		next[m.ID] = s
	}
	a.modelStatus = next
	return installed
}

// modelsChanged takes the models' statuses after a change (a download's
// progress, its end, a deletion).
func (a *app) modelsChanged() {
	before := a.modelStatus
	for _, id := range a.readModelStatus() {
		if m, _ := models.Lookup(id); m.Kind != models.VAD {
			a.toast(i18n.T("%s is installed.", uitext.ModelName(id)))
		}
	}
	for id, s := range a.modelStatus {
		if before[id].Installed != s.Installed {
			a.stReport = nil // Settings → Storage measures again
		}
	}
	a.checkModels()
	a.noteDownloads(before) // models_notify.go: macOS notifications while elsewhere
	if a.currentPage() == modelsPath[1:] {
		a.readModelMemory() // models_memory.go: a deleted model was unloaded
	}
}

// downloadModel starts downloading model id (and the models it needs).
func (a *app) downloadModel(id string) {
	a.step("download_model")
	m := a.models
	a.async(func() {
		m.Download(id) // a failure is the status's
		a.post(a.modelsChanged)
	})
}

// cancelModel stops model id's download.
func (a *app) cancelModel(id string) {
	m := a.models
	a.async(func() { m.Cancel(id) })
}

// localLibrary is the local models' Library under the remote services
// (internal/remote), when they are one (not in tests with fakes).
func (a *app) localLibrary() (*local.Library, bool) {
	l := a.local
	if r, ok := l.(*remote.Services); ok {
		l = r.Local()
	}
	if ap, ok := l.(*apple.Services); ok {
		l = ap.Local()
	}
	lib, ok := l.(*local.Library)
	return lib, ok
}

// installed is kind's installed models, the recommended one first
// (memory only).
func (a *app) installed(kind local.Kind) []local.Model {
	if a.local == nil {
		return nil
	}
	return a.local.Installed(kind)
}

// featureModel is the model a feature uses of kind, and whether one is
// installed that serves it.
func (a *app) featureModel(kind local.Kind, feature string) (local.Model, bool) {
	f := featureOf(a.prefs, feature)
	want := f.ASR
	if kind == local.LLM {
		want = f.LLM
	}
	return local.Choose(local.ForFeature(a.installed(kind), feature), want)
}

// featureOf is a feature's part of the settings (hotkey.Transcribe…).
func featureOf(s settings.Settings, feature string) settings.Feature {
	switch feature {
	case hotkey.Translate:
		return s.Features.Translate.Feature
	case hotkey.Rewrite:
		return s.Features.Rewrite.Feature
	}
	return s.Features.Transcribe.Feature
}

// setFeatureModel sets the model of kind a feature uses ("": the first
// installed).
func setFeatureModel(s *settings.Settings, kind local.Kind, feature, id string) {
	f := &s.Features.Transcribe.Feature
	switch feature {
	case hotkey.Translate:
		f = &s.Features.Translate.Feature
	case hotkey.Rewrite:
		f = &s.Features.Rewrite.Feature
	}
	if kind == local.ASR {
		f.ASR = id
	} else {
		f.LLM = id
	}
}
