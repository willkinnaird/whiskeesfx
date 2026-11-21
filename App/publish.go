package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var reVersion = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)

// bakeOfficial turns a profile plus Whiskee's own edits to it (point moves and profile-only tweaks) into the
// official profile file that ships with the next app update. Caller holds a.mu.
func (a *App) bakeOfficial(p *Profile, version, note string) (id, file string, err error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if !reVersion.MatchString(version) {
		return "", "", fmt.Errorf("the version should look like 0.1 or 1.2.3")
	}
	if p.Generic {
		return "", "", fmt.Errorf("the generic fallback can't be an official profile")
	}
	f := p.ini.Clone()
	set := func(sec, k, v string) {
		if !f.Set(sec, k, v) { // a section the profile doesn't have yet
			f.Lines = append(f.Lines, "", "["+sec+"]")
			f.Set(sec, k, v)
		}
	}
	for _, t := range a.cfg.Tweaks {
		if t.Scope != p.ID {
			continue
		}
		switch t.Section {
		case "@points":
			set("points", t.Key, t.Value)
		case "":
			set("globals", t.Key, t.Value)
		default:
			set(t.Section, t.Key, t.Value)
		}
	}
	for f.Delete("profile", "based_on") {
	}
	f.Set("profile", "official", "1")
	f.Set("profile", "version", version)
	if note = strings.TrimSpace(note); note != "" {
		f.Set("profile", "changes", "v"+version+": "+note)
	}
	id = strings.TrimPrefix(p.ID, "my-")
	return id, strings.TrimRight(f.String(), "\n") + "\n", nil
}

// publishToGitHub writes one file to the repo's main branch through the GitHub contents API.
func publishToGitHub(token, repo, path, content, message string) error {
	cl := &http.Client{Timeout: 25 * time.Second}
	api := "https://api.github.com/repos/" + repo + "/contents/" + path
	do := func(method, url string, body []byte) (*http.Response, error) {
		req, _ := http.NewRequest(method, url, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "WhiskeesFX/"+version)
		return cl.Do(req)
	}
	sha := ""
	if r, err := do("GET", api+"?ref=main", nil); err == nil {
		if r.StatusCode == 200 {
			var cur struct {
				SHA string `json:"sha"`
			}
			json.NewDecoder(r.Body).Decode(&cur)
			sha = cur.SHA
		}
		r.Body.Close()
	} else {
		return fmt.Errorf("couldn't reach GitHub: %v", err)
	}
	body := map[string]string{"message": message, "content": base64.StdEncoding.EncodeToString([]byte(content)), "branch": "main"}
	if sha != "" {
		body["sha"] = sha
	}
	b, _ := json.Marshal(body)
	r, err := do("PUT", api, b)
	if err != nil {
		return fmt.Errorf("couldn't reach GitHub: %v", err)
	}
	defer r.Body.Close()
	switch {
	case r.StatusCode == 200 || r.StatusCode == 201:
		return nil
	case r.StatusCode == 401 || r.StatusCode == 403:
		return fmt.Errorf("GitHub refused the token (it needs Contents: read and write on %s)", repo)
	default:
		var e struct {
			Message string `json:"message"`
		}
		json.NewDecoder(r.Body).Decode(&e)
		return fmt.Errorf("GitHub said %d: %s", r.StatusCode, e.Message)
	}
}
