//go:build windows

package main

import (
	"fmt"
	"net"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// ---- Tobii Game Hub / opentrack UDP input

func (a *app) headUDPLoop() {
	port := 0
	var conn *net.UDPConn
	buf := make([]byte, 512)
	for {
		a.mu.Lock()
		want := a.settings.HeadPort
		a.mu.Unlock()
		if conn == nil || want != port {
			if conn != nil {
				conn.Close()
				conn = nil
			}
			port = want
			c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
			if err != nil {
				a.mu.Lock()
				a.headErr = fmt.Sprintf("Can't listen on port %d (is opentrack open? close it): %v", port, err)
				a.mu.Unlock()
				time.Sleep(3 * time.Second)
				conn = nil
				port = 0
				continue
			}
			conn = c
			a.mu.Lock()
			a.headErr = ""
			a.mu.Unlock()
			logf("Listening for head tracking on UDP port %d", port)
		}
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		if h, ok := parseOpentrackUDP(buf[:n]); ok {
			a.mu.Lock()
			if !a.tobiiOK { // a directly connected Tobii takes priority
				a.head, a.lastHead, a.headDirect = h, time.Now(), false
			}
			a.mu.Unlock()
		}
	}
}

func (a *app) recenterHead() {
	a.mu.Lock()
	a.recenterReq = true
	a.mu.Unlock()
	logf("Head tracking re-centred")
}

func (a *app) toggleHead() {
	a.mu.Lock()
	a.settings.HeadEnabled = !a.settings.HeadEnabled
	s := a.settings
	a.mu.Unlock()
	logf("Head tracking %s", map[bool]string{true: "ON", false: "OFF"}[s.HeadEnabled])
	SaveSettings(s)
}

// ---- Global keyboard hotkey

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procRegisterHK   = user32.NewProc("RegisterHotKey")
	procUnregisterHK = user32.NewProc("UnregisterHotKey")
	procPeekMessage  = user32.NewProc("PeekMessageW")
	winmm            = syscall.NewLazyDLL("winmm.dll")
	procJoyGetPosEx  = winmm.NewProc("joyGetPosEx")
)

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
	private uint32
}

func (a *app) hotkeyLoop() {
	runtime.LockOSThread() // hotkey messages go to the registering thread
	reg := [3]string{}     // index 1 = toggle, 2 = recenter
	var msg winMsg
	for {
		a.mu.Lock()
		want := [3]string{"", a.settings.HeadHotkey, a.settings.RecenterHotkey}
		a.mu.Unlock()
		if want != reg {
			errs := ""
			for id := 1; id <= 2; id++ {
				procUnregisterHK.Call(0, uintptr(id))
				if mods, vk, ok := parseHotkey(want[id]); ok {
					if r, _, _ := procRegisterHK.Call(0, uintptr(id), uintptr(mods|0x4000), uintptr(vk)); r == 0 {
						errs += want[id] + " is already used by another program. "
					}
				} else if want[id] != "" {
					errs += "Couldn't understand \"" + want[id] + "\" (try Ctrl+Shift+H or F9). "
				}
			}
			reg = want
			a.mu.Lock()
			a.hotkeyErr = errs
			a.mu.Unlock()
		}
		for {
			r, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1) // PM_REMOVE
			if r == 0 {
				break
			}
			if msg.message == 0x0312 { // WM_HOTKEY
				if msg.wParam == 2 {
					a.recenterHead()
				} else {
					a.toggleHead()
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ---- Joystick button (any HOTAS / stick button, via winmm)

type joyInfoEx struct {
	size, flags                            uint32
	x, y, z, r, u, v                       uint32
	buttons, buttonNumber, pov, res1, res2 uint32
}

func joyButtons(id int) (uint32, bool) {
	ji := joyInfoEx{size: uint32(unsafe.Sizeof(joyInfoEx{})), flags: 0x80} // JOY_RETURNBUTTONS
	r, _, _ := procJoyGetPosEx.Call(uintptr(id), uintptr(unsafe.Pointer(&ji)))
	return ji.buttons, r == 0
}

func (a *app) joystickLoop() {
	var prev [16]uint32
	for {
		a.mu.Lock()
		st, target := a.settings, a.joyBinding
		a.mu.Unlock()

		if target != "" {
			// Wait for any newly pressed button on any stick.
			var base [16]uint32
			var ok [16]bool
			for i := 0; i < 16; i++ {
				base[i], ok[i] = joyButtons(i)
			}
			deadline := time.Now().Add(10 * time.Second)
			found := false
			for time.Now().Before(deadline) && !found {
				time.Sleep(15 * time.Millisecond)
				for i := 0; i < 16 && !found; i++ {
					if !ok[i] {
						continue
					}
					b, _ := joyButtons(i)
					if pressed := b &^ base[i]; pressed != 0 {
						for k := 0; k < 32; k++ {
							if pressed&(1<<k) != 0 {
								a.mu.Lock()
								if target == "recenter" {
									a.settings.JoyRecDevice, a.settings.JoyRecButton = i, k
								} else {
									a.settings.JoyDevice, a.settings.JoyButton = i, k
								}
								s := a.settings
								a.mu.Unlock()
								SaveSettings(s)
								prev[i] = b
								found = true
								break
							}
						}
					}
					base[i] &= b // a button held at the start counts once it's released and pressed again
				}
			}
			a.mu.Lock()
			a.joyBinding = ""
			a.mu.Unlock()
			continue
		}

		var cur [16]uint32
		var have [16]bool
		check := func(dev, btn int, action func()) {
			if dev < 0 || dev >= 16 || btn < 0 || btn >= 32 {
				return
			}
			if !have[dev] {
				cur[dev], have[dev] = joyButtons(dev)
				if !have[dev] {
					return
				}
			}
			if cur[dev]&(1<<btn) != 0 && prev[dev]&(1<<btn) == 0 {
				action()
			}
		}
		check(st.JoyDevice, st.JoyButton, a.toggleHead)
		check(st.JoyRecDevice, st.JoyRecButton, a.recenterHead)
		for i := 0; i < 16; i++ {
			if have[i] {
				prev[i] = cur[i]
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
}
