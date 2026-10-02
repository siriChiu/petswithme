package main

import (
	"math"
	"testing"
)

// Synthetic references deliberately isolate movement decisions from private art.
func flickerSocialManifest(speed float64, entry bool) *AnimationManifest {
	m := DefaultAnimationManifest()
	for _, name := range []string{"walk_left", "walk_right"} {
		a := m.Actions[name]
		duration := clipSeconds(a.Loop)
		a.Movement = &AnimationMovement{StrideRatio: speed * duration / 100, Verified: true}
		if entry {
			a.Start = []AnimationFrame{{Row: 0, Col: 0, DurationMS: 100}, {Row: 0, Col: 1, DurationMS: 100}}
		}
		m.Actions[name] = a
	}
	return m
}

func flickerSocialEngine(count int) *BehaviorEngine {
	bounds := Rect{0, 0, 4000, 1000}
	cats := make([]*Cat, count)
	for i := range cats {
		cats[i] = NewCat(i, 100, 100, bounds)
		cats[i].X, cats[i].Y = float64(100+i*112), 800
	}
	e := NewBehaviorEngine(cats, flickerSocialManifest(15, false))
	e.Social.NextAttempt = 1000
	for _, s := range e.States {
		s.NextDecision = 1000
	}
	return e
}

func flickerSetFollow(e *BehaviorEngine, a, b int, destination float64) {
	e.Social.Plans[1] = &SocialPlan{ID: 1, A: a, B: b, Leader: b, Phase: "follow", Started: 0, PhaseUntil: 20, Destination: destination, Bounds: e.Cats[a].Bounds}
	e.Social.Reservations[a], e.Social.Reservations[b] = 1, 1
}

func TestSocialDoesNotPairThroughAnInterveningCat(t *testing.T) {
	e := flickerSocialEngine(3)
	e.configureSocial()
	if e.Social.Start(e.Cats, 0, 2, 0) {
		t.Fatal("social plan paired the two outer cats across the middle cat")
	}
	// A cat on another vertical lane does not physically obstruct the pair.
	e.Cats[1].Y = 600
	if !e.Social.Start(e.Cats, 0, 2, 0) {
		t.Fatal("a clear route should still permit a social plan")
	}
}

func TestSocialBlockedFollowerKeepsIdleAndResumesAfterClear(t *testing.T) {
	e := flickerSocialEngine(3)
	e.Cats[0].X, e.Cats[1].X, e.Cats[2].X = 100, 206, 430
	// An existing plan can become blocked after a third cat moves into its path.
	flickerSetFollow(e, 0, 2, 430)
	const dt = .05
	for n := 0; n < 40; n++ {
		f := e.Tick(float64(n)*dt, dt, math.NaN(), math.NaN(), 0, false)[0]
		if math.Abs(e.Cats[0].X-100) > .001 {
			t.Fatal("blocked follower crossed its neighbor", e.Cats[0].X)
		}
		if n >= 10 && f.Action != "idle" {
			t.Fatalf("blocked follower restarted %q at %.2fs instead of holding idle", f.Action, float64(n)*dt)
		}
	}
	e.Cats[1].Y = 600
	sawMotion := false
	for n := 40; n < 80; n++ {
		f := e.Tick(float64(n)*dt, dt, math.NaN(), math.NaN(), 0, false)[0]
		sawMotion = sawMotion || f.Action == "walk_right"
	}
	if !sawMotion || e.Cats[0].X <= 110 {
		t.Fatal("follower did not resume once its route cleared", e.Cats[0].X)
	}
}

func TestSocialFasterFollowerAvoidsBriefIdleFlashes(t *testing.T) {
	for _, dt := range []float64{.05, .02} {
		e := flickerSocialEngine(2)
		e.SetManifest(0, flickerSocialManifest(15, false))
		e.SetManifest(1, flickerSocialManifest(10, false))
		flickerSetFollow(e, 0, 1, 3000)
		last, lastAt := "", 0.0
		changes := 0
		for n := 0; n < int(5/dt); n++ {
			now := float64(n) * dt
			f := e.Tick(now, dt, math.NaN(), math.NaN(), 0, false)[0]
			if f.Action == last {
				continue
			}
			if changes > 0 && now-lastAt < .2-1e-8 {
				t.Fatalf("dt=%.0fms: follower displayed %q for only %.0fms before %q", dt*1000, last, (now-lastAt)*1000, f.Action)
			}
			if last != "" {
				changes++
			}
			last, lastAt = f.Action, now
		}
		if e.Cats[0].X <= 120 {
			t.Fatal("anti-flicker behavior froze the follower", e.Cats[0].X)
		}
	}
}

func TestSocialEntryPosesReachMovementWithoutRestart(t *testing.T) {
	e := flickerSocialEngine(2)
	e.Cats[1].X = 260
	e.SetManifest(0, flickerSocialManifest(15, true))
	e.SetManifest(1, flickerSocialManifest(15, true))
	flickerSetFollow(e, 0, 1, 3000)
	generation := uint64(0)
	for n := 0; n < 20; n++ {
		now := float64(n) * .05
		f := e.Tick(now, .05, math.NaN(), math.NaN(), 0, false)[0]
		if f.Action != "walk_right" {
			t.Fatalf("entry interrupted by %q at %.2fs", f.Action, now)
		}
		if n == 0 {
			generation = f.Generation
		}
		if f.Generation != generation {
			t.Fatal("stationary entry was restarted before movement", f.Generation, generation)
		}
		if n < 4 && math.Abs(e.Cats[0].X-100) > .001 {
			t.Fatal("entry pose moved before its authored duration elapsed")
		}
	}
	if e.Cats[0].X <= 108 {
		t.Fatal("entry never completed into movement", e.Cats[0].X)
	}
}

// A timer landing a nanosecond beyond an entry boundary produces a valid,
// positive movement smaller than .01px. It is not evidence of collision.
func TestSocialTinyMovementAtEntryBoundaryKeepsAnimation(t *testing.T) {
	e := flickerSocialEngine(2)
	e.Cats[1].X = 260
	m := flickerSocialManifest(15, false)
	for _, name := range []string{"walk_left", "walk_right"} {
		a := m.Actions[name]
		a.Start = []AnimationFrame{{Row: 0, Col: 0, DurationMS: 120}}
		m.Actions[name] = a
	}
	e.SetManifest(0, m)
	flickerSetFollow(e, 0, 1, 3000)
	first := e.Tick(0, 0, math.NaN(), math.NaN(), 0, false)[0]
	e.Tick(.1, .1, math.NaN(), math.NaN(), 0, false)
	before := e.Cats[0].X
	boundary := e.Tick(.120000001, .020000001, math.NaN(), math.NaN(), 0, false)[0]
	moved := e.Cats[0].X - before
	if moved <= 0 || moved >= .01 {
		t.Fatalf("fixture did not exercise a tiny positive movement: %.12f", moved)
	}
	if boundary.Action != "walk_right" || boundary.Generation != first.Generation || e.States[0].Player.Phase != AnimationLoop {
		t.Fatalf("valid entry-to-loop movement was treated as blocked: action=%q generation=%d initial=%d phase=%v", boundary.Action, boundary.Generation, first.Generation, e.States[0].Player.Phase)
	}
	next := e.Tick(.17, .049999999, math.NaN(), math.NaN(), 0, false)[0]
	if next.Generation != first.Generation || e.Cats[0].X <= before+.5 {
		t.Fatal("locomotion restarted or failed to continue after the boundary", next.Generation, e.Cats[0].X)
	}
}
