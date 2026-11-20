package main

// Updates from GitHub Releases. A release (tag vX.Y.Z) carries:
//   WhiskeesFX.exe                     the app
//   whiskeesfx-visualfx.zip            Community package: the module
//   whiskeesfx-visualfx-effects.zip    Community package: the custom effects
//   defaults.zip                       visualfx.ini + profiles/*.ini
// Settings and profiles update at once. The two packages can only be swapped while the sim is closed,
// so if it's running they wait and go in as soon as it closes. Your tweaks are kept (they're applied on top).

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var defaultRepo = "whiskeesfx/whiskeesfx" // set at build time with -ldflags "-X main.defaultRepo=owner/name"
var version = "dev"                       // set at build time

type Release struct {
	Tag    string `json:"tag_name"`
	Name   string `json:"name"`
	Body   string `json:"body"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

var httpc = &http.Client{Timeout: 60 * time.Second}

func latestRelease(repo string) (*Release, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("User-Agent", "WhiskeesFX/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("no releases published yet")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var r Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (r *Release) asset(name string) string {
	for _, a := range r.Assets {
		if strings.EqualFold(a.Name, name) {
			return a.URL
		}
	}
	return ""
}

// newer: is tag a newer version than have? ("v1.10.0" > "v1.9.2"; "dev" is always older)
func newer(tag, have string) bool {
	if have == "" || have == "dev" {
		return tag != ""
	}
	pa, pb := vparts(tag), vparts(have)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func vparts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	for i, p := range strings.SplitN(v, ".", 3) {
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		out[i] = n
	}
	return out
}

// download streams a file to disk (never held in memory).
func download(url, dest string) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "WhiskeesFX/"+version)
	c := &http.Client{Timeout: 15 * time.Minute}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	os.MkdirAll(filepath.Dir(dest), 0755)
	f, err := os.Create(dest + ".part")
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(dest + ".part")
		return err
	}
	os.Remove(dest)
	return os.Rename(dest+".part", dest)
}

// unzip extracts into dir, refusing any path that would land outside it.
func unzip(src, dir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	root, _ := filepath.Abs(dir)
	for _, f := range r.File {
		p := filepath.Join(root, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(p, root+string(os.PathSeparator)) && p != root {
			return fmt.Errorf("bad path in zip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(p, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(p), 0755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(p)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func dlDir() string { return filepath.Join(localDir(), "downloads") }

type UpdateInfo struct {
	Current   string `json:"current"`
	Installed string `json:"installed"`
	Latest    string `json:"latest"`
	Notes     string `json:"notes"`
	AppNew    bool   `json:"appNew"`
	DataNew   bool   `json:"dataNew"`
	Pending   bool   `json:"pending"`
	Error     string `json:"error,omitempty"`
	Checked   string `json:"checked"`
	Busy      string `json:"busy,omitempty"`
}

var (
	updMu       sync.Mutex
	lastUpdate  UpdateInfo
	progressMsg string
)

func getUpdate() UpdateInfo { updMu.Lock(); defer updMu.Unlock(); return lastUpdate }
func getProgress() string   { updMu.Lock(); defer updMu.Unlock(); return progressMsg }
func setProgress(s string)  { updMu.Lock(); progressMsg = s; updMu.Unlock() }
func setBusy(s string)      { updMu.Lock(); lastUpdate.Busy = s; updMu.Unlock() }

func (a *App) checkUpdate() UpdateInfo {
	a.mu.Lock()
	repo, installed, pending := a.cfg.Repo, a.cfg.Installed, a.cfg.PendingPkgs
	a.mu.Unlock()
	u := UpdateInfo{Current: version, Installed: installed, Pending: pending != "", Checked: time.Now().Format("15:04")}
	r, err := latestRelease(repo)
	if err != nil {
		u.Error = err.Error()
	} else {
		u.Latest = r.Tag
		u.Notes = r.Body
		u.AppNew = newer(r.Tag, version) && r.asset("WhiskeesFX.exe") != ""
		u.DataNew = newer(r.Tag, installed)
	}
	updMu.Lock()
	u.Busy = lastUpdate.Busy
	lastUpdate = u
	updMu.Unlock()
	return u
}

// applyUpdate: settings + profiles now, packages now or when the sim closes, app last (it restarts).
func (a *App) applyUpdate(progress func(string)) error {
	a.mu.Lock()
	repo, devMode := a.cfg.Repo, a.cfg.DevMode
	a.mu.Unlock()
	r, err := latestRelease(repo)
	if err != nil {
		return err
	}
	tag := r.Tag
	if u := r.asset("defaults.zip"); u != "" && !devMode {
		progress("Downloading settings and aircraft profiles…")
		z := filepath.Join(dlDir(), tag, "defaults.zip")
		if err := download(u, z); err != nil {
			return err
		}
		tmp := filepath.Join(dlDir(), tag, "defaults")
		os.RemoveAll(tmp)
		if err := unzip(z, tmp); err != nil {
			return err
		}
		if fileExists(filepath.Join(tmp, "visualfx.ini")) {
			b, _ := os.ReadFile(filepath.Join(tmp, "visualfx.ini"))
			writeFileAtomic(filepath.Join(defaultsDir(), "visualfx.ini"), b)
		}
		files, _ := filepath.Glob(filepath.Join(tmp, "profiles", "*.ini"))
		for _, f := range files {
			b, _ := os.ReadFile(f)
			writeFileAtomic(filepath.Join(defaultsDir(), "profiles", filepath.Base(f)), b)
		}
	}
	for _, name := range []string{"whiskeesfx-visualfx.zip", "whiskeesfx-visualfx-effects.zip"} {
		if u := r.asset(name); u != "" {
			progress("Downloading " + name + "…")
			if err := download(u, filepath.Join(dlDir(), tag, name)); err != nil {
				return err
			}
		}
	}
	a.mu.Lock()
	a.cfg.PendingPkgs = tag
	a.cfg.Installed = tag
	a.saveConfig()
	a.mu.Unlock()
	a.installPendingPackages(progress)
	if newer(tag, version) {
		if u := r.asset("WhiskeesFX.exe"); u != "" {
			progress("Downloading the new app…")
			return selfUpdate(u)
		}
	}
	return nil
}

// installPendingPackages swaps the downloaded packages into Community if the sim is closed.
func (a *App) installPendingPackages(progress func(string)) {
	a.mu.Lock()
	tag, comm := a.cfg.PendingPkgs, a.cfg.Community
	a.mu.Unlock()
	if tag == "" {
		return
	}
	if simRunning() {
		progress("The sim is running: the new effects will be installed as soon as you close it.")
		return
	}
	for _, name := range []string{"whiskeesfx-visualfx.zip", "whiskeesfx-visualfx-effects.zip"} {
		z := filepath.Join(dlDir(), tag, name)
		if !fileExists(z) {
			continue
		}
		progress("Installing " + strings.TrimSuffix(name, ".zip") + "…")
		if err := installPackage(z, comm); err != nil {
			progress("Install failed: " + err.Error())
			return
		}
	}
	a.mu.Lock()
	a.cfg.PendingPkgs = ""
	a.saveConfig()
	a.mu.Unlock()
	os.RemoveAll(filepath.Join(dlDir(), tag))
	progress("Installed " + tag + ".")
}

// installPackage: the zip holds one top folder (the package). Extract beside it, then swap.
func installPackage(zipPath, community string) error {
	stage := filepath.Join(community, ".wfx-staging")
	os.RemoveAll(stage)
	if err := unzip(zipPath, stage); err != nil {
		os.RemoveAll(stage)
		return err
	}
	defer os.RemoveAll(stage)
	entries, _ := os.ReadDir(stage)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dst := filepath.Join(community, e.Name())
		old := dst + ".wfx-old"
		os.RemoveAll(old)
		if fileExists(dst) {
			if err := os.Rename(dst, old); err != nil {
				return fmt.Errorf("%s is in use (is the sim still closing?)", e.Name())
			}
		}
		if err := os.Rename(filepath.Join(stage, e.Name()), dst); err != nil {
			os.Rename(old, dst)
			return err
		}
		os.RemoveAll(old)
	}
	return nil
}

// installedPackageVersion reads package_version from a Community package's manifest.json.
func installedPackageVersion(community, name string) string {
	b, err := os.ReadFile(filepath.Join(community, name, "manifest.json"))
	if err != nil {
		return ""
	}
	var m struct {
		V string `json:"package_version"`
	}
	json.Unmarshal(b, &m)
	return m.V
}

// selfUpdate: Windows lets a running exe be renamed, so: download new, rename self to .old, put new in place,
// start it, quit. The new one deletes the .old on start.
func selfUpdate(url string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	newPath := exe + ".new"
	if err := download(url, newPath); err != nil {
		return err
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(newPath, exe); err != nil {
		os.Rename(old, exe)
		return err
	}
	// free the port first, so the new copy can take over
	if server != nil {
		server.Close()
	}
	cmd := exec.Command(exe, "--updated")
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	go func() { time.Sleep(300 * time.Millisecond); shutdown() }()
	return nil
}

func cleanupOldExe() {
	if exe, err := os.Executable(); err == nil {
		for i := 0; i < 10; i++ {
			if os.Remove(exe+".old") == nil || !fileExists(exe+".old") {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func atoiSafe(s string) int { n, _ := strconv.Atoi(s); return n }
