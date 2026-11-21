//go:build readmeshots

package main

// The README's screenshots on a Mac (scripts/readme-shots/mac-shots.sh):
// with -tags readmeshots, `VoxtMini --readme-shots <dir> --lang en`
// runs instead of the app: the main window and the overlay of a VoxtMini
// over a library in a temporary directory, with a family directory and a
// model library of its own there too (models of the right sizes and no
// bytes), made-up history and dictionary (readme_shots_data.go); no lock,
// no helper, nothing of the installed VoxtMini read or written. It walks
// the shots: for each it writes <dir>/step-N.ready ("<window number>
// <name>") and waits (10 s at most) for <dir>/step-N.done, which the
// script touches once it captured the window (screencapture -l: the real
// window, with its shadow). Besides its pages, it opens a plain editor
// window with made-up notes, the desktop the overlay's shots are put on.
// With --front it comes to the front (the windows look active); else it
// stays behind the app in front. Release builds do not have it.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/deliver"
	"github.com/hehehai/VoxtMini/internal/helper"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/macx"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/permission"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
	"github.com/hehehai/actnowkit/family"
	"github.com/hehehai/actnowkit/registry"
)

func init() { readmeShots = runReadmeShots }

// readmeShotsSize is the main window's content in the shots.
const readmeShotsW, readmeShotsH = 980, 640

// readmeModels are the installed models the pages show.
var readmeModels = []string{models.ASRAccurate, models.VADSilero, models.LLMBalanced}

// readmeNoApps finds no other ActNow app.
type readmeNoApps struct{}

func (readmeNoApps) AppPaths(string) []string                { return nil }
func (readmeNoApps) Running(string) []registry.Process       { return nil }
func (readmeNoApps) BundleID(appPath string) (string, error) { return registry.BundleIDAt(appPath) }

// readmeRec is a recording that plays, three seconds in (nothing sounds).
type readmeRec struct{}

func (readmeRec) Play()                   {}
func (readmeRec) Pause()                  {}
func (readmeRec) Playing() bool           { return true }
func (readmeRec) Position() time.Duration { return 2600 * time.Millisecond }
func (readmeRec) Duration() time.Duration { return 6400 * time.Millisecond }
func (readmeRec) Close()                  {}

func runReadmeShots(dir string) {
	tmp, err := os.MkdirTemp("", "voxtmini-readme-shots")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)
	lang := argAfter("--lang")
	mygo.App.SetName("VoxtMini README Shots")
	familyConfig = func(a *app) actnow.Config {
		return actnow.Config{Dir: family.At(filepath.Join(tmp, "ActNow")), Hub: filepath.Join(tmp, "hub"),
			BundleID: "dev.hehehai.voxtmini", Version: "0.1.0", System: readmeNoApps{}}
	}
	lib, err := openLibrary(tmp)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	a := newApp(lib, newHelperHost("", tmp))
	if err := readmeSetUp(a, lang); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mygo.App.WhenReady(func() {
		a.openWindow()
		cb := a.win.ContentBounds()
		a.win.SetContentBounds(mygo.Rectangle{X: cb.X, Y: cb.Y, Width: readmeShotsW, Height: readmeShotsH})
		readmeReady(a)
		go readmeWalk(a, dir, lang)
	})
	time.AfterFunc(3*time.Minute, func() { mygo.RunOnMain(mygo.App.Quit) })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// readmeSetUp gives a the settings, models and data of the shots.
func readmeSetUp(a *app, lang string) error {
	a.updateSettings(func(s *settings.Settings) {
		s.General.Language = lang
		s.Setup.Done = true // no setup guide over the page
	})
	if err := readmeSeedModels(a.models.Hub(), readmeModels); err != nil {
		return err
	}
	a.models.Close()
	a.models, a.local, a.modelStatus = nil, nil, nil
	a.openModels()
	if err := a.models.Use(context.Background(), readmeModels...); err != nil {
		return err
	}
	return seedReadme(a, lang)
}

// readmeReady is a VoxtMini that is ready: the helper, the permissions,
// the shortcuts; no cards. Main thread.
func readmeReady(a *app) {
	a.hstatus = helper.Status{State: helper.Ready}
	a.perms.mic, a.perms.access, a.perms.checked = permission.Granted, true, true
	a.hotkeys.Installed = true
	a.readModelStatus()
	a.clearNotices()
	a.problemDismissed = nil
}

// readmeStep is one shot: what it does to the app, and the window to
// capture.
type readmeStep struct {
	name  string
	apply func(a *app)
	win   func(a *app) *mygo.Window
}

func readmeSteps(lang string, editor **mygo.Window) []readmeStep {
	tx := readmeTexts(lang)
	main := func(a *app) *mygo.Window { return a.win }
	over := func(a *app) *mygo.Window { return a.ov.win }
	page := func(path string, then func(a *app)) func(a *app) {
		return func(a *app) {
			readmeReady(a)
			a.navigate(path)
			if then != nil {
				then(a)
			}
		}
	}
	now := time.Now()
	target := proto.App{PID: 1, Name: "Notes"}
	levels := []float64{0.15, 0.4, 0.75, 0.55, 0.9, 0.6, 0.35, 0.7, 0.45, 0.2}
	return []readmeStep{
		{"home", page("/home", func(a *app) { a.historyChanged() }), main},
		{"history", page("/history", func(a *app) {
			a.loadHistory()
			a.hist.sel = readmeHistMain
			a.hist.play.open = func(string) (recPlayer, error) { return readmeRec{}, nil }
			a.toggleRecording(store.Entry{ID: readmeHistMain, Audio: readmeHistMain + ".wav"})
			a.loadFollowUps(store.Entry{ID: readmeHistMain})
		}), main},
		{"dictionary", page("/dictionary", func(a *app) { a.loadDict() }), main},
		{"models", page(modelsPath, nil), main},
		{"editor", func(a *app) {
			if *editor == nil {
				*editor = readmeEditor(tx)
			}
			(*editor).Show()
			(*editor).Focus()
		}, func(*app) *mygo.Window { return *editor }},
		{"transcribe", func(a *app) {
			a.notice = nil
			a.sess = session.Snapshot{ID: "rd-1", Kind: store.KindTranscribe, State: session.Recording, StartedAt: now, HeardAt: now,
				Levels: levels, Partial: tx.live}
			a.overlaySync()
		}, over},
		{"translate", func(a *app) {
			s := session.Snapshot{ID: "rd-2", Kind: store.KindTranslate, State: session.Done, Card: true, Delivery: deliver.ReasonCard,
				Target: target, Selection: tx.trSource, TargetLanguage: tx.trLang, Result: tx.trResult}
			a.answer, a.followUp = answerCard{}, ""
			a.sess = s
			a.openAnswer(s)
			a.overlaySync()
		}, over},
		{"rewrite", func(a *app) {
			first := session.Snapshot{ID: "rd-3", Kind: store.KindRewrite, State: session.Done, Card: true, Delivery: deliver.ReasonCard,
				Target: target, Raw: tx.rwAsk, Title: tx.rwTitle, Result: tx.rwAnswer}
			a.answer, a.followUp = answerCard{}, ""
			a.sess = first
			a.openAnswer(first)
			a.followUp, a.answer.conversation = a.answer.id, true
			next := session.Snapshot{ID: "rd-4", Kind: store.KindRewrite, State: session.Done, Card: true, Delivery: deliver.ReasonCard,
				Target: target, Raw: tx.rwFollow, Title: tx.rwFollowTitle, Result: tx.rwFollowAnswer}
			a.sess = next
			a.openAnswer(next)
			a.overlaySync()
		}, over},
	}
}

// readmeWalk takes the shots, then quits; on a goroutine.
func readmeWalk(a *app, dir, lang string) {
	onMain := func(fn func()) {
		done := make(chan struct{})
		mygo.RunOnMain(func() { fn(); close(done) })
		<-done
	}
	if argAfter("--front") == "1" {
		onMain(func() { mygo.App.Focus(); a.win.Focus() })
	}
	time.Sleep(1500 * time.Millisecond) // the window laid out, the history read
	var editor *mygo.Window
	for n, st := range readmeSteps(lang, &editor) {
		onMain(func() {
			st.apply(a)
			if a.win != nil {
				a.win.Update(func() {})
			}
		})
		time.Sleep(1200 * time.Millisecond) // reads, the overlay's reveal
		wid := 0
		onMain(func() {
			if w := st.win(a); w != nil {
				wid = macx.WindowNumber(w.NativeHandle())
			}
		})
		if wid == 0 {
			fmt.Fprintf(os.Stderr, "readme shots: no window for %s\n", st.name)
			continue
		}
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("step-%d.ready", n)), fmt.Appendf(nil, "%d %s\n", wid, st.name), 0o644)
		done := filepath.Join(dir, fmt.Sprintf("step-%d.done", n))
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			if _, err := os.Stat(done); err == nil {
				break
			}
		}
	}
	os.WriteFile(filepath.Join(dir, "finished"), nil, 0o644)
	onMain(func() {
		a.closeOverlay()
		mygo.App.Quit()
	})
}

// readmeEditor is a plain text window with made-up notes: the app one
// types in, under the overlay's shots.
func readmeEditor(tx readmeText) *mygo.Window {
	view := func(c *ui.Context) {
		t := c.Theme()
		ui.Column(c).Fill().Background(ui.RGB(255, 255, 255)).Padding(28, 36).Gap(10).Children(func() {
			for i, line := range tx.editor {
				el := ui.Text(c.Key(itoa(i)), line).FontSize(px(t, 15)).TextColor(ui.RGB(40, 40, 44))
				if i == 0 || (line != "" && i == len(tx.editor)-1) {
					el.FontSize(px(t, i0(i, 22, 17))).FontWeight(700)
				}
			}
			ui.Box(c.Key("caret")).Width(2).Height(20).Background(ui.RGB(10, 110, 255))
		})
	}
	return mygo.NewWindow(mygo.WindowOptions{Title: tx.editorTitle, Width: 720, Height: 440, UseContentSize: true, Content: ui.View(view)})
}

// i0 is a when i is 0, else b.
func i0(i int, a, b float32) float32 {
	if i == 0 {
		return a
	}
	return b
}

// readmeSeedModels lays the models ids out in hub as a download would:
// each file of its size, sparse (no bytes), in the Hub's layout.
func readmeSeedModels(hub string, ids []string) error {
	for _, id := range ids {
		m, ok := models.Lookup(id)
		if !ok {
			return fmt.Errorf("readme shots: unknown model %q", id)
		}
		files := m.Files
		if len(files) == 0 {
			files = []models.File{{Path: "model.safetensors", Size: m.Bytes, SHA256: fmt.Sprintf("%064x", len(m.ID))}}
		}
		if err := readmeSeedRepo(hub, m.Repo, m.Revision, files); err != nil {
			return err
		}
		for _, x := range m.Aux {
			if err := readmeSeedRepo(hub, x.Repo, x.Revision, x.Files); err != nil {
				return err
			}
		}
	}
	return nil
}

func readmeSeedRepo(hub, repo, revision string, files []models.File) error {
	dir := filepath.Join(hub, "models--"+strings.ReplaceAll(repo, "/", "--"))
	for _, f := range files {
		blob := filepath.Join(dir, "blobs", f.SHA256)
		link := filepath.Join(dir, "snapshots", revision, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(blob), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(blob, nil, 0o644); err != nil {
			return err
		}
		if err := os.Truncate(blob, f.Size); err != nil {
			return err
		}
		target := strings.Repeat("../", strings.Count(f.Path, "/")+2) + "blobs/" + f.SHA256
		if err := os.Symlink(target, link); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}
