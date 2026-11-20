package main

import (
	"encoding/binary"
	"math"
	"strings"
)

// HeadPose is a real head-tracker pose in opentrack's "UDP over network" units:
// X, Y, Z in centimetres; Yaw, Pitch, Roll in degrees.
type HeadPose struct{ X, Y, Z, Yaw, Pitch, Roll float64 }

// parseOpentrackUDP decodes the 48-byte packet (6 little-endian doubles) that
// opentrack's UDP output and Tobii Game Hub send.
func parseOpentrackUDP(b []byte) (HeadPose, bool) {
	if len(b) < 48 {
		return HeadPose{}, false
	}
	var v [6]float64
	for i := range v {
		v[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[i*8:]))
		if math.IsNaN(v[i]) || math.IsInf(v[i], 0) || math.Abs(v[i]) > 1000 {
			return HeadPose{}, false
		}
	}
	return HeadPose{v[0], v[1], v[2], v[3], v[4], v[5]}, true
}

// toFreeTrack converts to FreeTrack units/signs exactly as opentrack's own
// freetrack output does (mm, radians, yaw and pitch negated).
func (h HeadPose) toFreeTrack(rotGain, moveGain float64) Pose {
	const d2r = math.Pi / 180
	return Pose{
		X:     h.X * 10 * moveGain,
		Y:     h.Y * 10 * moveGain,
		Z:     h.Z * 10 * moveGain,
		Yaw:   -h.Yaw * d2r * rotGain,
		Pitch: -h.Pitch * d2r * rotGain,
		Roll:  h.Roll * d2r * rotGain,
	}
}

// HeadMixer fades the real head pose in and out so toggling never snaps the view.
type HeadMixer struct {
	fade float64
	last Pose
}

func (m *HeadMixer) Update(head Pose, live, enabled bool, dt float64) Pose {
	target := 0.0
	if live && enabled {
		target = 1
		m.last = head
	}
	rate := 4.0 * dt
	if m.fade < target {
		m.fade = math.Min(target, m.fade+rate)
	} else {
		m.fade = math.Max(target, m.fade-rate)
	}
	f := m.fade * m.fade * (3 - 2*m.fade)
	p := m.last
	return Pose{p.X * f, p.Y * f, p.Z * f, p.Yaw * f, p.Pitch * f, p.Roll * f}
}

func addPose(a, b Pose) Pose {
	return Pose{a.X + b.X, a.Y + b.Y, a.Z + b.Z, a.Yaw + b.Yaw, a.Pitch + b.Pitch, a.Roll + b.Roll}
}

// parseHotkey turns "Ctrl+Shift+H" into Win32 modifier flags and a virtual-key code.
func parseHotkey(s string) (mods, vk uint32, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	parts := strings.Split(s, "+")
	for i, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		last := i == len(parts)-1
		switch {
		case !last && (p == "CTRL" || p == "CONTROL"):
			mods |= 0x2
		case !last && p == "ALT":
			mods |= 0x1
		case !last && p == "SHIFT":
			mods |= 0x4
		case !last && p == "WIN":
			mods |= 0x8
		case last:
			vk = keyCode(p)
			if vk == 0 {
				return 0, 0, false
			}
		default:
			return 0, 0, false
		}
	}
	return mods, vk, vk != 0
}

func keyCode(k string) uint32 {
	if len(k) == 1 && ((k[0] >= 'A' && k[0] <= 'Z') || (k[0] >= '0' && k[0] <= '9')) {
		return uint32(k[0])
	}
	if len(k) >= 2 && k[0] == 'F' {
		n := 0
		for _, c := range k[1:] {
			if c < '0' || c > '9' {
				n = -1
				break
			}
			n = n*10 + int(c-'0')
		}
		if n >= 1 && n <= 24 {
			return 0x70 + uint32(n-1)
		}
	}
	if len(k) == 4 && strings.HasPrefix(k, "NUM") && k[3] >= '0' && k[3] <= '9' {
		return 0x60 + uint32(k[3]-'0')
	}
	named := map[string]uint32{
		"PAUSE": 0x13, "SCROLLLOCK": 0x91, "INSERT": 0x2D, "INS": 0x2D, "HOME": 0x24, "END": 0x23,
		"PAGEUP": 0x21, "PAGEDOWN": 0x22, "DELETE": 0x2E, "DEL": 0x2E, "SPACE": 0x20,
	}
	return named[k]
}

// TobiiParams mirrors MSFS 2024's Tobii "Fine tune your input's curve" page: same names, same ranges.
type TobiiParams struct {
	Ratio      float64 // Eye vs Head Tracking Ratio (0..1, 1 = head only)
	EyeResp    float64 // Eye Tracking Responsiveness (0..1)
	SensYaw    float64 // Head Tracking Sensitivity: Yaw (0..1)
	SensPitch  float64 // Head Tracking Sensitivity: Pitch (0..1)
	SensRoll   float64 // Head Tracking Sensitivity: Roll (0..1)
	SensX      float64 // Head Tracking Sensitivity: Position X (0..1)
	SensY      float64 // Head Tracking Sensitivity: Position Y (0..1)
	SensZ      float64 // Head Tracking Sensitivity: Position Z (0..1)
	LimX       float64 // Head Tracking Limit: Position X (0..300 mm)
	LimY       float64 // Head Tracking Limit: Position Y (0..300 mm)
	LimZ       float64 // Head Tracking Limit: Position Z (0..300 mm)
	CenterStab float64 // Center Stabilization (0..1)
	AutoCenter bool    // Head Tracking Auto-center
	Boost      float64 // Camera Boost Ratio (0..1): extra turn the further you look round
	Smoothing  float64 // CameraFX extra (0..1): steadies the view when your head is still
	LimYaw     float64 // CameraFX extra: max view turn left/right (degrees)
	LimPitch   float64 // CameraFX extra: max view turn up/down (degrees)
	LimRoll    float64 // CameraFX extra: max view tilt (degrees)
}

// oneEuro is the "1€ filter" (Casiez et al.): strong smoothing when the signal is
// still, little smoothing when it moves fast, so it removes jitter without adding lag.
type oneEuro struct {
	x, dx float64
	init  bool
}

func lpAlpha(cutoff, dt float64) float64 {
	tau := 1 / (2 * math.Pi * cutoff)
	return 1 / (1 + tau/dt)
}

func (f *oneEuro) filter(v, dt, minCutoff, beta float64) float64 {
	if !f.init {
		f.x, f.dx, f.init = v, 0, true
		return v
	}
	d := (v - f.x) / dt
	f.dx += (d - f.dx) * lpAlpha(1.0, dt)
	cut := minCutoff + beta*math.Abs(f.dx)
	f.x += (v - f.x) * lpAlpha(cut, dt)
	return f.x
}

// MSFS doesn't publish its internal scaling, so these are calibrated so that a
// sensitivity of 0.09 gives about 3x head rotation and 1x head movement.
const rotScale, posScale = 33.3, 11.1

// Gaze is where you're looking on screen, 0..1 from the top-left corner.
type Gaze struct {
	X, Y  float64
	Valid bool
}

// HeadShaper turns a raw, absolute Tobii head pose (+ gaze) into a camera pose the
// way MSFS's built-in Tobii support does: centred, stabilised, amplified, with eye-driven
// "extended view" on top.
type HeadShaper struct {
	center   HeadPose
	have     bool
	Recenter bool
	filt     HeadPose
	eyeYaw   float64
	eyePitch float64
	euro     [6]oneEuro
	holdYaw  float64
	holdPit  float64
}

// hold is a moving dead zone: the output only follows once the input has moved more
// than h away, so small tremors never reach the camera wherever you're looking.
func hold(held *float64, v, h float64) float64 {
	if d := v - *held; math.Abs(d) > h {
		*held += d - math.Copysign(h, d)
	}
	return *held
}

func deadzone(v, z float64) float64 {
	if math.Abs(v) < z {
		return 0
	}
	return v - math.Copysign(z, v)
}

// edge maps a gaze offset (-0.5..0.5 from screen centre) to -1..1, ignoring the middle of
// the screen so reading the instruments doesn't move the view.
func edge(g float64) float64 {
	return math.Copysign(clamp((math.Abs(g)-0.2)/0.28, 0, 1), g)
}

func (s *HeadShaper) Feed(raw HeadPose, gaze Gaze, dt float64, p TobiiParams) HeadPose {
	if !s.have || s.Recenter {
		s.center, s.have, s.Recenter = raw, true, false
		s.filt = HeadPose{}
		s.euro = [6]oneEuro{}
		s.holdYaw, s.holdPit = 0, 0
	}
	d := HeadPose{raw.X - s.center.X, raw.Y - s.center.Y, raw.Z - s.center.Z,
		raw.Yaw - s.center.Yaw, raw.Pitch - s.center.Pitch, raw.Roll - s.center.Roll}

	if p.AutoCenter && math.Abs(d.Yaw) < 15 && math.Abs(d.Pitch) < 10 {
		k := dt / 8 // slowly absorb small drifts in how you sit, never a deliberate look
		s.center.X += d.X * k
		s.center.Y += d.Y * k
		s.center.Z += d.Z * k
		s.center.Yaw += d.Yaw * k
		s.center.Pitch += d.Pitch * k
		s.center.Roll += d.Roll * k
	}

	a := 1 - math.Exp(-dt/0.06) // ~60 ms smoothing against jitter
	s.filt.X += (d.X - s.filt.X) * a
	s.filt.Y += (d.Y - s.filt.Y) * a
	s.filt.Z += (d.Z - s.filt.Z) * a
	s.filt.Yaw += (d.Yaw - s.filt.Yaw) * a
	s.filt.Pitch += (d.Pitch - s.filt.Pitch) * a
	s.filt.Roll += (d.Roll - s.filt.Roll) * a
	f := s.filt
	if h := 0.6 * clamp(p.Smoothing, 0, 1); h > 0 {
		f.Yaw = hold(&s.holdYaw, f.Yaw, h)
		f.Pitch = hold(&s.holdPit, f.Pitch, h)
	} else {
		s.holdYaw, s.holdPit = f.Yaw, f.Pitch
	}

	dz := p.CenterStab * 8 // degrees
	ratio := clamp(p.Ratio, 0, 1)
	prog := 1 + 2*clamp(p.Boost, 0, 1) // Camera Boost: speeds up the outer part of the turn
	headCurve := func(x, sens, max float64) float64 {
		x = deadzone(x, dz)
		return clamp(sens*rotScale*x*(1+prog*math.Abs(x)/20), -ratio*max, ratio*max)
	}

	maxViewYaw, maxViewPitch := clamp(p.LimYaw, 0, 180), clamp(p.LimPitch, 0, 90)

	// Eyes: looking towards the screen edges turns the view further.
	ty, tp := 0.0, 0.0
	if gaze.Valid {
		ty = (1 - ratio) * maxViewYaw * edge(gaze.X-0.5)
		tp = (1 - ratio) * maxViewPitch * edge(0.5-gaze.Y)
	}
	tau := 0.9 - 0.8*clamp(p.EyeResp, 0, 1)
	ea := 1 - math.Exp(-dt/tau)
	s.eyeYaw += (ty - s.eyeYaw) * ea
	s.eyePitch += (tp - s.eyePitch) * ea

	lim := func(v, sens, limMM float64) float64 { // HeadPose position is in cm
		return clamp(v*sens*posScale, -limMM/10, limMM/10)
	}
	out := HeadPose{
		X:     lim(f.X, p.SensX, p.LimX),
		Y:     lim(f.Y, p.SensY, p.LimY),
		Z:     lim(f.Z, p.SensZ, p.LimZ),
		Yaw:   clamp(headCurve(f.Yaw, p.SensYaw, maxViewYaw)+s.eyeYaw, -maxViewYaw, maxViewYaw),
		Pitch: clamp(headCurve(f.Pitch, p.SensPitch, maxViewPitch)+s.eyePitch, -maxViewPitch, maxViewPitch),
		Roll:  clamp(deadzone(f.Roll, 0.5)*p.SensRoll*posScale, -p.LimRoll, p.LimRoll),
	}
	if sm := clamp(p.Smoothing, 0, 1); sm > 0 && dt > 0 {
		// Smoothing is applied to the final view angles, because that's where the
		// amplified jitter is. 0 = off, 1 = very steady (cut-off down to 0.15 Hz).
		minCut := 3.0 * math.Pow(0.05, sm)
		beta := 0.02
		v := []*float64{&out.Yaw, &out.Pitch, &out.Roll, &out.X, &out.Y, &out.Z}
		for i, ptr := range v {
			b := beta
			if i >= 3 {
				b = 0.1 // positions are in cm, rotations in degrees
			}
			*ptr = s.euro[i].filter(*ptr, dt, minCut, b)
		}
	}
	if s.Recenter {
		s.euro = [6]oneEuro{}
	}
	return out
}
