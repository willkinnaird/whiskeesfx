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
