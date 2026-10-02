package main

import (
	"math"
	"testing"
)

func partialGazeFixture(extra ...int) *BehaviorEngine {
	e := gazeFixture()
	m := e.Manifest
	m.GazeDirections = 32
	m.GazeNearestAuthored = true
	present := map[int]bool{}
	for _, i := range extra {
		present[i] = true
	}
	for i := 0; i < 32; i++ {
		if i%2 == 0 || present[i] {
			m.Actions["gaze_"+itoaDirection(i)] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{i * 8, 0, 8, 8}, DurationMS: 200}}}}
		} else {
			m.Actions["gaze_"+itoaDirection(i)] = AnimationAction{Fallback: "gaze_" + itoaDirection((i+1)%32)}
		}
	}
	return e
}
func partialGazeAt(e *BehaviorEngine, degrees float64) BehaviorFrame {
	ox, oy := ManifestGazeOrigin(e.Cats[0], e.Manifest)
	a := degrees * math.Pi / 180
	return e.Tick(1, .05, ox+1000*math.Sin(a), oy-1000*math.Cos(a), 0, false)[0]
}
func TestPartialGazeChoosesGenuineNearestAngles(t *testing.T) {
	for _, tc := range []struct {
		degrees float64
		want    int
	}{{0, 0}, {6, 0}, {12, 2}, {30, 2}, {155, 14}, {164, 15}, {173, 15}, {175, 16}, {349, 0}, {359, 0}, {-1, 0}} {
		e := partialGazeFixture(15)
		f := partialGazeAt(e, tc.degrees)
		if f.Action != "gaze_"+itoaDirection(tc.want) || f.Rect.X != tc.want*8 {
			t.Fatalf("angle%g selected%s source%+v, want%d", tc.degrees, f.Action, f.Rect, tc.want)
		}
	}
}
func TestPartialGazeHysteresisUsesActualNeighborSpacing(t *testing.T) {
	e := partialGazeFixture(15)
	for _, tc := range []struct {
		degrees float64
		want    int
	}{{0, 0}, {14.4, 0}, {15, 2}, {157.5, 14}, {164.5, 14}, {165, 15}, {175.5, 15}, {177, 16}, {168, 15}, {160, 14}, {337.5, 30}, {351.5, 30}, {353, 0}, {348, 0}, {345, 30}} {
		f := partialGazeAt(e, tc.degrees)
		if f.Action != "gaze_"+itoaDirection(tc.want) {
			t.Fatalf("angle%g selected%s want%d", tc.degrees, f.Action, tc.want)
		}
	}
}
func TestPartialGazeWithoutNewArtMatchesLegacySixteenSweep(t *testing.T) {
	old, partial := gazeFixture(), partialGazeFixture()
	for i := 0; i < 16; i++ {
		old.Manifest.Actions["gaze_"+itoaDirection(i)] = partial.Manifest.Actions["gaze_"+itoaDirection(i*2)]
	}
	for _, sign := range []float64{1, -1} {
		for d := 0; d <= 360; d++ {
			a, b := partialGazeAt(old, sign*float64(d)), partialGazeAt(partial, sign*float64(d))
			if a.Rect == nil || b.Rect == nil || *a.Rect != *b.Rect {
				t.Fatalf("angle%g changed old image selection: %s/%s", sign*float64(d), a.Action, b.Action)
			}
		}
	}
}
func TestPartialGazeAliasesAndMissingMoodDoNotCreateDirections(t *testing.T) {
	e := partialGazeFixture(15)
	a := e.Manifest.Actions["gaze_15"]
	e.Manifest.Actions["gaze_15"] = AnimationAction{Moods: map[string]AnimationClip{"happy": a.AnimationClip}}
	if f := partialGazeAt(e, 168.75); f.Action == "gaze_15" {
		t.Fatal("unavailable mood counted as genuine direction")
	}
	e.SetMood(0, "happy")
	e.States[0].GazeActive = false
	if f := partialGazeAt(e, 168.75); f.Action != "gaze_15" {
		t.Fatal("genuine mood-specific direction unavailable")
	}
	for i := 0; i < 32; i++ {
		e.Manifest.Actions["gaze_"+itoaDirection(i)] = AnimationAction{Fallback: "gaze_0"}
	}
	e.States[0].GazeActive = false
	if f := partialGazeAt(e, 90); f.Action != "idle" {
		t.Fatal("alias-only set pretended to have real art")
	}
}
