//go:build !windows && motionpreview

package main

import "testing"

func TestReviewPartialPreviewMatchesAuthoredDirections(t *testing.T) {
	for _, dirs := range [][]int{
		{0, 2, 4, 6, 8, 10, 12, 14, 15, 16, 18, 20, 22, 24, 26, 28, 30},
		{0, 1, 2, 4, 6, 8, 10, 12, 14, 15, 16, 18, 20, 22, 24, 26, 28, 30},
	} {
		e := reviewPartialFixture(t, 32, dirs)
		cat := previewCat{Manifest: e.States[0].Player.Manifest}
		// Use finer global samples to inspect both sides of partial-angle boundaries.
		for d := 0; d < 128; d++ {
			scene := previewScene{Action: "gaze_" + itoaDirection(d), Mood: "calm", GazeDirections: 128}
			want := "gaze_" + itoaDirection(reviewNearest(float64(d)/4, 32, dirs))
			if got := previewActionForCat(cat, scene); got != want {
				t.Fatalf("preview angle %.2f got %s, want %s", float64(d)/4, got, want)
			}
			if got := previewActionForSource(cat, scene).ResolvedAction; got != want {
				t.Fatalf("preview metadata angle %.2f got %s, want %s", float64(d)/4, got, want)
			}
		}
		literal := previewScene{Action: "gaze_3", Mood: "calm"}
		if got := previewActionForCat(cat, literal); got != literal.Action {
			t.Fatalf("literal action was silently changed: %s", got)
		}
	}
	e := reviewPartialFixture(t, 32, nil)
	cat := previewCat{Manifest: e.States[0].Player.Manifest}
	if got := previewActionForCat(cat, previewScene{Action: "gaze_1", Mood: "calm", GazeDirections: 32}); got != "idle" {
		t.Fatalf("preview without direct art is %s", got)
	}
}
