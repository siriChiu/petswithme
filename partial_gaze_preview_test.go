//go:build !windows && motionpreview

package main

import "testing"

func TestPreviewPartialGazeUsesActualAngleRatherThanAliasBins(t *testing.T) {
	e := partialGazeFixture(15)
	cat := previewCat{Manifest: e.Manifest}
	for _, tc := range []struct {
		d    int
		want string
	}{{6, "gaze_0"}, {30, "gaze_2"}, {164, "gaze_15"}, {359, "gaze_0"}} {
		scene := previewScene{Action: "gaze_" + itoaDirection(tc.d), Mood: "calm", GazeDirections: 360}
		if got := previewActionForCat(cat, scene); got != tc.want {
			t.Fatal("preview angle mapped incorrectly", tc.d, got, tc.want)
		}
	}
	// An explicitly requested action remains a literal catalog inspection.
	if got := previewActionForCat(cat, previewScene{Action: "gaze_1", Mood: "calm"}); got != "gaze_1" {
		t.Fatal("literal action request changed", got)
	}
}
