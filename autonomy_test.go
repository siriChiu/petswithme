package main

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Synthetic frame references test decisions only. They are not production art
// and must never be presented as visually validated cat actions.
func fullCapabilityFixture() *AnimationManifest {
	m := DefaultAnimationManifest()
	for _, action := range []string{"sit", "getup", "sleep", "rest", "social_rest", "groom", "stretch", "pounce", "greet"} {
		m.Actions[action] = AnimationAction{AnimationClip: atlasLoop(5, 100)}
	}
	for _, direction := range []string{"left", "right"} {
		m.Actions["run_"+direction] = AnimationAction{AnimationClip: atlasLoop(7, 50), Movement: &AnimationMovement{StrideRatio: .7, Verified: true}}
	}
	return m
}
func TestActivitySettingsMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, test := range []struct {
		json string
		want ActivityLevel
	}{
		{`{"quiet":true,"size":192}`, ActivityQuiet}, {`{"quiet":false,"size":96}`, ActivityNormal},
		{`{"quiet":true,"activity":"lively","size":144}`, ActivityLively},
		{`{"activity":"bogus","size":144}`, ActivityNormal},
	} {
		if err := os.WriteFile(path, []byte(test.json), 0600); err != nil {
			t.Fatal(err)
		}
		s := LoadSettings(path)
		if s.Activity != test.want || s.Quiet != (test.want == ActivityQuiet) {
			t.Fatalf("%s: %+v", test.json, s)
		}
		if err := SaveSettings(path, s); err != nil {
			t.Fatal(err)
		}
		if got := LoadSettings(path); got != s {
			t.Fatalf("lost activity: %+v %+v", s, got)
		}
	}
}
func TestTemperamentConfigValidation(t *testing.T) {
	dir := t.TempDir()
	for _, values := range []string{`{"energy":1.2,"sociability":0.4,"curiosity":0.5}`, `{"energy":-1}`, `{"energy":0.4,"surprise":1}`} {
		data := `{"version":1,"cats":[{"name":"Test","temperament":` + values + `}]}`
		os.WriteFile(filepath.Join(dir, "cats.json"), []byte(data), 0600)
		if _, err := LoadConfig(dir); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	cfg := DefaultConfig()
	cfg.Cats[0].Temperament = &Temperament{.2, .3, .4}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "cats.json"), data, 0600)
	got, err := LoadConfig(dir)
	if err != nil || *got.Cats[0].Temperament != *cfg.Cats[0].Temperament {
		t.Fatalf("%+v %v", got, err)
	}
	if (Temperament{math.NaN(), .4, .5}).Validate() == nil {
		t.Fatal("NaN accepted")
	}
}
func TestAutonomyDoesNotInventCapabilities(t *testing.T) {
	m := DefaultAnimationManifest()
	for _, a := range []string{"run_left", "run_right", "groom", "stretch", "sit", "pounce"} {
		if HasAuthoredAction(m, a, "calm") {
			t.Fatalf("invented %s", a)
		}
	}
	m.Actions["gaze_1"] = AnimationAction{Fallback: "gaze_0"}
	if !HasAuthoredAction(m, "gaze_1", "calm") {
		t.Fatal("valid nearest-direction gaze unavailable")
	}
	m.Actions["greet"] = AnimationAction{Fallback: "pet"}
	if HasAuthoredAction(m, "greet", "calm") {
		t.Fatal("pet alias called new greeting art")
	}
	e := NewBehaviorEngine(behaviorCats(1), m)
	for step := 0; step < 20000; step++ {
		f := quietCursor(e, float64(step)*.05, .05, false)[0]
		if !HasAuthoredAction(m, f.Action, f.Mood) {
			t.Fatalf("selected missing art: %s", f.Action)
		}
	}
}
func TestAutonomyRunNeedsOwnVerifiedFrames(t *testing.T) {
	m := DefaultAnimationManifest()
	run := m.Actions["walk_right"]
	run.Movement = &AnimationMovement{StrideRatio: .7, Verified: true}
	for i := range run.Loop {
		run.Loop[i].DurationMS = 20
	}
	m.Actions["run_right"] = run
	if HasRunAnimation(m, "run_right", "calm") {
		t.Fatal("accelerated walking called running")
	}
	run.AnimationClip = atlasLoop(7, 50)
	run.Movement.Verified = false
	m.Actions["run_right"] = run
	if HasRunAnimation(m, "run_right", "calm") || LocomotionPixelsPerSecond(m, "run_right", "calm", 144) != 0 {
		t.Fatal("unverified running enabled")
	}
	run.Movement.Verified = true
	m.Actions["run_right"] = run
	if !HasRunAnimation(m, "run_right", "calm") {
		t.Fatal("separate verified run unavailable")
	}
	want := 144 * .7 / .3
	if got := LocomotionPixelsPerSecond(m, "run_right", "calm", 144); math.Abs(got-want) > 1e-8 {
		t.Fatalf("run speed %v want %v", got, want)
	}
}
func TestAutonomyDecisionsHaveVarietyAndCooldown(t *testing.T) {
	e := NewBehaviorEngine(behaviorCats(1), fullCapabilityFixture())
	e.Cats[0].X = 600
	actions := map[string]bool{}
	destinations := map[int]bool{}
	dwells := map[int]bool{}
	last := ""
	for n := 0; n < 100; n++ {
		now := float64(n) * 50
		e.finishAutonomy(0, now)
		e.decideAutonomy(0, now)
		s := e.States[0]
		if last == s.LastChoice && s.LastChoice != "idle" {
			t.Fatalf("repeated %s", last)
		}
		last = s.LastChoice
		actions[s.Action] = true
		if s.HasDestination {
			destinations[int(s.TargetX)] = true
		}
		dwells[int((s.NextDecision-s.Until)*10)] = true
	}
	if len(actions) < 7 || len(destinations) < 5 || len(dwells) < 10 {
		t.Fatalf("limited variety: actions %v destinations %d dwells %d", actions, len(destinations), len(dwells))
	}
}
func TestAutonomyActivityAndTemperamentAffectFrequencyNotSpeed(t *testing.T) {
	count := func(level ActivityLevel, energy float64) (int, float64) {
		e := NewBehaviorEngine(behaviorCats(1), fullCapabilityFixture())
		e.SetActivity(level)
		e.SetTemperament(0, Temperament{energy, .5, .5})
		s := e.States[0]
		last := s.NextDecision
		decisions := 0
		for step := 0; step < 12000; step++ {
			quietCursor(e, float64(step)*.05, .05, false)
			if s.NextDecision != last {
				decisions++
				last = s.NextDecision
			}
		}
		return decisions, LocomotionPixelsPerSecond(s.Player.Manifest, "run_right", "calm", 144)
	}
	normal, speed := count(ActivityNormal, .5)
	lively, speed2 := count(ActivityLively, .5)
	if lively <= normal || speed != speed2 {
		t.Fatalf("normal %d lively %d speeds %g/%g", normal, lively, speed, speed2)
	}
	slow, _ := count(ActivityNormal, .1)
	energetic, _ := count(ActivityNormal, .9)
	if energetic <= slow {
		t.Fatalf("energy made no difference: %d/%d", slow, energetic)
	}
	t.Logf("600-second deterministic simulation: normal=%d decision changes, lively=%d; low energy=%d, high energy=%d; identical calibrated running speed", normal, lively, slow, energetic)
}
func TestAutonomyReachableDestinationAndFixedFloor(t *testing.T) {
	cats := behaviorCats(3)
	e := NewBehaviorEngine(cats, fullCapabilityFixture())
	e.SetActivity(ActivityLively)
	e.Social.NextAttempt = math.Inf(1)
	for step := 0; step < 12000; step++ {
		quietCursor(e, float64(step)*.05, .05, false)
		for i, c := range cats {
			if c.Y != 700 || c.X < 0 || c.X+float64(c.W) > 1200 {
				t.Fatalf("floor/bounds changed %+v", c)
			}
			if i > 0 && cats[i-1].X+float64(cats[i-1].W) > c.X+.001 {
				t.Fatal("cats crossed")
			}
		}
	}
}
func TestAutonomyNearbyCursorDoesNotFreezeRoaming(t *testing.T) {
	e := NewBehaviorEngine(behaviorCats(1), DefaultAnimationManifest())
	moved := 0.0
	for step := 0; step < 12000; step++ {
		x := e.Cats[0].X
		e.Tick(float64(step)*.05, .05, x+150, 740, 0, false)
		moved += math.Abs(e.Cats[0].X - x)
	}
	if moved < 100 {
		t.Fatalf("near cursor froze activity: %g", moved)
	}
}
func TestAutonomyQuietCancelsPairAndKeepsDirectControls(t *testing.T) {
	e := NewBehaviorEngine(behaviorCats(2), fullCapabilityFixture())
	e.Social.Start(e.Cats, 0, 1, 0)
	e.SetActivity(ActivityQuiet)
	x := e.Cats[0].X
	f := quietCursor(e, 1, .05, false)
	if len(e.Social.Reservations) != 0 || e.Cats[0].X != x || f[0].Action != "sleep" {
		t.Fatal("quiet did not settle")
	}
	e.Pet(0, 2)
	if quietCursor(e, 2, .05, false)[0].Action != "pet" {
		t.Fatal("quiet lost pet")
	}
	e.Cats[0].Dragging = true
	if quietCursor(e, 2.1, .05, false)[0].Action != "drag" {
		t.Fatal("quiet lost drag")
	}
}
func TestAutonomySocialChaseAndCancellation(t *testing.T) {
	e := NewBehaviorEngine(behaviorCats(2), fullCapabilityFixture())
	e.SetActivity(ActivityLively)
	e.Cats[1].X = e.Cats[0].X + float64(e.Cats[0].W) + socialGap(e.Cats[0], e.Cats[1])
	e.configureSocial()
	e.Social.NextAttempt = math.Inf(1)
	if !e.Social.Start(e.Cats, 0, 1, 0) {
		t.Fatal("start")
	}
	for _, p := range e.Social.Plans {
		p.Chase = true
		p.Phase = "greet"
		p.PhaseUntil = 0
	}
	f := quietCursor(e, .1, .05, false)
	found := false
	for _, v := range f {
		found = found || strings.HasPrefix(v.Action, "run_")
	}
	if !found {
		t.Fatalf("chase not separate run: %+v", f)
	}
	e.Cats[0].Dragging = true
	f = quietCursor(e, .2, .05, false)
	if len(e.Social.Reservations) != 0 || f[1].Action != "idle" {
		t.Fatal("drag did not release both partners")
	}
}
func TestAutonomySeedIsReproducibleAndDifferentSeedsVary(t *testing.T) {
	simulate := func(seed int64) []string {
		e := NewBehaviorEngine(behaviorCats(1), fullCapabilityFixture())
		e.Cats[0].Seed = rand.New(rand.NewSource(seed))
		values := []string{}
		for n := 0; n < 30; n++ {
			e.decideAutonomy(0, float64(n)*50)
			values = append(values, e.States[0].Action)
		}
		return values
	}
	a, b, c := simulate(1), simulate(1), simulate(2)
	if strings.Join(a, ",") != strings.Join(b, ",") || strings.Join(a, ",") == strings.Join(c, ",") {
		t.Fatal("seed contract failed")
	}
}

func TestAutonomySitGetupEndAndQuietInterrupt(t *testing.T) {
	m := fullCapabilityFixture()
	sit := m.Actions["sit"]
	sit.End = []AnimationFrame{{Row: 5, Col: 1, DurationMS: 100}, {Row: 5, Col: 2, DurationMS: 100}}
	m.Actions["sit"] = sit
	for _, interrupt := range []bool{false, true} {
		e := NewBehaviorEngine(behaviorCats(1), m)
		s := e.States[0]
		s.Action = "sit"
		s.Until = .1
		s.Autonomous = true
		s.Player.Play("sit", "calm")
		quietCursor(e, .1, .05, false)
		if !s.Ending || s.Player.Phase != AnimationEnd {
			t.Fatal("autonomous end skipped")
		}
		if interrupt {
			e.SetActivity(ActivityQuiet)
			f := quietCursor(e, .15, .05, false)[0]
			if f.Action != "sleep" || s.AfterAction != "" || s.Ending {
				t.Fatal("quiet resumed an automatic getup")
			}
		} else {
			for n := 2; n < 8; n++ {
				quietCursor(e, float64(n)*.05, .05, false)
			}
			if s.Action != "getup" {
				t.Fatalf("sit did not enter getup: %s", s.Action)
			}
			e.Cats[0].Dragging = true
			quietCursor(e, .4, .05, false)
			if s.AfterAction != "" {
				t.Fatal("drag kept queued action")
			}
		}
	}
}

func TestExperimentalRunOptInDoesNotForgeCalibration(t *testing.T) {
	m := fullCapabilityFixture()
	delete(m.Actions, "walk_left")
	delete(m.Actions, "walk_right")
	for _, action := range []string{"run_left", "run_right"} {
		a := m.Actions[action]
		a.Movement.Verified = false
		m.Actions[action] = a
	}
	e := NewBehaviorEngine(behaviorCats(2), m)
	if e.canRun(0, "run_right") || e.movementSpeed(0, "run_right") != 0 {
		t.Fatal("trial enabled by default")
	}
	e.SetExperimentalMovement(true)
	if !e.canRun(0, "run_right") || HasRunAnimation(m, "run_right", "calm") || e.movementSpeed(0, "run_right") != 23 {
		t.Fatal("trial forged verified stride or wrong conservative speed")
	}
	if e.movementAction(0, 1, false) != "run_right" {
		t.Fatal("run-only pack cannot move")
	}
	e.configureSocial()
	if !e.Social.Eligible(0, 1) {
		t.Fatal("run-only pair unavailable")
	}
	s := e.States[0]
	s.Action = "run_right"
	s.Priority = PriorityWander
	s.Until = 100
	s.TargetX = 500
	s.HasDestination = true
	e.SetExperimentalMovement(false)
	x := e.Cats[0].X
	quietCursor(e, 1, .1, false)
	if e.Cats[0].X != x || s.Action != "idle" {
		t.Fatal("turning off trial kept unverified movement")
	}
	m.Actions["run_right"] = AnimationAction{Fallback: "idle"}
	e.SetExperimentalMovement(true)
	if e.canRun(0, "run_right") {
		t.Fatal("experimental mode bypassed real-art gate")
	}
}
func TestExperimentalSettingRemembersExplicitOff(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	s := LoadSettings(p, true)
	if !s.ExperimentalMovement {
		t.Fatal("approved private preset lost")
	}
	s.ExperimentalMovement = false
	if err := SaveSettings(p, s); err != nil {
		t.Fatal(err)
	}
	if LoadSettings(p, true).ExperimentalMovement {
		t.Fatal("explicit off overwritten by pack default")
	}
	if LoadSettings(filepath.Join(t.TempDir(), "new.json")).ExperimentalMovement {
		t.Fatal("public default opted in")
	}
}
