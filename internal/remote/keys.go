package remote

import (
	"maps"
	"strings"
	"sync"
)

// Keyring keeps the services' keys by Keychain account ("asr.doubao").
// Its calls may block: off the main thread only.
type Keyring interface {
	Get(account string) (string, error)
	Set(account, label, value string) error // "" deletes
}

// Keys are the services' keys: the Keyring behind a memory of which
// accounts hold a key and its mask ("sk-…wxyz"), never the key itself.
// Has and Mask read memory only (views, Installed); Load, Get and Set
// block. Goroutine-safe.
type Keys struct {
	ring Keyring

	mu    sync.Mutex
	masks map[string]string // by account; "" none; missing: not read
}

// NewKeys is the keys of ring (nil: none can be read or saved).
func NewKeys(ring Keyring) *Keys {
	return &Keys{ring: ring, masks: map[string]string{}}
}

// Has says whether account holds a key, as last read. Memory only.
func (k *Keys) Has(account string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.masks[account] != ""
}

// Mask is account's key's mask ("" none) and whether it was read.
// Memory only.
func (k *Keys) Mask(account string) (string, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	m, ok := k.masks[account]
	return m, ok
}

// Masks is a copy of every mask read. Memory only.
func (k *Keys) Masks() map[string]string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return maps.Clone(k.masks)
}

// Load reads accounts' keys and keeps their masks; the first error is
// returned (its account's mask stays unread).
func (k *Keys) Load(accounts []string) error {
	var first error
	for _, acc := range accounts {
		if _, err := k.Get(acc); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Get reads account's key (and keeps its mask).
func (k *Keys) Get(account string) (string, error) {
	if k.ring == nil {
		k.remember(account, "")
		return "", nil
	}
	v, err := k.ring.Get(account)
	if err != nil {
		return "", err
	}
	k.remember(account, v)
	return v, nil
}

// Set saves account's key ("" deletes it), labelled label in the
// Keychain.
func (k *Keys) Set(account, label, value string) error {
	if k.ring == nil {
		return errNoKeyring
	}
	if err := k.ring.Set(account, label, value); err != nil {
		return err
	}
	k.remember(account, value)
	return nil
}

func (k *Keys) remember(account, value string) {
	m := ""
	if value != "" {
		m = Mask(value)
	}
	k.mu.Lock()
	k.masks[account] = m
	k.mu.Unlock()
}

// Mask shows a key without giving it away: "sk-…wxyz".
func Mask(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("•", len(s))
	}
	return s[:3] + "…" + s[len(s)-4:]
}

type keyringError string

func (e keyringError) Error() string { return string(e) }

const errNoKeyring = keyringError("remote: no keyring")
