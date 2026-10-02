package main

import (
	"math"
	"testing"
)

func reviewReachSocialRecovery(t *testing.T) (*BehaviorEngine, float64) {
	t.Helper()
	e := reviewSocialWithRecovery()
	e.Cats[1].X = 230
	flickerSetFollow(e, 0, 1, 230)
	for step := 0; step < 100; step++ {
		now := float64(step) * .05
		reviewTick(e, now, .05)
		if e.States[0].Player.Phase == AnimationEnd {
			return e, now
		}
	}
	t.Fatal("social arrival never entered declared recovery")
	return nil, 0
}

func TestReviewCandidateLeaderMovesDuringRecovery(t *testing.T) {
	e, start := reviewReachSocialRecovery(t)
	p := e.States[0].Player
	generation := p.Generation
	x := e.Cats[0].X
	// An updated leader target makes movement desirable on the next tick.
	e.Cats[1].X += 40
	e.Social.Plans[1].Destination += 100
	for step := 1; step < 4; step++ {
		f := reviewTick(e, start+float64(step)*.05, .05)[0]
		if p.Phase != AnimationEnd || p.Generation != generation || e.Cats[0].X != x {
			t.Fatalf("leader reopening gap interrupted the recovery at %.2fs: action=%s phase=%s generation=%d movement=%.5f", float64(step)*.05, f.Action, p.Phase, p.Generation, e.Cats[0].X-x)
		}
	}
	f := reviewTick(e, start+.2, .05)[0]
	if f.Action != "walk_right" || p.Phase != AnimationLoop || p.Generation <= generation || e.Cats[0].X != x {
		t.Fatal("completed recovery did not restart latest gait cleanly", f.Action, p.Phase, p.Generation, e.Cats[0].X-x)
	}
	reviewTick(e, start+.25, .05)
	if e.Cats[0].X <= x {
		t.Fatal("follower never resumed movement")
	}
}

func TestReviewCandidateSocialRecoveryInterruptions(t *testing.T) {
	for _, action := range []string{"drag", "pet", "quiet", "cpu"} {
		t.Run(action, func(t *testing.T) {
			e, now := reviewReachSocialRecovery(t)
			x := e.Cats[0].X
			want := action
			switch action {
			case "drag":
				e.Cats[0].Dragging = true
			case "pet":
				e.Pet(0, now+.01)
			case "quiet":
				e.SetActivity(ActivityQuiet)
				want = e.restingAction(0)
			case "cpu":
				e.States[0].Player.Manifest.Actions["knead"] = cpuFixture().Actions["knead"]
				e.Load.Active = true
				want = "knead"
			}
			f := reviewTick(e, now+.05, .05)[0]
			if f.Action != want || e.Cats[0].X != x || e.States[0].MotionTurning || len(e.Social.Reservations) != 0 {
				t.Fatalf("%s did not immediately replace social recovery: action=%s turn=%v reservations=%v", action, f.Action, e.States[0].MotionTurning, e.Social.Reservations)
			}
		})
	}
}

func TestReviewCandidateGreetingGetsFullDurationAfterBothSettle(t *testing.T) {
	e := reviewSocialWithRecovery()
	// Different recoveries require waiting for the slower participant.
	m := flickerSocialManifest(15, false)
	for _, action := range []string{"walk_left", "walk_right"} {
		a := m.Actions[action]
		a.End = []AnimationFrame{{Row: 9, Col: 2, DurationMS: 500}}
		m.Actions[action] = a
	}
	m.Actions["greet"] = e.States[0].Player.Manifest.Actions["greet"]
	e.SetManifest(1, m)
	e.Cats[1].X = 240
	e.configureSocial()
	if !e.Social.Start(e.Cats, 0, 1, 0) {
		t.Fatal("fixture cannot start approach")
	}
	firstGreeting := -1.0
	recoveryStarted := [2]float64{-1, -1}
	for step := 0; step < 200; step++ {
		now := float64(step) * .05
		frames := reviewTick(e, now, .05)
		for i, s := range e.States {
			if isLocomotion(s.Player.Action) && s.Player.Phase == AnimationEnd && recoveryStarted[i] < 0 {
				recoveryStarted[i] = now
			}
		}
		if frames[0].Action == "greet" || frames[1].Action == "greet" {
			if firstGreeting < 0 {
				firstGreeting = now
				if frames[0].Action != "greet" || frames[1].Action != "greet" || recoveryStarted[0] < 0 || recoveryStarted[1] < 0 || now-recoveryStarted[0] < .2-1e-8 || now-recoveryStarted[1] < .5-1e-8 {
					t.Fatalf("greeting began before both recoveries finished: start=%g endStarts=%v frames=%s/%s", now, recoveryStarted, frames[0].Action, frames[1].Action)
				}
			}
		} else if firstGreeting >= 0 {
			if now-firstGreeting < 2.4-1e-8 {
				t.Fatalf("settling consumed greeting time: only %.2fs visible", now-firstGreeting)
			}
			return
		}
	}
	t.Fatal("social recovery or greeting failed to finish")
}

func TestReviewCandidateQuietAfterPartnerCancelsSocial(t *testing.T) {
	e := reviewSocialWithRecovery()
	e.Cats[1].X = 280
	flickerSetFollow(e, 0, 1, 1000)
	reviewTick(e, 0, 0)
	reviewTick(e, .05, .05)
	e.Pet(1, .06) // Releases partner's reservation while its gait is visible.
	reviewTick(e, .1, .05)
	if e.States[0].Player.Phase != AnimationEnd {
		t.Fatal("fixture did not enter unpaired recovery")
	}
	e.SetActivity(ActivityQuiet)
	f := e.Tick(.15, .05, math.NaN(), math.NaN(), 0, false)[0]
	if isLocomotion(f.Action) || e.States[0].Ending {
		t.Fatalf("quiet waited for abandoned social recovery: action=%s autonomous=%v priority=%v ending=%v", f.Action, e.States[0].Autonomous, e.States[0].Priority, e.States[0].Ending)
	}
}

func TestReviewCandidateDelayedPetKeepsFullDuration(t *testing.T) {
	e := jumpFixture()
	a := e.States[0].Player.Manifest.Actions["pet"]
	a.Start = []AnimationFrame{{Row: 9, Col: 0, DurationMS: 100}}
	a.End = []AnimationFrame{{Row: 9, Col: 1, DurationMS: 300}}
	e.States[0].Player.Manifest.Actions["pet"] = a
	e.Pet(0, 0)
	for step := 0; step < 40; step++ {
		f := reviewTick(e, 3+float64(step)*.05, .05)[0]
		if f.Action != "pet" || e.States[0].Player.Phase == AnimationEnd {
			t.Fatalf("delayed pet was not allowed its full two seconds: action=%s phase=%s after %.2fs", f.Action, e.States[0].Player.Phase, float64(step)*.05)
		}
	}
	f := reviewTick(e, 5, .05)[0]
	if f.Action != "pet" || e.States[0].Player.Phase != AnimationEnd {
		t.Fatal("delayed pet did not enter normal recovery")
	}
	for step := 1; step <= 7; step++ {
		f = reviewTick(e, 5+float64(step)*.05, .05)[0]
		if step < 6 && f.Action != "pet" {
			t.Fatal("delayed pet cut recovery short")
		}
	}
	if f.Action != "idle" {
		t.Fatal("delayed pet never returned idle")
	}
}
