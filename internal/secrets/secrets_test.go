package secrets

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestItems(t *testing.T) {
	saved := security
	defer func() { security = saved }()
	var stdin string
	var args []string
	security = func(in string, a ...string) ([]byte, []byte, error) {
		stdin, args = in, a
		if a[0] == "find-generic-password" {
			return nil, []byte("keychain: \"login\"\nattributes:\npassword: \"sk-fromkeychain\"\n"), nil
		}
		return nil, nil, nil
	}
	k := LLM("deepseek", "VoxtMini DeepSeek API key")
	if k.Service != "VoxtMini" || k.Account != "llm.deepseek" || ASR("doubao", "").Account != "asr.doubao" {
		t.Fatalf("items: %+v", k)
	}
	if err := k.Set(" sk-abc "); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(args, []string{"-i"}) || !strings.Contains(stdin, `-s "VoxtMini" -a "llm.deepseek" -l "VoxtMini DeepSeek API key" -X `+hex.EncodeToString([]byte("sk-abc"))+"\n") ||
		strings.Contains(stdin, "sk-abc") || strings.Contains(strings.Join(args, " "), "sk-abc") {
		t.Errorf("set: args %q stdin %q", args, stdin)
	}
	// A sign-in's JSON has quotes and backslashes: hex carries it as is.
	const tokens = `{"access_token":"a\"b","refresh_token":"r"}`
	if err := k.Set(tokens); err != nil || !strings.Contains(stdin, "-X "+hex.EncodeToString([]byte(tokens))) {
		t.Errorf("json value: %v, stdin %q", err, stdin)
	}
	if err := Secret("a", `bad"label`).Set("v"); err == nil {
		t.Error("accepted a quote in the label")
	}
	if got, err := k.Get(); got != "sk-fromkeychain" || err != nil {
		t.Errorf("get: %q %v", got, err)
	}
	k.Set("")
	if args[0] != "delete-generic-password" {
		t.Errorf("empty value deletes: %q", args)
	}
	security = func(string, ...string) ([]byte, []byte, error) { return nil, nil, ErrNoKeychain }
	if got, err := k.Get(); got != "" || err != nil {
		t.Errorf("no keychain: %q %v", got, err)
	}
	if err := k.Delete(); err != nil {
		t.Errorf("delete without a keychain: %v", err)
	}
	if Mask("sk-abcdefghijkl") != "sk-…ijkl" || Mask("short") != "•••••" {
		t.Error(Mask("sk-abcdefghijkl"))
	}
}

func TestParsePassword(t *testing.T) {
	json := `{"a":"b\\c"}`
	for _, c := range []struct{ in, want string }{
		{"password: \"sk-abc\"\n", "sk-abc"},
		{"attributes:\n    \"acct\"<blob>=\"x\"\npassword: 0x" + strings.ToUpper(hex.EncodeToString([]byte(json))) + "  \"{\\\"a\\\"...\"\n", json},
		{"password: 0x" + hex.EncodeToString([]byte("密钥")) + "\n", "密钥"},
		{"password: \n", ""},
	} {
		if got, err := parsePassword(c.in); got != c.want || err != nil {
			t.Errorf("parsePassword(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "password: 0xZZ\n", "password: plain\n"} {
		if _, err := parsePassword(bad); err == nil {
			t.Errorf("parsePassword(%q) accepted", bad)
		}
	}
}

// TestMacKeychain saves, reads, replaces and removes a secret through
// /usr/bin/security. It runs with AURALINK_MAC=1 on macOS:
//
//	AURALINK_MAC=1 go test -run TestMacKeychain -v ./internal/secrets
func TestMacKeychain(t *testing.T) {
	if os.Getenv("AURALINK_MAC") == "" || runtime.GOOS != "darwin" {
		t.Skip("set AURALINK_MAC=1 on a Mac")
	}
	// The login keychain is locked in an SSH session ("User interaction is
	// not allowed"), so the commands run against a keychain of their own;
	// the app uses the login keychain, unlocked in the user's session.
	k := Item{Service: "VoxtMini (test)", Account: "llm.test"}
	if _, _, err := security("", "show-keychain-info"); err != nil && strings.Contains(err.Error(), "User interaction is not allowed") {
		file := filepath.Join(t.TempDir(), "test.keychain-db")
		if out, err := exec.Command("/usr/bin/security", "create-keychain", "-p", "test", file).CombinedOutput(); err != nil {
			t.Fatalf("create-keychain: %v %s", err, out)
		}
		defer exec.Command("/usr/bin/security", "delete-keychain", file).Run()
		if out, err := exec.Command("/usr/bin/security", "unlock-keychain", "-p", "test", file).CombinedOutput(); err != nil {
			t.Fatalf("unlock-keychain: %v %s", err, out)
		}
		k.File = file
		t.Log("login keychain locked (SSH session): testing against a temporary keychain")
	}
	k.Delete()
	const key = "sk-probe-0123456789abcdef"
	if err := k.Set(key); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got, err := k.Get(); err != nil || got != key {
		t.Fatalf("read back %q, %v", Mask(got), err)
	}
	if err := k.Set(key + "x"); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if got, _ := k.Get(); got != key+"x" {
		t.Errorf("after replacing: %q", Mask(got))
	}
	const tokens = `{"access_token":"a\\"b","email":"ü@example.com"}`
	if err := k.Set(tokens); err != nil {
		t.Fatalf("save json: %v", err)
	}
	if got, err := k.Get(); err != nil || got != tokens {
		t.Errorf("json read back %d bytes, %v", len(got), err)
	}
	if err := k.Set(""); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got, err := k.Get(); got != "" {
		t.Errorf("after removing: %q %v", Mask(got), err)
	}
}

// Mask shows a secret without giving it away in test output: "sk-…wxyz".
func Mask(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("•", len(s))
	}
	return s[:3] + "…" + s[len(s)-4:]
}
