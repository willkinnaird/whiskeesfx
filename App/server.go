package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed ui/index.html
var uiHTML []byte

//go:embed defaults/visualfx.ini
var embeddedIni []byte

//go:embed profiles/*.ini
var embeddedProfiles embed.FS

func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func readBody(r *http.Request, v any) bool {
	defer r.Body.Close()
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v) == nil
}

type GroupView struct {
	Name     string  `json:"name"`
	Count    int     `json:"count"`
	Playing  int     `json:"playing"`
	Off      bool    `json:"off"`
	Strength float64 `json:"strength"`
}

func (a *App) groupViews(evs []EffectView) []GroupView {
	var out []GroupView
	idx := map[string]int{}
	for _, e := range evs {
		if e.Marker {
			continue
		}
		i, ok := idx[e.Group]
		if !ok {
			g := GroupView{Name: e.Group, Strength: 1}
			if s := a.cfg.Groups[e.Group]; s != nil {
				g.Off = s.Off
				if s.Strength > 0 {
					g.Strength = s.Strength
				}
			}
			out = append(out, g)
			i = len(out) - 1
			idx[e.Group] = i
		}
		out[i].Count++
		if e.State == "playing" {
			out[i].Playing++
		}
	}
	return out
}

func (a *App) state(full bool) map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastSeen = time.Now()
	evs := a.effectViews()
	states := map[string]string{}
	for _, e := range evs {
		states[e.Section] = e.State
	}
	var profs []map[string]any
	for _, p := range a.profiles {
		profs = append(profs, map[string]any{"id": p.ID, "name": p.Name, "user": p.User, "generic": p.Generic, "match": p.Match, "official": p.Official})
	}
	active := map[string]any{}
	if a.active != nil {
		active = map[string]any{"id": a.active.ID, "name": a.active.Name, "generic": a.active.Generic, "user": a.active.User, "official": a.active.Official, "version": a.active.Version, "type": a.active.Type}
	}
	all := a.cfg.Groups["*"]
	master := map[string]any{"off": all != nil && all.Off, "strength": 1.0}
	if all != nil && all.Strength > 0 {
		master["strength"] = all.Strength
	}
	sim := a.status
	sim.Effects = nil
	out := map[string]any{
		"version": version, "view": viewOr(a.cfg.View), "rev": len(a.cfg.Tweaks), "sim": sim, "states": states,
		"title": a.title, "profile": active, "profiles": profs, "manualProfile": a.cfg.ManualProfile,
		"groups": a.groupViews(evs), "master": master, "tests": a.tests, "paused": a.paused, "message": a.message,
		"progress": getProgress(), "dismissed": a.cfg.Dismissed, "update": getUpdate(), "devMode": a.cfg.DevMode, "hasToken": a.cfg.GitHubToken != "", "supported": a.title == "" || a.supported(), "assign": a.cfg.Assign, "tweaks": len(a.cfg.Tweaks),
		"settings": map[string]any{
			"theme": themeOr(a.cfg.Theme), "onTop": a.cfg.OnTop, "keepInTray": a.cfg.KeepInTray, "autoUpdate": a.cfg.AutoUpdate, "startWithWindows": a.startsWin,
			"sourceIni": a.sourcePath(), "workDir": a.cfg.WorkDir, "community": a.cfg.Community, "repo": a.cfg.Repo,
			"pkgModule": a.pkgMod, "pkgEffects": a.pkgFx, "welcomed": a.cfg.Welcomed,
			"workDirOK": a.workOK, "communityOK": a.commOK,
		},
	}
	if full {
		out["effects"] = evs
	}
	return out
}

// guard: only answer requests addressed to 127.0.0.1 (stops other web pages reaching the app by DNS tricks)
func guard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if host := r.Host; host != "127.0.0.1:"+port && host != "localhost:"+port {
			http.Error(w, "forbidden", 403)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(uiHTML)
	})
	m.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, app.state(r.URL.Query().Get("full") == "1"))
	})
	post := func(path string, h func(r *http.Request) error) {
		m.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				http.Error(w, "POST only", 405)
				return
			}
			// only our own page may change things (blocks other web pages poking the local port)
			if o := r.Header.Get("Origin"); o != "" && !strings.HasPrefix(o, "http://127.0.0.1:") {
				http.Error(w, "forbidden", 403)
				return
			}
			if err := h(r); err != nil {
				jsonOut(w, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			jsonOut(w, map[string]any{"ok": true})
		})
	}
	post("/api/set", func(r *http.Request) error {
		var b struct{ Section, Key, Value, Scope string }
		if !readBody(r, &b) || b.Key == "" {
			return errf("bad request")
		}
		sec := b.Section
		app.mu.Lock()
		app.setTweak(sec, b.Key, strings.TrimSpace(b.Value), b.Scope)
		app.saveConfig()
		app.composeLocked()
		app.mu.Unlock()
		return nil
	})
	post("/api/reset", func(r *http.Request) error {
		var b struct{ Section string }
		readBody(r, &b)
		app.mu.Lock()
		app.resetSection(b.Section)
		app.saveConfig()
		app.composeLocked()
		app.mu.Unlock()
		return nil
	})
	post("/api/resetall", func(r *http.Request) error {
		app.mu.Lock()
		app.cfg.Tweaks = nil
		app.cfg.Groups = map[string]*GroupSetting{}
		app.saveConfig()
		app.composeLocked()
		app.mu.Unlock()
		return nil
	})
	post("/api/group", func(r *http.Request) error {
		var b struct {
			Name     string
			Off      *bool
			Strength *float64
		}
		if !readBody(r, &b) {
			return errf("bad request")
		}
		app.mu.Lock()
		g := app.cfg.Groups[b.Name]
		if g == nil {
			g = &GroupSetting{Strength: 1}
			app.cfg.Groups[b.Name] = g
		}
		if b.Off != nil {
			g.Off = *b.Off
		}
		if b.Strength != nil {
			s := *b.Strength
			if s < 0 {
				s = 0
			}
			if s > 3 {
				s = 3
			}
			g.Strength = s
		}
		app.saveConfig()
		app.composeLocked()
		app.mu.Unlock()
		return nil
	})
	post("/api/tests", func(r *http.Request) error {
		app.mu.Lock()
		t := app.tests
		app.mu.Unlock()
		if !readBody(r, &t) {
			return errf("bad request")
		}
		app.mu.Lock()
		app.tests = t
		app.composeLocked()
		app.mu.Unlock()
		return nil
	})
	post("/api/profile", func(r *http.Request) error {
		var b struct{ Manual string }
		readBody(r, &b)
		app.mu.Lock()
		app.cfg.ManualProfile = b.Manual
		app.saveConfig()
		app.active = app.pickActive()
		app.composeLocked()
		app.mu.Unlock()
		return nil
	})
	post("/api/profile/new", func(r *http.Request) error {
		app.mu.Lock()
		defer app.mu.Unlock()
		if app.title == "" {
			return errf("load an aircraft in the sim first")
		}
		var geo []KV
		if f := app.build(); f != nil {
			for _, kv := range f.Keys("") {
				if isGeometry(kv.Key) {
					geo = append(geo, KV{Key: kv.Key, Value: kv.Value})
				}
			}
		}
		var base *Profile
		for _, p := range app.profiles {
			if p.Generic {
				base = p
			}
		}
		if _, err := newUserProfile(myProfilesDir(), app.title, base, geo); err != nil {
			return err
		}
		app.profMod = ""
		return nil
	})
	post("/api/resume", func(r *http.Request) error {
		app.mu.Lock()
		app.paused = false
		app.message = ""
		app.composeLocked()
		app.mu.Unlock()
		return nil
	})
	post("/api/settings", func(r *http.Request) error {
		var b map[string]any
		if !readBody(r, &b) {
			return errf("bad request")
		}
		app.mu.Lock()
		defer app.mu.Unlock()
		if v, ok := b["view"].(string); ok && (v == "home" || v == "basic" || v == "advanced" || v == "profiles" || v == "settings") {
			app.cfg.View = v
		}
		if v, ok := b["githubToken"].(string); ok && app.cfg.DevMode {
			app.cfg.GitHubToken = strings.TrimSpace(v)
		}
		if v, ok := b["theme"].(string); ok && (v == "auto" || v == "light" || v == "dark") {
			app.cfg.Theme = v
		}
		if v, ok := b["onTop"].(bool); ok {
			app.cfg.OnTop = v
			go setOnTop(v)
		}
		if v, ok := b["keepInTray"].(bool); ok {
			app.cfg.KeepInTray = v
		}
		if v, ok := b["autoUpdate"].(bool); ok {
			app.cfg.AutoUpdate = v
		}
		if v, ok := b["startWithWindows"].(bool); ok {
			exe, _ := os.Executable()
			setStartWithWindows(v, exe)
			app.startsWin = queryStartsWithWindows()
		}
		if v, ok := b["repo"].(string); ok && strings.Count(v, "/") == 1 {
			app.cfg.Repo = strings.TrimSpace(v)
		}
		if v, ok := b["sourceIni"].(string); ok {
			v = strings.TrimSpace(v)
			if v == "" || v == filepath.Join(defaultsDir(), "visualfx.ini") {
				app.cfg.SourceIni, app.cfg.DevMode = "", false
			} else if fileExists(v) {
				app.cfg.SourceIni, app.cfg.DevMode = v, true
			} else {
				return errf("that file doesn't exist")
			}
			app.sourceMod = time.Time{}
		}
		app.saveConfig()
		return nil
	})
	post("/api/update/check", func(r *http.Request) error {
		u := app.checkUpdate()
		if u.Error != "" {
			return errf(u.Error)
		}
		return nil
	})
	post("/api/update/apply", func(r *http.Request) error {
		go func() {
			setBusy("Updating…")
			err := app.applyUpdate(setProgress)
			if err != nil {
				setProgress("Update failed: " + err.Error())
			}
			setBusy("")
			app.checkUpdate()
		}()
		return nil
	})
	post("/api/welcome", func(r *http.Request) error {
		app.mu.Lock()
		app.cfg.Welcomed = true
		app.saveConfig()
		app.mu.Unlock()
		return nil
	})
	post("/api/install-packages", func(r *http.Request) error {
		go func() {
			setBusy("Installing…")
			if err := app.installPackagesNow(setProgress); err != nil {
				setProgress("Couldn't install the effects: " + err.Error())
			}
			setBusy("")
			app.mu.Lock()
			app.slowAt = time.Time{} // re-read the installed versions on the next check
			app.mu.Unlock()
		}()
		return nil
	})
	post("/api/preset", func(r *http.Request) error {
		var b struct{ Name string }
		readBody(r, &b)
		k := map[string]float64{"subtle": 0.6, "realistic": 1, "cinematic": 1.5}[b.Name]
		if k == 0 {
			return errf("unknown preset")
		}
		app.mu.Lock()
		defer app.mu.Unlock()
		g := app.cfg.Groups["*"]
		if g == nil {
			g = &GroupSetting{}
			app.cfg.Groups["*"] = g
		}
		g.Strength, g.Off = k, false
		for name, gs := range app.cfg.Groups { // a preset starts every group from its shipped strength
			if name != "*" {
				gs.Strength = 1
			}
		}
		app.saveConfig()
		app.composeLocked()
		return nil
	})
	m.HandleFunc("/api/profiles", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, app.profilesView(r.URL.Query().Get("id")))
	})
	post("/api/airframes/scan", func(r *http.Request) error {
		app.mu.Lock()
		comm := app.cfg.Community
		app.mu.Unlock()
		go scanAirframes(comm)
		return nil
	})
	post("/api/assign", func(r *http.Request) error {
		var b struct{ Title, Profile string }
		if !readBody(r, &b) || b.Title == "" {
			return errf("bad request")
		}
		app.mu.Lock()
		defer app.mu.Unlock()
		if b.Profile == "" {
			delete(app.cfg.Assign, b.Title)
		} else {
			app.cfg.Assign[b.Title] = b.Profile
		}
		app.saveConfig()
		app.active = app.pickActive()
		app.composeLocked()
		return nil
	})
	post("/api/point", func(r *http.Request) error {
		var b struct {
			Profile, ID string
			Axis        int
			Delta       float64
			Set         *float64
		}
		if !readBody(r, &b) {
			return errf("bad request")
		}
		app.mu.Lock()
		defer app.mu.Unlock()
		p := app.profileByID(b.Profile)
		if p == nil {
			return errf("unknown profile")
		}
		if p.Official && !app.cfg.DevMode {
			return errf("Whiskee's official profiles are read-only. Make your own copy to move the dots.")
		}
		f := app.settingsFor(p)
		if f == nil {
			return errf("no settings loaded")
		}
		if err := app.movePoint(p, f, b.ID, b.Axis, b.Delta, b.Set); err != nil {
			return err
		}
		app.saveConfig()
		app.composeLocked()
		return nil
	})
	post("/api/profile/duplicate", func(r *http.Request) error {
		var b struct {
			ID, Name string
			Take     bool // the copy takes over every airframe using the original
		}
		readBody(r, &b)
		var afs []Airframe
		if b.Take {
			afs, _, _ = airframes()
		}
		app.mu.Lock()
		defer app.mu.Unlock()
		src := app.profileByID(b.ID)
		if src == nil {
			return errf("unknown profile")
		}
		name := strings.TrimSpace(b.Name)
		if name == "" {
			name = src.Name + " (copy)"
		}
		np, err := duplicateProfile(src, name, myProfilesDir())
		if err != nil {
			return err
		}
		// carry this profile's tweaks over to the copy
		for _, t := range append([]Tweak(nil), app.cfg.Tweaks...) {
			if t.Scope == src.ID {
				app.cfg.Tweaks = append(app.cfg.Tweaks, Tweak{np.ID, t.Section, t.Key, t.Value})
			}
		}
		if b.Take {
			if app.cfg.Assign == nil {
				app.cfg.Assign = map[string]string{}
			}
			titles := []string{app.title}
			for _, af := range afs {
				titles = append(titles, af.Title)
			}
			for _, t := range titles {
				if t == "" {
					continue
				}
				if id := app.cfg.Assign[t]; id == src.ID || (id == "" && pickProfile(app.profiles, t) == src) {
					app.cfg.Assign[t] = np.ID
				}
			}
		}
		app.saveConfig()
		app.profMod = ""
		return nil
	})
	post("/api/profile/reset", func(r *http.Request) error {
		var b struct{ ID string }
		readBody(r, &b)
		app.mu.Lock()
		defer app.mu.Unlock()
		var keep []Tweak
		for _, t := range app.cfg.Tweaks {
			if t.Scope != b.ID {
				keep = append(keep, t)
			}
		}
		app.cfg.Tweaks = keep
		app.saveConfig()
		app.composeLocked()
		return nil
	})
	post("/api/dismiss", func(r *http.Request) error {
		var b struct{ Key string }
		if !readBody(r, &b) || b.Key == "" {
			return errf("bad request")
		}
		app.mu.Lock()
		app.cfg.Dismissed[b.Key] = true
		app.saveConfig()
		app.mu.Unlock()
		return nil
	})
	post("/api/ignore", func(r *http.Request) error {
		var b struct {
			Text   string
			On     bool
			Remove bool
		}
		if !readBody(r, &b) || strings.TrimSpace(b.Text) == "" {
			return errf("type a word to hide, for example FSLTL")
		}
		app.mu.Lock()
		defer app.mu.Unlock()
		txt := strings.TrimSpace(b.Text)
		var out []IgnoreRule
		found := false
		for _, r := range app.cfg.Ignore {
			if strings.EqualFold(r.Text, txt) {
				found = true
				if b.Remove {
					continue
				}
				r.On = b.On
			}
			out = append(out, r)
		}
		if !found && !b.Remove {
			out = append(out, IgnoreRule{txt, true})
		}
		app.cfg.Ignore = out
		app.saveConfig()
		return nil
	})
	post("/api/bake", func(r *http.Request) error {
		app.mu.Lock()
		defer app.mu.Unlock()
		if !app.cfg.DevMode {
			return errf("only in developer mode")
		}
		err := app.bakeTweaks()
		app.saveConfig()
		return err
	})
	post("/api/typedefault", func(r *http.Request) error {
		var b struct{ Type, Profile string }
		readBody(r, &b)
		app.mu.Lock()
		defer app.mu.Unlock()
		if b.Type == "" {
			return errf("bad request")
		}
		if app.cfg.TypeDefault == nil {
			app.cfg.TypeDefault = map[string]string{}
		}
		if b.Profile == "" {
			delete(app.cfg.TypeDefault, b.Type)
		} else {
			app.cfg.TypeDefault[b.Type] = b.Profile
		}
		app.saveConfig()
		app.active = app.pickActive()
		app.composeLocked()
		return nil
	})
	post("/api/feature", func(r *http.Request) error {
		var b struct {
			Profile, ID string
			On          bool
		}
		readBody(r, &b)
		app.mu.Lock()
		defer app.mu.Unlock()
		p := app.profileByID(b.Profile)
		if p == nil {
			return errf("no such profile")
		}
		if p.Official && !app.cfg.DevMode {
			return errf("Whiskee's official profiles are read-only. Make your own copy to change its features.")
		}
		if !app.setFeature(p, b.ID, b.On) {
			return errf("unknown feature")
		}
		app.saveConfig()
		return nil
	})
	post("/api/publish", func(r *http.Request) error {
		var b struct{ ID, Version, Note string }
		readBody(r, &b)
		app.mu.Lock()
		if !app.cfg.DevMode {
			app.mu.Unlock()
			return errf("only Whiskee can publish official profiles (developer mode)")
		}
		p := app.profileByID(b.ID)
		token, repo := app.cfg.GitHubToken, app.cfg.Repo
		var file, id string
		var err error
		if p != nil {
			id, file, err = app.bakeOfficial(p, b.Version, b.Note)
		}
		app.mu.Unlock()
		if p == nil {
			return errf("no such profile")
		}
		if err != nil {
			return err
		}
		if token == "" {
			return errf("add your GitHub token in Settings first")
		}
		if repo == "" {
			repo = defaultRepo
		}
		if err := publishToGitHub(token, repo, "App/profiles/"+id+".ini", file,
			fmt.Sprintf("Whiskee official: %s v%s", p.Name, strings.TrimPrefix(strings.TrimSpace(b.Version), "v"))); err != nil {
			return err
		}
		app.mu.Lock()
		app.message = fmt.Sprintf("Published %s v%s. It goes out to everyone with the next app update.", p.Name, strings.TrimPrefix(strings.TrimSpace(b.Version), "v"))
		app.mu.Unlock()
		return nil
	})
	post("/api/hide", func(r *http.Request) error {
		hiding = true
		go func() { time.Sleep(100 * time.Millisecond); closeWindow() }()
		return nil
	})
	post("/api/quit", func(r *http.Request) error {
		go func() { time.Sleep(100 * time.Millisecond); shutdown() }()
		return nil
	})
	return m
}

type strErr string

func (e strErr) Error() string { return string(e) }
func errf(s string) error      { return strErr(s) }

func viewOr(v string) string {
	if v == "" {
		return "home"
	}
	return v
}

func themeOr(t string) string {
	if t == "" {
		return "auto"
	}
	return t
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

type ProfileListItem struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	User      bool     `json:"user"`
	Generic   bool     `json:"generic"`
	Active    bool     `json:"active"`
	Changed   bool     `json:"changed"`
	Official  bool     `json:"official"`
	Version   string   `json:"version"`
	Notes     []string `json:"notes"`
	BasedOn   string   `json:"basedOn"`
	Type      string   `json:"type"`
	IsDefault bool     `json:"isDefault"`
	Airframes []string `json:"airframes"`
}

type AirframeView struct {
	Title   string `json:"title"`
	Package string `json:"package"`
	Profile string `json:"profile"`
	How     string `json:"how"` // auto, assigned, none
	Loaded  bool   `json:"loaded"`
	Hidden  bool   `json:"hidden"`
}

// profilesView: everything the Profiles tab shows; id picks the profile being edited ("" = the one in use).
func (a *App) profilesView(id string) map[string]any {
	afs, scanned, busy := airframes()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastSeen = time.Now()
	if scanned.IsZero() && !busy {
		go scanAirframes(a.cfg.Community)
	}
	titles := map[string]bool{}
	var list []AirframeView
	add := func(t, pkg string) {
		if t == "" || titles[t] {
			return
		}
		titles[t] = true
		av := AirframeView{Title: t, Package: pkg, Loaded: t == a.title, How: "auto", Hidden: t != a.title && a.ignored(t, pkg)}
		if pid := a.cfg.Assign[t]; pid != "" && a.profileByID(pid) != nil {
			av.Profile, av.How = pid, "assigned"
		} else if p := pickProfile(a.profiles, t); p != nil {
			av.Profile = p.ID
			if p.Generic {
				av.How = "none"
			}
		}
		list = append(list, av)
	}
	add(a.title, "")
	for _, af := range afs {
		add(af.Title, af.Package)
	}
	changed := map[string]bool{}
	for _, t := range a.cfg.Tweaks {
		changed[t.Scope] = true
	}
	var profs []ProfileListItem
	for _, p := range a.profiles {
		it := ProfileListItem{ID: p.ID, Name: p.Name, User: p.User, Generic: p.Generic, Active: a.active == p, Changed: changed[p.ID],
			Official: p.Official, Version: p.Version, Notes: p.Notes, BasedOn: p.basedOn(), Type: p.Type,
			IsDefault: p.Type != "" && a.cfg.TypeDefault[p.Type] == p.ID}
		for _, av := range list {
			if av.Profile == p.ID && !av.Hidden {
				it.Airframes = append(it.Airframes, av.Title)
			}
		}
		profs = append(profs, it)
	}
	sel := a.profileByID(id)
	if sel == nil {
		sel = a.active
	}
	hidden := 0
	for _, av := range list {
		if av.Hidden {
			hidden++
		}
	}
	out := map[string]any{"profiles": profs, "airframes": list, "ignore": a.cfg.Ignore, "hiddenCount": hidden, "scanning": busy, "scanned": !scanned.IsZero(), "title": a.title, "tests": a.tests}
	if sel != nil {
		if f := a.settingsFor(sel); f != nil {
			out["selected"] = sel.ID
			out["points"] = a.pointViews(sel, f)
			out["features"] = a.featureViews(f)
		}
	}
	return out
}
