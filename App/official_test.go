package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfficialProfile(t *testing.T) {
	p, err := loadProfile("profiles/fenix-a321-cfm-sharklets.ini", false)
	if err != nil || !p.Official || p.Version == "" || len(p.Notes) == 0 {
		t.Fatalf("official profile not read: %v %+v", err, p)
	}
	u, _ := loadProfile("profiles/fenix-a321-cfm-sharklets.ini", true)
	if u.Official {
		t.Fatal("a user profile must never be official")
	}
	dir := t.TempDir()
	c, err := duplicateProfile(p, "My A321", dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Official || c.Version != "" || len(c.Notes) != 0 || c.basedOn() == "" {
		t.Fatalf("copy kept official marks: %+v based_on=%q", c, c.basedOn())
	}
	b, _ := os.ReadFile(filepath.Join(dir, c.ID+".ini"))
	if len(b) == 0 {
		t.Fatal("copy not written")
	}
}

func TestBakeOfficial(t *testing.T) {
	p, _ := loadProfile("profiles/fenix-a321-cfm-sharklets.ini", false)
	a := &App{}
	a.cfg.Tweaks = []Tweak{{p.ID, "@points", "engine_radius", "1.25"}, {p.ID, "", "nose_gear_z", "13.1"}, {p.ID, "effect.apu_steam", "offset", "0 1 -22"}, {"all", "", "x", "1"}}
	id, file, err := a.bakeOfficial(p, "v0.2", "Nose gear nudged")
	if err != nil || id != "fenix-a321-cfm-sharklets" {
		t.Fatal(err, id)
	}
	f := &Ini{Lines: strings.Split(file, "\n")}
	for _, c := range [][3]string{{"points", "engine_radius", "1.25"}, {"globals", "nose_gear_z", "13.1"}, {"effect.apu_steam", "offset", "0 1 -22"}, {"profile", "version", "0.2"}, {"profile", "official", "1"}, {"profile", "changes", "v0.2: Nose gear nudged"}} {
		if v, _ := f.Get(c[0], c[1]); v != c[2] {
			t.Errorf("%s.%s = %q, want %q", c[0], c[1], v, c[2])
		}
	}
	if _, _, err := a.bakeOfficial(p, "soon", ""); err == nil {
		t.Error("bad version accepted")
	}
}
