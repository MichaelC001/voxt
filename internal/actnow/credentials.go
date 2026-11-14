package actnow

// API keys shared with the family (spec §8): a service names the family's
// key of its vendor (settings.Provider.CredentialRef) instead of keeping
// a key of its own. The family's key lives in the login Keychain
// ("ActNow" / "vendor.<id>"), its hint and users in credentials.json
// (actnowkit's credentials.Store); VoxtMini reads it only after the user
// chose "Use this key" here (Store.Use), never before, not even for a
// connection test.

import (
	"context"
	"errors"
	"strings"

	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/actnowkit/credentials"
)

// Credentials is VoxtMini's access to the family's keys through kc (the
// zero Keychain: /usr/bin/security); nil when VoxtMini may not write the
// family directory (ReadOnly) or outside the family.
func (f *Family) Credentials(kc credentials.Keychain) *credentials.Store {
	if f == nil || f.readOnly {
		return nil
	}
	return &credentials.Store{Family: f.dir, BundleID: f.bundleID, Keychain: kc}
}

// extraVendors are VoxtMini's services that Auralink does not have,
// whose key is a family vendor's (spec §8.1: one vendor account, whatever
// an app uses it for).
var extraVendors = map[string]string{
	"asr.gemini": "gemini", // a Gemini API key serves the Live API too
	"asr.glm":    "zhipu",  // Z.ai's GLM-ASR takes the Zhipu key
	"asr.mimo":   "mimo",
}

// Vendor is the family vendor whose key a service of VoxtMini may share,
// by its Keychain account ("asr.aliyun", "llm.deepseek") and its endpoint
// ("" for the preset's). VoxtMini's services are Auralink's (the same
// accounts), so the family's table of Auralink's accounts decides, with
// VoxtMini's own services added (extraVendors). Custom services, local
// servers and the ChatGPT sign-in share none.
func Vendor(account, endpoint string) (credentials.Vendor, bool) {
	if id, ok := extraVendors[account]; ok {
		return credentials.VendorByID(id)
	}
	return credentials.VendorForLegacy("auralink", account, endpoint)
}

// SaveOptions are the family's fields of a service's key: Doubao speech's
// App ID, with which its key is an access token (spec §8.2).
func SaveOptions(vendor string, p settings.Provider) credentials.SaveOptions {
	if vendor == "volcengine-speech" && p.AppID != "" {
		return credentials.SaveOptions{AppID: p.AppID, Auth: credentials.AuthAppIDAccessToken}
	}
	return credentials.SaveOptions{}
}

// Ref is a service's CredentialRef for the family's key of vendor.
func Ref(vendor string) string { return credentials.Ref(vendor) }

// RefVendor is the vendor of the family key p uses, "" for its own key.
func RefVendor(p settings.Provider) string {
	v, _ := credentials.ParseRef(p.CredentialRef)
	return v
}

// SharedKeys is the family's keys as VoxtMini reads them
// (credentials.Store).
type SharedKeys interface {
	Secret(ctx context.Context, vendor string) (string, error)
}

// OwnKeys is a keyring of VoxtMini's own items (internal/remote.Keyring).
type OwnKeys interface {
	Get(account string) (string, error)
	Set(account, label, value string) error
}

// Ring reads a service's key: VoxtMini's own item first, then, for a
// service that names a family key (Refs), the family's, once VoxtMini
// agreed to use it. A family key not agreed to, or gone, is no key.
// Goroutine-safe when its functions are; it blocks (the Keychain).
type Ring struct {
	Own OwnKeys
	// Refs are the vendors of the services that name a family key, by
	// account (the settings now).
	Refs func() map[string]string
	// Shared is the family's keys (nil: outside the family, not read yet).
	Shared func() SharedKeys
}

// Get is account's key: "" when there is none to use.
func (r Ring) Get(account string) (string, error) {
	v, err := r.Own.Get(account)
	if err != nil || v != "" || r.Refs == nil || r.Shared == nil {
		return v, err
	}
	vendor := r.Refs()[account]
	sk := r.Shared()
	if vendor == "" || sk == nil {
		return "", nil
	}
	s, err := sk.Secret(context.Background(), vendor)
	switch {
	case errors.Is(err, credentials.ErrNotConsented), errors.Is(err, credentials.ErrMissing):
		return "", nil
	case err != nil:
		return "", err
	}
	return strings.TrimSpace(s), nil
}

// Set writes VoxtMini's own item (the family's is written through the
// Store when a key is shared).
func (r Ring) Set(account, label, value string) error { return r.Own.Set(account, label, value) }
