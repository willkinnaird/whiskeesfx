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
		"PMDG 737-800":        "generic",
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
