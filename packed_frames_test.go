package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
)

func packedFixture() (*Atlas, *Atlas, AnimationFrame, AnimationFrame) {
	full := image.NewNRGBA(image.Rect(0, 0, 80, 90))
	for y := 23; y < 82; y++ {
		for x := 17; x < 63; x++ {
			alpha := []uint8{0, 23, 24, 128, 255}[(x+y)%5]
			full.SetNRGBA(x, y, color.NRGBA{uint8(x * 3), uint8(y * 2), uint8(x + y), alpha})
		}
	}
	packed := image.NewNRGBA(image.Rect(0, 0, 54, 69))
	draw.Draw(packed, image.Rect(3, 5, 49, 64), full, image.Pt(17, 23), draw.Src)
	return &Atlas{Image: full}, &Atlas{Image: packed}, AnimationFrame{Rect: &FrameRect{0, 0, 80, 90}, DurationMS: 100}, AnimationFrame{Rect: &FrameRect{3, 5, 46, 59}, Canvas: &FrameCanvas{80, 90, 17, 23}, DurationMS: 100}
}
func TestPackedFrameRenderingIsByteIdentical(t *testing.T) {
	full, packed, a, b := packedFixture()
	for _, size := range [][2]int{{96, 108}, {144, 162}, {192, 216}, {201, 230}, {144, 144}} {
		for _, anchor := range []AnimationAnchor{{.5, 1}, {.5, 82.0 / 90}, {.2, .7}} {
			for _, action := range []string{"idle", "drag", "stretch", "gaze_31"} {
				x := RenderBehaviorPixels(full, BehaviorFrame{AnimationFrame: a, Anchor: anchor, Action: action}, size[0], size[1])
				y := RenderBehaviorPixels(packed, BehaviorFrame{AnimationFrame: b, Anchor: anchor, Action: action}, size[0], size[1])
				if !bytes.Equal(x, y) {
					t.Fatalf("packed pixels changed size%v anchor%v action%s", size, anchor, action)
				}
			}
		}
	}
}
func TestPackedFrameValidationAndLogicalSize(t *testing.T) {
	_, packed, _, f := packedFixture()
	m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{.5, 82.0 / 90}, Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: []AnimationFrame{f}}}}}
	if err := ValidateManifestPixels(m, packed); err != nil {
		t.Fatal(err)
	}
	if w, h := ManifestCanvas(m, packed); w != 80 || h != 90 {
		t.Fatalf("stored crop changed window size:%d,%d", w, h)
	}
	data, _ := json.Marshal(m)
	loaded, err := LoadAnimationManifest(strings.NewReader(string(data)), 54, 69)
	if err != nil || *loaded.Actions["idle"].Loop[0].Canvas != *f.Canvas {
		t.Fatal("logical canvas lost in JSON")
	}
	m.Anchor.Y = .4
	if ValidateManifestPixels(m, packed) == nil {
		t.Fatal("cropping hid visible ground clipping")
	}
}
func TestPackedFrameRejectsInvalidCanvases(t *testing.T) {
	_, packed, _, f := packedFixture()
	for _, bad := range []FrameCanvas{{0, 90, 17, 23}, {80, 0, 17, 23}, {5000, 90, 17, 23}, {80, 90, -1, 23}, {80, 90, 40, 23}, {80, 90, 17, 40}} {
		m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Actions: map[string]AnimationAction{}}
		f.Canvas = &bad
		m.Actions["idle"] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{f}}}
		if m.Validate(packed.Image.Bounds().Dx(), packed.Image.Bounds().Dy()) == nil {
			t.Fatalf("bad canvas accepted:%+v", bad)
		}
	}
	f.Rect = nil
	f.Canvas = &FrameCanvas{80, 90, 17, 23}
	m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: []AnimationFrame{f}}}}}
	if m.Validate(1024, 1408) == nil {
		t.Fatal("canvas accepted on ambiguous legacy frame")
	}
}
