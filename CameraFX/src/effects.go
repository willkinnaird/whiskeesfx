package main

import (
	"math"
	"math/rand"
)

// Telemetry is what we read from MSFS every sim frame. Field order must match
// simVars below (each is requested as FLOAT64).
type Telemetry struct {
	OnGround      float64 // bool
	GroundSpeedKt float64 // knots
	AccelZ        float64 // ft/s^2, body longitudinal, + = forward
	BrakeL        float64 // 0..1
	BrakeR        float64 // 0..1
	Spoilers      float64 // speedbrake handle 0..1
	IASKt         float64 // knots
	SurfaceType   float64 // enum
	CameraState   float64 // enum (2 = cockpit)
	VerticalSpeed float64 // ft/s
}

var simVars = []struct{ Name, Units string }{
	{"SIM ON GROUND", "bool"},
	{"GROUND VELOCITY", "knots"},
	{"ACCELERATION BODY Z", "feet per second squared"},
	{"BRAKE LEFT POSITION", "percent over 100"},
	{"BRAKE RIGHT POSITION", "percent over 100"},
	{"SPOILERS HANDLE POSITION", "percent over 100"},
	{"AIRSPEED INDICATED", "knots"},
	{"SURFACE TYPE", "enum"},
	{"CAMERA STATE", "enum"},
	{"VERTICAL SPEED", "feet per second"},
}

// Pose is the fake "head" offset we hand to the sim through FreeTrack.
// Translations in millimetres, rotations in radians.
// Conventions (as MSFS interprets them): +Y up, +Z backwards, +Pitch = look down.
type Pose struct {
	X, Y, Z          float64
	Yaw, Pitch, Roll float64
}

type surface struct {
	Name    string
	Rough   float64 // rumble / bob multiplier
	BumpAmp float64 // discrete bump strength
	Spacing float64 // average metres between noticeable bumps
}

func surfaceProfile(t int) surface {
	switch t {
	case 0:
		return surface{"Concrete", 1.0, 1.0, 12}
	case 4, 15, 17, 19, 23:
		names := map[int]string{4: "Asphalt", 15: "Oil treated", 17: "Bituminous", 19: "Macadam", 23: "Tarmac"}
		return surface{names[t], 0.75, 0.7, 25}
	case 1, 5, 7:
		names := map[int]string{1: "Grass", 5: "Short grass", 7: "Hard turf"}
		return surface{names[t], 1.7, 0.8, 4}
	case 3, 6:
		names := map[int]string{3: "Bumpy grass", 6: "Long grass"}
		return surface{names[t], 2.2, 1.1, 3}
	case 12, 13, 14, 21, 22:
		names := map[int]string{12: "Dirt", 13: "Coral", 14: "Gravel", 21: "Sand", 22: "Shale"}
		return surface{names[t], 2.0, 1.0, 3.5}
	case 16, 18, 20:
		names := map[int]string{16: "Steel mats", 18: "Brick", 20: "Planks"}
		return surface{names[t], 1.5, 1.2, 2.5}
	case 2:
		return surface{"Water", 0.3, 0, 0}
	case 8, 9:
		names := map[int]string{8: "Snow", 9: "Ice"}
		return surface{names[t], 0.6, 0.4, 15}
	default:
		return surface{"Other", 1.0, 0.9, 12}
	}
}

// osc is a damped spring (second-order system) used for every moving part.
type osc struct{ pos, vel float64 }

func (o *osc) step(dt, freqHz, zeta, target float64) {
	w := 2 * math.Pi * freqHz
	const sub = 0.002
	for dt > 1e-9 {
		h := math.Min(dt, sub)
		acc := w*w*(target-o.pos) - 2*zeta*w*o.vel
		o.vel += acc * h
		o.pos += o.vel * h
		dt -= h
	}
}

// vnoise is smooth "value noise" in -1..1.
type vnoise struct{ a, b, t float64 }

func (n *vnoise) next(r *rand.Rand, dt, period float64) float64 {
	if period < 1e-3 {
		period = 1e-3
	}
	n.t += dt / period
	for n.t >= 1 {
		n.t -= 1
		n.a = n.b
		n.b = r.Float64()*2 - 1
	}
	s := n.t * n.t * (3 - 2*n.t)
	return n.a + (n.b-n.a)*s
}

type kick struct {
	t, amp, pitchSign float64
}

const (
	bobHz, bobZeta       = 1.3, 0.25
	bounceHz, bounceZeta = 4.5, 0.30
	nodHz, nodZeta       = 2.2, 0.35
	lurchHz, lurchZeta   = 1.5, 0.45
)

// Engine turns telemetry into a head pose.
type Engine struct {
	r *rand.Rand

	bob, bounce, nod, lurch, lurchPitch osc
	bobN, rumY, rumR, bufY, bufR        vnoise

	dist, nextBump float64
	kicks          []kick

	accF float64

	wasOnGround bool
	airTime     float64
	lastVS      float64
	initialized bool

	fade      float64
	testTime  float64
	lurchTest float64

	// for display
	Surface string
	Active  bool
}

func NewEngine(seed int64) *Engine {
	return &Engine{r: rand.New(rand.NewSource(seed))}
}

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

func (e *Engine) TestBump() {
	e.kicks = append(e.kicks, kick{0, 6, 1}, kick{0.35, 7, -1})
	e.testTime = 2.5
}

func (e *Engine) TestLurch() {
	e.lurchTest = 1.4
	e.testTime = 3.5
}

func (e *Engine) addKick(amp, pitchSign float64) {
	e.bounce.vel += amp * 2 * math.Pi * bounceHz
	e.nod.vel += pitchSign * amp * 0.0015 * 2 * math.Pi * nodHz
}

// Update advances the simulation by dt seconds. fresh=false means telemetry is
// stale (sim paused, loading, or disconnected).
func (e *Engine) Update(t Telemetry, fresh bool, dt float64, s Settings) Pose {
	dt = clamp(dt, 0, 0.1)
	if dt == 0 {
		dt = 1e-3
	}
	if e.testTime > 0 {
		e.testTime -= dt
	}

	// 2 = MSFS cockpit camera. 7 = add-on (custom) camera, which is what MSFS reports
	// whenever ChasePlane is driving the view; we can't tell its views apart, so allow it.
	cam := int(math.Round(t.CameraState))
	inCockpit := cam == 2 || cam == 7
	active := s.Enabled && ((fresh && (!s.OnlyInCockpit || inCockpit)) || e.testTime > 0)
	e.Active = active
	target := 0.0
	if active {
		target = 1
	}
	if e.fade < target {
		e.fade = math.Min(target, e.fade+dt*2.5)
	} else {
		e.fade = math.Max(target, e.fade-dt*2.5)
	}

	onGround := fresh && t.OnGround > 0.5
	gs := math.Max(0, t.GroundSpeedKt)
	if !fresh {
		gs = 0
	}
	surf := surfaceProfile(int(math.Round(t.SurfaceType)))
	e.Surface = surf.Name
	sf := clamp(gs/25, 0, 1)       // taxi-speed factor
	hs := clamp((gs-30)/100, 0, 1) // high-speed (take-off/landing roll) factor

	// --- Ground bob: the slow rocking of the airframe on its oleos.
	bobTarget := 0.0
	if onGround {
		amp := s.GroundBob * surf.Rough * (3.0*sf + 2.0*hs)
		bobTarget = amp * e.bobN.next(e.r, dt, 0.7-0.3*hs)
	}
	e.bob.step(dt, bobHz, bobZeta, bobTarget)

	// --- Rumble from the surface, and speedbrake buffet.
	rumbleAmp := 0.0
	if onGround {
		rumbleAmp = s.Rumble * surf.Rough * (0.5*sf + 1.2*hs)
	}
	q := clamp(t.IASKt/250, 0, 1.3)
	q *= q
	sb := 0.0
	if fresh {
		sb = clamp(t.Spoilers, 0, 1) * q
	}
	buffetAmp := s.SpeedbrakeBuffet * sb * 1.0
	rumPeriod := 1 / (5 + gs/6)
	rY := rumbleAmp*e.rumY.next(e.r, dt, rumPeriod) + buffetAmp*e.bufY.next(e.r, dt, 1/9.0)
	rR := (rumbleAmp*e.rumR.next(e.r, dt, rumPeriod*1.3) + buffetAmp*e.bufR.next(e.r, dt, 1/7.0)) * 0.0025

	// --- Discrete bumps: slab joints, patches, centreline lights. Nose gear first, then mains.
	if onGround && gs > 1.5 && surf.BumpAmp > 0 {
		e.dist += gs * 0.5144 * dt
		if e.nextBump == 0 {
			e.nextBump = surf.Spacing * (0.5 + 1.5*e.r.Float64())
		}
		if e.dist >= e.nextBump {
			e.dist = 0
			e.nextBump = surf.Spacing * (0.5 + 1.5*e.r.Float64())
			amp := s.Bumps * surf.BumpAmp * clamp(gs/20, 0.3, 1.0) * (0.5 + 0.5*e.r.Float64()) * 5
			amp *= 1 - 0.3*hs
			e.kicks = append(e.kicks, kick{0, amp, 1})
			delay := s.Wheelbase / math.Max(gs*0.5144, 0.1)
			if delay < 0.6 && delay > 0.03 {
				e.kicks = append(e.kicks, kick{delay, amp * 1.3, -1})
			}
		}
	} else {
		e.dist = 0
	}
	remaining := e.kicks[:0]
	for _, k := range e.kicks {
		k.t -= dt
		if k.t <= 0 {
			e.addKick(k.amp, k.pitchSign)
		} else {
			remaining = append(remaining, k)
		}
	}
	e.kicks = remaining

	// --- Touchdown thump.
	if fresh {
		if onGround && !e.wasOnGround && e.initialized && e.airTime > 1.0 {
			vs := math.Max(0, -e.lastVS) // ft/s descending
			amp := s.TouchdownThump * clamp(vs/6, 0.3, 2.0) * 8
			e.bounce.vel -= amp * 2 * math.Pi * bounceHz
			e.bob.vel -= amp * 0.5 * 2 * math.Pi * bobHz
			e.nod.vel += amp * 0.001 * 2 * math.Pi * nodHz
		}
		if !onGround {
			e.airTime += dt
			e.lastVS = t.VerticalSpeed
		} else {
			e.airTime = 0
		}
		e.wasOnGround = onGround
		e.initialized = true
	}

	// --- Lurch under braking / speedbrake, and push-back under acceleration.
	ag := 0.0
	if fresh {
		ag = t.AccelZ / 32.174
	}
	if e.lurchTest > 0 {
		e.lurchTest -= dt
		ag = -0.35
	}
	e.accF += (ag - e.accF) * (1 - math.Exp(-dt/0.12))
	decel := clamp(-e.accF, 0, 0.8)
	push := clamp(e.accF, 0, 0.6)
	zT := -s.BrakeLurch*decel*70 + s.AccelPush*push*45
	pT := s.BrakeLurch*decel*0.10 - s.AccelPush*push*0.05
	if !onGround {
		zT -= s.SpeedbrakeLurch * sb * 15
		pT += s.SpeedbrakeLurch * sb * 0.02
	}
	e.lurch.step(dt, lurchHz, lurchZeta, zT)
	e.lurchPitch.step(dt, lurchHz, 0.5, pT)

	e.bounce.step(dt, bounceHz, bounceZeta, 0)
	e.nod.step(dt, nodHz, nodZeta, 0)

	scale := s.Master * e.fade
	p := Pose{
		Y:     clamp((e.bob.pos+e.bounce.pos+rY)*scale, -60, 60),
		Z:     clamp(e.lurch.pos*scale, -120, 120),
		Pitch: clamp((e.lurchPitch.pos+e.nod.pos)*scale, -0.3, 0.3),
		Roll:  clamp(rR*scale, -0.1, 0.1),
	}
	if s.InvertPitch {
		p.Pitch = -p.Pitch
	}
	if math.Abs(p.Y) < 1e-4 && math.Abs(p.Z) < 1e-4 && math.Abs(p.Pitch) < 1e-6 && math.Abs(p.Roll) < 1e-6 {
		p = Pose{} // exact zero lets the sim hand the camera back fully
	}
	return p
}
