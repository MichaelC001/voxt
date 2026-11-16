// Package logscrub takes what a log line may hold of the user's own out
// of it before the line leaves the Mac (the feedback's diagnostics,
// actnowkit's feedback.CollectConfig.Scrub): what was said and written
// (Privacy → Log what was said quotes it with %q), the path and query of
// links (only the scheme and the site stay; actnow.dev keeps its path
// without the query), the names of the user's files and folders (an
// extension stays), quoted text, the names of App enhancement's groups
// ("app enhancement: <group> by url", "auto key … of <group>: ") and of
// microphones ("recording from <device>: "). It runs after actnowkit's
// redact.Text (keys, passwords) and redact.Personal (the home folder is
// "~" by then, other users' folders /Users/USER, emails and IP
// addresses masked). Auralink's internal/logscrub, with the model
// library's names kept.
//
// Imports: nothing internal.
package logscrub

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

// Mask stands for what was taken out.
const Mask = "…"

// OwnHosts are the sites whose paths are VoxtMini's own requests (the
// updates, the changelog, the feedback), kept without their query.
var OwnHosts = []string{"actnow.dev"}

var (
	// urlRE finds links with a scheme (https://…, wss://…, file:///…).
	urlRE = regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.-]{1,15}://[^\s"'<>\x60]+`)
	// quoteRE finds quoted text, with the word before it.
	quoteRE = regexp.MustCompile(`(\S*\s?)"((?:[^"\\]|\\.)*)"`)
	// groupRE: App enhancement's group in "app enhancement: <group> by
	// <source>", and in "auto key "…" of <group>: <error>".
	groupRE = regexp.MustCompile(`\b(app enhancement: )(.+?)( by \S+)|\b(auto key .*? of )(.+?)(: )`)
	// deviceRE: a microphone's name in "recording from <device>: ".
	deviceRE = regexp.MustCompile(`\b(recording from )(.+?)(: )`)
	// pathRE finds paths in the home folder, other users' folders, other
	// volumes and the temporary folders; a folder's name may hold
	// spaces, and so may the last name when it ends in an extension
	// ("Interview with Anna.m4a"), else it ends at one.
	pathRE = regexp.MustCompile(`(?:~|/Users/[^/\s"':]+|/Volumes|/private/var/folders|/var/folders|/tmp)(?:/[^/\n"':,;()<>\[\]{}]+)*/(?:[^/\n"':,;()<>\[\]{}]+\.[A-Za-z][A-Za-z0-9]{0,4}\b|[^/\s"':,;()<>\[\]{}]+)/?`)
	// idRE: ids (ULIDs, hashes, numbers, dates) say nothing of the user.
	idRE = regexp.MustCompile(`^(?:[0-9A-Za-z]{20,40}|[0-9a-fA-F-]{8,}|\d+|\d{8}-\d{6}(?:-\d+)?)$`)
	// extRE: an extension that is one (".m4a", ".wav").
	extRE = regexp.MustCompile(`^\.[A-Za-z0-9]{1,10}$`)
)

// keepNames are folder and file names of macOS and of VoxtMini's data,
// not the user's.
var keepNames = map[string]bool{
	"~": true, "/Users/USER": true, "/Users/Shared": true, "/Volumes": true, "/tmp": true,
	"/private/var/folders": true, "/var/folders": true,
	"Library": true, "Application Support": true, "Caches": true, "Logs": true, "Containers": true, "Group Containers": true,
	"Preferences": true, "Data": true, "tmp": true, "T": true, "C": true,
	"Desktop": true, "Documents": true, "Downloads": true, "Pictures": true, "Movies": true, "Music": true, "Public": true,
	"Applications": true, "VoxtMini": true, "VoxtMini.app": true, "dev.hehehai.voxtmini": true, "Contents": true, "MacOS": true, "Resources": true,
	"helper": true, "VoxtMiniHelper": true, "VoxtMiniHelper.app": true, "audio": true, "recordings": true, "feedback": true, "outbox": true,
	"logs": true, "cache": true, "data": true, "ActNow": true, "actnow": true, "sounds": true,
	"voxtmini.db": true, "voxtmini.log": true, "helper.log": true, "llm.log": true, "crash.log": true, "stderr.log": true, "settings.json": true,
	"VoxtMiniLLM": true, "voxtmini-llm": true, "omni": true, "llm": true, "voxtmini-runtime": true, "models": true,
	".cache": true, "huggingface": true, "hub": true, "snapshots": true, "blobs": true, "refs": true,
}

// modelDir is a model's folder in the Hugging Face cache
// ("models--mlx-community--Qwen3-1.7B-4bit"): the catalogue's, not the
// user's.
func modelDir(name string) bool { return strings.HasPrefix(name, "models--") }

// Line takes the user's own out of one log line.
func Line(s string) string {
	s = urlRE.ReplaceAllStringFunc(s, link)
	s = groupRE.ReplaceAllStringFunc(s, func(m string) string {
		sub := groupRE.FindStringSubmatch(m)
		if sub[1] != "" {
			return sub[1] + Mask + sub[3]
		}
		return sub[4] + Mask + sub[6]
	})
	s = deviceRE.ReplaceAllString(s, "${1}"+Mask+"${3}")
	s = quoteRE.ReplaceAllStringFunc(s, quoted)
	s = pathRE.ReplaceAllStringFunc(s, filePath)
	return s
}

// quoted masks quoted text, but a link already scrubbed
// ("Get \"https://actnow.dev/…\"") and what is masked already.
func quoted(m string) string {
	sub := quoteRE.FindStringSubmatch(m)
	before, text := sub[1], sub[2]
	if text == "" || text == Mask || strings.Contains(text, "://") {
		return m
	}
	return before + `"` + Mask + `"`
}

// link keeps a link's scheme and site; actnow.dev keeps its path too,
// never the query.
func link(raw string) string {
	trail := ""
	for raw != "" && strings.ContainsRune(".,;:!?)]}", rune(raw[len(raw)-1])) {
		trail = raw[len(raw)-1:] + trail
		raw = raw[:len(raw)-1]
	}
	u, err := url.Parse(raw)
	if err != nil {
		scheme, _, _ := strings.Cut(raw, "://")
		return scheme + "://" + Mask + trail
	}
	out := u.Scheme + "://" + u.Host
	rest := u.EscapedPath() != "" && u.EscapedPath() != "/" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != ""
	if own(u.Hostname()) {
		out += u.EscapedPath()
		if u.RawQuery != "" || u.Fragment != "" {
			out += "?" + Mask
		}
	} else if rest {
		out += "/" + Mask
	} else if u.EscapedPath() == "/" {
		out += "/"
	}
	return out + trail
}

func own(host string) bool {
	host = strings.ToLower(host)
	for _, h := range OwnHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// filePath masks the names in a path that are not macOS's or VoxtMini's:
// "~/Desktop/Interview.m4a" → "~/Desktop/….m4a".
func filePath(p string) string {
	slash := strings.HasSuffix(p, "/")
	p = strings.TrimSuffix(p, "/")
	var head string
	switch {
	case strings.HasPrefix(p, "~"):
		head, p = "~", strings.TrimPrefix(p, "~")
	case strings.HasPrefix(p, "/Users/"):
		rest := strings.TrimPrefix(p, "/Users/")
		name, after, _ := strings.Cut(rest, "/")
		head, p = "/Users/"+name, "/"+after
		if !keepNames[head] {
			head = "/Users/USER"
		}
	default:
		for _, h := range []string{"/Volumes", "/private/var/folders", "/var/folders", "/tmp"} {
			if strings.HasPrefix(p, h) {
				head, p = h, strings.TrimPrefix(p, h)
				break
			}
		}
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	inModel := false // a model's files are the catalogue's too
	for i, name := range parts {
		inModel = inModel || modelDir(name)
		if keepNames[name] || inModel || idRE.MatchString(name) || len(name) <= 2 && head != "/Volumes" {
			continue
		}
		ext := path.Ext(name)
		if !extRE.MatchString(ext) || ext == name {
			ext = ""
		}
		parts[i] = Mask + ext
	}
	out := head + "/" + strings.Join(parts, "/")
	if slash {
		out += "/"
	}
	return out
}
