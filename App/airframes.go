package main

// Finds the aircraft installed in the Community folder by reading the titles in their aircraft.cfg / livery.cfg
// files, so people can see which profile each one uses and assign another.

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Airframe struct {
	Title   string `json:"title"`
	Package string `json:"package"`
}

var (
	afMu      sync.Mutex
	afList    []Airframe
	afScanned time.Time
	afBusy    bool
)

func scanAirframes(community string) {
	afMu.Lock()
	if afBusy {
		afMu.Unlock()
		return
	}
	afBusy = true
	afMu.Unlock()
	seen := map[string]bool{}
	var out []Airframe
	pkgs, _ := os.ReadDir(community)
	for _, p := range pkgs {
		root := filepath.Join(community, p.Name(), "SimObjects", "Airplanes")
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		walkCfg(root, 0, func(path string) {
			for _, t := range titlesIn(path) {
				if !seen[t] {
					seen[t] = true
					out = append(out, Airframe{Title: t, Package: p.Name()})
				}
			}
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title) })
	afMu.Lock()
	afList, afScanned, afBusy = out, time.Now(), false
	afMu.Unlock()
}

// walkCfg visits aircraft.cfg and livery.cfg files up to a few folders deep (following linked package folders).
func walkCfg(dir string, depth int, fn func(string)) {
	if depth > 5 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		name := strings.ToLower(e.Name())
		if name == "aircraft.cfg" || name == "livery.cfg" {
			fn(p)
			continue
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			switch name { // big folders with nothing for us
			case "texture", "model", "panel", "sound", "effects", "checklist":
				continue
			}
			if strings.HasPrefix(name, "texture") || strings.HasPrefix(name, "model") {
				continue
			}
			walkCfg(p, depth+1, fn)
		}
	}
}

func titlesIn(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 64*1024)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(strings.ToLower(l), "title") {
			continue
		}
		eq := strings.Index(l, "=")
		if eq < 0 || strings.TrimSpace(strings.ToLower(l[:eq])) != "title" {
			continue
		}
		v := strings.TrimSpace(l[eq+1:])
		if i := strings.Index(v, ";"); i >= 0 && !strings.HasPrefix(v, "\"") {
			v = strings.TrimSpace(v[:i])
		}
		v = strings.Trim(v, "\"")
		if v != "" && !strings.HasPrefix(v, "$$") && !strings.HasPrefix(v, "TT:") {
			out = append(out, v)
		}
	}
	return out
}

func airframes() ([]Airframe, time.Time, bool) {
	afMu.Lock()
	defer afMu.Unlock()
	return afList, afScanned, afBusy
}
