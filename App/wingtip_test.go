package main

import (
	"math"
	"os"
	"testing"
)

func TestWingtipBottomStaysWhenTopMoves(t *testing.T) {
	src, err := os.ReadFile("../VisualFX/visualfx.ini")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := loadProfile("profiles/fenix-a321-cfm-sharklets.ini", false)
	a := &App{source: ParseIni(string(src))}
	bottom := func() [3]float64 {
		for _, v := range a.pointViews(p, a.settingsFor(p)) {
			if v.ID == "wingtip_bottom" {
				return v.Dot
			}
		}
		t.Fatal("no wingtip_bottom")
		return [3]float64{}
	}
	before := bottom()
	for axis, d := range []float64{0.4, 0.3, -0.25} {
		if err := a.movePoint(p, a.settingsFor(p), "wingtip_top", axis, d, nil); err != nil {
			t.Fatal(err)
		}
	}
	after := bottom()
	for i := range before {
		if math.Abs(after[i]-before[i]) > 0.002 {
			t.Fatalf("bottom moved: %v -> %v", before, after)
		}
	}
}
