package uitext

// The words of the model catalogue (internal/models): what a model is
// called and what it is good at, in plain words
// (no family, size or quantisation: the plan's model policy), and why a
// download failed.

import (
	"errors"
	"io/fs"

	"github.com/hehehai/actnowkit/hfhub"

	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/models"
)

// ModelName is what the UI calls model id: a plain name for the
// recommended models, the model's own name (models.Model.Title) for the
// others, its id when it is not in the catalogue.
func ModelName(id string) string {
	switch id {
	case models.ASRAccurate:
		return i18n.T("Accurate speech recognition")
	case models.ASRFast:
		return i18n.T("Fast speech recognition")
	case models.VADSilero:
		return i18n.T("Voice detection")
	case models.LLMBalanced:
		return i18n.T("Balanced language model")
	case models.LLMFast:
		return i18n.T("Fast language model")
	case models.LLMTranslate:
		return i18n.T("Translation model")
	}
	if m, ok := models.Lookup(id); ok && m.Title != "" {
		return m.Title // the others: their own names (catalog_more.go)
	}
	if n := appleName(id); n != "" {
		return n
	}
	return id
}

// ModelAbout says what model id is good at.
func ModelAbout(id string) string {
	switch id {
	case models.ASRAccurate:
		return i18n.T("The most accurate: good with accents, dialects, names and mixed languages. Shows the text as you speak.")
	case models.ASRFast:
		return i18n.T("Quicker and lighter, for clear speech in a quiet place. Good for a Mac with 8 GB.")
	case models.VADSilero:
		return i18n.T("Tells speech from silence, so that only speech is recognised.")
	case models.LLMBalanced:
		return i18n.T("Cleans up dictation, translates and rewrites well: the best balance of quality and speed.")
	case models.LLMFast:
		return i18n.T("Answers sooner and uses less memory, for short clean-ups and simple rewrites.")
	case models.LLMTranslate:
		return i18n.T("Made for translation: quick and natural. Only Translate uses it.")
	}
	if s := appleAbout(id); s != "" {
		return s
	}
	return moreAbout(id) // catalog_more.go
}

// DownloadErrorText says why a model's download failed, and its detail
// (the error itself, for the ⓘ).
func DownloadErrorText(err error) (msg, more string) {
	if err == nil {
		return "", ""
	}
	more = err.Error()
	var pe *fs.PathError
	switch hfhub.Code(err) {
	case hfhub.CodeDiskSpace:
		return i18n.T("Not enough disk space: free some room, then try again."), more
	case hfhub.CodeChecksum:
		return i18n.T("A downloaded file was damaged. Try again."), more
	case hfhub.CodeNotFound, hfhub.CodeGated:
		return i18n.T("The download server does not have this model."), more
	case hfhub.CodeOffline:
		return i18n.T("The download failed. Check the internet connection, then try again."), more
	case hfhub.CodeBusy:
		return i18n.T("Another app is using this model. Try again later."), more
	}
	if errors.As(err, &pe) {
		return i18n.T("The model could not be saved to the disk."), more
	}
	return i18n.T("The download failed. Check the internet connection, then try again."), more
}
