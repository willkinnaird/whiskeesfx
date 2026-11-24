package main

// Test points: positions on the aircraft that the effects hang off. In test mode each one shows as a dot in the sim
// (the Marker effect; the selected one also glows orange) and the person nudges it until it sits on the aircraft.
// Every point maps to profile values, saved as tweaks scoped to that profile:
//   globals  -> global keys (engine_x, main_gear_z, ...)
//   effect   -> the offset of one effect and its followers (wingtip_vortex + wingtip_vortex_near, ...)
//   stored   -> an app-only value ("@points" tweak) that shifts a group of effects by how far it moved

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type PointDef struct {
	ID, Label, Group, Hint string
	Anchor                 string    // engines, mains, nose, sides, ref
	Kind                   string    // globals, effect, stored
	Globals                [3]string // for globals: key per axis ("" = not adjustable)
	Effect                 string    // for effect: primary effect (followers get the same offset)
	Followers              []string
	Store                  string   // for stored: key, e.g. "intake_z"
	Line                   []string // for line: effects spread evenly from the inner to the outer point (in order)
	Axes                   string   // which of x, y, z can be moved
	OutSign                float64
}

const markerGUID = "{6B1E0C52-3A7D-4F0E-9C2B-5D8E1A4F7C10}"
const mistGUID = "{4D2A7C10-8E35-4B61-9F02-6A1C3E5B7D81}"

var pointDefs = []PointDef{
	{ID: "engine_centre", Label: "Engine · centre", Group: "Engine", Anchor: "engines", Kind: "globals", Globals: [3]string{"engine_x", "engine_y", "engine_z"}, Axes: "xyz", OutSign: -1,
		Hint: "Look at the engine head-on: the dot belongs in the middle of the fan (the spinner)."},
	{ID: "engine_top", Label: "Engine · top", Group: "Engine", Anchor: "engines", Kind: "stored", Store: "engine_radius", Axes: "y",
		Hint: "Line it up with the top edge of the nacelle. If top and bottom both fit, the centre is right."},
	{ID: "engine_bottom", Label: "Engine · bottom", Group: "Engine", Anchor: "engines", Kind: "stored", Store: "engine_radius", Axes: "y",
		Hint: "Line it up with the bottom edge of the nacelle. The ground vortex into the intake starts here."},
	{ID: "intake_lip", Label: "Engine · intake lip", Group: "Engine", Anchor: "engines", Kind: "stored", Store: "intake_z", Axes: "z",
		Hint: "From the side, put it on the front rim of the intake. Intake condensation, fog and the tornado move with it."},
	{ID: "nozzle", Label: "Engine · back of nozzle", Group: "Engine", Anchor: "engines", Kind: "stored", Store: "nozzle_z", Axes: "z",
		Hint: "From the side, put it on the rear edge of the exhaust nozzle. Exhaust steam, heat shimmer and the start-up puff move with it."},
	{ID: "pylon_top", Label: "Pylon · top", Group: "Engine", Anchor: "engines", Kind: "effect", Effect: "pylon_vapour", Axes: "xyz", OutSign: -1,
		Hint: "Where the pylon meets the underside of the wing: the pylon vapour streams from here."},
	{ID: "wingtip_top", Label: "Wingtip · top", Group: "Wings", Anchor: "sides", Kind: "effect", Effect: "wingtip_vortex", Followers: []string{"wingtip_vortex_near"}, Axes: "xyz", OutSign: 1,
		Hint: "The very top of the winglet or sharklet (on a plain or raked tip, the tip itself), at its trailing edge. The vortex leaves from here."},
	{ID: "wingtip_bottom", Label: "Wingtip · bottom", Group: "Wings", Anchor: "sides", Kind: "stored", Store: "winglet_h", Axes: "xyz",
		Hint: "Where the winglet meets the wing. On a plain tip this sits just under the top dot."},
	{ID: "le_inner", Label: "Leading edge · inner", Group: "Wings", Anchor: "sides", Kind: "line", Line: wingVapour, Axes: "xyz", OutSign: 1,
		Hint: "On the wing's leading edge, just outboard of the engine pylon. The wing vapour sheet runs from here to the outer point."},
	{ID: "le_outer", Label: "Leading edge · outer", Group: "Wings", Anchor: "sides", Kind: "line", Line: wingVapour, Axes: "xyz", OutSign: 1,
		Hint: "On the leading edge near the tip, where the vapour sheet ends. Inner and outer together set the sheet's sweep angle."},
	{ID: "flap_end", Label: "Flap 1 · outer end", Group: "Wings", Anchor: "sides", Kind: "effect", Effect: "flap_vortex", Axes: "xyz", OutSign: 1,
		Hint: "The outboard end of the outer flap, at its trailing edge, with the flap lever at 1. The flap 1 vortex starts here."},
	{ID: "flap_2", Label: "Flap 2 · outer end", Group: "Wings", Anchor: "sides", Kind: "effect", Effect: "flap_vortex_2", Axes: "xyz", OutSign: 1,
		Hint: "The same flap end with the lever at 2 (further back and lower). The flap 2 vortex starts here."},
	{ID: "flap_3", Label: "Flap 3 · outer end", Group: "Wings", Anchor: "sides", Kind: "effect", Effect: "flap_vortex_3", Axes: "xyz", OutSign: 1,
		Hint: "The same flap end with the lever at 3. The flap 3 vortex starts here."},
	{ID: "flap_4", Label: "Flap FULL · outer end", Group: "Wings", Anchor: "sides", Kind: "effect", Effect: "flap_vortex_4", Axes: "xyz", OutSign: 1,
		Hint: "The same flap end with the lever at FULL (furthest back and lowest). The FULL vortex starts here."},
	{ID: "strake_in", Label: "Engine strake · inboard", Group: "Engine", Anchor: "engines", Kind: "effect", Effect: "strake_vortex", Axes: "xyz", OutSign: -1,
		Hint: "The small fin on the inboard side of the engine cowl, at its trailing tip. Only if this aircraft has strakes (Features)."},
	{ID: "strake_out", Label: "Engine strake · outboard", Group: "Engine", Anchor: "engines", Kind: "effect", Effect: "strake_vortex_out", Axes: "xyz", OutSign: -1,
		Hint: "The fin on the outboard side of the cowl, if there is one."},
	{ID: "main_gear", Label: "Main gear", Group: "Gear", Anchor: "mains", Kind: "globals", Globals: [3]string{"main_gear_x", "", "main_gear_z"}, Axes: "xz", OutSign: -1,
		Hint: "Where the main wheels touch the runway (the middle of the bogie)."},
	{ID: "nose_gear", Label: "Nose gear", Group: "Gear", Anchor: "nose", Kind: "globals", Globals: [3]string{"", "", "nose_gear_z"}, Axes: "z",
		Hint: "Where the nose wheels touch the runway."},
	{ID: "ground", Label: "Ground level", Group: "Gear", Anchor: "ref", Kind: "globals", Globals: [3]string{"", "ground_y", ""}, Axes: "y",
		Hint: "The dot under the middle of the aircraft should sit exactly on the tarmac. Spray and splashes start at this level."},
	{ID: "apu", Label: "APU exhaust", Group: "Tail", Anchor: "ref", Kind: "effect", Effect: "apu_steam", Axes: "xyz", OutSign: 1,
		Hint: "The APU exhaust outlet in the tail cone."},
}

var wingVapour = []string{"wing_vapour_1", "wing_vapour_2", "wing_vapour_3", "wing_vapour_4", "wing_vapour_5", "wing_vapour_6",
	"wing_vapour_7", "wing_vapour_8", "wing_vapour_9", "wing_vapour_10", "wing_vapour_11", "wing_vapour_12"}

var storeDefaults = map[string]float64{"engine_radius": 1.1, "intake_z": 1.6, "nozzle_z": -3.2, "winglet_h": 2.4, "winglet_dx": 0, "winglet_dz": 0}

func pointDef(id string) *PointDef {
	for i := range pointDefs {
		if pointDefs[i].ID == id {
			return &pointDefs[i]
		}
	}
	return nil
}

func parseVec(s string) [3]float64 {
	var v [3]float64
	for i, f := range strings.Fields(s) {
		if i < 3 {
			v[i], _ = strconv.ParseFloat(f, 64)
		}
	}
	return v
}

func fmtVec(v [3]float64) string {
	return fmt.Sprintf("%s %s %s", fmtNum(round3(v[0])), fmtNum(round3(v[1])), fmtNum(round3(v[2])))
}
func round3(x float64) float64 { return math.Round(x*1000) / 1000 }

// storeDefault: the profile's own [points] value, else the general default.
func storeDefault(p *Profile, key string) float64 {
	if p != nil && p.ini != nil {
		if v, ok := p.ini.Get("points", key); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
	}
	return storeDefaults[key]
}

func (a *App) storeValue(p *Profile, key string) float64 {
	id := ""
	if p != nil {
		id = p.ID
	}
	for _, t := range a.cfg.Tweaks {
		if t.Scope == id && t.Section == "@points" && t.Key == key {
			if f, err := strconv.ParseFloat(t.Value, 64); err == nil {
				return f
			}
		}
	}
	return storeDefault(p, key)
}

// applyPointShifts moves groups of effects by how far their stored point moved from the profile's default.
func (a *App) applyPointShifts(f *Ini, p *Profile) {
	shift := func(key string, axis int, names func(string) bool, sign float64) {
		d := (a.storeValue(p, key) - storeDefault(p, key)) * sign
		if d == 0 {
			return
		}
		for _, sec := range effectSections(f) {
			if !names(strings.TrimPrefix(sec, "effect.")) {
				continue
			}
			off, ok := effective(f, sec, "offset")
			if !ok {
				continue
			}
			v := parseVec(off)
			v[axis] += d
			f.Set(sec, "offset", fmtVec(v))
		}
	}
	shift("intake_z", 2, func(n string) bool { return strings.Contains(n, "intake_") }, 1)
	nozzle := map[string]bool{"exhaust_steam": true, "heat_haze": true, "heat_haze_asobo": true, "engine_start_puff": true}
	shift("nozzle_z", 2, func(n string) bool { return nozzle[n] }, 1)
	// a bigger engine: the ground vortex starts lower
	shift("engine_radius", 1, func(n string) bool { return n == "intake_swirl" || n == "snow_intake_vortex" }, -1)
	// extra directions on markers that natively move one way only: shift the effects that hang off them
	at := func(n string) string { v, _ := effective(f, "effect."+n, "at"); return strings.TrimSpace(v) }
	for id, follows := range extraFollow {
		for ax := 0; ax < 3; ax++ {
			key := extraKey(id, ax)
			d := a.storeValue(p, key)
			if d == 0 || follows == nil {
				continue
			}
			for _, sec := range effectSections(f) {
				n := strings.TrimPrefix(sec, "effect.")
				if !follows(n, at(n)) {
					continue
				}
				if off, ok := effective(f, sec, "offset"); ok {
					v := parseVec(off)
					v[ax] += d
					f.Set(sec, "offset", fmtVec(v))
				}
			}
		}
	}
}

// extraKey: the stored offset for a direction a marker doesn't natively move in ("intake_lip.dx").
func extraKey(id string, axis int) string { return id + ".d" + string("xyz"[axis]) }

// extraFollow: which effects move with a marker's extra directions (nil = only the dot moves).
var extraFollow = map[string]func(name, anchor string) bool{
	"intake_lip": func(n, _ string) bool { return strings.Contains(n, "intake_") },
	"nozzle": func(n, _ string) bool {
		return n == "exhaust_steam" || n == "heat_haze" || n == "heat_haze_asobo" || n == "engine_start_puff"
	},
	"engine_top":    nil,
	"engine_bottom": func(n, _ string) bool { return n == "intake_swirl" || n == "snow_intake_vortex" },
	"nose_gear":     func(_, a string) bool { return a == "nose" },
	"main_gear":     func(_, a string) bool { return a == "mains" },
	"ground":        nil,
}

type PointView struct {
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Group    string     `json:"group"`
	Hint     string     `json:"hint"`
	Axes     string     `json:"axes"`
	Value    [3]float64 `json:"value"` // what the person edits (globals / effect offset / stored value in [1] or [2])
	Editable [3]bool    `json:"editable"`
	Anchor   string     `json:"anchor"`
	Dot      [3]float64 `json:"dot"` // offset of the dot from its anchor
	Changed  bool       `json:"changed"`
	OutSign  float64    `json:"outSign"`
}

// pointViews computes every point for a profile from the settings as they'd be built for it.
func (a *App) pointViews(p *Profile, f *Ini) []PointView {
	g := func(k string) float64 {
		if k == "" {
			return 0
		}
		v, _ := f.Get("", k)
		x, _ := strconv.ParseFloat(strings.Fields(v + " 0")[0], 64)
		return x
	}
	tweaked := map[string]bool{}
	id := ""
	if p != nil {
		id = p.ID
	}
	for _, t := range a.cfg.Tweaks {
		if t.Scope == id {
			tweaked[t.Section+"|"+t.Key] = true
		}
	}
	tip := parseVec(func() string { s, _ := effective(f, "effect.wingtip_vortex", "offset"); return s }())
	var out []PointView
	for _, d := range pointDefs {
		pv := PointView{ID: d.ID, Label: d.Label, Group: d.Group, Hint: d.Hint, Axes: d.Axes, Anchor: d.Anchor, OutSign: d.OutSign}
		for i := range pv.Editable {
			pv.Editable[i] = true // every marker moves all six ways (extra directions are stored offsets)
		}
		switch d.Kind {
		case "globals":
			for i, k := range d.Globals {
				pv.Value[i] = g(k)
				if k != "" && tweaked["|"+k] {
					pv.Changed = true
				}
			}
			switch d.ID {
			case "engine_centre", "main_gear", "nose_gear":
				pv.Dot = [3]float64{0, 0, 0}
			case "ground":
				pv.Dot = [3]float64{0, pv.Value[1], 0} // on the centre line, under the reference point
			}
		case "effect":
			s, _ := effective(f, "effect."+d.Effect, "offset")
			pv.Value = parseVec(s)
			pv.Dot = pv.Value
			pv.Changed = tweaked["effect."+d.Effect+"|offset"]
		case "line":
			e := d.Line[0]
			if d.ID == "le_outer" {
				e = d.Line[len(d.Line)-1]
			}
			s, _ := effective(f, "effect."+e, "offset")
			pv.Value = parseVec(s)
			pv.Dot = pv.Value
			for _, n := range d.Line {
				pv.Changed = pv.Changed || tweaked["effect."+n+"|offset"]
			}
		case "stored":
			v := a.storeValue(p, d.Store)
			pv.Changed = tweaked["@points|"+d.Store]
			switch d.ID {
			case "engine_top":
				pv.Value[1], pv.Dot = v, [3]float64{0, v, 0}
			case "engine_bottom":
				pv.Value[1], pv.Dot = v, [3]float64{0, -v, 0}
			case "intake_lip", "nozzle":
				pv.Value[2], pv.Dot = v, [3]float64{0, 0, v}
			case "wingtip_bottom":
				// winglet root: down by winglet_h from the tip, plus a sideways / fore-aft offset for canted or raked winglets
				dx, dz := a.storeValue(p, "winglet_dx"), a.storeValue(p, "winglet_dz")
				pv.Value = [3]float64{dx, v, dz}
				pv.Dot = [3]float64{tip[0] + dx, tip[1] - v, tip[2] + dz}
				pv.Changed = pv.Changed || tweaked["@points|winglet_dx"] || tweaked["@points|winglet_dz"]
			}
		}
		if _, ok := extraFollow[d.ID]; ok {
			for i, ax := range "xyz" {
				if strings.ContainsRune(d.Axes, ax) {
					continue
				}
				e := a.storeValue(p, extraKey(d.ID, i))
				pv.Value[i] = e
				pv.Dot[i] += e
				pv.Changed = pv.Changed || tweaked["@points|"+extraKey(d.ID, i)]
			}
		}
		pv.Axes = "xyz"
		out = append(out, pv)
	}
	return out
}

// movePoint applies a nudge (or a typed value) to a profile.
func (a *App) movePoint(p *Profile, f *Ini, id string, axis int, delta float64, set *float64) error {
	d := pointDef(id)
	if d == nil || axis < 0 || axis > 2 {
		return fmt.Errorf("that point can't move that way")
	}
	if !strings.ContainsRune(d.Axes, rune("xyz"[axis])) {
		if _, ok := extraFollow[d.ID]; !ok {
			return fmt.Errorf("that point can't move that way")
		}
		scope := ""
		if p != nil {
			scope = p.ID
		}
		var cur float64
		for _, v := range a.pointViews(p, f) {
			if v.ID == id {
				cur = v.Value[axis]
			}
		}
		nv := cur + delta
		if set != nil {
			nv = *set
		}
		a.setTweak("@points", extraKey(d.ID, axis), fmtNum(round3(nv)), scope)
		return nil
	}
	scope := ""
	if p != nil {
		scope = p.ID
	}
	var pv PointView
	for _, v := range a.pointViews(p, f) {
		if v.ID == id {
			pv = v
		}
	}
	nv := pv.Value[axis] + delta
	if set != nil {
		nv = *set
	}
	switch d.Kind {
	case "globals":
		a.setTweak("", d.Globals[axis], fmtNum(round3(nv)), scope)
	case "effect":
		v := pv.Value
		moved := nv - v[axis]
		v[axis] = nv
		if d.ID == "wingtip_top" && moved != 0 {
			// the bottom marker is stored relative to the top one: shift that offset the other way so it stays put
			switch axis {
			case 0:
				a.setTweak("@points", "winglet_dx", fmtNum(round3(a.storeValue(p, "winglet_dx")-moved)), scope)
			case 1:
				a.setTweak("@points", "winglet_h", fmtNum(round3(math.Max(0, a.storeValue(p, "winglet_h")+moved))), scope)
			case 2:
				a.setTweak("@points", "winglet_dz", fmtNum(round3(a.storeValue(p, "winglet_dz")-moved)), scope)
			}
		}
		for _, e := range append([]string{d.Effect}, d.Followers...) {
			a.setTweak("effect."+e, "offset", fmtVec(v), scope)
		}
	case "line":
		// move one end, keep the other, and spread every effect in the line evenly between them
		a0 := parseVec(func() string { s, _ := effective(f, "effect."+d.Line[0], "offset"); return s }())
		a1 := parseVec(func() string { s, _ := effective(f, "effect."+d.Line[len(d.Line)-1], "offset"); return s }())
		if d.ID == "le_inner" {
			a0[axis] = nv
		} else {
			a1[axis] = nv
		}
		n := len(d.Line)
		for i, e := range d.Line {
			t := float64(i) / float64(n-1)
			var v [3]float64
			for k := 0; k < 3; k++ {
				v[k] = a0[k] + (a1[k]-a0[k])*t
			}
			a.setTweak("effect."+e, "offset", fmtVec(v), scope)
		}
	case "stored":
		store := d.Store
		if d.ID == "wingtip_bottom" && axis != 1 {
			store = [3]string{"winglet_dx", "", "winglet_dz"}[axis]
			a.setTweak("@points", store, fmtNum(round3(nv)), scope)
			return nil
		}
		if set == nil {
			// moving the bottom dot down (or the winglet root down) makes the value bigger
			if d.ID == "engine_bottom" || d.ID == "wingtip_bottom" {
				nv = pv.Value[axis] - delta
			}
		}
		if (d.Store == "engine_radius" || d.Store == "winglet_h") && nv < 0 {
			nv = 0
		}
		a.setTweak("@points", store, fmtNum(round3(nv)), scope)
	}
	return nil
}

// testDots appends one Marker per point (and an orange glow on the selected one) to the settings file.
func (a *App) testDots(f *Ini, p *Profile, selected string) {
	var b strings.Builder
	b.WriteString("\n# ---- test mode: position dots (added by the WhiskeesFX app, never saved) ----\n")
	for _, pv := range a.pointViews(p, f) {
		fmt.Fprintf(&b, "[effect.wfxdot_%s]\nguid=%s\nat=%s\noffset=%s\nwhen=always\non=1\n\n", pv.ID, markerGUID, pv.Anchor, fmtVec(pv.Dot))
		if pv.ID == selected {
			fmt.Fprintf(&b, "[effect.wfxdot_selected]\nguid=%s\nat=%s\noffset=%s\nwhen=always\non=1\n", mistGUID, pv.Anchor, fmtVec(pv.Dot))
			b.WriteString("param=gm_dir_y 0\nparam=gm_jitter 0\nparam=gm_spread_x 0.02\nparam=gm_spread_y 0.02\nparam=gm_spread_z 0.02\n" +
				"param=gm_size 0.35\nparam=gm_life 0.4\nparam=gm_rate 40\nparam=gm_alpha 0.9\nparam=gm_grey 1\nparam=gm_r 1\nparam=gm_g 0.45\nparam=gm_b 0.15\nparam=gm_glow 1\n\n")
		}
	}
	f.Lines = append(f.Lines, strings.Split(b.String(), "\n")...)
}
