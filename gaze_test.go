package main

import (
	"math"
	"testing"
)

func gazeFixture() *BehaviorEngine {
	e := NewBehaviorEngine(behaviorCats(1), DefaultAnimationManifest())
	e.States[0].NextDecision = 1e9
	e.Social.NextAttempt = 1e9
	return e
}
func TestGazeAllDirectionsGlobalCoordinatesAndSizes(t *testing.T) {
	for _, origin := range [][2]float64{{0, 0}, {-1800, -900}, {2400, 1200}} {
		for _, size := range []int{96, 144, 192} {
			e := gazeFixture()
			c := e.Cats[0]
			c.X, c.Y = origin[0], origin[1]
			c.W, c.H = size, size
			ox, oy := GazeOrigin(c)
			for d := 0; d < 16; d++ {
				angle := float64(d) * math.Pi / 8
				// An actual pointer can be anywhere on the virtual desktop, not only within 260px.
				f := e.Tick(float64(d), .05, ox+1600*math.Sin(angle), oy-1600*math.Cos(angle), 0, false)[0]
				if f.Action != "gaze_"+itoaDirection(d) {
					t.Fatalf("origin%v size%d direction%d: %s", origin, size, d, f.Action)
				}
				if c.X != origin[0] || c.Y != origin[1] {
					t.Fatal("gaze moved the cat")
				}
			}
		}
	}
}
func TestGazeContinuousQuietAndNoDirectionFlicker(t *testing.T) {
	e := gazeFixture()
	ox, oy := GazeOrigin(e.Cats[0])
	for n := 0; n < 160; n++ {
		if f := e.Tick(float64(n)*.25, .25, ox+1000, oy, 0, true)[0]; f.Action != "gaze_4" {
			t.Fatalf("quiet tracking stopped: %s", f.Action)
		}
	}
	// Select up, then hover just beyond its ordinary sector boundary.
	e.Tick(41, .05, ox, oy-1000, 0, true)
	a := .55 * math.Pi / 8
	if f := e.Tick(42, .05, ox+1000*math.Sin(a), oy-1000*math.Cos(a), 0, true)[0]; f.Action != "gaze_0" {
		t.Fatal("boundary tremor flickered")
	}
	a = .75 * math.Pi / 8
	if f := e.Tick(43, .05, ox+1000*math.Sin(a), oy-1000*math.Cos(a), 0, true)[0]; f.Action != "gaze_1" {
		t.Fatal("hysteresis prevented a real change")
	}
}
func TestGazeMissingInvalidAndDeadZone(t *testing.T) {
	e := gazeFixture()
	ox, oy := GazeOrigin(e.Cats[0])
	for _, p := range [][2]float64{{ox, oy}, {math.NaN(), 0}, {0, math.Inf(1)}} {
		if f := e.Tick(0, .05, p[0], p[1], 0, false)[0]; f.Action != "idle" {
			t.Fatalf("invalid or centered pointer %v: %s", p, f.Action)
		}
	}
	m := e.States[0].Player.Manifest
	m.Actions["gaze_4"] = AnimationAction{Fallback: "idle", DemoFallback: true}
	if f := e.Tick(1, .05, ox+1000, oy, 0, false)[0]; f.Action != "idle" {
		t.Fatal("idle alias pretended to track")
	}
	m.Actions["gaze_4"] = AnimationAction{Fallback: "gaze_3"}
	if f := e.Tick(2, .05, ox+1000, oy, 0, false)[0]; f.Action != "gaze_4" || f.Row != 9 || f.Col != 3 {
		t.Fatal("authored neighboring gaze fallback failed")
	}
}
func TestGazeRespectsInteractionsSleepAndCPU(t *testing.T) {
	e := gazeFixture()
	ox, oy := GazeOrigin(e.Cats[0])
	e.Pet(0, 0)
	if f := e.Tick(.1, .05, ox+1000, oy, 0, true)[0]; f.Action != "pet" {
		t.Fatal("gaze interrupted pet")
	}
	e.Cats[0].Dragging = true
	if f := e.Tick(.2, .05, ox-1000, oy, 0, true)[0]; f.Action != "drag" {
		t.Fatal("gaze interrupted drag")
	}
	e.Cats[0].Dragging = false
	if f := e.Tick(.3, .05, ox-1000, oy, 0, true)[0]; f.Action != "gaze_12" {
		t.Fatalf("release did not restore quiet gaze: %s", f.Action)
	}
	if f := e.Tick(.4, .05, ox-1000, oy, 200, true)[0]; f.Action == "gaze_12" {
		t.Fatal("inactive sleep was forced to stare")
	}
	e = gazeFixture()
	e.SetManifest(0, cpuFixture())
	e.Load.Active = true
	e.Load.last = 1
	if f := e.Tick(1, .05, ox+1000, oy, 0, false)[0]; f.Action != "knead" {
		t.Fatalf("CPU action lost priority: %s", f.Action)
	}
}

func TestGazeOriginPreservesHeadWhenCanvasPaddingChanges(t *testing.T) {
	e := gazeFixture()
	c := e.Cats[0]
	c.W, c.H = 192, 173
	c.X, c.Y = -960, 200
	m := e.States[0].Player.Manifest
	m.GazeOrigin = &AnimationAnchor{.48, .43}
	ox, oy := ManifestGazeOrigin(c, m)
	for d := 0; d < 16; d++ {
		a := float64(d) * math.Pi / 8
		f := e.Tick(float64(d), .05, ox+30*math.Sin(a), oy-30*math.Cos(a), 0, true)[0]
		if f.Action != "gaze_"+itoaDirection(d) {
			t.Fatalf("offset head direction%d: %s", d, f.Action)
		}
	}
	// Explicit point survives source JSON; out-of-range/NaN points are rejected.
	if err := m.Validate(1024, 1408); err != nil {
		t.Fatal(err)
	}
	for _, p := range []AnimationAnchor{{-.1, .5}, {.5, 1.1}, {math.NaN(), .5}} {
		m.GazeOrigin = &p
		if m.Validate(1024, 1408) == nil {
			t.Fatalf("invalid head point accepted:%v", p)
		}
	}
}

func TestGazeVariableCountsAndBackwardCompatibility(t *testing.T) {
	for _, count := range []int{4, 8, 16, 32} {
		e := gazeFixture()
		m := e.Manifest
		m.GazeDirections = count
		for d := 0; d < count; d++ {
			m.Actions["gaze_"+itoaDirection(d)] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{d * 8, 0, 8, 8}, DurationMS: 200}}}}
		}
		if err := m.Validate(256, 88); err != nil {
			t.Fatal(err)
		}
		ox, oy := ManifestGazeOrigin(e.Cats[0], m)
		for d := 0; d < count; d++ {
			a := float64(d) * 2 * math.Pi / float64(count)
			f := e.Tick(float64(d), .05, ox+1000*math.Sin(a), oy-1000*math.Cos(a), 0, true)[0]
			if f.Action != "gaze_"+itoaDirection(d) || f.Rect.X != d*8 {
				t.Fatalf("count%d direction%d:%+v", count, d, f)
			}
		}
	}
	m := DefaultAnimationManifest()
	if ManifestGazeDirections(m) != 16 {
		t.Fatal("old manifests changed meaning")
	}
	for _, bad := range []int{-1, 2, 17, 64} {
		m.GazeDirections = bad
		if m.Validate(1024, 1408) == nil {
			t.Fatalf("bad count%d accepted", bad)
		}
	}
	if GazeDirectionCount(1, 0, 32) != 8 || GazeDirectionCount(0, 1, 32) != 16 || GazeDirectionCount(-1, 0, 32) != 24 {
		t.Fatal("32 cardinal mapping incorrect")
	}
}
