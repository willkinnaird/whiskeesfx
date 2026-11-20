package main

import (
	"fmt"
	"math"
	"testing"
)

type stats struct{ minY, maxY, minZ, maxZ, minP, maxP float64 }

func run(e *Engine, s Settings, secs float64, f func(t float64) Telemetry) stats {
	st := stats{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
	dt := 1.0 / 60
	for t := 0.0; t < secs; t += dt {
		p := e.Update(f(t), true, dt, s)
		st.minY, st.maxY = math.Min(st.minY, p.Y), math.Max(st.maxY, p.Y)
		st.minZ, st.maxZ = math.Min(st.minZ, p.Z), math.Max(st.maxZ, p.Z)
		st.minP, st.maxP = math.Min(st.minP, p.Pitch), math.Max(st.maxP, p.Pitch)
		if math.IsNaN(p.Y) || math.IsNaN(p.Z) || math.IsNaN(p.Pitch) {
			panic("NaN")
		}
	}
	return st
}

func TestScenarios(t *testing.T) {
	s := DefaultSettings()
	base := Telemetry{OnGround: 1, CameraState: 2}

	e := NewEngine(1)
	st := run(e, s, 5, func(float64) Telemetry { return base })
	fmt.Printf("parked:        Y %+.2f..%+.2f  Z %+.2f..%+.2f mm\n", st.minY, st.maxY, st.minZ, st.maxZ)
	if st.maxY-st.minY > 0.01 {
		t.Fatal("parked aircraft should not move")
	}

	e = NewEngine(2)
	tx := base
	tx.GroundSpeedKt = 18
	st = run(e, s, 20, func(float64) Telemetry { return tx })
	fmt.Printf("taxi 18kt:     Y %+.2f..%+.2f mm  pitch %+.3f..%+.3f rad\n", st.minY, st.maxY, st.minP, st.maxP)
	if st.maxY-st.minY < 3 {
		t.Fatal("taxi should bob noticeably")
	}

	e = NewEngine(3)
	gr := base
	gr.GroundSpeedKt, gr.SurfaceType = 18, 1
	st = run(e, s, 20, func(float64) Telemetry { return gr })
	fmt.Printf("grass 18kt:    Y %+.2f..%+.2f mm\n", st.minY, st.maxY)

	e = NewEngine(4)
	st = run(e, s, 6, func(tt float64) Telemetry {
		x := base
		x.GroundSpeedKt = 20
		if tt > 2 && tt < 4 {
			x.AccelZ = -0.3 * 32.174 // firm braking
		}
		return x
	})
	fmt.Printf("brake 0.3g:    Z %+.2f..%+.2f mm  pitch %+.3f..%+.3f rad\n", st.minZ, st.maxZ, st.minP, st.maxP)
	if st.minZ > -15 {
		t.Fatal("braking should lurch forward (negative Z)")
	}

	e = NewEngine(5)
	st = run(e, s, 6, func(tt float64) Telemetry {
		return Telemetry{CameraState: 2, IASKt: 280, Spoilers: 1, AccelZ: -0.06 * 32.174}
	})
	fmt.Printf("speedbrake:    Z %+.2f..%+.2f mm  Y %+.2f..%+.2f\n", st.minZ, st.maxZ, st.minY, st.maxY)

	e = NewEngine(6)
	st = run(e, s, 6, func(tt float64) Telemetry {
		if tt < 3 {
			return Telemetry{CameraState: 2, IASKt: 140, GroundSpeedKt: 135, VerticalSpeed: -8}
		}
		return Telemetry{OnGround: 1, CameraState: 2, IASKt: 135, GroundSpeedKt: 130, AccelZ: -0.15 * 32.174}
	})
	fmt.Printf("landing:       Y %+.2f..%+.2f  Z %+.2f..%+.2f mm\n", st.minY, st.maxY, st.minZ, st.maxZ)

	// ChasePlane (camera state 7) counts as cockpit
	e = NewEngine(8)
	cp := tx
	cp.CameraState = 7
	st = run(e, s, 5, func(float64) Telemetry { return cp })
	if st.maxY-st.minY < 1 {
		t.Fatal("effects should run under ChasePlane")
	}

	// external view: nothing
	e = NewEngine(7)
	ex := tx
	ex.CameraState = 3
	st = run(e, s, 5, func(float64) Telemetry { return ex })
	if st.maxY != 0 || st.minY != 0 {
		t.Fatal("no effect outside the cockpit")
	}
}
