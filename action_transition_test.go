package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Regressions for ordinary transitions, delayed presentation and interruptions.
// All frame references below are synthetic and are not candidate artwork.
func reviewTick(e *BehaviorEngine, now, dt float64) []BehaviorFrame {
	return e.Tick(now, dt, math.NaN(), math.NaN(), 0, false)
}

func reviewSocialWithRecovery() *BehaviorEngine {
	e := flickerSocialEngine(2)
	m := flickerSocialManifest(15, false)
	for _, name := range []string{"walk_left", "walk_right"} {
		a := m.Actions[name]
		a.End = []AnimationFrame{{Row: 9, Col: 0, DurationMS: 100}, {Row: 9, Col: 1, DurationMS: 100}}
		m.Actions[name] = a
	}
	m.Actions["greet"] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{{Row: 8, Col: 0, DurationMS: 200}}}}
	for i := range e.States {
		e.SetManifest(i, m)
	}
	return e
}

func TestReviewSocialArrivalPreservesDeclaredRecovery(t *testing.T) {
	e := reviewSocialWithRecovery()
	// Follower has a gap to close; the leader has already reached its destination.
	e.Cats[1].X = 230
	flickerSetFollow(e, 0, 1, 230)
	seenMotion, seenRecovery, settled := false, map[int]bool{}, false
	for step := 0; step < 80; step++ {
		f := reviewTick(e, float64(step)*.05, .05)[0]
		p := e.States[0].Player
		if f.Action == "walk_right" && p.Phase == AnimationLoop {
			seenMotion = true
		}
		if p.Action == "walk_right" && p.Phase == AnimationEnd {
			seenRecovery[p.Index] = true
		}
		if seenMotion && f.Action == "idle" {
			settled = true
			if !seenRecovery[0] || !seenRecovery[1] {
				t.Fatalf("social arrival cut directly from gait to idle at %.2fs, skipping declared recovery: seen=%v", float64(step)*.05, seenRecovery)
			}
			break
		}
	}
	if !seenMotion || !settled {
		t.Fatal("fixture did not exercise movement followed by arrival")
	}
}

func TestReviewSocialApproachPreservesRecoveryBeforeGreeting(t *testing.T) {
	e := reviewSocialWithRecovery()
	e.Cats[1].X = 240
	e.configureSocial()
	if !e.Social.Start(e.Cats, 0, 1, 0) {
		t.Fatal("fixture could not start social approach")
	}
	seenMotion, seenRecovery := false, map[int]bool{}
	for step := 0; step < 160; step++ {
		f := reviewTick(e, float64(step)*.05, .05)[0]
		p := e.States[0].Player
		seenMotion = seenMotion || f.Action == "walk_right"
		if p.Action == "walk_right" && p.Phase == AnimationEnd {
			seenRecovery[p.Index] = true
		}
		if seenMotion && f.Action == "greet" {
			if !seenRecovery[0] || !seenRecovery[1] {
				t.Fatalf("social approach cut directly into greeting at %.2fs, skipping declared recovery: seen=%v", float64(step)*.05, seenRecovery)
			}
			return
		}
	}
	t.Fatal("fixture never reached greeting")
}

func TestReviewPetFirstPresentationAfterDelayedTimer(t *testing.T) {
	e := jumpFixture()
	// The current private packs also declare a pet entry and recovery.
	a := e.States[0].Player.Manifest.Actions["pet"]
	a.Start = []AnimationFrame{{Row: 9, Col: 0, DurationMS: 100}}
	a.End = []AnimationFrame{{Row: 9, Col: 1, DurationMS: 300}}
	e.States[0].Player.Manifest.Actions["pet"] = a
	e.Pet(0, 0)
	f := reviewTick(e, 3, .25)[0]
	if f.Action != "pet" || e.States[0].Player.Phase != AnimationStart {
		t.Fatalf("pet entry/loop expired before any pet frame was presented: got %q phase=%s", f.Action, e.States[0].Player.Phase)
	}
}

func TestReviewDragReleaseFreshPetAndRecovery(t *testing.T) {
	for _, beforeReleaseTick := range []bool{false, true} {
		e := jumpFixture()
		m := e.States[0].Player.Manifest
		a := m.Actions["pet"]
		a.End = []AnimationFrame{{Row: 9, Col: 0, DurationMS: 100}, {Row: 9, Col: 1, DurationMS: 100}}
		m.Actions["pet"] = a
		e.Play(0, 0)
		reviewTick(e, 0, 0)
		e.Cats[0].Dragging = true
		reviewTick(e, .05, .05)
		e.Cats[0].Dragging = false
		if beforeReleaseTick {
			reviewTick(e, .1, .05)
		}
		e.Pet(0, .11)
		seenEnd := map[int]bool{}
		for step := 0; step < 55; step++ {
			f := reviewTick(e, .12+float64(step)*.05, .05)[0]
			p := e.States[0].Player
			if step == 0 && f.Action != "pet" {
				t.Fatalf("fresh release pet lost, intervening tick=%v", beforeReleaseTick)
			}
			if f.Action == "play" || f.Action == "drag" {
				t.Fatal("stale interrupted action resumed")
			}
			if f.Action == "pet" && p.Phase == AnimationEnd {
				seenEnd[p.Index] = true
			}
		}
		if !seenEnd[0] || !seenEnd[1] || e.States[0].Action != "idle" {
			t.Fatal("release pet recovery did not complete", seenEnd)
		}
	}
}

func TestReviewCPUStretchInterruptAndResume(t *testing.T) {
	for _, interrupt := range []string{"pet", "drag", "quiet"} {
		for _, phase := range []AnimationPhase{AnimationStart, AnimationLoop, AnimationEnd} {
			t.Run(interrupt+"/"+string(phase), func(t *testing.T) {
				e := cpuEngine(1)
				m := e.States[0].Player.Manifest
				stretch := m.Actions["stretch"]
				stretch.Start = []AnimationFrame{{Rect: &FrameRect{80, 0, 8, 8}, DurationMS: 200}}
				stretch.End = []AnimationFrame{{Rect: &FrameRect{88, 0, 8, 8}, DurationMS: 200}, {Rect: &FrameRect{96, 0, 8, 8}, DurationMS: 200}}
				m.Actions["stretch"] = stretch
				e.Load.Active = true
				s := e.States[0]
				s.BusyElapsed, s.BusyNextStretch = 300, 300
				now := 0.0
				for n := 0; n < 20; n++ {
					reviewTick(e, now, .05)
					if s.Action == "stretch" && s.Player.Phase == phase {
						break
					}
					now += .05
				}
				if s.Action != "stretch" || s.Player.Phase != phase {
					t.Fatal("fixture did not reach stretch phase", phase)
				}
				switch interrupt {
				case "pet":
					e.Pet(0, now+.01)
				case "drag":
					e.Cats[0].Dragging = true
				case "quiet":
					e.SetActivity(ActivityQuiet)
				}
				f := reviewTick(e, now+.05, .05)[0]
				if s.BusyStretching || s.BusyStretchEnding || f.Action == "stretch" {
					t.Fatal("interruption retained stretch")
				}
				e.Cats[0].Dragging = false
				e.SetActivity(ActivityNormal)
				for n := 2; n < 65; n++ {
					f = reviewTick(e, now+float64(n)*.05, .05)[0]
					if f.Action == "stretch" {
						t.Fatal("canceled stretch resumed without a new interval")
					}
				}
				if f.Action != "knead" || s.Ending {
					t.Fatal("CPU did not recover to kneading", f.Action, s.Ending)
				}
			})
		}
	}
}

// Optional private-manifest verification uses only frame metadata and never
// reads or changes the source art. Synthetic cases above remain self-contained.
func TestReviewPrivatePackSocialRecovery(t *testing.T) {
	pack := os.Getenv("PETSWITHME_REVIEW_PACK")
	if pack == "" {
		t.Skip("set PETSWITHME_REVIEW_PACK to an existing private pack for metadata-only checks")
	}
	for _, cat := range []string{"baobao", "maomao", "wawa"} {
		for _, scenario := range []string{"arrival", "approach"} {
			t.Run(cat+"/"+scenario, func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(pack, "art", cat, "animations.json"))
				if err != nil {
					t.Fatal(err)
				}
				var m AnimationManifest
				if err := json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				e := flickerSocialEngine(2)
				e.ExperimentalMovement = true
				for i := range e.States {
					e.SetManifest(i, &m)
				}
				if len(m.Resolve("run_right", "calm").End) == 0 {
					t.Fatal("private fixture has no declared recovery")
				}
				e.Cats[1].X = 230
				next := "idle"
				if scenario == "arrival" {
					flickerSetFollow(e, 0, 1, 230)
				} else {
					next = "greet"
					e.configureSocial()
					if !e.Social.Start(e.Cats, 0, 1, 0) {
						t.Fatal("could not start private-manifest approach")
					}
				}
				moving, recovered := false, false
				for step := 0; step < 200; step++ {
					f := reviewTick(e, float64(step)*.05, .05)[0]
					p := e.States[0].Player
					moving = moving || (f.Action == "run_right" && p.Phase == AnimationLoop)
					recovered = recovered || (p.Action == "run_right" && p.Phase == AnimationEnd)
					if moving && f.Action == next {
						if !recovered {
							t.Fatalf("current private OLD8run skipped %.0fms declared End before %s at %.2fs", clipSeconds(m.Resolve("run_right", "calm").End)*1000, next, float64(step)*.05)
						}
						return
					}
				}
				t.Fatal("private fixture did not reach expected transition")
			})
		}
	}
}
