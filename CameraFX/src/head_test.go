package main

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestHead(t *testing.T) {
	b := make([]byte, 48)
	for i, v := range []float64{1, 2, 3, 30, -10, 5} {
		binary.LittleEndian.PutUint64(b[i*8:], math.Float64bits(v))
	}
	h, ok := parseOpentrackUDP(b)
	if !ok || h.Yaw != 30 || h.Z != 3 {
		t.Fatal("parse", h, ok)
	}
	p := h.toFreeTrack(1, 1)
	if math.Abs(p.Yaw+30*math.Pi/180) > 1e-9 || p.X != 10 {
		t.Fatal("convert", p)
	}
	var m HeadMixer
	var out Pose
	for i := 0; i < 60; i++ {
		out = m.Update(p, true, true, 1.0/60)
	}
	if math.Abs(out.Yaw-p.Yaw) > 1e-9 {
		t.Fatal("fade in", out)
	}
	for i := 0; i < 60; i++ {
		out = m.Update(p, true, false, 1.0/60)
	}
	if out.Yaw != 0 {
		t.Fatal("fade out", out)
	}
	for _, c := range []struct {
		s       string
		mods, k uint32
		ok      bool
	}{{"Ctrl+Shift+H", 6, 'H', true}, {"F9", 0, 0x78, true}, {"alt+num5", 1, 0x65, true}, {"Ctrl+", 0, 0, false}, {"Banana", 0, 0, false}} {
		m, k, ok := parseHotkey(c.s)
		if ok != c.ok || (ok && (m != c.mods || k != c.k)) {
			t.Fatal("hotkey", c.s, m, k, ok)
		}
	}
}

func TestShaper(t *testing.T) {
	p := TobiiParams{Ratio: 0.85, EyeResp: 0.5, SensYaw: 0.09, SensPitch: 0.09, SensRoll: 0.09,
		SensX: 0.09, SensY: 0.09, SensZ: 0.09, LimX: 300, LimY: 300, LimZ: 300, CenterStab: 0.2,
		LimYaw: 160, LimPitch: 75, LimRoll: 30}
	centre := Gaze{0.5, 0.5, true}
	var s HeadShaper
	base := HeadPose{X: 1, Y: 2, Z: 60, Yaw: 3, Pitch: -2}
	if out := s.Feed(base, centre, 1.0/60, p); out != (HeadPose{}) {
		t.Fatal("first sample should centre", out)
	}
	var o HeadPose
	for i := 0; i < 120; i++ {
		o = s.Feed(HeadPose{X: 1, Y: 2, Z: 60, Yaw: 18, Pitch: -2}, centre, 1.0/60, p)
	}
	if o.Yaw < 40 || o.Yaw > 100 {
		t.Fatal("15 deg head turn should give a big view turn", o.Yaw)
	}
	for i := 0; i < 120; i++ {
		o = s.Feed(base, centre, 1.0/60, p)
	}
	if math.Abs(o.Yaw) > 0.5 {
		t.Fatal("back to centre", o.Yaw)
	}
	for i := 0; i < 240; i++ {
		o = s.Feed(base, Gaze{0.98, 0.5, true}, 1.0/60, p)
	}
	if o.Yaw < 15 {
		t.Fatal("looking at the right edge should turn the view right", o.Yaw)
	}
	p.Ratio = 1
	for i := 0; i < 240; i++ {
		o = s.Feed(base, Gaze{0.98, 0.5, true}, 1.0/60, p)
	}
	if math.Abs(o.Yaw) > 0.5 {
		t.Fatal("ratio 1 = head only", o.Yaw)
	}
	p.LimYaw = 45
	for i := 0; i < 240; i++ {
		o = s.Feed(HeadPose{X: 1, Y: 2, Z: 60, Yaw: 40, Pitch: -2}, centre, 1.0/60, p)
	}
	if o.Yaw > 45.01 {
		t.Fatal("yaw limit", o.Yaw)
	}
	p.LimYaw = 160
	s.Recenter = true
	if o = s.Feed(HeadPose{Yaw: 40}, centre, 1.0/60, p); math.Abs(o.Yaw) > 0.5 {
		t.Fatal("recenter", o)
	}
}
