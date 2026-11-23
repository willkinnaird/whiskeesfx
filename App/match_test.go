package main

import "testing"

func TestPickProfile(t *testing.T) {
	ps := loadProfiles("profiles")
	cases := map[string]string{
		"FenixA321 CFM SL SC": "fenix-a321-cfm-sharklets",
		"FenixA320 IAE":       "fenix-a320-iae-fences",
		"FenixA319 CFM SL":    "fenix-a319-cfm-sharklets",
		"FenixA319 IAE SLOW":  "fenix-a319-iae-fences",
		"Fenix A320 CFM":      "fenix-a320-cfm-fences",
		"Some Bizjet":         "generic",
	}
	for title, want := range cases {
		if got := pickProfile(ps, title); got == nil || got.ID != want {
			id := "<nil>"
			if got != nil {
				id = got.ID
			}
			t.Errorf("%q: got %s, want %s", title, id, want)
		}
	}
}

// Only the Fenix A319/A320/A321 are supported for now: everything else falls back to generic (and gets no effects).
func TestPickOthers(t *testing.T) {
	ps := loadProfiles("profiles")
	for title, want := range map[string]string{
		"PMDG 737-800 Ryanair":        "generic",
		"PMDG 777-300ER Emirates":     "generic",
		"PMDG 777F Lufthansa":         "generic",
		"iFly 737 MAX 8 Ryanair":      "generic",
		"iniBuilds A350-900 Finnair":  "generic",
		"iniBuilds A350-1000 Qatar":   "generic",
		"FlyByWire A380-842 Emirates": "generic",
		"FenixA321 CFM SL SC":         "fenix-a321-cfm-sharklets",
	} {
		if got := pickProfile(ps, title); got == nil || got.ID != want {
			t.Errorf("%q: got %v, want %s", title, got, want)
		}
	}
}
