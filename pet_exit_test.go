package main

import (
	"math"
	"testing"
)

func petExitFixture(returnCenter bool) *BehaviorEngine {
	f := func(col, ms int) AnimationFrame { return AnimationFrame{Row: 3, Col: col, DurationMS: ms} }
	m := DefaultAnimationManifest()
	loop := []AnimationFrame{f(0, 250), f(1, 200), f(2, 100), f(3, 300), f(4, 220)}
	if returnCenter {
		loop = []AnimationFrame{f(0, 250), f(1, 200), f(2, 100), f(3, 200), f(2, 100), f(4, 220)}
	}
	m.Actions["pet"] = AnimationAction{AnimationClip: AnimationClip{Start: []AnimationFrame{f(5, 150)}, Loop: loop, End: []AnimationFrame{f(6, 300)}}}
	m.Actions["knead"] = cpuFixture().Actions["knead"]
	e := NewBehaviorEngine(behaviorCats(1), m)
	e.Social.NextAttempt = 1000
	e.States[0].NextDecision = 1000
	return e
}

func petExitTick(e *BehaviorEngine, now, dt float64) BehaviorFrame {
	return e.Tick(now, dt, math.NaN(), math.NaN(), 0, false)[0]
}

func TestPetExitFinishesNearbyCycleOnce(t *testing.T) {
	for _, returnCenter := range []bool{false, true} {
		for _, first := range []float64{0, 3} {
			for _, quiet := range []bool{false, true} {
				e := petExitFixture(returnCenter)
				if quiet {
					e.SetActivity(ActivityQuiet)
				}
				e.Pet(0, 0)
				settled, endAt, idleAt := false, -1.0, -1.0
				for step := 0; step <= 55; step++ {
					now := first + float64(step)*.05
					f := petExitTick(e, now, .05)
					s := e.States[0]
					if step == 0 && math.Abs(s.Until-(first+2)) > 1e-9 {
						t.Fatal("lost first-presentation deadline")
					}
					if s.PetExitPlanned && s.Until > s.PetExitNominalUntil+petExitGraceSeconds+1e-9 {
						t.Fatal("grace exceeded")
					}
					if f.Action == "pet" && s.Player.Phase == AnimationLoop && f.Col == 4 && now >= first+2 {
						settled = true
					}
					if f.Action == "pet" && s.Player.Phase == AnimationEnd && endAt < 0 {
						endAt = now
						if s.Player.ElapsedMS != 0 {
							t.Fatal("recovery first pose skipped")
						}
					}
					if endAt >= 0 && f.Action == "idle" && idleAt < 0 {
						idleAt = now
					}
					if f.Action == "pet" && !e.DirectAnimationActive() {
						t.Fatal("manual cadence lost")
					}
				}
				if !settled || math.Abs(endAt-first-2.3) > 1e-9 || math.Abs(idleAt-first-2.6) > 1e-9 {
					t.Fatal("unexpected bounded flow", settled, endAt, idleAt, first)
				}
			}
		}
	}
}

func TestPetExitStallsNeverCatchUpAnotherLoop(t *testing.T) {
	for _, scene := range []struct {
		name                   string
		start, resume, wantEnd int
	}{
		{"before_deadline", 1000, 1900, 2000},
		{"across_deadline", 1900, 2800, 2800},
		{"during_grace", 2050, 3000, 3000},
	} {
		t.Run(scene.name, func(t *testing.T) {
			e := petExitFixture(true)
			e.Pet(0, 0)
			last, endAt := -1.0, -1
			for ms := 0; ms <= scene.wantEnd+350; ms += 50 {
				if ms >= scene.start && ms < scene.resume {
					continue
				}
				now := float64(ms) / 1000
				dt := 0.0
				if last >= 0 {
					dt = now - last
				}
				last = now
				f := petExitTick(e, now, dt)
				if f.Action == "pet" && e.States[0].Player.Phase == AnimationEnd && endAt < 0 {
					endAt = ms
					if e.States[0].Player.ElapsedMS != 0 {
						t.Fatal("stall consumed recovery")
					}
				}
			}
			if endAt != scene.wantEnd {
				t.Fatal("stall added catch-up playback", endAt, scene.wantEnd)
			}
		})
	}
	// An expired callback may have just crossed a loop boundary. Even a short
	// next cycle would fit the grace window, but it must not be started as catch-up.
	for _, lateMS := range []int{0, 13, 45} {
		e := petExitFixture(false)
		a := e.States[0].Player.Manifest.Actions["pet"]
		a.Start = nil
		a.Loop = []AnimationFrame{{DurationMS: 50}}
		e.States[0].Player.Manifest.Actions["pet"] = a
		e.Pet(0, 0)
		petExitTick(e, 0, 0)
		for ms := 50; ms <= 1950; ms += 50 {
			petExitTick(e, float64(ms)/1000, .05)
		}
		petExitTick(e, float64(2000+lateMS)/1000, float64(50+lateMS)/1000)
		if e.States[0].Player.Phase != AnimationEnd || e.States[0].Until != 2 {
			t.Fatal("extra short cycle after expiry", lateMS)
		}
	}
}

func TestPetExitInterruptionsAndLowerPriorities(t *testing.T) {
	for _, event := range []string{"drag", "press", "repeat", "manifest", "player_replace", "cpu_quiet", "play"} {
		t.Run(event, func(t *testing.T) {
			e := petExitFixture(true)
			e.Pet(0, 0)
			for ms := 0; ms <= 2050; ms += 50 {
				petExitTick(e, float64(ms)/1000, .05)
			}
			s := e.States[0]
			if !s.PetExitPlanned || s.Until <= 2 {
				t.Fatal("not waiting in grace")
			}
			switch event {
			case "drag":
				e.Cats[0].Dragging = true
			case "press":
				e.Cancel(0, 2.1)
				e.Cats[0].Pressed = true
			case "repeat":
				e.Pet(0, 2.1)
			case "manifest":
				e.SetManifest(0, s.Player.Manifest)
			case "player_replace":
				s.Player.Play("pet", "happy")
			case "cpu_quiet":
				e.Load.Active = true
				e.SetActivity(ActivityQuiet)
			case "play":
				if e.Request(0, "play", PriorityPlay, 2.1, 1) {
					t.Fatal("lower-priority action interrupted pet")
				}
			}
			f := petExitTick(e, 2.1, .05)
			switch event {
			case "drag":
				if f.Action != "drag" || s.PetExitPlanned {
					t.Fatal("drag failed to cancel wait")
				}
			case "press":
				if f.Action != "idle" || s.PetExitPlanned {
					t.Fatal("press failed to cancel wait")
				}
			case "repeat":
				if s.Player.Phase != AnimationStart || s.PetExitPlanned || math.Abs(s.Until-4.1) > 1e-9 {
					t.Fatal("repeat inherited old exit")
				}
			case "manifest", "player_replace":
				if s.Player.Phase != AnimationEnd || s.Until > 2 {
					t.Fatal("replacement extended old deadline")
				}
			case "cpu_quiet", "play":
				if f.Action != "pet" || !e.DirectAnimationActive() {
					t.Fatal("manual pet lost priority")
				}
			}
		})
	}
}

func TestPetExitIsPetOnlyAndHardBounded(t *testing.T) {
	for _, action := range []string{"greet", "stretch", "play"} {
		e := petExitFixture(true)
		e.States[0].Player.Manifest.Actions[action] = e.States[0].Player.Manifest.Actions["pet"]
		if !e.Request(0, action, PriorityPlay, 0, 2) {
			t.Fatal("fixture request refused")
		}
		for ms := 0; ms <= 2000; ms += 50 {
			petExitTick(e, float64(ms)/1000, .05)
		}
		if e.States[0].Player.Phase != AnimationEnd || e.States[0].PetExitPlanned {
			t.Fatal("non-pet expiry changed", action)
		}
	}
	for _, cadence := range []int{25, 50, 63, 100} {
		for start := 100; start <= 2000; start += 100 {
			for length := 0; length <= 1200; length += 100 {
				e := petExitFixture(true)
				e.Pet(0, 0)
				last, endAt := -1.0, -1.0
				for ms := 0; ms <= 5000; ms += cadence {
					if ms >= start && ms < start+length {
						continue
					}
					now := float64(ms) / 1000
					dt := 0.0
					if last >= 0 {
						dt = now - last
					}
					last = now
					f := petExitTick(e, now, dt)
					s := e.States[0]
					if s.PetExitPlanned && s.Until > s.PetExitNominalUntil+petExitGraceSeconds+1e-9 {
						t.Fatal("budget moved")
					}
					if f.Action == "pet" && s.Player.Phase == AnimationEnd {
						endAt = now
						break
					}
				}
				first := math.Ceil(2350/float64(cadence)) * float64(cadence) / 1000
				for first*1000+1e-6 >= float64(start) && first*1000 < float64(start+length)-1e-6 {
					first += float64(cadence) / 1000
				}
				if endAt < 0 || endAt > first+1e-6 {
					t.Fatal("missed first available hard-limit callback", cadence, start, length, endAt, first)
				}
			}
		}
	}
}

func TestPetExitResetCannotTruncateLaterAction(t *testing.T) {
	e := petExitFixture(true)
	e.Pet(0, 0)
	for ms := 0; ms <= 2700; ms += 50 {
		petExitTick(e, float64(ms)/1000, .05)
	}
	s := e.States[0]
	if s.PetExitPlanned || s.PetExitNominalUntil != 0 {
		t.Fatal("completed Pet left a stale deadline")
	}
	s.Action = "greet"
	s.Priority = PrioritySocial
	s.Autonomous = true
	s.Until = 20
	if !e.SetManifest(0, s.Player.Manifest) || s.Until != 20 {
		t.Fatal("stale Pet deadline truncated new action")
	}
}

func TestPetExitExactGraceUsesClampedFloatTolerance(t *testing.T) {
	e := petExitFixture(false)
	a := e.States[0].Player.Manifest.Actions["pet"]
	a.Loop = []AnimationFrame{{DurationMS: 250}, {DurationMS: 300}, {DurationMS: 300}, {DurationMS: 250}}
	e.States[0].Player.Manifest.Actions["pet"] = a
	first := 126.19952477522789
	e.Pet(0, first)
	last := first
	for i := 0; i <= 40; i++ {
		now := first + float64(i)*.05
		petExitTick(e, now, now-last)
		last = now
	}
	s := e.States[0]
	if !s.PetExitPlanned || s.Until != s.PetExitNominalUntil+petExitGraceSeconds || s.Player.Phase != AnimationLoop {
		t.Fatal("lost exact 350ms grace due to residue", s.Until, s.PetExitNominalUntil, s.Player.Phase)
	}
}

func TestPetExitCompletedLoopBeatsDeadlineUlp(t *testing.T) {
	e := petExitFixture(true)
	first := 5.952345805306791
	e.Pet(0, first)
	last := first
	for i := 0; i <= 45; i++ {
		now := first + float64(i)*.05
		petExitTick(e, now, now-last)
		last = now
	}
	s := e.States[0]
	boundary := first + 2.29
	s.Until = math.Nextafter(boundary, math.Inf(1))
	petExitTick(e, boundary, boundary-last)
	if s.Player.Phase != AnimationEnd || s.Player.ElapsedMS != 0 {
		t.Fatal("loop wrapped because of one deadline ulp")
	}
}

func TestPetExitBlocksActiveCPUAndSocialUntilRecovery(t *testing.T) {
	e := petExitFixture(true)
	e.Pet(0, 0)
	for ms := 0; ms <= 2600; ms += 50 {
		if ms == 2050 {
			e.Load.Active = true
		}
		f := petExitTick(e, float64(ms)/1000, .05)
		if ms >= 2050 && ms < 2600 && f.Action != "pet" {
			t.Fatal("CPU took over grace/recovery", ms, f.Action)
		}
		if ms == 2600 && f.Action != "knead" {
			t.Fatal("eligible CPU did not resume", f.Action)
		}
	}
	m := petExitFixture(true).Manifest
	m.Actions["greet"] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{{Row: 3, DurationMS: 250}}}}
	e = NewBehaviorEngine(behaviorCats(2), m)
	e.Social.NextAttempt = 1000
	e.configureSocial()
	e.Cats[1].X = e.Cats[0].X + float64(e.Cats[0].W) + socialGap(e.Cats[0], e.Cats[1])
	e.Cats[1].Y = e.Cats[0].Y
	for _, s := range e.States {
		s.NextDecision = 1000
	}
	e.Pet(0, 0)
	for ms := 0; ms <= 2600; ms += 50 {
		attempt := ms == 2050 || ms == 2400
		if attempt {
			e.Social.Cooldown = map[int]float64{}
			if !e.Social.Start(e.Cats, 0, 1, float64(ms)/1000) {
				t.Fatal("eligible social fixture failed")
			}
		}
		f := petExitTick(e, float64(ms)/1000, .05)
		if attempt && (f.Action != "pet" || len(e.Social.Plans) != 0 || len(e.Social.Reservations) != 0) {
			t.Fatal("social invitation took over grace/recovery", ms, f.Action)
		}
	}
	e.Social.Cooldown = map[int]float64{}
	if !e.Social.Start(e.Cats, 0, 1, 2.7) {
		t.Fatal("social did not become eligible after recovery")
	}
	f := petExitTick(e, 2.7, .1)
	if f.Action != "greet" {
		t.Fatal("social action failed to resume", f.Action)
	}
}
