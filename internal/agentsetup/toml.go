package agentsetup

// Safe TOML upsert for Codex (~/.codex/config.toml): manages only the
// [mcp_servers.voxtmini] table, keeping sub-tables and other tables intact.

import (
	"bytes"
	"encoding/json"
	"strings"
)

const tomlHeader = "[mcp_servers." + ServerName + "]"

func tomlBody(cmd Command) []string {
	q, _ := json.Marshal(cmd.Path)
	args := cmd.Args
	if args == nil {
		args = []string{}
	}
	a, _ := json.Marshal(args)
	return []string{
		"command = " + string(q),
		"args = " + string(a),
		"startup_timeout_sec = 60",
		"tool_timeout_sec = 180",
	}
}

func withTOML(old []byte, cmd Command) ([]byte, error) {
	text := strings.ReplaceAll(string(old), "\r\n", "\n")
	if strings.Contains(text, `"""`) || strings.Contains(text, "'''") {
		return nil, ErrEditByHand
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	header := -1
	inServers := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "[") {
			name := strings.ReplaceAll(strings.Trim(stripComment(t), "[] \t"), " ", "")
			inServers = name == "mcp_servers"
			switch {
			case name == "mcp_servers."+ServerName:
				if header >= 0 {
					return nil, ErrEditByHand
				}
				header = i
			case strings.Contains(name, `"`+ServerName+`"`) || strings.Contains(name, "'"+ServerName+"'"):
				return nil, ErrEditByHand
			}
			continue
		}
		key := strings.ReplaceAll(strings.TrimSpace(strings.SplitN(t, "=", 2)[0]), " ", "")
		if strings.HasPrefix(key, "mcp_servers") || (inServers && strings.Trim(key, `"'`) == ServerName) {
			return nil, ErrEditByHand
		}
	}
	body := tomlBody(cmd)
	if header < 0 {
		var b bytes.Buffer
		for _, l := range lines {
			b.WriteString(l + "\n")
		}
		if len(lines) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(tomlHeader + "\n" + strings.Join(body, "\n") + "\n")
		return b.Bytes(), nil
	}
	end := header + 1
	for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "[") {
		end++
	}
	last := end
	for last > header+1 {
		t := strings.TrimSpace(lines[last-1])
		if t != "" && !strings.HasPrefix(t, "#") {
			break
		}
		last--
	}
	if strings.Join(lines[header+1:last], "\n") == strings.Join(body, "\n") {
		return nil, nil
	}
	out := append(append(append([]string{}, lines[:header+1]...), body...), lines[last:]...)
	return []byte(strings.Join(out, "\n") + "\n"), nil
}

func stripComment(t string) string {
	if i := strings.Index(t, "#"); i >= 0 {
		return t[:i]
	}
	return t
}
