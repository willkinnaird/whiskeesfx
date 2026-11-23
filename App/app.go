package main

// The app's state and the one place the live settings file is written:
//   live = defaults  +  aircraft profile  +  your tweaks  +  Basic-view group settings  +  test switches

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Tweak struct {
	Scope   string `json:"scope"` // "all" or a profile id
	Section string `json:"section"`
	Key     string `json:"key"`
	Value   string `json:"value"`
}

type GroupSetting struct {
	Off      bool    `json:"off"`
	Strength float64 `json:"strength"` // 1 = as shipped
}

type Config struct {
	SourceIni     string                   `json:"sourceIni"` // defaults file ("" = the app's own copy)
	WorkDir       string                   `json:"workDir"`
	Community     string                   `json:"community"`
	Repo          string                   `json:"repo"`
	Installed     string                   `json:"installed"` // release tag of the installed packages/defaults
	PendingPkgs   string                   `json:"pendingPkgs,omitempty"`
	Tweaks        []Tweak                  `json:"tweaks"`
	Groups        map[string]*GroupSetting `json:"groups"`
	ManualProfile string                   `json:"manualProfile"` // "" = automatic
	Assign        map[string]string        `json:"assign"`        // aircraft title -> profile id, chosen by you
	Dismissed     map[string]bool          `json:"dismissed"`     // notices closed with their X
	Ignore        []IgnoreRule             `json:"ignore"`        // airframes to hide from the Profiles tab (AI traffic etc.)
	IgnoreSeeded  bool                     `json:"ignoreSeeded"`
	View          string                   `json:"view"`
	Theme         string                   `json:"theme"` // auto, light, dark
	Welcomed      bool                     `json:"welcomed"`
	OnTop         bool                     `json:"onTop"`
	KeepInTray    bool                     `json:"keepInTray"`
	AutoUpdate    bool                     `json:"autoUpdate"`
	DevMode       bool                     `json:"devMode"`
	TypeDefault   map[string]string        `json:"typeDefault,omitempty"` // aircraft type -> profile used when nothing is assigned
	FeedbackURL   string                   `json:"feedbackUrl,omitempty"` // developer override for the feedback webhook
	GitHubToken   string                   `json:"githubToken,omitempty"` // developer only: lets Publish write official profiles to the repo
}

type IgnoreRule struct {
	Text string `json:"text"`
	On   bool   `json:"on"`
}

// ignored: an airframe whose title or package contains any switched-on rule.
func (a *App) ignored(title, pkg string) bool {
	t, p := strings.ToLower(title), strings.ToLower(pkg)
	for _, r := range a.cfg.Ignore {
		w := strings.ToLower(strings.TrimSpace(r.Text))
		if r.On && w != "" && (strings.Contains(t, w) || strings.Contains(p, w)) {
			return true
		}
	}
	return false
}

type Tests struct {
	Humid    int    `json:"humid"`  // -1 auto, 0 never, 1 always
	Runway   string `json:"runway"` // auto, wet, dry
	AllOff   bool   `json:"allOff"`
	Solo     string `json:"solo"`
	Markers  bool   `json:"markers"`
	TestMode bool   `json:"testMode"`
	Selected string `json:"selected"` // the test point being moved
}

type App struct {
	mu        sync.Mutex
	cfg       Config
	tests     Tests
	source    *Ini
	sourceMod time.Time
	profiles  []*Profile
	profMod   string
	active    *Profile
	title     string
	status    SimStatus
	statusMod time.Time
	lastHash  [20]byte
	lastLive  time.Time
	paused    bool   // the live file was changed outside the app: don't overwrite until resumed
	message   string // last problem, shown in the UI
	notes     map[string][2]string
	lastSeen  time.Time // last UI request
	views     []EffectView
	slowAt    time.Time
	startsWin bool
	pkgMod    string
	pkgFx     string
	workOK    bool
	commOK    bool
	viewsOK   bool
}

var app = &App{tests: Tests{Humid: -1, Runway: "auto"}}

// ---- folders ----------------------------------------------------------------------------------------

func appDir() string {
	if d := os.Getenv("WFX_HOME"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("APPDATA"), "WhiskeesFX")
}
func localDir() string {
	if d := os.Getenv("WFX_HOME"); d != "" {
		return filepath.Join(d, "local")
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "WhiskeesFX")
}
func defaultsDir() string   { return filepath.Join(appDir(), "defaults") }
func myProfilesDir() string { return filepath.Join(appDir(), "myprofiles") }
func configPath() string    { return filepath.Join(appDir(), "config.json") }

const devMaster = `C:\MSFS 2024 SDK\WhiskeesFX\VisualFX\visualfx.ini`

func findWorkDir() string {
	if d := os.Getenv("WFX_WORK"); d != "" {
		return d
	}
	tail := filepath.Join("WASM", "MSFS2024", "whiskeesfx-visualfx", "work")
	for _, base := range []string{
		filepath.Join(os.Getenv("APPDATA"), "Microsoft Flight Simulator 2024"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Packages\Microsoft.Limitless_8wekyb3d8bbwe\LocalState`),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Packages\Microsoft.Limitless_8wekyb3d8bbwe\LocalCache`),
	} {
		if _, err := os.Stat(filepath.Join(base, "WASM")); err == nil {
			return filepath.Join(base, tail)
		}
	}
	return filepath.Join(os.Getenv("APPDATA"), "Microsoft Flight Simulator 2024", tail)
}

// findCommunity reads InstalledPackagesPath from UserCfg.opt (Steam or MS Store).
func findCommunity() string {
	if d := os.Getenv("WFX_COMMUNITY"); d != "" {
		return d
	}
	for _, cfg := range []string{
		filepath.Join(os.Getenv("APPDATA"), "Microsoft Flight Simulator 2024", "UserCfg.opt"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Packages\Microsoft.Limitless_8wekyb3d8bbwe\LocalCache\UserCfg.opt`),
	} {
		b, err := os.ReadFile(cfg)
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "InstalledPackagesPath") {
				p := strings.Trim(strings.TrimSpace(strings.TrimPrefix(l, "InstalledPackagesPath")), `"`)
				if p != "" {
					return filepath.Join(p, "Community")
				}
			}
		}
	}
	return filepath.Join(os.Getenv("APPDATA"), "Microsoft Flight Simulator 2024", "Packages", "Community")
}

// ---- config -----------------------------------------------------------------------------------------

func (a *App) load() {
	os.MkdirAll(appDir(), 0755)
	a.cfg = Config{Groups: map[string]*GroupSetting{}, KeepInTray: true, AutoUpdate: true, Repo: defaultRepo}
	if b, err := os.ReadFile(configPath()); err == nil {
		json.Unmarshal(b, &a.cfg)
		a.cfg.View = "" // always open on the WhiskeesFX launcher
	}
	if a.cfg.Groups == nil {
		a.cfg.Groups = map[string]*GroupSetting{}
	}
	if a.cfg.Assign == nil {
		a.cfg.Assign = map[string]string{}
	}
	if a.cfg.Dismissed == nil {
		a.cfg.Dismissed = map[string]bool{}
	}
	if !a.cfg.IgnoreSeeded { // AI traffic packages are full of aircraft titles nobody flies
		a.cfg.Ignore = append(a.cfg.Ignore, IgnoreRule{"FSLTL", true}, IgnoreRule{"FSTraffic", true})
		a.cfg.IgnoreSeeded = true
	}
	if a.cfg.Repo == "" {
		a.cfg.Repo = defaultRepo
	}
	if a.cfg.WorkDir == "" {
		a.cfg.WorkDir = findWorkDir()
	}
	if a.cfg.Community == "" {
		a.cfg.Community = findCommunity()
	}
	// First run on the developer's PC: the master copy is the source of the defaults.
	if a.cfg.SourceIni == "" && !fileExists(filepath.Join(defaultsDir(), "visualfx.ini")) && fileExists(devMaster) {
		a.cfg.SourceIni = devMaster
		a.cfg.DevMode = true
	}
	ensureDefaults()
	a.saveConfig()
}

func (a *App) saveConfig() {
	b, _ := json.MarshalIndent(a.cfg, "", "  ")
	writeFileAtomic(configPath(), b)
}

// ensureDefaults writes the copy built into the app if there's no downloaded one yet.
func ensureDefaults() {
	os.MkdirAll(filepath.Join(defaultsDir(), "profiles"), 0755)
	if !fileExists(filepath.Join(defaultsDir(), "visualfx.ini")) {
		writeFileAtomic(filepath.Join(defaultsDir(), "visualfx.ini"), embeddedIni)
	}
	// Shipped profiles always match this app's copies (yours live in myprofiles and your edits are tweaks,
	// so nothing of yours is touched): new ones appear, fixed ones update, retired ones go.
	entries, _ := embeddedProfiles.ReadDir("profiles")
	ship := map[string]bool{}
	for _, e := range entries {
		ship[e.Name()] = true
		p := filepath.Join(defaultsDir(), "profiles", e.Name())
		b, _ := embeddedProfiles.ReadFile("profiles/" + e.Name())
		if cur, err := os.ReadFile(p); err != nil || string(cur) != string(b) {
			writeFileAtomic(p, b)
		}
	}
	old, _ := filepath.Glob(filepath.Join(defaultsDir(), "profiles", "*.ini"))
	for _, f := range old {
		if !ship[filepath.Base(f)] {
			os.Remove(f)
		}
	}
}

func (a *App) sourcePath() string {
	if a.cfg.SourceIni != "" {
		return a.cfg.SourceIni
	}
	return filepath.Join(defaultsDir(), "visualfx.ini")
}
func (a *App) livePath() string   { return filepath.Join(a.cfg.WorkDir, "visualfx.ini") }
func (a *App) statusPath() string { return filepath.Join(a.cfg.WorkDir, "status.txt") }

// ---- the regular check (every 2 s; cheap: file times only, reads only what changed) -----------------

func (a *App) tick() {
	a.mu.Lock()
	defer a.mu.Unlock()
	changed := false
	sourceChanged := false
	if fi, err := os.Stat(a.sourcePath()); err == nil && !fi.ModTime().Equal(a.sourceMod) {
		if b, err := os.ReadFile(a.sourcePath()); err == nil {
			a.source = ParseIni(string(b))
			a.sourceMod = fi.ModTime()
			a.notes = nil
			changed, sourceChanged = true, true
			a.paused = false // new defaults always win
			a.message = ""
		}
	}
	if sig := profilesSignature(); sig != a.profMod {
		a.profiles = loadProfiles(filepath.Join(defaultsDir(), "profiles"), myProfilesDir())
		a.profMod = sig
		changed = true
	}
	if fi, err := os.Stat(a.statusPath()); err == nil && !fi.ModTime().Equal(a.statusMod) {
		a.statusMod = fi.ModTime()
		a.status = readStatus(a.statusPath())
		a.viewsOK = false
		if a.status.Title != "" && a.status.Title != a.title {
			a.title = a.status.Title
			changed = true
		}
	} else if err == nil {
		a.status.Age = time.Since(fi.ModTime()).Seconds()
		fresh := a.status.Age < 5
		if fresh != a.status.Fresh {
			a.viewsOK = false
		}
		a.status.Fresh = fresh
	}
	if p := a.pickActive(); p != a.active {
		a.active = p
		changed = true
	}
	// Was the live file edited by someone else since we wrote it?
	if !a.paused && !sourceChanged && !a.lastLive.IsZero() {
		if fi, err := os.Stat(a.livePath()); err == nil && !fi.ModTime().Equal(a.lastLive) {
			if b, err := os.ReadFile(a.livePath()); err == nil && sha1.Sum(b) != a.lastHash {
				a.paused = true
				a.message = "The live settings file was changed outside the app. The app won't overwrite it until you press Resume."
			} else {
				a.lastLive = fi.ModTime()
			}
		}
	}
	if changed {
		a.composeLocked()
	}
	if time.Since(a.slowAt) > 30*time.Second { // things that cost a process or a file read: twice a minute
		a.slowAt = time.Now()
		a.startsWin = queryStartsWithWindows()
		a.pkgMod = installedPackageVersion(a.cfg.Community, "whiskeesfx-visualfx")
		a.pkgFx = installedPackageVersion(a.cfg.Community, "whiskeesfx-visualfx-effects")
		a.workOK = dirExists(a.cfg.WorkDir) || dirExists(filepath.Dir(a.cfg.WorkDir))
		a.commOK = dirExists(a.cfg.Community)
	}
	tip := "WhiskeesFX"
	if a.active != nil {
		tip += " · " + a.active.Name
	}
	if a.cfg.Groups["*"] != nil && a.cfg.Groups["*"].Off {
		tip += " · paused"
	}
	setTrayTip(tip)
}

func profilesSignature() string {
	var sb strings.Builder
	for _, d := range []string{filepath.Join(defaultsDir(), "profiles"), myProfilesDir()} {
		files, _ := filepath.Glob(filepath.Join(d, "*.ini"))
		for _, f := range files {
			if fi, err := os.Stat(f); err == nil {
				sb.WriteString(f + fi.ModTime().String() + ";")
			}
		}
	}
	return sb.String()
}

func (a *App) pickActive() *Profile {
	if a.cfg.ManualProfile != "" {
		for _, p := range a.profiles {
			if p.ID == a.cfg.ManualProfile {
				return p
			}
		}
	}
	if id := a.cfg.Assign[a.title]; id != "" {
		if p := a.profileByID(id); p != nil {
			return p
		}
	}
	auto := pickProfile(a.profiles, a.title)
	if auto != nil && auto.Type != "" {
		if id := a.cfg.TypeDefault[auto.Type]; id != "" {
			if p := a.profileByID(id); p != nil {
				return p
			}
		}
	}
	return auto
}

// supported: only the Fenix A319/A320/A321 for now; anything else gets no effects.
func (a *App) supported() bool { return a.active != nil && !a.active.Generic }

// ---- compose ----------------------------------------------------------------------------------------

func (a *App) compose() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.composeLocked()
}

// settingsFor: the settings as they are for one aircraft profile (defaults, profile, your tweaks, moved points),
// before the Basic-view groups and the test switches.
func (a *App) settingsFor(p *Profile) *Ini {
	if a.source == nil {
		return nil
	}
	f := a.source.Clone()
	p.apply(f)
	pid := ""
	if p != nil {
		pid = p.ID
	}
	for _, scope := range []string{"all", pid} {
		for _, t := range a.cfg.Tweaks {
			if t.Scope == scope && t.Section != "@points" {
				f.Set(t.Section, t.Key, t.Value)
			}
		}
	}
	a.applyPointShifts(f, p)
	return f
}

func (a *App) build() *Ini {
	f := a.settingsFor(a.active)
	if f == nil {
		return nil
	}
	applyOffsetAdjust(f)
	a.applyGroups(f)
	a.applyTests(f)
	return f
}

// applyOffsetAdjust: an effect's "offset_adjust = dx dy dz" (set in the defaults file) is added to its offset when the
// live settings are written, so the effect can sit a set distance from its marker (e.g. wing vapour 0.5 m under the
// leading-edge points) while the markers stay where they were placed. The key itself is not passed to the module.
func applyOffsetAdjust(f *Ini) {
	// "offset_from = effect.NAME" first: take another effect's offset (after profile tweaks and moved markers), so a
	// layer such as wingtip_vortex_ribs always sits exactly where its parent effect is
	for _, sec := range effectSections(f) {
		src, ok := f.Get(sec, "offset_from")
		if !ok {
			continue
		}
		f.Delete(sec, "offset_from")
		src = strings.TrimSpace(src)
		if !strings.HasPrefix(src, "effect.") {
			src = "effect." + src
		}
		if off, ok := f.Get(src, "offset"); ok {
			f.Set(sec, "offset", off)
		}
	}
	for _, sec := range effectSections(f) {
		adj, ok := f.Get(sec, "offset_adjust")
		if !ok {
			continue
		}
		f.Delete(sec, "offset_adjust")
		off, _ := f.Get(sec, "offset")
		o, d := parseVec(off), parseVec(adj)
		for i := range o {
			o[i] = round3(o[i] + d[i])
		}
		f.Set(sec, "offset", fmtVec(o))
	}
}

func (a *App) profileByID(id string) *Profile {
	for _, p := range a.profiles {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (a *App) activeID() string {
	if a.active == nil {
		return ""
	}
	return a.active.ID
}

func (a *App) composeLocked() {
	a.viewsOK = false
	if a.paused {
		return
	}
	f := a.build()
	if f == nil {
		return
	}
	out := []byte(f.String())
	h := sha1.Sum(out)
	if cur, err := os.ReadFile(a.livePath()); err == nil && sha1.Sum(cur) == h {
		a.lastHash = h
		if fi, err := os.Stat(a.livePath()); err == nil {
			a.lastLive = fi.ModTime()
		}
		return
	}
	os.MkdirAll(a.cfg.WorkDir, 0755)
	if err := writeFileAtomic(a.livePath(), out); err != nil {
		a.message = "Couldn't write the live settings: " + err.Error()
		return
	}
	a.lastHash = h
	if fi, err := os.Stat(a.livePath()); err == nil {
		a.lastLive = fi.ModTime()
	}
}

// effects lists [effect.*] section names in file order.
func effectSections(f *Ini) []string {
	var out []string
	for _, s := range f.Sections() {
		if strings.HasPrefix(s, "effect.") {
			out = append(out, s)
		}
	}
	return out
}

func (a *App) groupOf(sec string) string {
	if a.notes == nil {
		a.notes = map[string][2]string{}
		for _, s := range effectSections(a.source) {
			n, g := a.source.Notes(s)
			a.notes[s] = [2]string{n, g}
		}
	}
	g := a.notes[sec][1]
	if g == "" {
		g = "Other"
	}
	return g
}

func isMarker(sec string) bool { return strings.HasPrefix(sec, "effect.mark_") }

// effective resolves a key through the effect's preset chain.
func effective(f *Ini, sec, key string) (string, bool) {
	for depth := 0; depth < 8 && sec != ""; depth++ {
		if v, ok := f.Get(sec, key); ok {
			return v, true
		}
		p, ok := f.Get(sec, "preset")
		if !ok {
			break
		}
		sec = "preset." + p
	}
	return "", false
}

// paramCount: how many dials the sim will be sent for this effect (own + inherited, each name once).
func paramCount(f *Ini, sec string) int {
	seen := map[string]bool{}
	for depth := 0; depth < 8 && sec != ""; depth++ {
		for _, kv := range f.Keys(sec) {
			if strings.HasPrefix(kv.Key, "param:") {
				seen[kv.Key] = true
			}
		}
		p, ok := f.Get(sec, "preset")
		if !ok {
			break
		}
		sec = "preset." + p
	}
	return len(seen)
}

const maxDials = 16 // the sim ignores every dial of an effect that gets more than this

func (a *App) applyGroups(f *Ini) {
	all := a.cfg.Groups["*"]
	for _, sec := range effectSections(f) {
		if isMarker(sec) {
			continue
		}
		g := a.cfg.Groups[a.groupOf(sec)]
		if (all != nil && all.Off) || (g != nil && g.Off) {
			f.Set(sec, "on", "0")
			continue
		}
		k := 1.0
		if all != nil && all.Strength > 0 {
			k *= all.Strength
		}
		if g != nil && g.Strength > 0 {
			k *= g.Strength
		}
		if k != 1 {
			scaleAlpha(f, sec, k)
		}
	}
}

// scaleAlpha multiplies an effect's peak opacity (RPN: "<expr> k *").
func scaleAlpha(f *Ini, sec string, k float64) bool {
	expr, ok := effective(f, sec, "param:gm_alpha")
	if !ok {
		return false
	}
	if _, own := f.Get(sec, "param:gm_alpha"); !own && paramCount(f, sec) >= maxDials {
		return false // adding a dial here would make the sim ignore all of them
	}
	next := expr + " " + fmtNum(k) + " *"
	if len(next) > 290 { // the module takes formulas up to 300 characters
		return false
	}
	f.Set(sec, "param:gm_alpha", next)
	return true
}

func (a *App) applyTests(f *Ini) {
	t := a.tests
	if t.Humid >= 0 {
		f.Set("", "humid_force", strconv.Itoa(t.Humid))
	}
	switch t.Runway {
	case "wet":
		f.Set("", "wet_threshold", "-1")
	case "dry":
		f.Set("", "wet_threshold", "2")
		f.Set("", "rain_counts_as_wet", "0")
	}
	if t.AllOff || (a.title != "" && !a.supported()) {
		f.Set("", "enabled", "0")
	}
	if t.TestMode { // every effect paused so the position dots are easy to see
		for _, sec := range effectSections(f) {
			f.Set(sec, "on", "0")
		}
		a.testDots(f, a.active, t.Selected)
		return
	}
	for _, sec := range effectSections(f) {
		if isMarker(sec) {
			if t.Markers {
				f.Set(sec, "on", "1")
			}
			continue
		}
		if t.Solo != "" {
			if sec == t.Solo {
				f.Set(sec, "on", "1")
			} else {
				f.Set(sec, "on", "0")
			}
		}
	}
}

// ---- edits from the UI ------------------------------------------------------------------------------

// geometry keys belong to the aircraft, so they're saved for the active profile only.
func isGeometry(key string) bool {
	for _, p := range []string{"engine_", "main_gear_", "nose_gear_", "ground_y", "var"} {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

func (a *App) setTweak(sec, key, value string, scope string) {
	if scope == "" {
		scope = "all"
		if sec == "" && isGeometry(key) && a.active != nil {
			scope = a.active.ID
		}
	}
	for i := range a.cfg.Tweaks {
		t := &a.cfg.Tweaks[i]
		if t.Scope == scope && t.Section == sec && t.Key == key {
			t.Value = value
			return
		}
	}
	a.cfg.Tweaks = append(a.cfg.Tweaks, Tweak{scope, sec, key, value})
}

func (a *App) resetSection(sec string) {
	var keep []Tweak
	for _, t := range a.cfg.Tweaks {
		if t.Section != sec {
			keep = append(keep, t)
		}
	}
	a.cfg.Tweaks = keep
}

// bakeTweaks (developer): write every "all" tweak into the defaults file and forget them.
func (a *App) bakeTweaks() error {
	if a.source == nil {
		return fmt.Errorf("no defaults loaded")
	}
	f := a.source.Clone()
	var keep []Tweak
	for _, t := range a.cfg.Tweaks {
		if t.Scope == "all" {
			f.Set(t.Section, t.Key, t.Value)
		} else {
			keep = append(keep, t)
		}
	}
	if err := writeFileAtomic(a.sourcePath(), []byte(f.String())); err != nil {
		return err
	}
	a.cfg.Tweaks = keep
	return nil
}

// ---- view model for the UI -------------------------------------------------------------------------

type Dial struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Own     bool   `json:"own"`     // set in the effect itself (else from its preset)
	Tweaked bool   `json:"tweaked"` // changed by you in the app
	Comment string `json:"comment,omitempty"`
}

type EffectView struct {
	Section string `json:"section"`
	Name    string `json:"name"`
	Group   string `json:"group"`
	Note    string `json:"note"`
	On      bool   `json:"on"`
	When    string `json:"when"`
	At      string `json:"at"`
	Offset  string `json:"offset"`
	Preset  string `json:"preset"`
	Dials   []Dial `json:"dials"`
	Count   int    `json:"count"`
	Tweaked bool   `json:"tweaked"`
	State   string `json:"state"`
	Marker  bool   `json:"marker"`
}

func (a *App) effectViews() []EffectView {
	if a.source == nil {
		return nil
	}
	if a.viewsOK {
		return a.views
	}
	defer func() { a.viewsOK = true }()
	// what the module actually gets, minus the test switches, so the UI shows real values
	f := a.settingsFor(a.active)
	tweaked := map[string]bool{}
	for _, t := range a.cfg.Tweaks {
		tweaked[t.Section+"|"+t.Key] = true
		tweaked[t.Section] = true
	}
	var out []EffectView
	for _, sec := range effectSections(f) {
		ev := EffectView{Section: sec, Name: strings.TrimPrefix(sec, "effect."), Group: a.groupOf(sec), Marker: isMarker(sec)}
		ev.Note = a.notes[sec][0]
		on, _ := effective(f, sec, "on")
		ev.On = on != "0"
		ev.When, _ = effective(f, sec, "when")
		ev.At, _ = effective(f, sec, "at")
		ev.Offset, _ = effective(f, sec, "offset")
		ev.Preset, _ = f.Get(sec, "preset")
		seen := map[string]bool{}
		s := sec
		for depth := 0; depth < 8 && s != ""; depth++ {
			for _, kv := range f.Keys(s) {
				if strings.HasPrefix(kv.Key, "param:") && !seen[kv.Key] {
					seen[kv.Key] = true
					ev.Dials = append(ev.Dials, Dial{Key: kv.Key[6:], Value: kv.Value, Own: s == sec, Tweaked: tweaked[sec+"|"+kv.Key], Comment: kv.Comment})
				}
			}
			p, ok := f.Get(s, "preset")
			if !ok {
				break
			}
			s = "preset." + p
		}
		sort.SliceStable(ev.Dials, func(i, j int) bool { return dialOrder(ev.Dials[i].Key) < dialOrder(ev.Dials[j].Key) })
		ev.Count = len(ev.Dials)
		ev.Tweaked = tweaked[sec]
		ev.State = "idle"
		if !ev.On {
			ev.State = "off"
		}
		if st, ok := a.status.Effects[ev.Name]; ok && a.status.Fresh {
			ev.State = st.State
		}
		out = append(out, ev)
	}
	a.views = out
	return out
}

var dialRank = map[string]int{}

func init() {
	order := []string{"gm_alpha", "gm_rate", "gm_size", "gm_life", "gm_grey", "gm_blue", "gm_r", "gm_g", "gm_b", "gm_glow",
		"gm_dir_x", "gm_dir_y", "gm_dir_z", "gm_speed", "gm_pitch", "gm_heading", "gm_cone", "gm_cone_in", "gm_jitter",
		"gm_spread_x", "gm_spread_y", "gm_spread_z", "gm_gravity", "gm_drag", "gm_wind", "gm_swirl", "gm_swirl_rate", "gm_swirl_up",
		"gm_grow", "gm_grow_curve", "gm_size_min", "gm_life_min", "gm_shape", "gm_fade_in", "gm_fade_out", "gm_spin", "gm_burst", "gm_burst_time"}
	for i, k := range order {
		dialRank[k] = i
	}
}

func dialOrder(k string) int {
	if r, ok := dialRank[k]; ok {
		return r
	}
	return 1000
}

// ---- helpers ------------------------------------------------------------------------------------------

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func writeFileAtomic(path string, data []byte) error {
	os.MkdirAll(filepath.Dir(path), 0755)
	tmp := path + ".wfxtmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	var err error
	for i := 0; i < 8; i++ { // the module may have the file open for a moment: try again shortly
		if err = os.Rename(tmp, path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	os.Remove(tmp)
	return err
}

func fmtNum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
func itoa(i int) string       { return strconv.Itoa(i) }
func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
