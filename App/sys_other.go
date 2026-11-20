//go:build !windows

package main

// Stand-ins so the app can be built and tested on Linux (no tray, no Edge).

import (
	"fmt"
	"os"
	"os/exec"
)

func hiddenCmd(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }
func simRunning() bool                                { return os.Getenv("WFX_SIM_RUNNING") == "1" }
func openWindow(url string)                           { fmt.Println("open window:", url) }
func closeWindow()                                    {}
func setStartWithWindows(on bool, exe string) error   { return nil }
func queryStartsWithWindows() bool                    { return false }
func setTrayTip(s string)                             {}

var onWindowClosed func()

type TrayMenu struct {
	Open, Basic, Advanced, Pause, Quit func()
	Paused                             func() bool
}

var quitCh = make(chan struct{})

func runTray(menu TrayMenu) { <-quitCh }
func quitTray()             { close(quitCh) }
