package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/core"
)

// TestRingRedacts: what the ring keeps and what reaches the file have lost
// their credentials but kept the words around them (actnowkit's
// redact.Text, v0.18.0: before it, "Authorization: Bearer <token>" kept the
// token and lost the word Bearer).
func TestRingRedacts(t *testing.T) {
	cases := []struct{ in, want, secret string }{
		{"Authorization: Bearer abcdefghijklmnop", "Authorization: Bearer REDACTED", "abcdefghijklmnop"},
		{"Proxy-Authorization: Basic dXNlcjpwYXNzd29yZA==", "Proxy-Authorization: Basic REDACTED", "dXNlcjpw"},
		{"x-api-key: abcdef0123456789", "x-api-key: REDACTED", "abcdef0123456789"},
		{"Cookie: session=abc123; csrf=def456", "Cookie: REDACTED", "abc123"},
		{`token refresh {"access_token":"at-123","refresh_token":"rt-456","expires_in":3600}`, `token refresh {"access_token":"REDACTED","refresh_token":"REDACTED","expires_in":3600}`, "at-123"},
		{"POST /v1beta/models/g:generate?key=AIzaSyA1234567890abcdefghijklmnopqrs&alt=sse", "POST /v1beta/models/g:generate?key=REDACTED&alt=sse", "AIza"},
		{"remote llm: 401 for sk-proj-abcdefghijklmnopqrstuvwx", "remote llm: 401 for REDACTED", "sk-proj"},
		{"loaded mlx-community/Qwen3-ASR-0.6B-4bit in 2.1 s", "loaded mlx-community/Qwen3-ASR-0.6B-4bit in 2.1 s", ""},
	}
	dir := t.TempDir()
	r := New(Options{Size: 32, Dir: dir})
	for _, c := range cases {
		r.Append(core.LogEntry{Source: "app", Message: c.in})
		if got := r.Tail(1, "")[0].Message; got != c.want {
			t.Errorf("ring: %q\n got %q\nwant %q", c.in, got, c.want)
		}
		if got := Redact(c.in); got != c.want {
			t.Errorf("Redact(%q) = %q", c.in, got)
		}
	}
	r.Close()
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if c.secret != "" && strings.Contains(string(b), c.secret) {
			t.Errorf("%s holds %q:\n%s", FileName, c.secret, b)
		}
	}
}
