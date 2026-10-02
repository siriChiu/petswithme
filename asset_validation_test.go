package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomManifestRejectsBlankFrame(t *testing.T) {
	a := &Atlas{Image: image.NewNRGBA(image.Rect(0, 0, 32, 16)), CellW: 4, CellH: 1}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			a.Image.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	frames := []AnimationFrame{{Rect: &FrameRect{0, 0, 16, 16}, DurationMS: 200}}
	m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{.5, 1}, Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: frames}}}}
	if e := ValidateManifestPixels(m, a); e != nil {
		t.Fatal(e)
	}
	action := m.Actions["idle"]
	action.Loop = append(action.Loop, AnimationFrame{Rect: &FrameRect{16, 0, 16, 16}, DurationMS: 200})
	m.Actions["idle"] = action
	if e := ValidateManifestPixels(m, a); e == nil {
		t.Fatal("accepted blank custom frame")
	}
}
func TestInitialFrameHonorsAnchor(t *testing.T) {
	m := DefaultAnimationManifest()
	action := m.Actions["idle"]
	action.Loop[0].Anchor = &AnimationAnchor{.25, .75}
	m.Actions["idle"] = action
	frame := InitialBehaviorFrame(m)
	if frame.Anchor.X != .25 || frame.Anchor.Y != .75 {
		t.Fatal(frame.Anchor)
	}
}
func TestCharacterSummaryTruthfullyLabelsArt(t *testing.T) {
	summary := CharacterSummary([]CatSpec{{Name: "One", Sprite: "art.png"}, {Name: "Two", Demo: true}})
	if !strings.Contains(summary, "One（自訂素材）") || !strings.Contains(summary, "Two（示範外觀）") {
		t.Fatal(summary)
	}
}

func TestOpenLocalAssetBoundary(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "art.png")
	if e := os.WriteFile(inside, []byte("QA"), 0600); e != nil {
		t.Fatal(e)
	}
	f, e := OpenLocalAsset(root, "art.png")
	if e != nil {
		t.Fatal(e)
	}
	f.Close()
	if _, e = OpenLocalAsset(root, "../escape.png"); e == nil {
		t.Fatal("accepted traversal")
	}
	if _, e = OpenLocalAsset(root, "."); e == nil {
		t.Fatal("accepted directory")
	}
	outside := filepath.Join(t.TempDir(), "external.png")
	os.WriteFile(outside, []byte("QA"), 0600)
	if e = os.Symlink(outside, filepath.Join(root, "link.png")); e != nil {
		t.Log("symlink creation unavailable; direct path checks ran")
		return
	}
	if _, e = OpenLocalAsset(root, "link.png"); e == nil {
		t.Fatal("accepted escaping link")
	}
}

func TestGroundAnchorCannotSilentlyClipVisibleFeet(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			im.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	a := &Atlas{Image: im}
	frames := []AnimationFrame{{Rect: &FrameRect{0, 0, 8, 8}, DurationMS: 100}}
	m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{.5, .75}, Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: frames}}}}
	if e := ValidateManifestPixels(m, a); e == nil {
		t.Fatal("accepted clipping grounded artwork")
	}
	idleFrames := []AnimationFrame{{Rect: &FrameRect{0, 0, 8, 8}, Anchor: &AnimationAnchor{.5, 1}, DurationMS: 100}}
	m.Actions["idle"] = AnimationAction{AnimationClip: AnimationClip{Loop: idleFrames}}
	m.Actions["drag"] = AnimationAction{AnimationClip: AnimationClip{Loop: frames}}
	if e := ValidateManifestPixels(m, a); e != nil {
		t.Fatal("airborne drag should preserve full canvas:", e)
	}
	m.Actions["idle"] = AnimationAction{Fallback: "drag"}
	if e := ValidateManifestPixels(m, a); e == nil {
		t.Fatal("grounded alias clipped a drag fallback")
	}
}
