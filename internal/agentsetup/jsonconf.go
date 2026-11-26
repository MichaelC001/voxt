package agentsetup

// Order-preserving JSON upsert for mcpServers.voxtmini (Claude Code,
// Claude Desktop, Cursor, WorkBuddy). Duplicate keys return ErrEditByHand.

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
)

type member struct {
	key string
	val json.RawMessage
}

func members(data []byte) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	var out []member
	seen := map[string]bool{}
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return nil, err
		}
		ks := k.(string)
		if seen[ks] {
			return nil, ErrEditByHand
		}
		seen[ks] = true
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, member{ks, v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("trailing data after JSON object")
	}
	return out, nil
}

func object(ms []member) []byte {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range ms {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(m.key)
		b.Write(k)
		b.WriteByte(':')
		b.Write(m.val)
	}
	b.WriteByte('}')
	return b.Bytes()
}

func set(ms []member, key string, val json.RawMessage) []member {
	for i := range ms {
		if ms[i].key == key {
			ms[i].val = val
			return ms
		}
	}
	return append(ms, member{key, val})
}

func get(ms []member, key string) json.RawMessage {
	for _, m := range ms {
		if m.key == key {
			return m.val
		}
	}
	return nil
}

func sameJSON(a, b []byte) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func withJSON(old []byte, cmd Command) ([]byte, error) {
	if len(bytes.TrimSpace(old)) == 0 {
		old = []byte("{}")
	}
	top, err := members(old)
	if err != nil {
		return nil, err
	}
	servers := []member{}
	if raw := get(top, "mcpServers"); raw != nil {
		if servers, err = members(raw); errors.Is(err, ErrEditByHand) {
			return nil, err
		} else if err != nil {
			return nil, errors.New("mcpServers is not a JSON object")
		}
	}
	args := cmd.Args
	if args == nil {
		args = []string{}
	}
	entry, _ := json.Marshal(map[string]any{"command": cmd.Path, "args": args})
	if cur := get(servers, ServerName); cur != nil && sameJSON(cur, entry) {
		return nil, nil
	}
	top = set(top, "mcpServers", object(set(servers, ServerName, entry)))
	var out bytes.Buffer
	if err := json.Indent(&out, object(top), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
