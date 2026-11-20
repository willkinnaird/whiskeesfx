package main

// Reads the module's status.txt (rewritten every second while the sim runs).

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type EffectStatus struct {
	State     string `json:"state"` // playing, idle, off, failed
	Instances int    `json:"instances"`
	Playing   int    `json:"playing"`
}

type SimStatus struct {
	Fresh    bool                    `json:"fresh"` // written in the last 5 s: the sim is running the module
	Age      float64                 `json:"age"`
	Title    string                  `json:"title"`
	Wet      bool                    `json:"wet"`
	Water    float64                 `json:"water"`
	Humid    bool                    `json:"humid"`
	Raining  bool                    `json:"raining"`
	OnGround bool                    `json:"onGround"`
	AGL      float64                 `json:"agl"`
	GS       float64                 `json:"gs"`
	IAS      float64                 `json:"ias"`
	N1       float64                 `json:"n1"`
	Reverser float64                 `json:"reverser"`
	Effects  map[string]EffectStatus `json:"effects"`
	Events   []string                `json:"events"`
	Vars     []string                `json:"vars"`
}

var reNum = func(key string) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(key) + `\s*=?\s*(-?[0-9.]+)`)
}

var (
	reWet      = reNum("wet=")
	reWater    = reNum("runway water=")
	reHumid    = reNum("humid=")
	reRaining  = reNum("raining=")
	reGround   = reNum("on ground=")
	reAGL      = reNum("agl=")
	reGS       = reNum("gs=")
	reIAS      = reNum("ias=")
	reN1       = regexp.MustCompile(`(?m)^N1 (-?[0-9.]+)`)
	reRev      = regexp.MustCompile(`reverser (-?[0-9.]+)`)
	reEffLine  = regexp.MustCompile(`^\s{2}(\S+)\s+at=\S+\s*(.*)$`)
	reInstance = regexp.MustCompile(`\[\d+ (\w+)`)
)

func numOf(re *regexp.Regexp, s string) float64 {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return v
}

func readStatus(path string) SimStatus {
	st := SimStatus{Effects: map[string]EffectStatus{}}
	fi, err := os.Stat(path)
	if err != nil {
		return st
	}
	st.Age = time.Since(fi.ModTime()).Seconds()
	st.Fresh = st.Age < 5
	b, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	section := "head"
	var head []string
	for _, l := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(l, "effects:"):
			section = "effects"
			continue
		case strings.HasPrefix(l, "recent events"):
			section = "events"
			continue
		}
		switch section {
		case "head":
			head = append(head, l)
			if strings.HasPrefix(l, "aircraft title:") {
				st.Title = strings.TrimSpace(strings.TrimPrefix(l, "aircraft title:"))
			}
			if strings.HasPrefix(l, "v") && strings.Contains(l, " = ") {
				st.Vars = append(st.Vars, strings.TrimSpace(l))
			}
		case "effects":
			m := reEffLine.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			es := EffectStatus{State: "idle"}
			rest := m[2]
			if strings.Contains(rest, "switched off") {
				es.State = "off"
			}
			for _, im := range reInstance.FindAllStringSubmatch(rest, -1) {
				es.Instances++
				if im[1] == "PLAYING" {
					es.Playing++
				}
				if im[1] == "FAILED" && es.State != "playing" {
					es.State = "failed"
				}
			}
			if es.Playing > 0 {
				es.State = "playing"
			}
			st.Effects[m[1]] = es
		case "events":
			if s := strings.TrimSpace(l); s != "" {
				st.Events = append(st.Events, s)
			}
		}
	}
	h := strings.Join(head, "\n")
	st.Wet = numOf(reWet, h) > 0
	st.Water = numOf(reWater, h)
	st.Humid = numOf(reHumid, h) > 0
	st.Raining = numOf(reRaining, h) > 0
	st.OnGround = numOf(reGround, h) > 0
	st.AGL = numOf(reAGL, h)
	st.GS = numOf(reGS, h)
	st.IAS = numOf(reIAS, h)
	st.N1 = numOf(reN1, h)
	st.Reverser = numOf(reRev, h)
	if len(st.Events) > 40 {
		st.Events = st.Events[len(st.Events)-40:]
	}
	return st
}
