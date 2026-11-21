package main

import (
	"os"
	"path/filepath"
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
