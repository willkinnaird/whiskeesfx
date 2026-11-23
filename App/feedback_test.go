package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFeedbackSends(t *testing.T) {
	var got map[string]any
	var file string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(1 << 20)
		json.Unmarshal([]byte(r.FormValue("payload_json")), &got)
		f, _, _ := r.FormFile("files[0]")
		b, _ := io.ReadAll(f)
		file = string(b)
		w.WriteHeader(204)
	}))
	defer srv.Close()
	a := &App{title: "FenixA320 CFM"}
	a.cfg.FeedbackURL = srv.URL
	a.active = &Profile{Name: "Fenix A320 CFM sharklets", Official: true, Type: "Fenix A3xx", Version: "0.1"}
	if err := a.sendFeedback("suggestion", "more spray", "whiskee#1"); err != nil {
		t.Fatal(err)
	}
	e := got["embeds"].([]any)[0].(map[string]any)
	if e["title"] != "💡 Suggestion" || e["description"] != "more spray" {
		t.Fatalf("embed %v", e)
	}
	js, _ := json.Marshal(e["fields"])
	for _, w := range []string{"FenixA320 CFM", "Whiskee official", "whiskee#1"} {
		if !strings.Contains(string(js), w) {
			t.Errorf("fields missing %q: %s", w, js)
		}
	}
	if !strings.Contains(file, "aircraft: FenixA320 CFM") {
		t.Errorf("details file: %q", file)
	}
	if err := a.sendFeedback("bug", "again", ""); err == nil || !strings.Contains(err.Error(), "wait") {
		t.Errorf("want rate limit, got %v", err)
	}
	fbLast = fbLast.AddDate(0, 0, -1)
	if err := (&App{}).sendFeedback("bug", "x", ""); err == nil {
		t.Error("want error with no webhook")
	}
	if err := a.sendFeedback("bug", "  ", ""); err == nil {
		t.Error("want error for empty message")
	}
}
