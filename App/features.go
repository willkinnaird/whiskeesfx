package main

import "strings"

// Features: parts of the airframe that not every aircraft has (engine strakes, winglets...). Each switches a set
// of effects on or off for one profile, saved as profile tweaks "effect.X on=0/1" (baked in when published).
type FeatureDef struct {
	ID, Label, Hint string
	Effects         []string
}

var featureDefs = []FeatureDef{
	{"strakes", "Engine strakes (nacelle fins)", "The small fins on the engine cowls that throw a vortex over the wing at high angles of attack. Most CFM and IAE nacelles on the A320 family have them; many engines don't.",
		[]string{"strake_vortex", "strake_vortex_out"}},
	{"wingtip", "Wingtip vortices", "The rope from each wingtip in humid air.", []string{"wingtip_vortex", "wingtip_vortex_near", "wingtip_vortex_ribs"}},
	{"flaps", "Flap-edge vortices", "The weaker rope from the outer end of the flaps, one per flap setting (1, 2, 3, FULL).", []string{"flap_vortex", "flap_vortex_ribs", "flap_vortex_2", "flap_vortex_2_ribs", "flap_vortex_3", "flap_vortex_3_ribs", "flap_vortex_4", "flap_vortex_4_ribs"}},
	{"wingvapour", "Wing vapour", "The vapour sheet over the wing in humid air at high angles of attack.", wingVapour},
	{"apu", "APU steam", "Steam and heat from the APU exhaust in the tail.", []string{"apu_steam"}},
}

type FeatureView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
	On    bool   `json:"on"`
}

func (a *App) featureViews(f *Ini) []FeatureView {
	var out []FeatureView
	for _, d := range featureDefs {
		on, any := false, false
		for _, e := range d.Effects {
			if _, ok := f.Get("effect."+e, "when"); !ok {
				if _, ok2 := effective(f, "effect."+e, "guid"); !ok2 {
					continue
				}
			}
			any = true
			if v, ok := effective(f, "effect."+e, "on"); !ok || strings.TrimSpace(v) != "0" {
				on = true
			}
		}
		if any {
			out = append(out, FeatureView{d.ID, d.Label, d.Hint, on})
		}
	}
	return out
}

func (a *App) setFeature(p *Profile, id string, on bool) bool {
	for _, d := range featureDefs {
		if d.ID != id {
			continue
		}
		v := "0"
		if on {
			v = "1"
		}
		for _, e := range d.Effects {
			a.setTweak("effect."+e, "on", v, p.ID)
		}
		return true
	}
	return false
}
