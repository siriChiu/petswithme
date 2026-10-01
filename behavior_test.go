package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func animationFixture() *AnimationManifest {
	return &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{.5, 1}, Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Start: []AnimationFrame{{Row: 0, Col: 0, DurationMS: 100}}, Loop: []AnimationFrame{{Row: 0, Col: 1, DurationMS: 100}, {Row: 0, Col: 2, DurationMS: 200}}, End: []AnimationFrame{{Row: 0, Col: 3, DurationMS: 100}}}}}}
}
func behaviorCats(n int) []*Cat {
	cats := make([]*Cat, n)
	for i := range cats {
		cats[i] = NewCat(i, 100, 100, Rect{0, 0, 1200, 800})
		cats[i].X = 100 + float64(i)*220
		cats[i].Y = 700
	}
	return cats
}
func quietCursor(e *BehaviorEngine, now, dt float64, quiet bool) []BehaviorFrame {
	return e.Tick(now, dt, -10000, -10000, 0, quiet)
}

func TestBehaviorDefaultManifestUsesExistingFrames(t *testing.T) {
	m := DefaultAnimationManifest()
	if err := m.Validate(1024, 1408); err != nil {
		t.Fatal(err)
	}
	for name := range m.Actions {
		clip := m.Resolve(name, "happy")
		if len(clip.Loop) == 0 {
			t.Fatalf("%s has no drawable fallback", name)
		}
		for _, f := range clip.Loop {
			if f.Row < 0 || f.Row >= 11 || f.Col < 0 || f.Col >= rowFrames[f.Row] {
				t.Fatalf("%s invents frame %+v", name, f)
			}
		}
	}
	if !m.Actions["groom"].DemoFallback {
		t.Fatal("groom reused art should be marked as a fallback")
	}
}
func TestBehaviorManifestRoundTripAndRect(t *testing.T) {
	m := animationFixture()
	m.Actions["rect"] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{2, 3, 7, 11}, Anchor: &AnimationAnchor{.3, .9}, DurationMS: 175}}}}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAnimationManifest(strings.NewReader(string(b)), 80, 110)
	if err != nil {
		t.Fatal(err)
	}
	f := loaded.Resolve("rect", "calm").Loop[0]
	if f.Rect.W != 7 || f.DurationMS != 175 || f.Anchor.X != .3 {
		t.Fatalf("lost manifest fields: %+v", f)
	}
}
func TestBehaviorManifestRejectsInvalid(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AnimationManifest)
	}{{"version", func(m *AnimationManifest) { m.SchemaVersion = 3 }}, {"global fallback", func(m *AnimationManifest) { m.Fallback = "missing" }}, {"cycle", func(m *AnimationManifest) {
		m.Actions["x"] = AnimationAction{Fallback: "y"}
		m.Actions["y"] = AnimationAction{Fallback: "x"}
	}}, {"unknown fallback", func(m *AnimationManifest) { m.Actions["x"] = AnimationAction{Fallback: "missing"} }}, {"duration", func(m *AnimationManifest) { a := m.Actions["idle"]; a.Loop[0].DurationMS = 0; m.Actions["idle"] = a }}, {"frame", func(m *AnimationManifest) { a := m.Actions["idle"]; a.Loop[0].Row = 11; m.Actions["idle"] = a }}, {"rect", func(m *AnimationManifest) {
		a := m.Actions["idle"]
		a.Loop[0].Rect = &FrameRect{79, 0, 2, 2}
		m.Actions["idle"] = a
	}}, {"anchor", func(m *AnimationManifest) { m.Anchor.X = math.NaN() }}, {"empty action", func(m *AnimationManifest) { m.Actions["empty"] = AnimationAction{} }}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := animationFixture()
			tt.mutate(m)
			if m.Validate(80, 110) == nil {
				t.Fatal("invalid manifest was accepted")
			}
		})
	}
	for _, data := range []string{`{}`, `{"schemaVersion":1,"unexpected":1}`, `{} {}`, strings.Repeat(" ", 1024*1024+1)} {
		if _, err := LoadAnimationManifest(strings.NewReader(data), 80, 110); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}
func TestBehaviorManifestFallbackAndMood(t *testing.T) {
	m := animationFixture()
	m.Actions["alias"] = AnimationAction{Fallback: "idle", Moods: map[string]AnimationClip{"happy": {Loop: []AnimationFrame{{Row: 3, Col: 1, DurationMS: 20}}}}}
	if got := m.Resolve("alias", "happy").Loop[0]; got.Row != 3 {
		t.Fatal("mood did not win")
	}
	if got := m.Resolve("alias", "calm").Loop[0]; got.Col != 1 {
		t.Fatal("action fallback missing")
	}
	if got := m.Resolve("absent", "calm").Loop[0]; got.Col != 1 {
		t.Fatal("global fallback missing")
	}
}
func TestBehaviorPlayerPhasesAndDurations(t *testing.T) {
	p := NewAnimationPlayer(animationFixture())
	if p.Phase != AnimationStart {
		t.Fatal("missing start")
	}
	f, done := p.Tick(.1)
	if done || p.Phase != AnimationLoop || f.Col != 1 {
		t.Fatalf("start transition: %+v %v", f, p.Phase)
	}
	f, _ = p.Tick(.1)
	if f.Col != 2 {
		t.Fatal("per-frame duration not respected")
	}
	f, _ = p.Tick(.19)
	if f.Col != 2 {
		t.Fatal("long frame cut short")
	}
	f, _ = p.Tick(.011)
	if f.Col != 1 {
		t.Fatal("loop failed")
	}
	p.Stop()
	f, done = p.Tick(.1)
	if !done || f.Col != 3 || p.Phase != AnimationDone {
		t.Fatal("end completion missing")
	}
	_, done = p.Tick(1)
	if done {
		t.Fatal("completion reported twice")
	}
}
func TestBehaviorIndependentPlayersAndInterrupt(t *testing.T) {
	m := animationFixture()
	a, b := NewAnimationPlayer(m), NewAnimationPlayer(m)
	a.Tick(.25)
	if b.Index != 0 || b.Phase != AnimationStart {
		t.Fatal("players share a cursor")
	}
	generation := a.Generation
	a.Stop()
	a.Play("idle", "happy")
	if a.Generation <= generation {
		t.Fatal("generation did not invalidate canceled completion")
	}
	_, completed := a.Tick(.01)
	if completed {
		t.Fatal("stale completion delivered after replacement")
	}
	if a.Mood != "happy" || b.Mood != "calm" {
		t.Fatal("players share mood")
	}
}
func TestBehaviorPlayerSkipsHugeTimeAndBadDelta(t *testing.T) {
	p := NewAnimationPlayer(animationFixture())
	p.Tick(1e12)
	if p.Phase != AnimationLoop {
		t.Fatal("large tick failed")
	}
	before := p.Frame()
	p.Tick(math.NaN())
	p.Tick(-3)
	if p.Frame() != before {
		t.Fatal("invalid dt advanced player")
	}
}
func TestBehaviorPerCatManifestAndGazeFallback(t *testing.T) {
	cats := behaviorCats(2)
	e := NewBehaviorEngine(cats, nil)
	custom := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{.2, .8}, Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{0, 0, 32, 32}, DurationMS: 100}}}}}}
	if !e.SetManifest(0, custom) {
		t.Fatal("could not assign manifest")
	}
	out := e.Tick(1, .1, cats[0].X+50, cats[0].Y-20, 0, false)
	if out[0].Rect == nil || out[0].Anchor.X != .2 {
		t.Fatal("custom gaze failed to fall back or lost its anchor")
	}
	if out[1].Rect != nil {
		t.Fatal("custom manifest leaked to second cat")
	}
}
func TestBehaviorPriorityAndQuiet(t *testing.T) {
	e := NewBehaviorEngine(behaviorCats(1), nil)
	x := e.Cats[0].X
	out := quietCursor(e, 100, .1, true)
	if out[0].Action != "sleep" || e.Cats[0].X != x {
		t.Fatal("quiet did not stop autonomous movement")
	}
	e.Play(0, 101)
	out = quietCursor(e, 101, .1, true)
	if out[0].Action != "play" {
		t.Fatal("quiet blocked explicit play")
	}
	e.Pet(0, 102)
	if e.Request(0, "play", PriorityPlay, 102, 2) {
		t.Fatal("play interrupted pet")
	}
	out = quietCursor(e, 102, .1, true)
	if out[0].Action != "pet" {
		t.Fatal("pet did not override play")
	}
	e.Cats[0].Dragging = true
	out = quietCursor(e, 102.1, .1, true)
	if out[0].Action != "drag" {
		t.Fatal("drag not highest priority")
	}
	e.Pet(0, 102.2)
	out = quietCursor(e, 102.2, .1, true)
	if out[0].Action != "drag" {
		t.Fatal("pet interrupted drag")
	}
	e.Cats[0].Dragging = false
	out = quietCursor(e, 103, .1, true)
	if out[0].Action != "sleep" {
		t.Fatal("stale pet/play resumed after drag")
	}
}
func TestBehaviorExplicitActionUsesEndPhase(t *testing.T) {
	e := NewBehaviorEngine(behaviorCats(1), animationFixture())
	e.Request(0, "idle", PriorityPlay, 0, 1)
	quietCursor(e, 0, .1, false)
	out := quietCursor(e, 1, .025, false)
	if e.States[0].Player.Phase != AnimationEnd || out[0].Col != 3 {
		t.Fatal("engine skipped end phase")
	}
	quietCursor(e, 1.2, .1, false)
	quietCursor(e, 1.3, .1, false)
	if e.States[0].Priority != PriorityIdle {
		t.Fatal("completed explicit action retained priority")
	}
}
func TestBehaviorSocialReservationsAndCancellation(t *testing.T) {
	cats := behaviorCats(3)
	s := NewSocialCoordinator()
	if !s.Start(cats, 0, 1, 0) {
		t.Fatal("pair could not start")
	}
	if s.Start(cats, 1, 2, 0) {
		t.Fatal("cat reserved twice")
	}
	s.Cancel(0, 1)
	if len(s.Reservations) != 0 || len(s.Plans) != 0 {
		t.Fatal("both participants not released")
	}
	if s.Start(cats, 0, 1, 2) {
		t.Fatal("cooldown ignored")
	}
	if !s.Start(cats, 0, 1, 40) {
		t.Fatal("cooldown never ended")
	}
	cats[1].Dragging = true
	s.Tick(cats, nil, 41, .1, false)
	if len(s.Reservations) != 0 {
		t.Fatal("drag did not cancel pair")
	}
}
func TestBehaviorSocialRejectsOtherMonitorAndDistantCats(t *testing.T) {
	cats := behaviorCats(2)
	s := NewSocialCoordinator()
	cats[1].Bounds = Rect{1200, 0, 2400, 800}
	if s.Start(cats, 0, 1, 0) {
		t.Fatal("cross-monitor social accepted")
	}
	cats[1].Bounds = cats[0].Bounds
	cats[1].X = 1000
	if s.Start(cats, 0, 1, 0) {
		t.Fatal("distant social accepted")
	}
	cats[1].X = 300
	cats[1].Y = 400
	if s.Start(cats, 0, 1, 0) {
		t.Fatal("different floor social accepted")
	}
}
func TestBehaviorSocialFlowAndCollision(t *testing.T) {
	cats := behaviorCats(2)
	e := NewBehaviorEngine(cats, nil)
	e.Social.NextAttempt = 1000
	if !e.Social.Start(cats, 0, 1, 0) {
		t.Fatal("cannot start")
	}
	seen := map[string]bool{}
	for n := 0; n < 250; n++ {
		now := float64(n) * .1
		quietCursor(e, now, .1, false)
		for _, p := range e.Social.Plans {
			seen[p.Phase] = true
		}
		if cats[0].X+float64(cats[0].W) > cats[1].X+.001 {
			t.Fatalf("social cats overlap at t=%g", now)
		}
	}
	for _, phase := range []string{"approach", "greet", "follow", "rest"} {
		if !seen[phase] {
			t.Errorf("missing social phase %s", phase)
		}
	}
	if len(e.Social.Plans) != 0 {
		t.Fatal("interaction failed to finish")
	}
}
func TestBehaviorSocialPartnerStopsAfterPetOrQuiet(t *testing.T) {
	for _, cause := range []string{"pet", "quiet", "drag", "monitor"} {
		t.Run(cause, func(t *testing.T) {
			cats := behaviorCats(2)
			e := NewBehaviorEngine(cats, nil)
			e.Social.Start(cats, 0, 1, 0)
			quietCursor(e, 0, .1, false)
			quiet := false
			switch cause {
			case "pet":
				e.Pet(0, 1)
			case "quiet":
				quiet = true
			case "drag":
				cats[0].Dragging = true
			case "monitor":
				cats[0].Bounds = Rect{-1200, 0, 0, 800}
			}
			out := quietCursor(e, 1, .1, quiet)
			if len(e.Social.Reservations) > 0 {
				t.Fatal("stale reservation")
			}
			if out[1].Action != "idle" && out[1].Action != "sleep" {
				t.Fatalf("partner continued after cancellation: %s", out[1].Action)
			}
		})
	}
}
func TestBehaviorSweptCollisionAndMonitorBounds(t *testing.T) {
	cats := behaviorCats(3)
	e := NewBehaviorEngine(cats, nil)
	e.moveToward(0, 10000, 700, 10000)
	if cats[0].X+100 > cats[1].X {
		t.Fatal("large movement tunneled through a cat")
	}
	e.moveToward(2, -10000, 700, 10000)
	if cats[2].X < cats[1].X+100 {
		t.Fatal("left movement tunneled through a cat")
	}
	cats[1].Bounds = Rect{-1200, 0, 0, 800}
	cats[2].Bounds = cats[1].Bounds
	e.moveToward(0, -10000, 700, 10000)
	if cats[0].X < 0 {
		t.Fatal("wander escaped monitor")
	}
}
func TestBehaviorLongSimulationRemainsBounded(t *testing.T) {
	e := NewBehaviorEngine(behaviorCats(3), nil)
	for step := 0; step < 6000; step++ {
		now := float64(step) * .1
		if step%307 == 0 {
			e.Pet(step%3, now)
		}
		if step%419 == 0 {
			e.Play(step%3, now)
		}
		quietCursor(e, now, .1, step%1000 > 950)
		for i, c := range e.Cats {
			if !finite(c.X) || !finite(c.Y) || c.X < float64(c.Bounds.Left) || c.X > float64(c.Bounds.Right-c.W) || c.Y < float64(c.Bounds.Top) || c.Y > float64(c.Bounds.Bottom-c.H) {
				t.Fatalf("cat %d out of bounds at %f", i, now)
			}
		}
		if len(e.Social.Reservations) > 2 {
			t.Fatal("three cats reserved in overlapping pairs")
		}
	}
}

func TestBehaviorFallbackPreservesSpecificEntryExit(t *testing.T) {
	m := animationFixture()
	m.Actions["alias"] = AnimationAction{Fallback: "idle", AnimationClip: AnimationClip{Start: []AnimationFrame{{Row: 4, Col: 0, DurationMS: 100}}, End: []AnimationFrame{{Row: 4, Col: 4, DurationMS: 100}}}}
	clip := m.Resolve("alias", "calm")
	if clip.Start[0].Row != 4 || clip.Loop[0].Row != 0 || clip.End[0].Col != 4 {
		t.Fatal("fallback lost specific entry or exit")
	}
}
func TestBehaviorNewPetAfterDragReleaseWins(t *testing.T) {
	e := NewBehaviorEngine(behaviorCats(1), nil)
	e.Cats[0].Dragging = true
	quietCursor(e, 1, .1, false)
	e.Cats[0].Dragging = false
	e.Pet(0, 2)
	out := quietCursor(e, 2, .1, false)
	if out[0].Action != "pet" {
		t.Fatal("fresh release feedback was mistaken for stale work")
	}
}
func TestBehaviorOverlappingDragPositionOnlySeparates(t *testing.T) {
	cats := behaviorCats(2)
	cats[1].X = cats[0].X + 50
	e := NewBehaviorEngine(cats, nil)
	x := cats[0].X
	e.moveToward(0, x+100, cats[0].Y, 30)
	if cats[0].X != x {
		t.Fatal("overlapping cat moved through neighbor")
	}
	e.moveToward(0, x-100, cats[0].Y, 30)
	if cats[0].X >= x {
		t.Fatal("overlapping cat cannot move apart")
	}
}
