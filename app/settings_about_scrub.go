package main

// What the diagnostic bundle (writeBundle) says of the settings and the
// side logs: enough to see how VoxtMini is set up, not what the user
// wrote. The settings hold no key, but an endpoint may carry one in its
// query, ExtraBody may carry a token, and the prompts are the user's own
// words: endpoints and other URLs go through redact.URL, prompts and
// extra bodies become their length, every other string goes through
// redact.Text. The side logs (helper.log, llm.log, written raw by their
// processes' supervisors) go through redact.Text line by line.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/actnowkit/redact"
)

// scrubSettings is the settings as the bundle's settings.json.
func scrubSettings(prefs settings.Settings) []byte {
	data, err := json.Marshal(prefs)
	if err != nil {
		return nil
	}
	var doc any
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // "<16 chars>", "&" in a URL: as written
	enc.SetIndent("", "  ")
	enc.Encode(scrubValue("", doc))
	return b.Bytes()
}

// scrubValue scrubs v, the value of key in its object ("" in an array
// or at the top).
func scrubValue(key string, v any) any {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			v[k] = scrubValue(k, x)
		}
		return v
	case []any:
		for i, x := range v {
			v[i] = scrubValue(key, x)
		}
		return v
	case string:
		return scrubString(key, v)
	}
	return v
}

// scrubString scrubs a string by its key's name.
func scrubString(key, s string) string {
	k := strings.ToLower(key)
	switch {
	case s == "":
		return s
	case strings.Contains(k, "prompt") || strings.Contains(k, "extrabody") || strings.Contains(k, "instruction"):
		return fmt.Sprintf("<%d chars>", utf8.RuneCountInString(s))
	case strings.Contains(k, "endpoint") || strings.Contains(k, "url") || strings.Contains(k, "proxy"):
		return redact.URL(s)
	}
	return redact.Text(s)
}

// copyRedacted copies a log file line by line through redact.Text.
func copyRedacted(w io.Writer, r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	bw := bufio.NewWriter(w)
	for sc.Scan() {
		bw.WriteString(redact.Text(sc.Text()))
		bw.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return bw.Flush()
}
