package main

import (
	"math"
	"testing"
)

func deadlineFixture() *BehaviorEngine {
	e := jumpFixture()
	clip := AnimationClip{}
	for i := 0; i < 8; i++ {
		clip.Loop = append(clip.Loop, AnimationFrame{Row: 0, Col: i, DurationMS: 120})
	}
	e.States[0].Player.Manifest.Actions["pet"] = AnimationAction{AnimationClip: clip}
	e.Request(0, "pet", PriorityPet, 0, 100)
	e.Tick(0, 0, math.NaN(), math.NaN(), 0, false)
	return e
}
func TestFrameDeadlineRespectsResourceAndLifecyclePolicy(t *testing.T) {
	e := deadlineFixture()
	p := e.States[0].Player
	p.ElapsedMS = 100
	if got := FrameDeadlineMS(e, 50, 0); got != 20 {
		t.Fatal(got)
	}
	if got := FrameDeadlineMS(e, 50, 1.4); got != 19 {
		t.Fatal("render time not accounted", got)
	}
	p.ElapsedMS = 50
	if got := FrameDeadlineMS(e, 50, 30); got != 20 {
		t.Fatal("between-tick callback postpones movement wake", got)
	}
	p.ElapsedMS = 100
	for _, base := range []int{0, 16, 100, 250} {
		if got := FrameDeadlineMS(e, base, 1); got != base {
			t.Fatal("resource policy changed", base, got)
		}
	}
	if got := FrameDeadlineMS(e, 50, 100); got != 10 {
		t.Fatal("near-due timer should remain bounded", got)
	}
	p.Play("pet", "calm")
	p.clip.Loop[0].DurationMS = 40
	if got := FrameDeadlineMS(e, 50, 30); got != 20 {
		t.Fatal("new generation borrowed old clock", got)
	}
	p.Phase = AnimationDone
	if got := FrameDeadlineMS(e, 50, 0); got != 50 {
		t.Fatal("completed action scheduled extra update", got)
	}
}
func TestFrameDeadlineNominal120msPosesStayEven(t *testing.T) {
	e := deadlineFixture()
	now := 0.0
	last := 0
	lastAt := 0.0
	changes := 0
	for n := 0; now < 3; n++ {
		if n > 200 {
			t.Fatal("timer busy loop")
		}
		ms := FrameDeadlineMS(e, 50, 0)
		dt := float64(ms) / 1000
		now += dt
		e.Tick(now, dt, math.NaN(), math.NaN(), 0, false)
		index := e.States[0].Player.Index
		if index != last {
			if math.Abs(now-lastAt-.12) > .0011 {
				t.Fatal("uneven authored pose", now-lastAt)
			}
			last = index
			lastAt = now
			changes++
		}
	}
	if changes < 24 {
		t.Fatal("skipped pose progression", changes)
	}
}
func TestFrameDeadlineSkipsStaticLoopAndKeepsOneShotCompletion(t *testing.T) {
	e := deadlineFixture()
	p := e.States[0].Player
	p.clip.Loop = p.clip.Loop[:1]
	p.ElapsedMS = 115
	if FrameDeadlineMS(e, 50, 0) != 50 {
		t.Fatal("static loop creates extra wakeups")
	}
	p.LoopOnce = true
	if FrameDeadlineMS(e, 50, 0) != 10 {
		t.Fatal("one-shot final pose cannot complete promptly")
	}
	e.States = append(e.States, &CatBehavior{Player: NewAnimationPlayer(p.Manifest)})
	e.States[1].Player.Play("pet", "calm")
	e.States[1].Player.ElapsedMS = 90
	e.States[1].LastPresentedGeneration = e.States[1].Player.Generation
	p.Phase = AnimationDone
	if FrameDeadlineMS(e, 50, 0) != 30 {
		t.Fatal("next cat deadline not selected")
	}
}
