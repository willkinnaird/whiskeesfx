//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// Layout of opentrack/FreeTrack's FTHeap ("FT_SharedMem"), read by NPClient64.dll inside MSFS.
type ftHeap struct {
	DataID              uint32
	CamWidth, CamHeight int32
	Yaw, Pitch, Roll    float32
	X, Y, Z             float32
	RawYaw, RawPitch    float32
	RawRoll             float32
	RawX, RawY, RawZ    float32
	Pts                 [8]float32
	GameID              int32
	Table               [2]int32
	GameID2             int32
}

type FreeTrack struct {
	h    syscall.Handle
	heap *ftHeap
}

func OpenFreeTrack() (*FreeTrack, error) {
	name, _ := syscall.UTF16PtrFromString("FT_SharedMem")
	size := uint32(unsafe.Sizeof(ftHeap{}))
	h, err := syscall.CreateFileMapping(syscall.InvalidHandle, nil, syscall.PAGE_READWRITE, 0, size, name)
	if h == 0 {
		return nil, fmt.Errorf("CreateFileMapping: %w", err)
	}
	addr, err := syscall.MapViewOfFile(h, syscall.FILE_MAP_WRITE, 0, 0, uintptr(size))
	if addr == 0 {
		syscall.CloseHandle(h)
		return nil, fmt.Errorf("MapViewOfFile: %w", err)
	}
	ft := &FreeTrack{h: h, heap: (*ftHeap)(unsafe.Pointer(addr))}
	ft.heap.CamWidth, ft.heap.CamHeight = 640, 480
	return ft, nil
}

func (f *FreeTrack) Write(p Pose) {
	d := f.heap
	d.X, d.Y, d.Z = float32(p.X), float32(p.Y), float32(p.Z)
	d.Yaw, d.Pitch, d.Roll = float32(p.Yaw), float32(p.Pitch), float32(p.Roll)
	d.RawX, d.RawY, d.RawZ = d.X, d.Y, d.Z
	d.RawYaw, d.RawPitch, d.RawRoll = d.Yaw, d.Pitch, d.Roll
	// Same handshake opentrack does: acknowledge the game ID with an all-zero (no encryption) table.
	if gid := atomic.LoadInt32(&d.GameID); gid != atomic.LoadInt32(&d.GameID2) {
		atomic.StoreInt32(&d.Table[0], 0)
		atomic.StoreInt32(&d.Table[1], 0)
		atomic.StoreInt32(&d.GameID2, gid)
	}
	atomic.AddUint32(&d.DataID, 1)
}

// ---- NPClient registry setup (HKCU\Software\NaturalPoint\NATURALPOINT\NPClient Location, "Path")

var (
	advapi           = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKey = advapi.NewProc("RegCreateKeyExW")
	procRegSetValue  = advapi.NewProc("RegSetValueExW")
	npKeyPath        = `Software\NaturalPoint\NATURALPOINT\NPClient Location`
)

func readNPClientPath() string {
	var k syscall.Handle
	kp, _ := syscall.UTF16PtrFromString(npKeyPath)
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, kp, 0, syscall.KEY_READ, &k) != nil {
		return ""
	}
	defer syscall.RegCloseKey(k)
	vn, _ := syscall.UTF16PtrFromString("Path")
	buf := make([]uint16, 1024)
	n := uint32(len(buf) * 2)
	var typ uint32
	if syscall.RegQueryValueEx(k, vn, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &n) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func writeNPClientPath(dir string) error {
	if !strings.HasSuffix(dir, `\`) {
		dir += `\`
	}
	var k syscall.Handle
	kp, _ := syscall.UTF16PtrFromString(npKeyPath)
	r, _, _ := procRegCreateKey.Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(kp)), 0, 0, 0,
		uintptr(syscall.KEY_WRITE), 0, uintptr(unsafe.Pointer(&k)), 0)
	if r != 0 {
		return fmt.Errorf("RegCreateKeyEx error %d", r)
	}
	defer syscall.RegCloseKey(k)
	vn, _ := syscall.UTF16PtrFromString("Path")
	val, _ := syscall.UTF16FromString(dir)
	r, _, _ = procRegSetValue.Call(uintptr(k), uintptr(unsafe.Pointer(vn)), 0, syscall.REG_SZ,
		uintptr(unsafe.Pointer(&val[0])), uintptr(len(val)*2))
	if r != 0 {
		return fmt.Errorf("RegSetValueEx error %d", r)
	}
	return nil
}

type TrackingStatus struct {
	Level   string `json:"level"` // ok | warn | error
	Message string `json:"message"`
	Path    string `json:"path"`
}

// CheckTracking makes sure MSFS will load an NPClient64.dll that reads FreeTrack shared memory.
// It only changes the registry when nothing usable is configured, unless force is set.
func CheckTracking(exeDir string, force bool) TrackingStatus {
	cur := readNPClientPath()
	curOK := cur != "" && fileExists(filepath.Join(cur, "NPClient64.dll"))
	same := curOK && strings.EqualFold(filepath.Clean(cur), filepath.Clean(exeDir))
	if same {
		return TrackingStatus{"ok", "Head-tracking hook points at CameraFX. If the camera doesn't move, restart MSFS once.", cur}
	}
	if curOK && !force {
		msg := "Another head-tracking program is registered (" + cur + "). It will work if it's opentrack, as long as opentrack itself isn't running. Use the button to switch to CameraFX's copy."
		if strings.Contains(strings.ToLower(cur), "naturalpoint") || strings.Contains(strings.ToLower(cur), "trackir") {
			msg = "Real TrackIR software is registered (" + cur + "). CameraFX can't drive the camera through it. Use the button to switch to CameraFX (re-running TrackIR's software can switch it back)."
		}
		return TrackingStatus{"warn", msg, cur}
	}
	if err := writeNPClientPath(exeDir); err != nil {
		return TrackingStatus{"error", "Couldn't register the head-tracking hook: " + err.Error(), cur}
	}
	return TrackingStatus{"warn", "Head-tracking hook registered. Restart MSFS once so it picks it up.", exeDir}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }
