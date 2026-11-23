package main

import (
	"strings"
	"testing"
)

func TestOffsetAdjust(t *testing.T) {
	f := ParseIni("[effect.wing_vapour_1]\npreset=vapour\noffset=2.4 0.16 0.11\noffset_adjust=0 -0.5 0\n\n[effect.other]\noffset=1 1 1\n")
	applyOffsetAdjust(f)
	if v, _ := f.Get("effect.wing_vapour_1", "offset"); v != fmtVec([3]float64{2.4, -0.34, 0.11}) {
		t.Fatalf("offset %q", v)
	}
	if strings.Contains(f.String(), "offset_adjust") {
		t.Fatal("offset_adjust passed through to the module")
	}
	if v, _ := f.Get("effect.other", "offset"); v != "1 1 1" {
		t.Fatalf("other changed: %q", v)
	}
}

func TestOffsetFrom(t *testing.T) {
	f := ParseIni("[effect.wingtip_vortex]\noffset=17.1 4.2 -9.6\n\n[effect.wingtip_vortex_ribs]\noffset_from=wingtip_vortex\noffset=0 0 0\noffset_adjust=0 0 -0.5\n")
	applyOffsetAdjust(f)
	if v, _ := f.Get("effect.wingtip_vortex_ribs", "offset"); v != fmtVec([3]float64{17.1, 4.2, -10.1}) {
		t.Fatalf("ribs offset %q", v)
	}
	if strings.Contains(f.String(), "offset_from") {
		t.Fatal("offset_from passed through")
	}
}
