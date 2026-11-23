package main

import "testing"

func TestTypeDefault(t *testing.T) {
	a := &App{profiles: loadProfiles("profiles")}
	a.cfg.Assign = map[string]string{}
	a.title = "FenixA321 IAE SL Jet2"
	if p := a.pickActive(); p == nil || p.ID != "fenix-a321-iae-sharklets" || p.Type != "Fenix A321" {
		t.Fatalf("auto: %+v", p)
	}
	a.cfg.TypeDefault = map[string]string{"Fenix A321": "fenix-a321-cfm-fences"}
	if p := a.pickActive(); p.ID != "fenix-a321-cfm-fences" {
		t.Fatalf("type default not used: %s", p.ID)
	}
	a.cfg.Assign[a.title] = "fenix-a321-cfm-sharklets"
	if p := a.pickActive(); p.ID != "fenix-a321-cfm-sharklets" {
		t.Fatalf("remembered choice should beat the type default: %s", p.ID)
	}
	a.title = "PMDG 737-800 Ryanair"
	a.active = a.pickActive()
	if a.supported() {
		t.Fatal("non-Fenix aircraft should be unsupported")
	}
}
