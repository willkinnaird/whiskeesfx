package main

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func jitter(t *testing.T, smoothing float64) (centre, side, lagDeg float64) {
	p := DefaultSettings().tobiiParams()
	p.Smoothing = smoothing
	r := rand.New(rand.NewSource(1))
	measure := func(yaw float64) float64 {
		var s HeadShaper
		s.Feed(HeadPose{Z: 60}, Gaze{}, 1.0/33, p)
		var vals []float64
		for i := 0; i < 330; i++ { // 10 s at Tobii's ~33 Hz
			n := r.NormFloat64() * 0.35
			o := s.Feed(HeadPose{Z: 60, Yaw: yaw + n, Pitch: r.NormFloat64() * 0.35}, Gaze{}, 1.0/33, p)
			if i > 100 {
				vals = append(vals, o.Yaw)
			}
		}
		m := 0.0
		for _, v := range vals {
			m += v
		}
		m /= float64(len(vals))
		sd := 0.0
		for _, v := range vals {
			sd += (v - m) * (v - m)
		}
		return math.Sqrt(sd / float64(len(vals)))
	}
	centre, side = measure(0), measure(15)
	// lag: quick 20 deg turn in 0.25 s; how far behind is the view 0.1 s after you stop?
	var s HeadShaper
	s.Feed(HeadPose{Z: 60}, Gaze{}, 1.0/33, p)
	var o HeadPose
	for i := 0; i < 12; i++ {
		y := math.Min(20, float64(i)*20/8)
		o = s.Feed(HeadPose{Z: 60, Yaw: y}, Gaze{}, 1.0/33, p)
	}
	var full HeadPose
	for i := 0; i < 200; i++ {
		full = s.Feed(HeadPose{Z: 60, Yaw: 20}, Gaze{}, 1.0/33, p)
	}
	return centre, side, full.Yaw - o.Yaw
}

func TestJitter(t *testing.T) {
	for _, sm := range []float64{0, 0.5, 0.7, 0.9} {
		c, s, l := jitter(t, sm)
		fmt.Printf("smoothing %.1f: view shake at centre %.2f°, looking 15° to the side %.2f°, lag after a quick turn %.1f°\n", sm, c, s, l)
	}
}
