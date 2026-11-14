package actnow

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/hehehai/actnowkit/credentials"
)

// FakeKeychain is a family Keychain in memory, for tests (this package's
// and the app's): it answers the /usr/bin/security commands
// credentials.Keychain runs. Never the real Keychain in tests.
type FakeKeychain struct {
	mu    sync.Mutex
	items map[string]string // by account
}

// NewFakeKeychain is an empty FakeKeychain.
func NewFakeKeychain() *FakeKeychain { return &FakeKeychain{items: map[string]string{}} }

// Keychain is the credentials.Keychain that uses it.
func (f *FakeKeychain) Keychain() credentials.Keychain { return credentials.Keychain{Run: f.run} }

// Item is the secret saved under account ("vendor.deepseek"), "" none.
func (f *FakeKeychain) Item(account string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.items[account]
}

func (f *FakeKeychain) run(stdin string, args ...string) ([]byte, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	arg := func(flag string) string {
		for i, a := range args {
			if a == flag && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	switch {
	case len(args) == 1 && args[0] == "-i": // add-generic-password … -a "<account>" … -X <hex>
		acc, hx := "", ""
		fs := strings.Fields(stdin)
		for i, w := range fs {
			if w == "-a" && i+1 < len(fs) {
				acc = strings.Trim(fs[i+1], `"`)
			}
			if w == "-X" && i+1 < len(fs) {
				hx = fs[i+1]
			}
		}
		b, err := hex.DecodeString(hx)
		if acc == "" || err != nil {
			return nil, nil, fmt.Errorf("fake security: bad add %q", stdin)
		}
		f.items[acc] = string(b)
		return nil, nil, nil
	case len(args) > 0 && args[0] == "find-generic-password":
		v, ok := f.items[arg("-a")]
		if !ok {
			return nil, nil, credentials.ErrNotFound
		}
		return nil, []byte("password: 0x" + hex.EncodeToString([]byte(v)) + "\n"), nil
	case len(args) > 0 && args[0] == "delete-generic-password":
		if _, ok := f.items[arg("-a")]; !ok {
			return nil, nil, credentials.ErrNotFound
		}
		delete(f.items, arg("-a"))
		return nil, nil, nil
	}
	return nil, nil, fmt.Errorf("fake security: %v", args)
}
