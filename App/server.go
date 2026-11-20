package main

import (
	"embed"
	"encoding/json"
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
		profs = append(profs, map[string]any{"id": p.ID, "name": p.Name, "user": p.User, "generic": p.Generic, "match": p.Match})
	}
	active := map[string]any{}
	if a.active != nil {
		active = map[string]any{"id": a.active.ID, "name": a.active.Name, "generic": a.active.Generic, "user": a.active.User}
	}
	all := a.cfg.Groups["*"]
	master := map[string]any{"off": all != nil && all.Off, "strength": 1.0}
	if all != nil && all.Strength > 0 {
		master["strength"] = all.Strength
	}
	sim := a.status
	sim.Effects = nil
	out := map[string]any{
		"version": version, "view": a.cfg.View, "rev": len(a.cfg.Tweaks), "sim": sim, "states": states,
		"title": a.title, "profile": active, "profiles": profs, "manualProfile": a.cfg.ManualProfile,
		"groups": a.groupViews(evs), "master": master, "tests": a.tests, "paused": a.paused, "message": a.message,
		"progress": getProgress(), "update": getUpdate(), "devMode": a.cfg.DevMode, "tweaks": len(a.cfg.Tweaks),
		"settings": map[string]any{
			"keepInTray": a.cfg.KeepInTray, "autoUpdate": a.cfg.AutoUpdate, "startWithWindows": a.startsWin,
			"sourceIni": a.sourcePath(), "workDir": a.cfg.WorkDir, "community": a.cfg.Community, "repo": a.cfg.Repo,
			"pkgModule":  installedPackageVersion(a.cfg.Community, "whiskeesfx-visualfx"),
			"pkgEffects": installedPackageVersion(a.cfg.Community, "whiskeesfx-visualfx-effects"),
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
		if v, ok := b["view"].(string); ok && (v == "basic" || v == "advanced") {
			app.cfg.View = v
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
