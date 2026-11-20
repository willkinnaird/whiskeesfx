package main

// Aircraft profiles: chosen automatically from the loaded aircraft's title (read from the module's status.txt).
//
//   [profile]
//   name = Fenix A321
//   match = FenixA321          (any number of lines; case-insensitive "title contains")
//   [globals]                  global settings for this aircraft (positions, var1..var8, ...)
//   engine_x = -5.75
//   [replace]                  text swaps over the whole settings file, e.g. which N1 variable effects read
//   (L:A320_Sound:ENG_{e}_N1, Number) => (A:TURB ENG N1:{e}, Percent)
//   [effect.NAME]              per-effect changes for this aircraft (on=0, param=..., offset=...)

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Profile struct {
	ID      string   `json:"id"` // file name without .ini
	Name    string   `json:"name"`
	Match   []string `json:"match"`
	User    bool     `json:"user"` // lives in myprofiles (yours, never replaced by updates)
	Generic bool     `json:"generic"`
	File    string   `json:"file"`
	ini     *Ini
	replace [][2]string
}

func loadProfile(path string, user bool) (*Profile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ini := ParseIni(string(b))
	p := &Profile{ID: strings.TrimSuffix(filepath.Base(path), ".ini"), User: user, File: path, ini: ini}
	for _, kv := range ini.Keys("profile") {
		switch kv.Key {
		case "name":
			p.Name = kv.Value
		case "match":
			if kv.Value != "" {
				p.Match = append(p.Match, kv.Value)
			}
		case "generic":
			p.Generic = kv.Value == "1"
		}
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	if a, bEnd, ok := ini.sectionRange("replace"); ok {
		for i := a; i < bEnd; i++ {
			s := strings.TrimSpace(ini.Lines[i])
			if s == "" || s[0] == '#' {
				continue
			}
			if k := strings.Index(s, "=>"); k > 0 {
				p.replace = append(p.replace, [2]string{strings.TrimSpace(s[:k]), strings.TrimSpace(s[k+2:])})
			}
		}
	}
	return p, nil
}

func loadProfiles(dirs ...string) []*Profile {
	byID := map[string]*Profile{}
	for i, d := range dirs {
		files, _ := filepath.Glob(filepath.Join(d, "*.ini"))
		for _, f := range files {
			if p, err := loadProfile(f, i > 0); err == nil {
				byID[p.ID] = p // user folder (later) wins over shipped
			}
		}
	}
	var out []*Profile
	for _, p := range byID {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Generic != out[j].Generic {
			return !out[i].Generic
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// pickProfile: the profile whose longest match text the title contains; else the generic one.
func pickProfile(profiles []*Profile, title string) *Profile {
	t := strings.ToLower(title)
	var best *Profile
	bestLen := 0
	var generic *Profile
	for _, p := range profiles {
		if p.Generic {
			generic = p
		}
		for _, m := range p.Match {
			if n := matchScore(t, m); n > bestLen {
				best, bestLen = p, n
			}
		}
	}
	if best != nil {
		return best
	}
	return generic
}

// matchScore: a match line is words joined by "&"; every word must be in the title (a plain word as a whole
// word, anything with spaces or dashes as written), and a word starting
// with "!" must not be ("FenixA320 & IAE & !SL"). The score is how many letters matched, so the most
// specific profile wins.
func matchScore(titleLower, match string) int {
	score := 0
	for _, w := range strings.Split(match, "&") {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" {
			continue
		}
		if strings.HasPrefix(w, "!") {
			if neg := strings.TrimSpace(w[1:]); neg != "" && containsWord(titleLower, neg) {
				return 0
			}
			continue
		}
		found := false
		if !isAlnum(w) { // "737-800", "fenix a320": as written
			found = strings.Contains(titleLower, w)
		} else {
			found = containsWord(titleLower, w)
		}
		if !found {
			return 0
		}
		score += len(w)
	}
	return score
}

// containsWord: neg appears as a whole word (so "!SL" doesn't trip on "SLOW").
func containsWord(s, w string) bool {
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '_' || r == '-' || r == '/' }) {
		if f == w {
			return true
		}
	}
	return false
}

// apply puts the profile on top of the settings file.
func (p *Profile) apply(f *Ini) {
	if p == nil {
		return
	}
	for _, r := range p.replace {
		f.ReplaceAll(r[0], r[1])
	}
	for _, kv := range p.ini.Keys("globals") {
		f.Set("", kv.Key, kv.Value)
	}
	for _, sec := range p.ini.Sections() {
		if !strings.HasPrefix(sec, "effect.") && !strings.HasPrefix(sec, "preset.") {
			continue
		}
		if _, _, ok := f.sectionRange(sec); !ok {
			continue
		}
		for _, kv := range p.ini.Keys(sec) {
			f.Set(sec, kv.Key, kv.Value)
		}
	}
}

func (p *Profile) save() error {
	return writeFileAtomic(p.File, []byte(p.ini.String()))
}

// newUserProfile makes myprofiles/<id>.ini for the given title, starting from a copy of base (positions etc.).
func newUserProfile(dir, title string, base *Profile, geometry []KV) (*Profile, error) {
	os.MkdirAll(dir, 0755)
	id := slug(title)
	if id == "" {
		id = "aircraft"
	}
	path := filepath.Join(dir, id+".ini")
	var sb strings.Builder
	sb.WriteString("# Made by the WhiskeesFX app for: " + title + "\n")
	sb.WriteString("[profile]\nname=" + title + "\nmatch=" + title + "\n\n[globals]\n")
	for _, kv := range geometry {
		sb.WriteString(kv.Key + "=" + kv.Value + "\n")
	}
	sb.WriteString("\n[replace]\n")
	if base != nil {
		for _, r := range base.replace {
			sb.WriteString(r[0] + " => " + r[1] + "\n")
		}
	}
	sb.WriteString("\n")
	if err := writeFileAtomic(path, []byte(sb.String())); err != nil {
		return nil, err
	}
	return loadProfile(path, true)
}

func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
		if b.Len() > 48 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func isAlnum(w string) bool {
	for _, r := range w {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// duplicateProfile copies a profile into your own folder under a new name (matching nothing until you assign it).
func duplicateProfile(src *Profile, name, dir string) (*Profile, error) {
	os.MkdirAll(dir, 0755)
	id := "my-" + slug(name)
	path := filepath.Join(dir, id+".ini")
	for i := 2; fileExists(path); i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.ini", id, i))
	}
	f := src.ini.Clone()
	f.Set("profile", "name", name)
	for {
		if !f.Delete("profile", "match") {
			break
		}
	}
	f.Delete("profile", "generic")
	if err := writeFileAtomic(path, []byte("# Copy of "+src.Name+", made in the WhiskeesFX app\n"+f.String())); err != nil {
		return nil, err
	}
	return loadProfile(path, true)
}
