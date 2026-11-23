package main

// In-app feedback: posts a short report to a private Discord channel through a webhook, with the
// user's setup attached so a report says exactly which aircraft, profile and versions it came from.
// The webhook URL is added at release time (-X main.feedbackURL=..., from the FEEDBACK_WEBHOOK secret);
// a developer can override it in config (feedbackUrl) to test.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var feedbackURL = "" // set at build time

var fbMu sync.Mutex
var fbLast time.Time

func (a *App) feedbackTarget() string {
	if a.cfg.FeedbackURL != "" {
		return a.cfg.FeedbackURL
	}
	return feedbackURL
}

var fbKinds = map[string]struct {
	label string
	color int
}{
	"bug":        {"🐞 Bug", 0xE5735B},
	"suggestion": {"💡 Suggestion", 0xE0B45A},
	"praise":     {"👍 Praise", 0x8CC4A6},
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// feedbackReport builds the Discord payload and the attached details file (pure, for tests).
func (a *App) feedbackReport(kind, msg, contact string) (map[string]any, string) {
	k, ok := fbKinds[kind]
	if !ok {
		k = fbKinds["bug"]
	}
	prof, ptype, pver := "none", "", ""
	if a.active != nil {
		prof, ptype, pver = a.active.Name, a.active.Type, a.active.Version
		if a.active.Official {
			prof += " (Whiskee official)"
		} else if a.active.User {
			prof += " (user copy)"
		}
	}
	aircraft := a.title
	if aircraft == "" {
		aircraft = "none loaded"
	}
	simState := "not running"
	if a.status.Fresh {
		simState = fmt.Sprintf("running · %s · IAS %.0f kt · N1 %.0f%% · water %.2f", map[bool]string{true: "on ground", false: "airborne"}[a.status.OnGround], a.status.IAS, a.status.N1, a.status.Water)
	}
	field := func(n, v string, in bool) map[string]any {
		return map[string]any{"name": n, "value": clip(v, 1000), "inline": in}
	}
	fields := []map[string]any{
		field("App", version, true),
		field("Packages", fmt.Sprintf("module %s · effects %s", orDash(a.pkgMod), orDash(a.pkgFx)), true),
		field("Aircraft", aircraft, false),
		field("Profile", strings.TrimSpace(prof+" "+ptype+" "+pver), false),
		field("Sim", simState, false),
		field("Tweaks", fmt.Sprint(len(a.cfg.Tweaks)), true),
	}
	if c := clip(contact, 100); c != "" {
		fields = append(fields, field("Contact", c, true))
	}
	payload := map[string]any{
		"username":         "WhiskeesFX feedback",
		"allowed_mentions": map[string]any{"parse": []string{}},
		"embeds": []map[string]any{{
			"title":       k.label,
			"description": clip(msg, 3500),
			"color":       k.color,
			"fields":      fields,
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		}},
	}
	var d strings.Builder
	fmt.Fprintf(&d, "WhiskeesFX %s\naircraft: %s\nprofile: %s\n\n--- tweaks ---\n", version, aircraft, prof)
	for _, t := range a.cfg.Tweaks {
		b, _ := json.Marshal(t)
		d.Write(b)
		d.WriteByte('\n')
	}
	if b, err := os.ReadFile(a.statusPath()); err == nil {
		d.WriteString("\n--- status.txt ---\n")
		d.WriteString(clip(string(b), 60000))
	}
	return payload, d.String()
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

func (a *App) sendFeedback(kind, msg, contact string) error {
	if strings.TrimSpace(msg) == "" {
		return fmt.Errorf("write a message first")
	}
	fbMu.Lock()
	if time.Since(fbLast) < 30*time.Second {
		fbMu.Unlock()
		return fmt.Errorf("thanks! please wait a few seconds before sending another")
	}
	fbMu.Unlock()
	a.mu.Lock()
	url := a.feedbackTarget()
	payload, details := a.feedbackReport(kind, msg, contact)
	a.mu.Unlock()
	if url == "" {
		return fmt.Errorf("Feedback isn't set up in this build; please comment on the flightsim.to page instead")
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	pj, _ := json.Marshal(payload)
	mw.WriteField("payload_json", string(pj))
	fw, _ := mw.CreateFormFile("files[0]", "details.txt")
	io.WriteString(fw, details)
	mw.Close()
	req, _ := http.NewRequest("POST", url, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("User-Agent", "WhiskeesFX/"+version)
	r, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("couldn't send (are you online?)")
	}
	defer r.Body.Close()
	if r.StatusCode >= 300 {
		return fmt.Errorf("couldn't send (error %d); please try again later", r.StatusCode)
	}
	fbMu.Lock()
	fbLast = time.Now()
	fbMu.Unlock()
	return nil
}
