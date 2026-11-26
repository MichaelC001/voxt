package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTools(t *testing.T) {
	r := newRig(t, Policy{Enabled: true}, true)
	cs := r.connect("reader")

	ov, isErr := callTool(t, cs, "voxtmini_overview", nil)
	if isErr || ov["appVersion"] != "0.2.0" {
		t.Fatalf("voxtmini_overview: %v", ov)
	}

	hist, isErr := callTool(t, cs, "list_history", map[string]any{"kind": "transcribe", "limit": 10})
	if isErr || len(hist["entries"].([]any)) != 1 {
		t.Fatalf("list_history: %v", hist)
	}

	detail, isErr := callTool(t, cs, "get_history", map[string]any{"id": "h1"})
	if isErr || detail["result"] != "Hello VoxtMini" {
		t.Fatalf("get_history: %v", detail)
	}

	nf, isErr := callTool(t, cs, "get_history", map[string]any{"id": "missing"})
	if !isErr || nf["error"] != "not_found" {
		t.Fatalf("get_history missing: %v (isErr=%v)", nf, isErr)
	}

	if out, isErr := callTool(t, cs, "list_terms", nil); isErr || len(out["terms"].([]any)) != 1 {
		t.Fatalf("list_terms: %v", out)
	}
	if out, isErr := callTool(t, cs, "list_categories", nil); isErr || len(out["categories"].([]any)) != 1 {
		t.Fatalf("list_categories: %v", out)
	}
	if out, isErr := callTool(t, cs, "list_app_groups", nil); isErr || len(out["groups"].([]any)) != 1 {
		t.Fatalf("list_app_groups: %v", out)
	}
	if out, isErr := callTool(t, cs, "list_microphones", nil); isErr || len(out["microphones"].([]any)) != 2 {
		t.Fatalf("list_microphones: %v", out)
	}

	models, isErr := callTool(t, cs, "list_models", map[string]any{"kind": "all"})
	if isErr || len(models["models"].([]any)) != 4 {
		t.Fatalf("list_models: %v", models)
	}
	for _, raw := range models["models"].([]any) {
		m := raw.(map[string]any)
		for k := range m {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "key") || strings.Contains(lk, "secret") || strings.Contains(lk, "token") {
				t.Errorf("list_models exposed sensitive field %q", k)
			}
		}
	}
}

func TestManageTools(t *testing.T) {
	r := newRig(t, Policy{Enabled: true, AllowManage: true}, true)
	cs := r.connect("manager")

	out, isErr := callTool(t, cs, "upsert_terms", map[string]any{
		"terms": []map[string]any{
			{"spelling": "MyGo", "hints": []string{"my go"}, "category": "Tech"},
			{"spelling": "MLX", "hints": []string{"m l x"}, "category": "Tech"},
		},
	})
	if isErr || len(out["terms"].([]any)) != 2 {
		t.Fatalf("upsert_terms: %v", out)
	}

	if out, isErr := callTool(t, cs, "delete_term", map[string]any{"id": "t1"}); isErr || out["deleted"] != true {
		t.Fatalf("delete_term: %v", out)
	}
	if out, isErr := callTool(t, cs, "delete_term", map[string]any{"id": "t1"}); !isErr || out["error"] != "not_found" {
		t.Fatalf("delete_term second time: %v", out)
	}

	if out, isErr := callTool(t, cs, "upsert_category", map[string]any{"name": "Work"}); isErr || out["name"] != "Work" {
		t.Fatalf("upsert_category: %v", out)
	}
	if out, isErr := callTool(t, cs, "upsert_app_group", map[string]any{
		"name":      "Browser",
		"domains":   []string{"github.com"},
		"after_key": "Return",
	}); isErr || out["name"] != "Browser" {
		t.Fatalf("upsert_app_group: %v", out)
	}
	if out, isErr := callTool(t, cs, "delete_app_group", map[string]any{"id": "g1"}); isErr || out["deleted"] != true {
		t.Fatalf("delete_app_group: %v", out)
	}
	if out, isErr := callTool(t, cs, "delete_history", map[string]any{"id": "h1"}); isErr || out["deleted"] != true {
		t.Fatalf("delete_history: %v", out)
	}
}

func TestModelToolsAndAudioValidation(t *testing.T) {
	r := newRig(t, Policy{Enabled: true, AllowLocalModels: true, AllowRemoteModels: false}, true)
	cs := r.connect("model-user")

	// Valid audio file inside allowed root.
	wavPath := filepath.Join(r.audioDir, "clip.wav")
	if err := os.WriteFile(wavPath, []byte("RIFF0000WAVEfmt "), 0o644); err != nil {
		t.Fatal(err)
	}
	out, isErr := callTool(t, cs, "transcribe_audio", map[string]any{"audio_path": wavPath})
	if isErr || out["text"] != "transcribed audio text" || out["model"] != "local-asr" {
		t.Fatalf("transcribe_audio valid: %v", out)
	}

	// Relative path -> invalid_path.
	if out, isErr := callTool(t, cs, "transcribe_audio", map[string]any{"audio_path": "clip.wav"}); !isErr || out["error"] != "invalid_path" {
		t.Fatalf("relative path: %v", out)
	}

	// Missing file -> not_found.
	if out, isErr := callTool(t, cs, "transcribe_audio", map[string]any{"audio_path": filepath.Join(r.audioDir, "nope.wav")}); !isErr || out["error"] != "not_found" {
		t.Fatalf("missing file: %v", out)
	}

	// Unsupported extension -> invalid_path.
	txtPath := filepath.Join(r.audioDir, "secret.txt")
	_ = os.WriteFile(txtPath, []byte("hello"), 0o644)
	if out, isErr := callTool(t, cs, "transcribe_audio", map[string]any{"audio_path": txtPath}); !isErr || out["error"] != "invalid_path" {
		t.Fatalf("bad extension: %v", out)
	}

	// Symlink escaping allowed root -> invalid_path.
	outsideDir := t.TempDir()
	outsideWav := filepath.Join(outsideDir, "escape.wav")
	_ = os.WriteFile(outsideWav, []byte("RIFF0000WAVE"), 0o644)
	linkPath := filepath.Join(r.audioDir, "link.wav")
	_ = os.Symlink(outsideWav, linkPath)
	if out, isErr := callTool(t, cs, "transcribe_audio", map[string]any{"audio_path": linkPath}); !isErr || out["error"] != "invalid_path" {
		t.Fatalf("symlink escape: %v", out)
	}

	// Oversized file (>50 MB) -> too_large.
	bigPath := filepath.Join(r.audioDir, "big.wav")
	f, err := os.Create(bigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxAudioBytes + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	if out, isErr := callTool(t, cs, "transcribe_audio", map[string]any{"audio_path": bigPath}); !isErr || out["error"] != "too_large" {
		t.Fatalf("oversized file: %v", out)
	}

	// Remote model blocked when AllowRemoteModels is false -> remote_off.
	if out, isErr := callTool(t, cs, "transcribe_audio", map[string]any{"audio_path": wavPath, "model_id": "remote-asr"}); !isErr || out["error"] != "remote_off" {
		t.Fatalf("remote_asr blocked: %v", out)
	}
	if out, isErr := callTool(t, cs, "translate_text", map[string]any{"text": "hi", "target_language": "zh-Hans", "model_id": "remote-llm"}); !isErr || out["error"] != "remote_off" {
		t.Fatalf("remote_llm blocked: %v", out)
	}

	// Local translate, rewrite, chat succeed.
	if out, isErr := callTool(t, cs, "translate_text", map[string]any{"text": "hello", "target_language": "zh-Hans"}); isErr || out["targetLanguage"] != "zh-Hans" {
		t.Fatalf("translate_text: %v", out)
	}
	if out, isErr := callTool(t, cs, "rewrite_text", map[string]any{"instruction": "make formal", "text": "hey"}); isErr || out["text"] != "rewritten:make formal" {
		t.Fatalf("rewrite_text: %v", out)
	}
	if out, isErr := callTool(t, cs, "chat_model", map[string]any{"prompt": "ping"}); isErr || out["text"] != "reply:ping" {
		t.Fatalf("chat_model: %v", out)
	}

	// Enable AllowRemoteModels -> remote succeeds.
	r.host.mu.Lock()
	r.host.policy.AllowRemoteModels = true
	r.host.mu.Unlock()
	if out, isErr := callTool(t, cs, "transcribe_audio", map[string]any{"audio_path": wavPath, "model_id": "remote-asr"}); isErr || out["source"] != "remote" {
		t.Fatalf("remote_asr allowed: %v", out)
	}
}

func TestTriggerTools(t *testing.T) {
	r := newRig(t, Policy{Enabled: true, AllowTrigger: true}, true)
	cs := r.connect("trigger-user")

	st, isErr := callTool(t, cs, "session_status", nil)
	if isErr || st["state"] != "idle" {
		t.Fatalf("session_status: %v", st)
	}

	tr, isErr := callTool(t, cs, "trigger_session", map[string]any{
		"feature": "transcribe",
		"action":  "start",
		"device":  "USB-Mic-1",
	})
	if isErr || tr["state"] != "recording" || tr["microphone"] != "USB-Mic-1" {
		t.Fatalf("trigger_session: %v", tr)
	}

	r.host.mu.Lock()
	r.host.busy = true
	r.host.mu.Unlock()
	busy, isErr := callTool(t, cs, "trigger_session", map[string]any{
		"feature": "transcribe",
		"action":  "start",
	})
	if !isErr || busy["error"] != "busy" {
		t.Fatalf("trigger_session busy: %v", busy)
	}
}
