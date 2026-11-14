// Package dict is the user's dictionary as the rest of VoxtMini uses it
// (plan §2.3, D4; 0.1 is kept by hand): checking a term, the hints sent to
// speech recognition, and importing and exporting the terms as CSV or
// plain text. The terms live in the store (store.Dictionary); the
// language model's glossary is the composer's (compose.Glossary).
//
// Imports: store.
package dict

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hehehai/VoxtMini/internal/store"
)

// Limits of a term.
const (
	MaxTermChars  = 80
	MaxAliases    = 10
	MaxNoteChars  = 500
	MaxImportRows = 10000
	// MaxImportBytes is the largest file Import is given: 10,000 full
	// rows are well under it.
	MaxImportBytes = 5 << 20
)

// Errors of Check.
var (
	ErrEmpty   = errors.New("dict: the term is empty")
	ErrLong    = errors.New("dict: the term is too long")
	ErrNewline = errors.New("dict: a term is one line")
)

// Clean trims a term, its aliases (one line each, without blanks, the
// term itself and duplicates, ignoring case; at most MaxAliases) and its
// note.
func Clean(t store.Term) store.Term {
	t.Term = oneLine(t.Term)
	t.Note = strings.TrimSpace(t.Note)
	var aliases []string
	for _, a := range t.Aliases {
		a = oneLine(a)
		if a == "" || strings.EqualFold(a, t.Term) || slices.ContainsFunc(aliases, func(b string) bool { return strings.EqualFold(a, b) }) {
			continue
		}
		if len(aliases) == MaxAliases {
			break
		}
		aliases = append(aliases, a)
	}
	t.Aliases = aliases
	return t
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Check says whether a term can be kept (after Clean).
func Check(t store.Term) error {
	switch {
	case strings.TrimSpace(t.Term) == "":
		return ErrEmpty
	case strings.ContainsAny(strings.TrimSpace(t.Term), "\r\n"):
		return ErrNewline
	case utf8.RuneCountInString(strings.TrimSpace(t.Term)) > MaxTermChars:
		return ErrLong
	}
	return nil
}

// SplitAliases reads aliases typed in one field: separated by commas
// (also "，", "、") or semicolons.
func SplitAliases(s string) []string {
	f := func(r rune) bool { return r == ',' || r == ';' || r == '，' || r == '、' || r == '；' || r == '\n' }
	var out []string
	for _, a := range strings.FieldsFunc(s, f) {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// Scoped are the terms a session in app enhancement group `group` ("":
// none matched) uses: those of no group, of that group, and of a group
// that no longer exists (exists says; nil: every group exists).
func Scoped(terms []store.Term, group string, exists func(id string) bool) []store.Term {
	var out []store.Term
	for _, t := range terms {
		if t.Group == "" || t.Group == group || (exists != nil && !exists(t.Group)) {
			out = append(out, t)
		}
	}
	return out
}

// ByUse orders terms for the speech model: those of group first (a
// session's own group, "" for none), then the most used, the last used,
// the most recently changed.
func ByUse(terms []store.Term, group string) []store.Term {
	ts := slices.Clone(terms)
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		if ga, gb := group != "" && a.Group == group, group != "" && b.Group == group; ga != gb {
			return ga
		}
		if a.Uses != b.Uses {
			return a.Uses > b.Uses
		}
		if !a.LastUsed.Equal(b.LastUsed) {
			return a.LastUsed.After(b.LastUsed)
		}
		return a.UpdatedAt.After(b.UpdatedAt)
	})
	return ts
}

// Hints are the terms sent to speech recognition: the enabled ones, each
// once (ignoring case), in ByUse's order (a service that takes fewer
// cuts the list from the end). Not the aliases: they are what
// recognition gets wrong.
func Hints(terms []store.Term, group string) []string {
	ts := ByUse(terms, group)
	var out []string
	seen := map[string]bool{}
	for _, t := range ts {
		w := oneLine(t.Term)
		if !t.Enabled || w == "" || seen[strings.ToLower(w)] {
			continue
		}
		seen[strings.ToLower(w)] = true
		out = append(out, w)
	}
	return out
}

// ---- Import and export ----

// Formats.
const (
	CSV  = "csv"
	Text = "txt"
)

// FormatOf is the format of a file by its name (".txt" is Text, else
// CSV).
func FormatOf(name string) string {
	if strings.HasSuffix(strings.ToLower(name), ".txt") {
		return Text
	}
	return CSV
}

// csvHeader is the first row of an exported CSV.
var csvHeader = []string{"term", "aliases", "note", "enabled", "category"}

// Export writes terms in format: CSV with a header (term, aliases joined
// by "; ", note, enabled as true/false, the category's name, "" for the
// default one; a cell a spreadsheet would read as a formula starts with
// "'", which Import drops), or text, a term per line with its aliases after " = ",
// disabled terms commented out, the default category's terms first and
// each other category's after a "[Name]" line. category names a term's
// category ("" for the default one or one that no longer exists).
func Export(terms []store.Term, format string, category func(id string) string) []byte {
	name := func(t store.Term) string {
		if category == nil || t.Category == "" {
			return ""
		}
		return category(t.Category)
	}
	var b bytes.Buffer
	if format == Text {
		b.WriteString("# VoxtMini dictionary: a term per line, \"Term = alias, alias\" for what speech recognition hears instead; \"[Category]\" starts a category.\n")
		ts := slices.Clone(terms)
		sort.SliceStable(ts, func(i, j int) bool { return strings.ToLower(name(ts[i])) < strings.ToLower(name(ts[j])) })
		cur := ""
		for _, t := range ts {
			if n := name(t); n != cur {
				cur = n
				b.WriteString("\n[" + n + "]\n")
			}
			line := t.Term
			if len(t.Aliases) > 0 {
				line += " = " + strings.Join(t.Aliases, ", ")
			}
			if !t.Enabled {
				line = "# " + line
			}
			b.WriteString(line + "\n")
		}
		return b.Bytes()
	}
	b.WriteString("\ufeff") // Excel reads UTF-8 with a BOM
	w := csv.NewWriter(&b)
	w.Write(csvHeader)
	for _, t := range terms {
		w.Write([]string{cell(t.Term), cell(strings.Join(t.Aliases, "; ")), cell(t.Note), fmt.Sprint(t.Enabled), cell(name(t))})
	}
	w.Flush()
	return b.Bytes()
}

// formulaStart are the first characters that make a spreadsheet (Excel,
// Numbers) read a CSV cell as a formula, which may run commands or send
// data away (CSV injection).
const formulaStart = "=+-@\t\r"

// cell is an exported CSV cell: one a spreadsheet would read as a
// formula gets a leading "'" (OWASP's advice), which uncell drops again
// on import. So that the round trip is exact, so does one that already
// starts with "'"s before such a character.
func cell(s string) string {
	if escaped(s) {
		return "'" + s
	}
	return s
}

// uncell undoes cell.
func uncell(s string) string {
	if strings.HasPrefix(s, "'") && escaped(s) {
		return s[1:]
	}
	return s
}

// escaped reports that s, past its leading "'"s, starts with a formula's
// first character.
func escaped(s string) bool {
	s = strings.TrimLeft(s, "'")
	return s != "" && strings.ContainsRune(formulaStart, rune(s[0]))
}

// ErrFormat: the file is not a dictionary Import reads.
var ErrFormat = errors.New("dict: not a dictionary file")

// Import reads terms (without ids or times) from data in format. CSV:
// the columns term, aliases, note, enabled, category, by a header naming
// them or in that order; text: a term per line, "Term = alias, alias",
// "#" for a comment, "[Name]" for the category of the lines after it
// ("[]" the default one). An imported term's Category is its category's
// name ("" for the default one), not an id: Categorize maps them. Rows
// without a term are skipped (skipped counts them).
func Import(data []byte, format string) (terms []store.Term, skipped int, err error) {
	if len(data) > MaxImportBytes {
		return nil, 0, fmt.Errorf("%w: larger than %d bytes", ErrFormat, MaxImportBytes)
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if !utf8.Valid(data) {
		return nil, 0, fmt.Errorf("%w: not UTF-8 text", ErrFormat)
	}
	add := func(t store.Term) {
		t = Clean(t)
		if Check(t) != nil {
			skipped++
			return
		}
		terms = append(terms, t)
	}
	if format == Text {
		category := ""
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
				continue
			}
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				category = oneLine(line[1 : len(line)-1])
				continue
			}
			term, aliases, _ := strings.Cut(line, " = ")
			if t, a, ok := strings.Cut(line, "\t"); ok && !strings.Contains(line, " = ") {
				term, aliases = t, a
			}
			add(store.Term{Term: term, Aliases: SplitAliases(aliases), Enabled: true, Category: category})
			if len(terms) > MaxImportRows {
				return nil, 0, fmt.Errorf("%w: more than %d terms", ErrFormat, MaxImportRows)
			}
		}
		return terms, skipped, nil
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	col := map[string]int{"term": 0, "aliases": 1, "note": 2, "enabled": 3, "category": 4}
	first := true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("%w: %v", ErrFormat, err)
		}
		if first {
			first = false
			if h := headerColumns(rec); h != nil {
				col = h
				continue
			}
		}
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return uncell(rec[i])
			}
			return ""
		}
		add(store.Term{Term: get("term"), Aliases: SplitAliases(get("aliases")), Note: get("note"), Enabled: parseBool(get("enabled"), true),
			Category: oneLine(get("category"))})
		if len(terms) > MaxImportRows {
			return nil, 0, fmt.Errorf("%w: more than %d terms", ErrFormat, MaxImportRows)
		}
	}
	return terms, skipped, nil
}

// headerColumns maps a CSV header's names to their columns, nil when rec
// is not a header (it has no "term" column).
func headerColumns(rec []string) map[string]int {
	names := map[string]string{
		"term": "term", "word": "term", "词条": "term", "术语": "term",
		"aliases": "aliases", "alias": "aliases", "别名": "aliases",
		"note": "note", "notes": "note", "备注": "note",
		"enabled": "enabled", "enable": "enabled", "启用": "enabled",
		"category": "category", "分类": "category",
	}
	out := map[string]int{}
	for i, h := range rec {
		if n, ok := names[strings.ToLower(strings.TrimSpace(h))]; ok {
			if _, dup := out[n]; !dup {
				out[n] = i
			}
		}
	}
	if _, ok := out["term"]; !ok {
		return nil
	}
	return out
}

func parseBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "on", "是", "启用":
		return true
	case "0", "false", "no", "n", "off", "否", "停用", "禁用":
		return false
	}
	return def
}

// Merge applies imported terms to the dictionary have: a new spelling
// (ignoring case) is added as given, a known one gets the aliases it
// lacked, the note when it had none and the category when it was in the
// default one. It returns the terms to save (without ids for the new
// ones) and how many were added and updated. The imported terms'
// categories are ids here (Categorize).
func Merge(have, imported []store.Term) (put []store.Term, added, updated int) {
	byTerm := map[string]int{}
	cur := slices.Clone(have)
	for i, t := range cur {
		byTerm[strings.ToLower(strings.TrimSpace(t.Term))] = i
	}
	changed := map[int]bool{}
	for _, in := range imported {
		key := strings.ToLower(strings.TrimSpace(in.Term))
		i, ok := byTerm[key]
		if !ok {
			byTerm[key] = len(cur)
			cur = append(cur, in)
			changed[len(cur)-1] = true
			added++
			continue
		}
		t := cur[i]
		before := len(t.Aliases)
		aliases := t.Aliases
		t = Clean(t)
		t.Aliases = Clean(store.Term{Term: t.Term, Aliases: append(slices.Clone(aliases), in.Aliases...)}).Aliases
		more := false
		if t.Note == "" && in.Note != "" {
			t.Note, more = in.Note, true
		}
		if t.Category == "" && in.Category != "" {
			t.Category, more = in.Category, true
		}
		if len(t.Aliases) != before || more {
			cur[i] = t
			if !changed[i] && i < len(have) {
				updated++
			}
			changed[i] = true
		}
	}
	for i := range cur {
		if changed[i] {
			put = append(put, cur[i])
		}
	}
	return put, added, updated
}

// Categorize gives imported terms (Import: Category a name) the ids of
// their categories: those of cats by name (ignoring case), new ones
// (newID makes their ids) for the others, which it returns to be saved.
func Categorize(imported []store.Term, cats []store.Category, newID func() string) (terms []store.Term, added []store.Category) {
	byName := map[string]string{}
	for _, c := range cats {
		byName[strings.ToLower(c.Name)] = c.ID
	}
	terms = slices.Clone(imported)
	for i, t := range terms {
		if t.Category == "" {
			continue
		}
		key := strings.ToLower(t.Category)
		id, ok := byName[key]
		if !ok {
			id = newID()
			byName[key] = id
			added = append(added, store.Category{ID: id, Name: t.Category})
		}
		terms[i].Category = id
	}
	return terms, added
}
