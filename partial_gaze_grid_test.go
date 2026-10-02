package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// Independent circular-distance reference checks for sparse direction grids.
func reviewPartialFixture(t *testing.T, count int, directions []int) *BehaviorEngine {
	t.Helper()
	e := gazeFixture()
	m := e.States[0].Player.Manifest
	for action := range m.Actions {
		if strings.HasPrefix(action, "gaze_") {
			delete(m.Actions, action)
		}
	}
	if err := json.Unmarshal([]byte(`{"gazeNearestAuthored":true}`), m); err != nil {
		t.Fatal(err)
	}
	m.GazeDirections = count
	for _, d := range directions {
		m.Actions["gaze_"+itoaDirection(d)] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{d * 8, 0, 8, 8}, DurationMS: 200}}}}
	}
	e.States[0].Mood = "calm"
	return e
}

func reviewPoint(e *BehaviorEngine, sector, radius float64) string {
	m := e.States[0].Player.Manifest
	ox, oy := ManifestGazeOrigin(e.Cats[0], m)
	a := sector * 2 * math.Pi / float64(ManifestGazeDirections(m))
	return e.pointerGaze(0, ox+radius*math.Sin(a), oy-radius*math.Cos(a))
}

func reviewNearest(sector float64, count int, dirs []int) int {
	want, best, bestClockwise := -1, math.Inf(1), math.Inf(1)
	for _, d := range dirs {
		cw := math.Mod(float64(d)-sector+float64(count)*2, float64(count))
		distance := math.Min(cw, float64(count)-cw)
		if distance < best-1e-10 || (math.Abs(distance-best) < 1e-10 && cw < bestClockwise) {
			want, best, bestClockwise = d, distance, cw
		}
	}
	return want
}

func TestReviewPartialGazeNearestAllGrids(t *testing.T) {
	for _, count := range []int{4, 8, 16, 32} {
		sets := [][]int{{0}, {0, count / 2}, {0, 1, count - 1}}
		evens := []int{}
		for i := 0; i < count; i += 2 {
			evens = append(evens, i)
		}
		sets = append(sets, evens)
		if count == 32 {
			sets = append(sets, append(append([]int{}, evens...), 15))
			sets = append(sets, append(append([]int{}, evens...), 1, 15))
		}
		for _, dirs := range sets {
			e := reviewPartialFixture(t, count, dirs)
			for step := 0; step < count*80; step++ {
				sector := float64(step) / 80
				e.States[0].GazeActive = false
				want := "gaze_" + itoaDirection(reviewNearest(sector, count, dirs))
				if got := reviewPoint(e, sector, 1000); got != want {
					t.Fatalf("grid %d authored %v sector %.8f: got %s, want %s", count, dirs, sector, got, want)
				}
			}
		}
	}
}

func TestReviewPartialGazeHysteresisAndWrap(t *testing.T) {
	dirs := []int{0, 2, 4, 6, 8, 10, 12, 14, 15, 16, 18, 20, 22, 24, 26, 28, 30}
	for _, steps := range [][]struct {
		sector float64
		want   int
	}{
		{{14, 14}, {14.64, 14}, {14.66, 15}, {14.36, 15}, {14.34, 14}},
		{{15, 15}, {15.64, 15}, {15.66, 16}, {15.36, 16}, {15.34, 15}},
		{{30, 30}, {31.29, 30}, {31.31, 0}, {30.71, 0}, {30.69, 30}},
		{{0, 0}, {1.29, 0}, {1.31, 2}, {.71, 2}, {.69, 0}},
	} {
		e := reviewPartialFixture(t, 32, dirs)
		for _, step := range steps {
			if got := reviewPoint(e, step.sector, 1000); got != "gaze_"+itoaDirection(step.want) {
				t.Fatalf("sector %.2f: got %s, want gaze_%d", step.sector, got, step.want)
			}
		}
	}
}

func TestReviewPartialGazeAliasesMoodsAndNoArtwork(t *testing.T) {
	e := reviewPartialFixture(t, 32, []int{0, 2})
	m, s := e.States[0].Player.Manifest, e.States[0]
	m.Actions["gaze_1"] = AnimationAction{Fallback: "gaze_0"}
	if got := reviewPoint(e, 1.4, 1000); got != "gaze_2" {
		t.Fatalf("alias displaced nearest direct art: %s", got)
	}
	a := m.Actions["gaze_1"]
	a.Moods = map[string]AnimationClip{"happy": {Loop: []AnimationFrame{{Rect: &FrameRect{8, 0, 8, 8}, DurationMS: 200}}}}
	m.Actions["gaze_1"] = a
	s.GazeActive, s.Mood = false, "happy"
	if got := reviewPoint(e, 1, 1000); got != "gaze_1" {
		t.Fatalf("direct mood art omitted: %s", got)
	}
	s.Mood = "calm"
	if got := reviewPoint(e, .8, 1000); got != "gaze_0" {
		t.Fatalf("unavailable previous mood art retained: %s", got)
	}
	a.DemoFallback = true
	m.Actions["gaze_1"] = a
	s.GazeActive, s.Mood = false, "happy"
	if got := reviewPoint(e, .8, 1000); got != "gaze_0" {
		t.Fatalf("demo mood art accepted: %s", got)
	}
	for name := range m.Actions {
		if strings.HasPrefix(name, "gaze_") {
			delete(m.Actions, name)
		}
	}
	m.Actions["gaze_0"] = AnimationAction{Fallback: "idle"}
	m.Actions["gaze_1"] = AnimationAction{Fallback: "gaze_0"}
	if got := reviewPoint(e, 1, 1000); got != "idle" || s.GazeActive {
		t.Fatalf("alias-only manifest pretended to track: %s, active=%v", got, s.GazeActive)
	}
}

func TestReviewPartialGazeRadialHysteresisAndInvalidCursor(t *testing.T) {
	e := reviewPartialFixture(t, 32, []int{0, 15, 16})
	e.Cats[0].W = 144
	for _, step := range []struct {
		radius float64
		want   string
	}{{9, "gaze_15"}, {7, "gaze_15"}, {5, "idle"}, {7, "idle"}, {9, "gaze_15"}} {
		if got := reviewPoint(e, 15, step.radius); got != step.want {
			t.Fatalf("radius %.1f: got %s, want %s", step.radius, got, step.want)
		}
	}
	for _, p := range [][2]float64{{math.NaN(), 0}, {0, math.Inf(1)}, {math.Inf(-1), 0}} {
		if got := e.pointerGaze(0, p[0], p[1]); got != "idle" || e.States[0].GazeActive {
			t.Fatalf("invalid cursor %v: %s", p, got)
		}
	}
}
