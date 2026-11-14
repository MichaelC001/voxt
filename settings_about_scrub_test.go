package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestBundleScrubbed: the diagnostic bundle's settings keep their shape
// but not a key in an endpoint's query, an extra body or a prompt; its
// side logs lose the keys a line held.
func TestBundleScrubbed(t *testing.T) {
	var prefs settings.Settings
	prefs.Providers.LLM = []settings.Provider{{ID: "custom", Endpoint: "https://llm.example/v1?key=AIzaSecretSecretSecretSecretSecret12&x=1",
		ExtraBody: `{"token":"tok-123456"}`, Prompt: "my private words"}}
	prefs.Features.Transcribe.Prompt = "spell my boss's name Zhang"
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	os.MkdirAll(logs, 0o755)
	os.WriteFile(filepath.Join(logs, "helper.log"), []byte("request with key sk-abcdefghijklmnopqrstu and Bearer abcdefghijklmnop\nAuthorization: Bearer qrstuvwxyz012345\n{\"refresh_token\":\"rt-456\"}\nplain line\n"), 0o644)
	out := filepath.Join(dir, "bundle.zip")
	if err := writeBundle(out, "diag", prefs, logs); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	files := map[string]string{}
	for _, f := range zr.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		files[f.Name] = string(b)
	}
	s := files["settings.json"]
	for _, secret := range []string{"AIzaSecret", "tok-123456", "my private words", "Zhang"} {
		if strings.Contains(s, secret) {
			t.Errorf("settings.json holds %q:\n%s", secret, s)
		}
	}
	for _, kept := range []string{`"custom"`, "https://llm.example/v1?key=REDACTED&x=1", `"<16 chars>"`, `"<26 chars>"`} {
		if !strings.Contains(s, kept) {
			t.Errorf("settings.json lacks %q:\n%s", kept, s)
		}
	}
	if l := files["logs/helper.log"]; strings.Contains(l, "abcdefghijklmnop") || strings.Contains(l, "sk-abc") ||
		strings.Contains(l, "qrstuvwxyz012345") || strings.Contains(l, "rt-456") ||
		!strings.Contains(l, "Authorization: Bearer REDACTED") || !strings.Contains(l, "plain line\n") {
		t.Errorf("helper.log %q", l)
	}
}
