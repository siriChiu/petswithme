package main

import (
	"math"
	"testing"
)

func jumpFixture() *BehaviorEngine {
	m := fullCapabilityFixture()
	loop := []AnimationFrame{}
	for i, ms := range []int{150, 100, 150, 100, 200} {
		loop = append(loop, AnimationFrame{Row: 4, Col: i, DurationMS: ms})
	}
	m.Actions["play"] = AnimationAction{AnimationClip: AnimationClip{Loop: loop, End: []AnimationFrame{{Row: 4, Col: 4, DurationMS: 200}}}}
	e := NewBehaviorEngine(behaviorCats(1), m)
	e.Social.NextAttempt = 1000
	e.States[0].NextDecision = 1000
	return e
}
func TestPlayPerformsOneJumpAndCompleteLanding(t *testing.T) {
	e := jumpFixture()
	e.Play(0, 0)
	x, y := e.Cats[0].X, e.Cats[0].Y
	order := []int{}
	last := -1
	endStart := -1.0
	idleAt := -1.0
	for n := 0; n < 130; n++ {
		now := float64(n) * .05
		f := e.Tick(now, .05, math.NaN(), math.NaN(), 0, false)[0]
		s := e.States[0]
		if f.Action == "play" && s.Player.Phase == AnimationLoop && s.Player.Index != last {
			last = s.Player.Index
			order = append(order, last)
		}
		if s.Player.Phase == AnimationEnd && endStart < 0 {
			endStart = now
		}
		if f.Action == "idle" && idleAt < 0 {
			idleAt = now
		}
		if e.Cats[0].X != x || e.Cats[0].Y != y {
			t.Fatal("jump root moved; source art already includes height")
		}
	}
	if len(order) != 5 {
		t.Fatal("jump repeated or lost a phase", order)
	}
	for i, v := range order {
		if i != v {
			t.Fatal("wrong phase order", order)
		}
	}
	if endStart < .7-1e-8 || idleAt-endStart < .2-1e-8 || idleAt > 1.0 {
		t.Fatal("landing cut short or jump lingered", endStart, idleAt)
	}
}
func TestJumpFirstPresentationCanBeDelayed(t *testing.T) {
	e := jumpFixture()
	e.Play(0, 0)
	first := e.Tick(3, .25, math.NaN(), math.NaN(), 0, false)[0]
	if first.Action != "play" || e.States[0].Player.Index != 0 {
		t.Fatal("wall-clock timeout skipped jump before first draw")
	}
	for n := 1; n <= 12; n++ {
		e.Tick(3+float64(n)*.1, .1, math.NaN(), math.NaN(), 0, false)
	}
	if e.States[0].Action != "idle" || e.States[0].OneShot {
		t.Fatal("delayed jump never completed")
	}
}
func TestJumpRepeatedRequestAndDragInterruption(t *testing.T) {
	e := jumpFixture()
	e.Play(0, 0)
	e.Tick(0, 0, math.NaN(), math.NaN(), 0, false)
	e.Tick(.25, .25, math.NaN(), math.NaN(), 0, false)
	generation := e.States[0].Player.Generation
	e.Play(0, .26)
	e.Tick(.3, .05, math.NaN(), math.NaN(), 0, false)
	if e.States[0].Player.Generation <= generation || e.States[0].Player.Index != 0 {
		t.Fatal("repeat play did not restart")
	}
	e.Cats[0].Dragging = true
	f := e.Tick(.35, .05, math.NaN(), math.NaN(), 0, false)[0]
	if f.Action != "drag" || e.States[0].OneShot {
		t.Fatal("drag waited for landing")
	}
	e.Cats[0].Dragging = false
	e.Tick(.4, .05, math.NaN(), math.NaN(), 0, false)
	if e.States[0].Action != "idle" {
		t.Fatal("release resumed canceled jump")
	}
}
func TestSingleJumpPreservesFirstRecoveryOnDelayedTick(t *testing.T) {
	e := jumpFixture()
	p := e.States[0].Player
	p.Play("play", "calm")
	p.LoopOnce = true
	p.Tick(.65)
	p.Tick(.25)
	if p.Phase != AnimationEnd || p.Index != 0 || p.ElapsedMS != 0 {
		t.Fatal("timer overflow consumed landing before presentation")
	}
	_, done := p.Tick(.2)
	if !done {
		t.Fatal("jump did not complete")
	}
	_, done = p.Tick(.2)
	if done {
		t.Fatal("completion repeated")
	}
}

func TestJumpMoodChangeStillCompletes(t *testing.T) {
	e := jumpFixture()
	e.Play(0, 0)
	e.Tick(0, 0, math.NaN(), math.NaN(), 0, false)
	e.Tick(.1, .1, math.NaN(), math.NaN(), 0, false)
	e.SetMood(0, "calm")
	for n := 0; n < 30; n++ {
		e.Tick(.15+float64(n)*.05, .05, math.NaN(), math.NaN(), 0, false)
	}
	if e.States[0].OneShot || e.States[0].Action != "idle" {
		t.Fatal("mood replacement lost single-shot completion")
	}
}
