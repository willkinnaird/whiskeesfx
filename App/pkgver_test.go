package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestNoDowngrade(t *testing.T) {
	d := t.TempDir()
	z := filepath.Join(d, "p.zip")
	f, _ := os.Create(z)
	w := zip.NewWriter(f)
	m, _ := w.Create("whiskeesfx-visualfx-effects/manifest.json")
	m.Write([]byte(`{"package_version": "1.0.0"}`))
	w.Close()
	f.Close()
	if v := zipPackageVersion(z); v != "1.0.0" {
		t.Fatalf("zip version %q", v)
	}
	if !newer("1.1.0", "1.0.0") || newer("1.0.0", "1.0.0") {
		t.Fatal("compare")
	}
}
