// Package secrets keeps VoxtMini's secrets (provider API keys) in the
// macOS login Keychain, as generic password items of the service
// "VoxtMini", one account per secret: "asr.<provider>" and "llm.<provider>"
// (settings.Provider.ID). Secrets never go into the settings, the
// database or the logs.
//
// It runs /usr/bin/security, as AudioSmith's hub does (this file is its
// Keychain code, hub/token.go, without the Hugging Face token). Every
// function may block: call them off the main thread. Off macOS there is
// no Keychain: Get finds nothing and Set fails with ErrNoKeychain.
//
// Imports: nothing internal.
package secrets

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Service is the Keychain service of VoxtMini's secrets.
const Service = "VoxtMini"

// ErrNoKeychain is returned off macOS, where there is no Keychain to use.
var ErrNoKeychain = errors.New("no keychain on this system")

// Item is a generic password item of the login keychain (or of the
// keychain file File, for tests). Label is what Keychain Access shows; ""
// uses "VoxtMini <account>".
type Item struct{ Service, Account, Label, File string }

// Secret is the Keychain item of one of VoxtMini's secrets, e.g.
// Secret("llm.deepseek", "VoxtMini DeepSeek API key").
func Secret(account, label string) Item {
	return Item{Service: Service, Account: account, Label: label}
}

// ASR and LLM name the items of a provider's key by its settings id.
func ASR(provider, label string) Item { return Secret("asr."+provider, label) }
func LLM(provider, label string) Item { return Secret("llm."+provider, label) }

// with appends the keychain file to a security command, when there is one.
func (k Item) with(args ...string) []string {
	if k.File != "" {
		args = append(args, k.File)
	}
	return args
}

// security runs /usr/bin/security with args, feeding it stdin, and
// returns its standard output and error; swapped in tests.
var security = func(stdin string, args ...string) (stdout, stderr []byte, err error) {
	if runtime.GOOS != "darwin" {
		return nil, nil, ErrNoKeychain
	}
	cmd := exec.Command("/usr/bin/security", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var eb bytes.Buffer
	cmd.Stderr = &eb
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(eb.String()); msg != "" {
			return out, eb.Bytes(), fmt.Errorf("security: %s", msg)
		}
		return out, eb.Bytes(), err
	}
	return out, eb.Bytes(), nil
}

// missing reports an error that means the item is not there.
func missing(err error) bool {
	return errors.Is(err, ErrNoKeychain) || strings.Contains(err.Error(), "could not be found") || isExit(err, 44)
}

// Get returns the secret saved, "" when there is none. It uses -g rather
// than -w, as actnowkit's credentials do: -w prints a secret with quotes,
// backslashes or non-ASCII (a sign-in's JSON) as bare hex, which cannot be
// told apart from a key made of hex digits; -g marks hex with "0x".
func (k Item) Get() (string, error) {
	_, stderr, err := security("", k.with("find-generic-password", "-s", k.Service, "-a", k.Account, "-g")...)
	if err != nil {
		if missing(err) {
			return "", nil
		}
		return "", err
	}
	v, err := parsePassword(string(stderr))
	return strings.TrimSpace(v), err
}

// parsePassword reads security -g's password line (actnowkit's
// credentials.parsePassword):
//
//	password: "printable"
//	password: 0xE5AF86…  "\345\257…"
//	password:            (empty)
func parsePassword(out string) (string, error) {
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(line, "password:")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		switch {
		case rest == "":
			return "", nil
		case strings.HasPrefix(rest, "0x"):
			h, _, _ := strings.Cut(rest[2:], " ")
			b, err := hex.DecodeString(h)
			if err != nil {
				return "", fmt.Errorf("security: bad password line: %w", err)
			}
			return string(b), nil
		case len(rest) >= 2 && rest[0] == '"' && rest[len(rest)-1] == '"':
			return rest[1 : len(rest)-1], nil
		}
		return "", errors.New("security: unreadable password line")
	}
	return "", errors.New("security: no password in the output")
}

// safeWord: names passed through security's interactive mode go in plain
// double quotes, so they must not hold quotes, backslashes or line breaks.
func safeWord(s string) bool { return !strings.ContainsAny(s, "\"\\\n\r\x00") }

// Set saves value, replacing what was saved; "" deletes it. The value goes
// to security on its standard input (interactive mode), hex-encoded (-X,
// as actnowkit's credentials), not its arguments, so that other processes
// cannot read it from the process list; any value works (a sign-in's
// JSON too).
func (k Item) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return k.Delete()
	}
	label := k.Label
	if label == "" {
		label = "VoxtMini " + k.Account
	}
	if !safeWord(k.Service) || !safeWord(k.Account) || !safeWord(label) || !safeWord(k.File) {
		return errors.New("secrets: service, account, label or keychain name has quotes, backslashes or line breaks")
	}
	cmd := fmt.Sprintf(`add-generic-password -U -s "%s" -a "%s" -l "%s" -X %s`, k.Service, k.Account, label, hex.EncodeToString([]byte(value)))
	if k.File != "" {
		cmd += ` "` + k.File + `"`
	}
	_, _, err := security(cmd+"\n", "-i")
	return err
}

// Delete removes the saved secret; none saved is not an error.
func (k Item) Delete() error {
	_, _, err := security("", k.with("delete-generic-password", "-s", k.Service, "-a", k.Account)...)
	if err != nil && missing(err) {
		return nil
	}
	return err
}

func isExit(err error, code int) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == code
}
