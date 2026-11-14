package actnow

import (
	"context"
	"testing"

	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/actnowkit/conformance"
	"github.com/hehehai/actnowkit/credentials"
)

func TestVendor(t *testing.T) {
	for _, c := range []struct{ acc, endpoint, want string }{
		{"llm.deepseek", "", "deepseek"},
		{"asr.aliyun", "", "dashscope"},
		{"llm.dashscope", "", "dashscope"},
		{"asr.doubao", "", "volcengine-speech"},
		{"llm.volcengine", "", "volcengine-ark"},
		{"asr.openai", "", "openai"},
		{"asr.openai", "https://relay.example.com/v1/audio/transcriptions", ""},
		{"llm.moonshot", "https://api.moonshot.ai/v1", "moonshot-intl"},
		{"asr.gemini", "", "gemini"},
		{"asr.glm", "", "zhipu"},
		{"asr.mimo", "", "mimo"},
		{"llm.custom", "", ""},
		{"llm.custom-2", "", ""},
		{"llm.ollama", "", ""},
	} {
		v, ok := Vendor(c.acc, c.endpoint)
		if v.ID != c.want || ok != (c.want != "") {
			t.Errorf("%s %s: %q %v, want %q", c.acc, c.endpoint, v.ID, ok, c.want)
		}
	}
	if o := SaveOptions("volcengine-speech", settings.Provider{AppID: "123"}); o.AppID != "123" || o.Auth != credentials.AuthAppIDAccessToken {
		t.Error(o)
	}
	if RefVendor(settings.Provider{CredentialRef: Ref("deepseek")}) != "deepseek" || RefVendor(settings.Provider{}) != "" {
		t.Error("ref")
	}
}

type memOwn map[string]string

func (m memOwn) Get(a string) (string, error) { return m[a], nil }
func (m memOwn) Set(a, _, v string) error     { m[a] = v; return nil }

// TestRing: VoxtMini's own item first; a service naming a family key
// reads it only once VoxtMini agreed to use it; a key gone is none.
func TestRing(t *testing.T) {
	fx := conformance.New(t)
	ctx := context.Background()
	kc := NewFakeKeychain()
	f := open(t, fx, settings.Defaults())
	cs := f.Credentials(kc.Keychain())
	other := &credentials.Store{Family: fx.Family, BundleID: "dev.hehehai.auralink", Keychain: kc.Keychain()}
	if err := other.Save(ctx, "deepseek", "sk-shared-0001", credentials.SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	own := memOwn{"llm.openai": "sk-own"}
	refs := map[string]string{"llm.deepseek": "deepseek"}
	r := Ring{Own: own, Refs: func() map[string]string { return refs }, Shared: func() SharedKeys { return cs }}
	if v, _ := r.Get("llm.openai"); v != "sk-own" {
		t.Error("own", v)
	}
	if v, err := r.Get("llm.deepseek"); v != "" || err != nil {
		t.Errorf("read before consent: %q %v", v, err)
	}
	if _, err := cs.Use(ctx, "deepseek"); err != nil {
		t.Fatal(err)
	}
	if v, _ := r.Get("llm.deepseek"); v != "sk-shared-0001" {
		t.Error("shared", v)
	}
	if err := other.DeleteForAll(ctx, "deepseek"); err != nil {
		t.Fatal(err)
	}
	if v, err := r.Get("llm.deepseek"); v != "" || err != nil {
		t.Errorf("gone: %q %v", v, err)
	}
	if v, _ := (Ring{Own: own}).Get("llm.deepseek"); v != "" {
		t.Error("outside the family", v)
	}
}
