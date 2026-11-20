//go:build windows

package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

//go:embed ui.html
var uiHTML []byte

const addr = "127.0.0.1:47820"

func logf(f string, a ...any) { log.Printf(f, a...) }

type app struct {
	mu        sync.Mutex
	settings  Settings
	tel       Telemetry
	lastData  time.Time
	connected bool
	simErr    string
	pose      Pose
	tracking  TrackingStatus
	engine    *Engine
	exeDir    string
	quit      chan struct{}

	head        HeadPose
	lastHead    time.Time
	headErr     string
	hotkeyErr   string
	joyBinding  string
	mixer       HeadMixer
	headDirect  bool
	tobiiOK     bool
	tobiiStatus string
	recenterReq bool
}

func main() {
	log.SetFlags(log.Ltime)
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)

	ln, err := net.Listen("tcp", addr)
	if err != nil { // already running: just show the existing window
		openBrowser("http://" + addr)
		return
	}

	fmt.Println("WhiskeesFX CameraFX - cockpit motion and head tracking for MSFS 2024")
	fmt.Println("Settings page: http://" + addr)
	fmt.Println("Close this window to stop.")

	a := &app{settings: LoadSettings(), engine: NewEngine(time.Now().UnixNano()), exeDir: exeDir, quit: make(chan struct{})}
	a.tracking = CheckTracking(exeDir, false)
	logf("Head tracking: %s", a.tracking.Message)

	ft, err := OpenFreeTrack()
	if err != nil {
		logf("FreeTrack shared memory unavailable: %v", err)
	}

	go a.simLoop()
	go a.effectLoop(ft)
	go a.headUDPLoop()
	go a.tobiiLoop()
	go a.hotkeyLoop()
	go a.joystickLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(uiHTML)
	})
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/settings", a.handleSettings)
	mux.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		if r.URL.Query().Get("kind") == "lurch" {
			a.engine.TestLurch()
		} else {
			a.engine.TestBump()
		}
		a.mu.Unlock()
	})
	mux.HandleFunc("/api/togglehead", func(w http.ResponseWriter, r *http.Request) { a.toggleHead() })
	mux.HandleFunc("/api/recenter", func(w http.ResponseWriter, r *http.Request) { a.recenterHead() })
	mux.HandleFunc("/api/bindjoy", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		target := r.URL.Query().Get("target") // "toggle" or "recenter"
		if target != "recenter" {
			target = "toggle"
		}
		if r.URL.Query().Get("clear") == "1" {
			if target == "recenter" {
				a.settings.JoyRecDevice, a.settings.JoyRecButton = -1, -1
			} else {
				a.settings.JoyDevice, a.settings.JoyButton = -1, -1
			}
			s := a.settings
			a.mu.Unlock()
			SaveSettings(s)
			return
		}
		a.joyBinding = target
		a.mu.Unlock()
	})
	mux.HandleFunc("/api/fixtracking", func(w http.ResponseWriter, r *http.Request) {
		st := CheckTracking(exeDir, true)
		a.mu.Lock()
		a.tracking = st
		a.mu.Unlock()
		json.NewEncoder(w).Encode(st)
	})
	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("bye"))
		go func() { time.Sleep(200 * time.Millisecond); close(a.quit) }()
	})
	go http.Serve(ln, mux)
	openBrowser("http://" + addr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-a.quit:
	}
	if ft != nil {
		for i := 0; i < 5; i++ { // hand the camera back cleanly
			ft.Write(Pose{})
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func openBrowser(url string) {
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func (a *app) simLoop() {
	sc, err := LoadSimConnect(filepath.Join(a.exeDir, "SimConnect.dll"))
	if err != nil {
		a.mu.Lock()
		a.simErr = err.Error()
		a.mu.Unlock()
		logf("%v", err)
		return
	}
	for {
		if err := sc.Connect("WhiskeesFX CameraFX"); err != nil {
			a.mu.Lock()
			a.connected, a.simErr = false, "Waiting for MSFS..."
			a.mu.Unlock()
			time.Sleep(3 * time.Second)
			continue
		}
		logf("Connected to MSFS")
		a.mu.Lock()
		a.connected, a.simErr, a.lastData = true, "", time.Now()
		a.mu.Unlock()
		for {
			quit := sc.Poll(func(t Telemetry) {
				a.mu.Lock()
				a.tel, a.lastData = t, time.Now()
				a.mu.Unlock()
			})
			a.mu.Lock()
			silent := time.Since(a.lastData) > 20*time.Second
			a.mu.Unlock()
			if quit || silent {
				break
			}
			time.Sleep(4 * time.Millisecond)
		}
		logf("Disconnected from MSFS")
		sc.Close()
		a.mu.Lock()
		a.connected = false
		a.mu.Unlock()
		time.Sleep(2 * time.Second)
	}
}

func (a *app) effectLoop(ft *FreeTrack) {
	tick := time.NewTicker(time.Second / 90)
	last := time.Now()
	for now := range tick.C {
		dt := now.Sub(last).Seconds()
		last = now
		a.mu.Lock()
		fresh := a.connected && time.Since(a.lastData) < 250*time.Millisecond
		p := a.engine.Update(a.tel, fresh, dt, a.settings)
		headLive := time.Since(a.lastHead) < 500*time.Millisecond
		rg, mg := a.settings.HeadRotGain, a.settings.HeadMovGain
		if a.headDirect { // direct Tobii is already scaled by the MSFS-style Tobii settings
			rg, mg = 1, 1
		}
		hp := a.mixer.Update(a.head.toFreeTrack(rg, mg), headLive, a.settings.HeadEnabled, dt)
		a.pose = p
		p = addPose(p, hp)
		a.mu.Unlock()
		if ft != nil {
			ft.Write(p)
		}
	}
}

func (a *app) handleState(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	fresh := a.connected && time.Since(a.lastData) < 250*time.Millisecond
	json.NewEncoder(w).Encode(map[string]any{
		"settings": a.settings, "telemetry": a.tel, "pose": a.pose,
		"connected": a.connected, "fresh": fresh, "simError": a.simErr,
		"tracking": a.tracking, "surface": a.engine.Surface, "active": a.engine.Active,
		"headLive": time.Since(a.lastHead) < 500*time.Millisecond, "head": a.head, "headErr": a.headErr,
		"hotkeyErr": a.hotkeyErr, "joyBinding": a.joyBinding,
		"tobiiOK": a.tobiiOK, "tobiiStatus": a.tobiiStatus, "headDirect": a.headDirect,
	})
}

func (a *app) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var s Settings
		if r.URL.Query().Get("reset") == "1" {
			s = DefaultSettings()
		} else if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		a.mu.Lock()
		// These are changed by the hotkey/joystick/toggle endpoints; never let a stale page overwrite them.
		s.HeadEnabled, s.JoyDevice, s.JoyButton = a.settings.HeadEnabled, a.settings.JoyDevice, a.settings.JoyButton
		s.JoyRecDevice, s.JoyRecButton = a.settings.JoyRecDevice, a.settings.JoyRecButton
		s.sanitize()
		a.settings = s
		a.mu.Unlock()
		if err := SaveSettings(s); err != nil {
			logf("saving settings: %v", err)
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	json.NewEncoder(w).Encode(a.settings)
}
