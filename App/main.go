package main

// WhiskeesFX: a small tray app for the VisualFX module.
//   - one background program (no window of its own, no graphics): a local web server on 127.0.0.1
//   - the settings window is opened on demand and fully closed when hidden, so nothing draws while you fly
//   - every 2 s it checks file times; it only reads a file when it has changed

import (
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const port = "47821"

var (
	url        = "http://127.0.0.1:" + port + "/"
	server     *http.Server
	shutOnce   sync.Once
	stopTicker = make(chan struct{})
	hiding     bool
)

func main() {
	// keep the background program small: collect garbage early and cap the heap
	debug.SetGCPercent(50)
	debug.SetMemoryLimit(48 << 20)
	tray, updated := false, false
	for _, a := range os.Args[1:] {
		if a == "--tray" {
			tray = true
		}
		if a == "--updated" {
			updated = true
			go cleanupOldExe()
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	for i := 0; err != nil && updated && i < 50; i++ { // the old copy is still letting go of the port
		time.Sleep(200 * time.Millisecond)
		ln, err = net.Listen("tcp", "127.0.0.1:"+port)
	}
	if err != nil {
		// already running: ask it to show its window, then leave
		c := &http.Client{Timeout: 2 * time.Second}
		if resp, err := c.Post(url+"api/show", "application/json", strings.NewReader("{}")); err == nil {
			resp.Body.Close()
		} else {
			openWindow(url)
		}
		return
	}
	app.load()
	app.tick()

	mux := routes()
	mux.HandleFunc("/api/show", func(w http.ResponseWriter, r *http.Request) { openWindow(url); w.Write([]byte("{}")) })
	server = &http.Server{Handler: guard(mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go server.Serve(ln)

	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		n := 0
		for {
			select {
			case <-stopTicker:
				return
			case <-t.C:
				app.tick()
				n++
				if n%5 == 0 { // every 10 s: put waiting packages in once the sim has closed
					app.mu.Lock()
					pending := app.cfg.PendingPkgs != ""
					app.mu.Unlock()
					if pending && !simRunning() {
						app.installPendingPackages(setProgress)
					}
				}
			}
		}
	}()

	go func() { // update check: at start and every 6 hours
		time.Sleep(5 * time.Second)
		for {
			app.mu.Lock()
			auto := app.cfg.AutoUpdate
			app.mu.Unlock()
			if auto {
				app.checkUpdate()
			}
			select {
			case <-stopTicker:
				return
			case <-time.After(6 * time.Hour):
			}
		}
	}()

	onWindowClosed = func() {
		app.mu.Lock()
		keep := app.cfg.KeepInTray
		app.mu.Unlock()
		if !keep && !hiding {
			shutdown()
		}
		hiding = false
	}
	if !tray {
		openWindow(url)
	}
	runTray(TrayMenu{
		Open:     func() { openWindow(url) },
		Basic:    func() { setView("basic"); openWindow(url) },
		Advanced: func() { setView("advanced"); openWindow(url) },
		Pause: func() {
			app.mu.Lock()
			g := app.cfg.Groups["*"]
			if g == nil {
				g = &GroupSetting{Strength: 1}
				app.cfg.Groups["*"] = g
			}
			g.Off = !g.Off
			app.saveConfig()
			app.composeLocked()
			app.mu.Unlock()
		},
		Paused: func() bool { app.mu.Lock(); defer app.mu.Unlock(); g := app.cfg.Groups["*"]; return g != nil && g.Off },
		Quit:   shutdown,
	})
	shutdown()
}

func setView(v string) {
	app.mu.Lock()
	app.cfg.View = v
	app.saveConfig()
	app.mu.Unlock()
}

func shutdown() {
	shutOnce.Do(func() {
		close(stopTicker)
		closeWindow()
		if server != nil {
			server.Close()
		}
		quitTray()
		go func() { time.Sleep(2 * time.Second); os.Exit(0) }()
	})
}
