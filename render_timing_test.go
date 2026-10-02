package main

import (
	"math"
	"testing"
)

func TestLowResourceTimerPolicy(t *testing.T) {
	s := NormalizeSettings(Settings{CPU: DefaultCPUSettings()})
	if RenderIntervalMS(s, false, false, false, false) != 50 || RenderIntervalMS(s, true, false, false, false) != 100 || RenderIntervalMS(s, false, true, false, false) != 16 || RenderIntervalMS(s, true, true, true, false) != 0 {
		t.Fatal("timer policy changed")
	}
	s.Activity = ActivityQuiet
	if RenderIntervalMS(s, false, false, false, false) != 250 {
		t.Fatal("quiet did not throttle")
	}
}
func TestMoreGeneratedFramesPreserveActionDuration(t *testing.T) {
	duration := func(count int) float64 {
		clip := AnimationClip{}
		for i := 0; i < count; i++ {
			clip.Loop = append(clip.Loop, AnimationFrame{Row: 0, Col: i % 6, DurationMS: 1200 / count})
		}
		m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Actions: map[string]AnimationAction{"idle": {AnimationClip: clip}}}
		p := NewAnimationPlayer(m)
		p.Tick(.7)
		return float64(p.Index)*float64(1200/count) + p.ElapsedMS
	}
	if math.Abs(duration(6)-duration(12)) > .001 {
		t.Fatal("additional poses accelerated the clip")
	}
}

func TestQuietRequestedJumpShowsEveryPoseThenThrottles(t *testing.T) {
	e := jumpFixture()
	e.SetActivity(ActivityQuiet)
	settings := NormalizeSettings(Settings{Quiet: true, Activity: ActivityQuiet, CPU: DefaultCPUSettings()})
	e.Play(0, 0)
	seen := map[int]bool{}
	endAt, idleAt := -1.0, -1.0
	now, dt := 0.0, 0.0
	for n := 0; n < 30; n++ {
		f := e.Tick(now, dt, math.NaN(), math.NaN(), 0, true)[0]
		s := e.States[0]
		if f.Action == "play" && s.Player.Phase == AnimationLoop {
			seen[s.Player.Index] = true
		}
		if s.Player.Phase == AnimationEnd && endAt < 0 {
			endAt = now
		}
		interval := RenderIntervalMS(settings, true, false, false, e.DirectAnimationActive())
		if !e.DirectAnimationActive() {
			idleAt = now
			if interval != 250 {
				t.Fatal("completed action did not throttle", interval)
			}
			break
		}
		if interval != 50 {
			t.Fatal("quiet interaction skips short poses", interval, now, f.Action, s.Priority, s.Autonomous, s.Player.Phase)
		}
		dt = float64(interval) / 1000
		now += dt
	}
	if len(seen) != 5 || endAt < 0 || idleAt-endAt < .2-1e-8 || idleAt > 1.0 {
		t.Fatal("quiet jump lost pose, recovery or timing", seen, endAt, idleAt)
	}
	if RenderIntervalMS(settings, true, true, false, true) != 16 || RenderIntervalMS(settings, true, true, true, true) != 0 {
		t.Fatal("interaction overrode drag/hidden priority")
	}
}

func TestAutonomousActionDoesNotRaiseQuietCadence(t *testing.T) {
	e := jumpFixture()
	e.States[0].Priority = PriorityPlay
	e.States[0].Autonomous = true
	if e.DirectAnimationActive() {
		t.Fatal("autonomous action treated as user interaction")
	}
	var absent *BehaviorEngine
	if absent.DirectAnimationActive() {
		t.Fatal("nil engine active")
	}
}
