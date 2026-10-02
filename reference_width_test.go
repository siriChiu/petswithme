package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDisplayReferenceWidthKeepsExistingSizeSemantics(t *testing.T) {
	for _, size := range []int{96, 144, 192} {
		for _, dpi := range []int{96, 144, 192, 288} {
			oldW, oldH := ManifestDisplaySize(nil, 320, 288, size, dpi)
			if oldW != size*dpi/96 || oldH != oldW*288/320 {
				t.Fatal("legacy size changed")
			}
			m := DefaultAnimationManifest()
			m.ReferenceWidth = 320
			w, h := ManifestDisplaySize(m, 384, 288, size, dpi)
			if w != oldW*384/320 || h != oldH {
				t.Fatal("padding shrank body", size, dpi, w, h, oldW, oldH)
			}
			fw, fh := FitSize(w, h, Rect{-1600, -100, 0, 900})
			if fw > w || fh > h || fw <= 0 || fh <= 0 {
				t.Fatal("invalid monitor fitting")
			}
		}
	}
}
func TestReferenceWidthValidation(t *testing.T) {
	for _, ref := range []int{-1, 1, 15, 4097} {
		m := DefaultAnimationManifest()
		m.ReferenceWidth = ref
		if m.Validate(1536, 2288) == nil {
			t.Fatal("accepted reference", ref)
		}
	}
	for _, ref := range []int{0, 16, 320, 384, 4096} {
		m := DefaultAnimationManifest()
		m.ReferenceWidth = ref
		if err := m.Validate(1536, 2288); err != nil {
			t.Fatal(ref, err)
		}
	}
}

func TestReferenceWidthLoadsFromManifest(t *testing.T) {
	m := DefaultAnimationManifest()
	m.ReferenceWidth = 320
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := LoadAnimationManifest(bytes.NewReader(b), 1536, 2288)
	if err != nil || parsed.ReferenceWidth != 320 {
		t.Fatal("reference failed to load", err)
	}
}
