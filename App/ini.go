package main

// A line-preserving model of visualfx.ini: edits change single lines and keep every comment.

import (
	"strings"
)

type Ini struct {
	Lines []string
}

func ParseIni(text string) *Ini {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return &Ini{Lines: strings.Split(text, "\n")}
}

func (f *Ini) String() string { return strings.Join(f.Lines, "\n") }

func (f *Ini) Clone() *Ini {
	c := make([]string, len(f.Lines))
	copy(c, f.Lines)
	return &Ini{Lines: c}
}

// splitComment splits "value   # note" into ("value", "   # note").
func splitComment(v string) (string, string) {
	if i := strings.Index(v, "#"); i >= 0 {
		j := i
		for j > 0 && (v[j-1] == ' ' || v[j-1] == '\t') {
			j--
		}
		return v[:j], v[j:]
	}
	return strings.TrimRight(v, " \t"), ""
}

func sectionName(line string) (string, bool) {
	s := strings.TrimSpace(line)
	if strings.HasPrefix(s, "[") {
		if i := strings.Index(s, "]"); i > 0 {
			return s[1:i], true
		}
	}
	return "", false
}

// keyOf returns key, value for "key=value" (value without comment); key "param" becomes "param:NAME".
func keyOf(line string) (string, string, bool) {
	s := strings.TrimSpace(line)
	if s == "" || s[0] == '#' || s[0] == ';' || s[0] == '[' {
		return "", "", false
	}
	eq := strings.Index(s, "=")
	if eq < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(s[:eq])
	v, _ := splitComment(strings.TrimSpace(s[eq+1:]))
	if k == "param" {
		v = strings.TrimSpace(v)
		sp := strings.IndexAny(v, " \t")
		if sp < 0 {
			return "param:" + v, "", true
		}
		return "param:" + v[:sp], strings.TrimSpace(v[sp:]), true
	}
	return k, strings.TrimSpace(v), true
}

// sectionRange gives [start,end) line indexes of a section's body ("" = globals before the first section).
func (f *Ini) sectionRange(sec string) (int, int, bool) {
	start := -1
	if sec == "" {
		start = 0
	}
	for i, l := range f.Lines {
		if n, ok := sectionName(l); ok {
			if start >= 0 {
				return start, i, true
			}
			if n == sec {
				start = i + 1
			}
		}
	}
	if start >= 0 {
		return start, len(f.Lines), true
	}
	return 0, 0, false
}

// globalRange: lines that the module reads as globals = before the first [effect./preset.] section, plus
// [aircraft.*] style sections are ignored here (the app does profiles itself).
func (f *Ini) Get(sec, key string) (string, bool) {
	a, b, ok := f.sectionRange(sec)
	if !ok {
		return "", false
	}
	for i := a; i < b; i++ {
		if k, v, ok := keyOf(f.Lines[i]); ok && k == key {
			return v, true
		}
	}
	return "", false
}

// Set replaces the value of key in section (keeping any trailing comment) or adds the line at the section's end.
// value "" with del=true removes the line.
func (f *Ini) Set(sec, key, value string) bool {
	a, b, ok := f.sectionRange(sec)
	if !ok {
		return false
	}
	newLine := func(old string) string {
		comment := ""
		if old != "" {
			s := strings.TrimSpace(old)
			eq := strings.Index(s, "=")
			_, comment = splitComment(s[eq+1:])
		}
		if strings.HasPrefix(key, "param:") {
			return "param=" + key[6:] + " " + value + comment
		}
		return key + "=" + value + comment
	}
	for i := a; i < b; i++ {
		if k, _, ok := keyOf(f.Lines[i]); ok && k == key {
			f.Lines[i] = newLine(f.Lines[i])
			return true
		}
	}
	// add: after the last key line of the section (globals: after the last key line before the first section)
	ins := a
	for i := a; i < b; i++ {
		if _, _, ok := keyOf(f.Lines[i]); ok {
			ins = i + 1
		}
	}
	if key == "on" || key == "preset" {
		// put "on" near the top so it's easy to find by hand
		for i := a; i < b; i++ {
			if k, _, ok := keyOf(f.Lines[i]); ok && (k == "when" || k == "guid") {
				ins = i + 1
				break
			}
		}
	}
	f.Lines = append(f.Lines[:ins], append([]string{newLine("")}, f.Lines[ins:]...)...)
	return true
}

func (f *Ini) Delete(sec, key string) bool {
	a, b, ok := f.sectionRange(sec)
	if !ok {
		return false
	}
	for i := a; i < b; i++ {
		if k, _, ok := keyOf(f.Lines[i]); ok && k == key {
			f.Lines = append(f.Lines[:i], f.Lines[i+1:]...)
			return true
		}
	}
	return false
}

func (f *Ini) Sections() []string {
	var out []string
	for _, l := range f.Lines {
		if n, ok := sectionName(l); ok {
			out = append(out, n)
		}
	}
	return out
}

type KV struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
}

func (f *Ini) Keys(sec string) []KV {
	a, b, ok := f.sectionRange(sec)
	if !ok {
		return nil
	}
	var out []KV
	for i := a; i < b; i++ {
		if k, v, ok := keyOf(f.Lines[i]); ok {
			s := strings.TrimSpace(f.Lines[i])
			_, c := splitComment(s[strings.Index(s, "=")+1:])
			out = append(out, KV{k, v, strings.TrimLeft(strings.TrimSpace(c), "# ")})
		}
	}
	return out
}

// Notes: the comment block right above a section header, and the group banner it falls under.
func (f *Ini) Notes(sec string) (note, group string) {
	idx := -1
	for i, l := range f.Lines {
		if n, ok := sectionName(l); ok && n == sec {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	var nl []string
	for i := idx - 1; i >= 0; i-- {
		s := strings.TrimSpace(f.Lines[i])
		if !strings.HasPrefix(s, "#") || strings.HasPrefix(s, "# ====") {
			break
		}
		nl = append([]string{strings.TrimSpace(strings.TrimLeft(s, "#"))}, nl...)
	}
	note = strings.Join(nl, " ")
	// banner: "# ====" / "# TITLE" / "# ====" somewhere above
	for i := idx - 1; i >= 2; i-- {
		if isBanner(f.Lines[i]) && isBanner(f.Lines[i-2]) {
			group = strings.TrimSpace(strings.TrimLeft(f.Lines[i-1], "#"))
			break
		}
	}
	return
}

func isBanner(l string) bool { return strings.HasPrefix(strings.TrimSpace(l), "# ====") }

// ReplaceAll does a plain text replacement over every non-comment line.
func (f *Ini) ReplaceAll(from, to string) {
	if from == "" {
		return
	}
	for i, l := range f.Lines {
		s := strings.TrimSpace(l)
		if s == "" || s[0] == '#' {
			continue
		}
		f.Lines[i] = strings.ReplaceAll(l, from, to)
	}
}
