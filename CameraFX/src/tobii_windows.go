//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

// Direct Tobii Eye Tracker head tracking through Tobii Stream Engine
// (tobii_stream_engine.dll, the same library opentrack's Tobii input uses).

type tobiiHeadPoseRaw struct {
	TimestampUs int64
	PosValidity int32
	Pos         [3]float32
	RotValidity [3]int32
	Rot         [3]float32
}

type tobiiGazeRaw struct {
	TimestampUs int64
	Validity    int32
	XY          [2]float32
}

const tobiiValid = 1

type tobiiLib struct {
	apiCreate, apiDestroy, enumURLs, devCreate, devDestroy, headSub, gazeSub, process *syscall.Proc
}

var (
	tobiiLatest tobiiHeadPoseRaw
	tobiiFresh  bool
	tobiiURL    string
	urlCallback = syscall.NewCallback(func(url *byte, _ uintptr) uintptr {
		if tobiiURL == "" && url != nil {
			tobiiURL = bytePtrToString(url)
		}
		return 0
	})
	tobiiGaze    tobiiGazeRaw
	gazeCallback = syscall.NewCallback(func(g *tobiiGazeRaw, _ uintptr) uintptr {
		if g != nil {
			tobiiGaze = *g
		}
		return 0
	})
	poseCallback = syscall.NewCallback(func(p *tobiiHeadPoseRaw, _ uintptr) uintptr {
		if p != nil {
			tobiiLatest = *p
			tobiiFresh = true
		}
		return 0
	})
)

func bytePtrToString(p *byte) string {
	var b []byte
	for i := 0; i < 1024; i++ {
		c := *(*byte)(unsafe.Add(unsafe.Pointer(p), i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

func loadTobii(dir string) (*tobiiLib, error) {
	dll, err := syscall.LoadDLL(filepath.Join(dir, "tobii_stream_engine.dll"))
	if err != nil {
		return nil, err
	}
	l := &tobiiLib{}
	for name, p := range map[string]**syscall.Proc{
		"tobii_api_create": &l.apiCreate, "tobii_api_destroy": &l.apiDestroy,
		"tobii_enumerate_local_device_urls": &l.enumURLs, "tobii_device_create": &l.devCreate,
		"tobii_device_destroy": &l.devDestroy, "tobii_head_pose_subscribe": &l.headSub,
		"tobii_gaze_point_subscribe":     &l.gazeSub,
		"tobii_device_process_callbacks": &l.process,
	} {
		if *p, err = dll.FindProc(name); err != nil {
			return nil, err
		}
	}
	return l, nil
}

func (a *app) setTobiiStatus(s string, ok bool) {
	a.mu.Lock()
	a.tobiiStatus, a.tobiiOK = s, ok
	a.mu.Unlock()
}

// tobiiLoop connects to the eye tracker, keeps reconnecting if it goes away, and
// publishes the shaped head pose into a.head / a.lastHead.
func (a *app) tobiiLoop() {
	lib, err := loadTobii(a.exeDir)
	if err != nil {
		a.setTobiiStatus("Tobii library missing next to CameraFX.exe: "+err.Error(), false)
		return
	}
	var shaper HeadShaper
	for {
		var api, dev uintptr
		if r, _, _ := lib.apiCreate.Call(uintptr(unsafe.Pointer(&api)), 0, 0); r != 0 || api == 0 {
			a.setTobiiStatus(fmt.Sprintf("Couldn't start Tobii (error %d). Is Tobii Experience running?", r), false)
			time.Sleep(5 * time.Second)
			continue
		}
		tobiiURL = ""
		lib.enumURLs.Call(api, urlCallback, 0)
		if tobiiURL == "" {
			a.setTobiiStatus("No Tobii eye tracker found. Check it's plugged in and Tobii Experience is running.", false)
			lib.apiDestroy.Call(api)
			time.Sleep(5 * time.Second)
			continue
		}
		u := cstr(tobiiURL)
		r, _, _ := lib.devCreate.Call(api, uintptr(unsafe.Pointer(u)), uintptr(unsafe.Pointer(&dev)))
		if r != 0 || dev == 0 {
			a.setTobiiStatus(fmt.Sprintf("Found the Tobii but couldn't connect (error %d).", r), false)
			lib.apiDestroy.Call(api)
			time.Sleep(5 * time.Second)
			continue
		}
		if r, _, _ := lib.headSub.Call(dev, poseCallback, 0); r != 0 {
			a.setTobiiStatus(fmt.Sprintf("Tobii connected, but head tracking isn't available from it (error %d).", r), false)
			lib.devDestroy.Call(dev)
			lib.apiDestroy.Call(api)
			time.Sleep(10 * time.Second)
			continue
		}
		eyes := true
		if r, _, _ := lib.gazeSub.Call(dev, gazeCallback, 0); r != 0 {
			eyes = false
			logf("Tobii eye (gaze) tracking unavailable, error %d: head only", r)
		}
		logf("Tobii connected (%s)", tobiiURL)
		if eyes {
			a.setTobiiStatus("Tobii connected", true)
		} else {
			a.setTobiiStatus("Tobii connected (head only, eye tracking unavailable)", true)
		}

		last := time.Now()
		var have HeadPose
		failures := 0
		for {
			time.Sleep(8 * time.Millisecond)
			tobiiFresh = false
			if r, _, _ := lib.process.Call(dev); r != 0 {
				failures++
				if failures > 100 {
					break // lost the device; reconnect
				}
				continue
			}
			failures = 0
			now := time.Now()
			dt := now.Sub(last).Seconds()
			last = now
			if !tobiiFresh {
				continue
			}
			p := tobiiLatest
			// Same axis conversion opentrack uses (Tobii: +x right, +y up, +z towards user; radians).
			if p.PosValidity == tobiiValid {
				have.X, have.Y, have.Z = -float64(p.Pos[0])*0.1, float64(p.Pos[1])*0.1, float64(p.Pos[2])*0.1
			}
			const r2d = 180 / 3.141592653589793
			if p.RotValidity[0] == tobiiValid {
				have.Pitch = float64(p.Rot[0]) * r2d
			}
			if p.RotValidity[1] == tobiiValid {
				have.Yaw = -float64(p.Rot[1]) * r2d
			}
			if p.RotValidity[2] == tobiiValid {
				have.Roll = float64(p.Rot[2]) * r2d
			}
			if p.PosValidity != tobiiValid && p.RotValidity[1] != tobiiValid {
				continue // head not seen: let it go stale so the view eases back to centre
			}
			gz := Gaze{float64(tobiiGaze.XY[0]), float64(tobiiGaze.XY[1]), eyes && tobiiGaze.Validity == tobiiValid}
			a.mu.Lock()
			if a.recenterReq {
				shaper.Recenter, a.recenterReq = true, false
			}
			a.head = shaper.Feed(have, gz, dt, a.settings.tobiiParams())
			a.lastHead = now
			a.headDirect = true
			a.mu.Unlock()
		}
		logf("Tobii connection lost, reconnecting")
		a.setTobiiStatus("Tobii connection lost, reconnecting…", false)
		lib.devDestroy.Call(dev)
		lib.apiDestroy.Call(api)
		time.Sleep(2 * time.Second)
	}
}
