//go:build !windows && motionpreview

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func previewTestPack(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	im := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	// Synthetic QA colors, not cat artwork. The lower half is transparent.
	draw.Draw(im, image.Rect(0, 0, 8, 8), image.NewUniform(color.NRGBA{R: 200, G: 20, B: 40, A: 255}), image.Point{}, draw.Src)
	draw.Draw(im, image.Rect(8, 0, 16, 8), image.NewUniform(color.NRGBA{R: 20, G: 40, B: 200, A: 255}), image.Point{}, draw.Src)
	f, err := os.Create(filepath.Join(root, "qa.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, im); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	manifest := AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{X: .5, Y: 1}, Actions: map[string]AnimationAction{
		"idle":   {AnimationClip: AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{X: 0, Y: 0, W: 8, H: 8}, DurationMS: 100}, {Rect: &FrameRect{X: 8, Y: 0, W: 8, H: 8}, DurationMS: 100}}}},
		"gaze_0": {Fallback: "idle"},
		"gaze_1": {Fallback: "gaze_0"},
	}}
	previewTestWriteJSON(t, filepath.Join(root, "qa.json"), manifest)
	config := Config{Version: 1, Cats: []CatSpec{{Name: "QA one", Sprite: "qa.png", Animations: "qa.json"}, {Name: "QA 二", Sprite: "qa.png", Animations: "qa.json"}, {Name: "QA three", Sprite: "qa.png", Animations: "qa.json"}}}
	previewTestWriteJSON(t, filepath.Join(root, "cats.json"), config)
	return root
}

func previewTestWriteJSON(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}

func previewTestOptions(root, out string) previewOptions {
	return previewOptions{Root: root, Out: out, FPS: 10, Width: 8, ActionDuration: 200 * time.Millisecond, GazeDuration: 200 * time.Millisecond}
}

func TestPreviewRequiresPrivatePack(t *testing.T) {
	if _, err := loadPreviewCats(t.TempDir(), 144); err == nil || !strings.Contains(err.Error(), "cats.json is required") {
		t.Fatalf("missing config was not rejected: %v", err)
	}
	root := t.TempDir()
	previewTestWriteJSON(t, filepath.Join(root, "cats.json"), DefaultConfig())
	if _, err := loadPreviewCats(root, 144); err == nil || !strings.Contains(err.Error(), "no demo artwork is substituted") {
		t.Fatalf("demo config was not rejected: %v", err)
	}
	root = previewTestPack(t)
	if err := os.Remove(filepath.Join(root, "qa.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPreviewCats(root, 144); err == nil || !strings.Contains(err.Error(), "animation manifest") {
		t.Fatalf("missing manifest was not rejected: %v", err)
	}
}

func TestPreviewRejectsEmptyDeclaredPixels(t *testing.T) {
	root := previewTestPack(t)
	m := AnimationManifest{SchemaVersion: 1, Fallback: "idle", Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{X: 0, Y: 8, W: 8, H: 8}, DurationMS: 100}}}}}}
	previewTestWriteJSON(t, filepath.Join(root, "qa.json"), m)
	if _, err := loadPreviewCats(root, 144); err == nil || !strings.Contains(err.Error(), "visible pixels") {
		t.Fatalf("empty source was not rejected: %v", err)
	}
}

func TestPreviewRejectsPackSymlinkEscape(t *testing.T) {
	root := previewTestPack(t)
	outside := filepath.Join(t.TempDir(), "external.png")
	if err := os.Rename(filepath.Join(root, "qa.png"), outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "qa.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPreviewCats(root, 144); err == nil {
		t.Fatal("followed a sprite symlink outside the supplied pack")
	}
}

func TestPreviewScheduleAndFallbackMetadata(t *testing.T) {
	cats, err := loadPreviewCats(previewTestPack(t), 144)
	if err != nil {
		t.Fatal(err)
	}
	opts := previewOptions{FPS: 10, ActionDuration: 3 * time.Second, GazeDuration: 4800 * time.Millisecond}
	scenes := previewSchedule(opts, cats)
	if len(scenes) != 23 || scenes[0].StartFrame != 0 || scenes[len(scenes)-1].EndFrame != 258 {
		t.Fatalf("wrong default schedule: %#v", scenes)
	}
	if scenes[3].Action != "pet" || scenes[3].Mood != "happy" || scenes[5].Action != "play" || scenes[5].Mood != "playful" {
		t.Fatal("incorrect action/mood schedule")
	}
	for i, s := range scenes {
		if i > 0 && s.StartFrame != scenes[i-1].EndFrame {
			t.Fatal("gap or overlap in frame schedule")
		}
		if s.StartSeconds != float64(s.StartFrame)/10 || s.EndSeconds != float64(s.EndFrame)/10 {
			t.Fatal("time and frame schedule disagree")
		}
	}
	for direction, s := range scenes[7:] {
		if s.Action != fmt.Sprintf("gaze_%d", direction) || s.EndFrame-s.StartFrame != 3 {
			t.Fatalf("gaze direction missing or wrong duration: %#v", s)
		}
	}
	odd := scenes[8].Sources[0]
	if strings.Join(odd.FallbackChain, ",") != "gaze_1,gaze_0,idle" || odd.ResolvedAction != "idle" || odd.GlobalFallback {
		t.Fatalf("declared fallback not recorded faithfully: %#v", odd)
	}
	missing := scenes[1].Sources[0]
	if !missing.GlobalFallback || strings.Join(missing.FallbackChain, ",") != "walk_right,idle" {
		t.Fatalf("global fallback not recorded faithfully: %#v", missing)
	}
}

func TestPreviewPerFrameAnchorAndPremultiplication(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(im, image.Rect(0, 0, 8, 4), image.NewUniform(color.NRGBA{R: 200, G: 100, B: 50, A: 128}), image.Point{}, draw.Src)
	m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{X: .5, Y: 0}, Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{X: 0, Y: 0, W: 8, H: 4}, Anchor: &AnimationAnchor{X: .5, Y: 1}, DurationMS: 100}}}}}}
	cat := previewCat{Atlas: &Atlas{Image: im}, Manifest: m, Player: NewAnimationPlayer(m), Width: 8, Height: 8}
	rendered := previewImage(&cat)
	if got := rendered.RGBAAt(0, 0); got != (color.RGBA{}) {
		t.Fatalf("frame anchor override was ignored: top=%v", got)
	}
	if got := rendered.RGBAAt(0, 4); got != (color.RGBA{R: 100, G: 50, B: 25, A: 128}) {
		t.Fatalf("anchor or premultiplied channel conversion is wrong: %v", got)
	}
	background := image.NewRGBA(rendered.Bounds())
	draw.Draw(background, background.Bounds(), image.NewUniform(previewBackground), image.Point{}, draw.Src)
	draw.Draw(background, background.Bounds(), rendered, image.Point{}, draw.Over)
	got := background.RGBAAt(0, 4)
	if got.A != 255 || got.R < 220 || got.R > 222 || got.G < 168 || got.G > 170 || got.B < 139 || got.B > 141 {
		t.Fatalf("incorrect alpha composition: %v", got)
	}
}

func TestPreviewLegacyFrameCoordinates(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 64, 88))
	want := color.RGBA{R: 33, G: 66, B: 99, A: 255}
	draw.Draw(im, image.Rect(16, 8, 24, 16), image.NewUniform(want), image.Point{}, draw.Src)
	m := &AnimationManifest{Fallback: "idle", Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: []AnimationFrame{{Row: 1, Col: 2, DurationMS: 100}}}}}}
	cat := previewCat{Atlas: &Atlas{Image: im, CellW: 8, CellH: 8}, Manifest: m, Player: NewAnimationPlayer(m), Width: 8, Height: 8}
	if got := previewImage(&cat).RGBAAt(0, 0); got != want {
		t.Fatalf("row/column renderer mismatch: got %v, want %v", got, want)
	}
}

func TestPreviewRenderSequence(t *testing.T) {
	root, out := previewTestPack(t), filepath.Join(t.TempDir(), "frames")
	opts := previewTestOptions(root, out)
	meta, err := renderMotionPreview(opts)
	if err != nil {
		t.Fatal(err)
	}
	if meta.FrameCount != 30 || len(meta.Cats) != 3 || meta.Cats[1].Name != "QA 二" {
		t.Fatalf("unexpected sequence metadata: %#v", meta)
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != meta.FrameCount+1 {
		t.Fatalf("output count=%d, err=%v", len(entries), err)
	}
	for frame, want := range []color.RGBA{{R: 200, G: 20, B: 40, A: 255}, {R: 20, G: 40, B: 200, A: 255}} {
		f, err := os.Open(filepath.Join(out, fmt.Sprintf(previewFramePattern, frame)))
		if err != nil {
			t.Fatal(err)
		}
		im, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if im.Bounds() != image.Rect(0, 0, 900, 360) {
			t.Fatalf("unexpected canvas: %v", im.Bounds())
		}
		if got := color.RGBAModel.Convert(im.At(0, 0)).(color.RGBA); got != previewBackground {
			t.Fatalf("wrong neutral background: %v", got)
		}
		for i, cat := range meta.Cats {
			if got := color.RGBAModel.Convert(im.At(cat.Left, cat.Top)).(color.RGBA); got != want {
				t.Fatalf("frame %d cat %d: got %v, want %v", frame, i, got, want)
			}
		}
	}
	b, err := os.ReadFile(filepath.Join(out, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved previewMetadata
	if err := json.Unmarshal(b, &saved); err != nil || saved.FrameCount != meta.FrameCount || len(saved.Scenes) != 23 {
		t.Fatalf("bad saved metadata: %v", err)
	}
	// A shorter rerun must never leave stale PNGs or overwrite prior output.
	if _, err := renderMotionPreview(opts); err == nil || !strings.Contains(err.Error(), "will not be overwritten") {
		t.Fatalf("existing output was not protected: %v", err)
	}
	otherOut := filepath.Join(t.TempDir(), "second")
	opts.Out = otherOut
	if _, err := renderMotionPreview(opts); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		first, err := os.ReadFile(filepath.Join(out, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(otherOut, entry.Name()))
		if err != nil || !bytes.Equal(first, second) {
			t.Fatalf("output is not deterministic: %s, %v", entry.Name(), err)
		}
	}
}

func TestPreviewCLIValidation(t *testing.T) {
	var output bytes.Buffer
	if err := previewMain([]string{"--help"}, &output, &output); err != nil || !strings.Contains(output.String(), "--root PRIVATE_PACK") {
		t.Fatalf("help failed: %s %v", output.String(), err)
	}
	for _, args := range [][]string{{}, {"--fps", "0", "--root", "x", "--out", "y"}, {"--width", "281", "--root", "x", "--out", "y"}, {"--action-duration", "0s", "--root", "x", "--out", "y"}, {"--gaze-duration", "61s", "--root", "x", "--out", "y"}, {"unexpected"}} {
		if err := previewMain(args, &output, &output); err == nil {
			t.Fatalf("accepted invalid CLI arguments: %v", args)
		}
	}
}
