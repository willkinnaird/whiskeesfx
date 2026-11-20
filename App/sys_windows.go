//go:build windows

package main

// Windows-only pieces: hidden child processes, the app window (Edge in app mode), and the tray icon,
// written straight against user32/shell32 (no extra libraries, no cgo).

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const createNoWindow = 0x08000000

func hiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return c
}

func simRunning() bool {
	out, err := hiddenCmd("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	s := string(out)
	return contains(s, "FlightSimulator2024.exe") || contains(s, "\"FlightSimulator.exe\"")
}

// ---- the app window ----------------------------------------------------------------------------

var onWindowClosed func()

var (
	winMu  sync.Mutex
	winCmd *exec.Cmd
)

func edgePath() string {
	for _, p := range []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Microsoft\Edge\Application\msedge.exe`),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// openWindow shows the UI as its own window. Its own Edge profile keeps it a separate, light process that
// exits completely (freeing all its memory and graphics memory) when the window is closed.
func openWindow(url string) {
	winMu.Lock()
	defer winMu.Unlock()
	if winCmd != nil {
		// already open: bring that window forward instead of opening a second one
		if h := findAppWindow(); h != 0 {
			if r, _, _ := pIsIconic.Call(h); r != 0 {
				pShowWindow.Call(h, 9) // SW_RESTORE
			}
			pSetForegroundWindow.Call(h)
		}
		return
	}
	if e := edgePath(); e != "" {
		c := hiddenCmd(e,
			"--app="+url,
			"--user-data-dir="+uiProfileDir(),
			"--window-size=1280,860",
			"--no-first-run", "--no-default-browser-check",
			"--disable-gpu", // drawn by the CPU: the window never takes any graphics memory from the sim
			"--hide-crash-restore-bubble",
			"--disable-extensions", "--disable-sync", "--disable-background-networking",
			"--disable-component-update", "--disable-features=msEdgeSidebarV2,msHubApps,EdgeCollections,Translate",
		)
		if c.Start() == nil {
			winCmd = c
			go func(c *exec.Cmd) {
				c.Wait()
				winMu.Lock()
				if winCmd == c {
					winCmd = nil
				}
				winMu.Unlock()
				if onWindowClosed != nil {
					onWindowClosed()
				}
			}(c)
			return
		}
	}
	startDetached(hiddenCmd("rundll32", "url.dll,FileProtocolHandler", url))
}

// startDetached starts a helper and waits for it in the background so its process handle is always released.
func startDetached(c *exec.Cmd) {
	if c.Start() == nil {
		go c.Wait()
	}
}

// findAppWindow: the Edge app window shows the page title "WhiskeesFX".
func findAppWindow() uintptr {
	h, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("Chrome_WidgetWin_1"))), uintptr(unsafe.Pointer(u16("WhiskeesFX"))))
	return h
}

// closeWindow shuts the UI window: politely first, then the whole Edge process tree if it lingers.
// Either way every bit of its memory goes back; the app keeps running in the tray.
func closeWindow() {
	winMu.Lock()
	c := winCmd
	winMu.Unlock()
	if c == nil || c.Process == nil {
		return
	}
	if h := findAppWindow(); h != 0 {
		pPostMessageW.Call(h, wmClose, 0, 0)
		for i := 0; i < 20; i++ {
			time.Sleep(100 * time.Millisecond)
			winMu.Lock()
			gone := winCmd != c
			winMu.Unlock()
			if gone {
				return
			}
		}
	}
	hiddenCmd("taskkill", "/PID", itoa(c.Process.Pid), "/T", "/F").Run()
}

func uiProfileDir() string { return filepath.Join(localDir(), "window") }

func setStartWithWindows(on bool, exe string) error {
	key := `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	if on {
		return hiddenCmd("reg", "add", key, "/v", "WhiskeesFX", "/t", "REG_SZ", "/d", `"`+exe+`" --tray`, "/f").Run()
	}
	hiddenCmd("reg", "delete", key, "/v", "WhiskeesFX", "/f").Run()
	return nil
}

func queryStartsWithWindows() bool {
	return hiddenCmd("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "WhiskeesFX").Run() == nil
}

// ---- tray icon ---------------------------------------------------------------------------------

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassExW       = user32.NewProc("RegisterClassExW")
	pCreateWindowExW        = user32.NewProc("CreateWindowExW")
	pDefWindowProcW         = user32.NewProc("DefWindowProcW")
	pGetMessageW            = user32.NewProc("GetMessageW")
	pTranslateMessage       = user32.NewProc("TranslateMessage")
	pDispatchMessageW       = user32.NewProc("DispatchMessageW")
	pPostQuitMessage        = user32.NewProc("PostQuitMessage")
	pPostMessageW           = user32.NewProc("PostMessageW")
	pCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	pAppendMenuW            = user32.NewProc("AppendMenuW")
	pTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	pDestroyMenu            = user32.NewProc("DestroyMenu")
	pSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	pGetCursorPos           = user32.NewProc("GetCursorPos")
	pCreateIcon             = user32.NewProc("CreateIcon")
	pDestroyIcon            = user32.NewProc("DestroyIcon")
	pDestroyWindow          = user32.NewProc("DestroyWindow")
	pRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")
	pShellNotifyIconW       = shell32.NewProc("Shell_NotifyIconW")
	pFindWindowW            = user32.NewProc("FindWindowW")
	pShowWindow             = user32.NewProc("ShowWindow")
	pIsIconic               = user32.NewProc("IsIconic")
	pLoadImageW             = user32.NewProc("LoadImageW")
	pGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	pGetModuleHandleW       = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmCommand     = 0x0111
	wmApp         = 0x8000
	wmTray        = wmApp + 1
	wmLButtonUp   = 0x0202
	wmLButtonDbl  = 0x0203
	wmRButtonUp   = 0x0205
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nifMessage    = 1
	nifIcon       = 2
	nifTip        = 4
	mfString      = 0
	mfSeparator   = 0x800
	mfChecked     = 0x8
	tpmRightAlign = 0x8
	tpmBottomAlgn = 0x20
	tpmReturnCmd  = 0x100
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

type point struct{ x, y int32 }
type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	priv    uint32
}

type TrayMenu struct {
	Open, Basic, Advanced, Pause, Quit func()
	Paused                             func() bool
}

var (
	trayHwnd    uintptr
	trayIcon    uintptr
	trayNid     notifyIconData
	trayMenu    TrayMenu
	taskbarMsg  uintptr
	trayStarted bool
)

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func makeIcon(hInst uintptr) uintptr {
	// 32x32 BGRA: amber disc with a dark spray arc. The AND mask is all zeros (alpha does the shape).
	const n = 32
	xor := make([]byte, n*n*4)
	and := make([]byte, n*n/8)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := float64(x)-15.5, float64(y)-15.5
			d := dx*dx + dy*dy
			i := (y*n + x) * 4
			if d <= 15.5*15.5 {
				b, g, r := byte(0x3B), byte(0xA9), byte(0xF2)
				// arc: a ring segment in the lower half
				if d > 6.5*6.5 && d < 9.5*9.5 && dy > -2 {
					b, g, r = 0x16, 0x11, 0x0E
				}
				xor[i], xor[i+1], xor[i+2], xor[i+3] = b, g, r, 0xFF
			}
		}
	}
	h, _, _ := pCreateIcon.Call(hInst, n, n, 1, 32, uintptr(unsafe.Pointer(&and[0])), uintptr(unsafe.Pointer(&xor[0])))
	return h
}

// loadAppIcon takes the icon built into the exe (resource 1) at the tray's size for this screen's scaling.
func loadAppIcon(hInst uintptr) uintptr {
	cx, _, _ := pGetSystemMetrics.Call(49) // SM_CXSMICON
	cy, _, _ := pGetSystemMetrics.Call(50) // SM_CYSMICON
	h, _, _ := pLoadImageW.Call(hInst, 1, 1 /* IMAGE_ICON */, cx, cy, 0)
	return h
}

func trayAdd() {
	trayNid = notifyIconData{}
	trayNid.cbSize = uint32(unsafe.Sizeof(trayNid))
	trayNid.hWnd = trayHwnd
	trayNid.uID = 1
	trayNid.uFlags = nifMessage | nifIcon | nifTip
	trayNid.uCallbackMessage = wmTray
	trayNid.hIcon = trayIcon
	copy(trayNid.szTip[:], syscall.StringToUTF16("WhiskeesFX"))
	pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&trayNid)))
}

var lastTip string

func setTrayTip(s string) {
	if !trayStarted || s == lastTip {
		return
	}
	lastTip = s
	t := syscall.StringToUTF16(s)
	if len(t) > 127 {
		t = append(t[:126], 0)
	}
	var tip [128]uint16
	copy(tip[:], t)
	trayNid.szTip = tip
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&trayNid)))
}

func showTrayMenu() {
	m, _, _ := pCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	defer pDestroyMenu.Call(m) // menus are destroyed every time: no handle leak
	pAppendMenuW.Call(m, mfString, 1, uintptr(unsafe.Pointer(u16("Open WhiskeesFX"))))
	pAppendMenuW.Call(m, mfString, 2, uintptr(unsafe.Pointer(u16("Basic settings"))))
	pAppendMenuW.Call(m, mfString, 3, uintptr(unsafe.Pointer(u16("Advanced settings"))))
	pAppendMenuW.Call(m, mfSeparator, 0, 0)
	flags := uintptr(mfString)
	if trayMenu.Paused != nil && trayMenu.Paused() {
		flags |= mfChecked
	}
	pAppendMenuW.Call(m, flags, 4, uintptr(unsafe.Pointer(u16("Pause all effects"))))
	pAppendMenuW.Call(m, mfSeparator, 0, 0)
	pAppendMenuW.Call(m, mfString, 5, uintptr(unsafe.Pointer(u16("Quit"))))
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(trayHwnd)
	cmd, _, _ := pTrackPopupMenu.Call(m, tpmRightAlign|tpmBottomAlgn|tpmReturnCmd, uintptr(pt.x), uintptr(pt.y), 0, trayHwnd, 0)
	call := func(f func()) {
		if f != nil {
			go f()
		}
	}
	switch cmd {
	case 1:
		call(trayMenu.Open)
	case 2:
		call(trayMenu.Basic)
	case 3:
		call(trayMenu.Advanced)
	case 4:
		call(trayMenu.Pause)
	case 5:
		call(trayMenu.Quit)
	}
}

func wndProc(hwnd, m, wp, lp uintptr) uintptr {
	switch {
	case m == wmTray:
		switch lp & 0xFFFF {
		case wmLButtonUp, wmLButtonDbl:
			if trayMenu.Open != nil {
				go trayMenu.Open()
			}
		case wmRButtonUp:
			showTrayMenu()
		}
		return 0
	case taskbarMsg != 0 && m == taskbarMsg:
		trayAdd() // Explorer restarted: put the icon back
		return 0
	case m == wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	case m == wmDestroy:
		pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&trayNid)))
		if trayIcon != 0 {
			pDestroyIcon.Call(trayIcon)
			trayIcon = 0
		}
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// runTray blocks on the Windows message loop; call it from main. It returns after quitTray.
func runTray(menu TrayMenu) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	trayMenu = menu
	hInst, _, _ := pGetModuleHandleW.Call(0)
	cls := u16("WhiskeesFXTray")
	wc := wndClassEx{lpfnWndProc: syscall.NewCallback(wndProc), hInstance: hInst, lpszClassName: cls}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	trayHwnd, _, _ = pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(u16("WhiskeesFX tray"))), 0, 0, 0, 0, 0, 0, 0, hInst, 0)
	if trayHwnd == 0 {
		select {} // no tray possible: keep serving
	}
	taskbarMsg, _, _ = pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))
	trayIcon = loadAppIcon(hInst)
	if trayIcon == 0 {
		trayIcon = makeIcon(hInst)
	}
	trayAdd()
	trayStarted = true
	var mm msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&mm)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&mm)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&mm)))
	}
}

func quitTray() {
	if trayHwnd != 0 {
		pPostMessageW.Call(trayHwnd, wmClose, 0, 0)
	} else {
		os.Exit(0)
	}
}
