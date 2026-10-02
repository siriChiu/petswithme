package main

import (
	"math"
	"testing"
)

func motionEngine() *BehaviorEngine {
	m := fullCapabilityFixture()
	c := NewCat(0, 192, 173, Rect{0, 0, 2000, 1000})
	c.X = 400
	c.Direction = 1
	e := NewBehaviorEngine([]*Cat{c}, m)
	e.Social.NextAttempt = 1000
	s := e.States[0]
	s.Action = "run_right"
	s.Priority = PriorityWander
	s.Until = 100
	s.NextDecision = 1000
	s.HasDestination = true
	s.TargetX = 1200
	return e
}
func TestMotionDoesNotBorrowPreviousActionTick(t *testing.T) {
	e := motionEngine()
	s := e.States[0]
	x := e.Cats[0].X
	f := e.Tick(1, .25, math.NaN(), math.NaN(), 0, false)[0]
	if e.Cats[0].X != x || f.Action != "run_right" || s.Player.Index != 0 {
		t.Fatal("moved before first gait pose", e.Cats[0].X-x, f.Action, s.Player.Index)
	}
	e.Tick(1.05, .05, math.NaN(), math.NaN(), 0, false)
	if e.Cats[0].X <= x {
		t.Fatal("visible loop did not travel")
	}
	x = e.Cats[0].X
	s.Action = "run_left"
	s.TargetX = 100
	e.Cats[0].Direction = -1
	f = e.Tick(1.3, .25, math.NaN(), math.NaN(), 0, false)[0]
	if e.Cats[0].X != x || f.Action != "run_left" || s.Player.Index != 0 {
		t.Fatal("reverse borrowed old tick", e.Cats[0].X-x, f.Action, s.Player.Index)
	}
	e.Tick(1.35, .05, math.NaN(), math.NaN(), 0, false)
	if e.Cats[0].X >= x {
		t.Fatal("new direction did not travel")
	}
}
func TestMotionEntryLoopAndRecoveryDisplacement(t *testing.T) {
	e := motionEngine()
	s := e.States[0]
	m := s.Player.Manifest
	a := m.Actions["run_right"]
	a.Start = []AnimationFrame{{Row: 0, Col: 0, DurationMS: 100}, {Row: 0, Col: 1, DurationMS: 100}}
	a.End = []AnimationFrame{{Row: 0, Col: 2, DurationMS: 100}, {Row: 0, Col: 3, DurationMS: 100}}
	m.Actions["run_right"] = a
	x := e.Cats[0].X
	e.Tick(0, 0, math.NaN(), math.NaN(), 0, false)
	e.Tick(.15, .15, math.NaN(), math.NaN(), 0, false)
	if e.Cats[0].X != x {
		t.Fatal("entry slides", e.Cats[0].X-x)
	}
	speed := e.movementSpeed(0, "run_right")
	e.Tick(.25, .1, math.NaN(), math.NaN(), 0, false)
	if math.Abs(e.Cats[0].X-x-speed*.05) > 1e-8 {
		t.Fatal("entry boundary integrated wrong time", e.Cats[0].X-x)
	}
	s.TargetX = e.Cats[0].X
	seen := map[int]bool{}
	x = e.Cats[0].X
	for n := 0; n < 7; n++ {
		e.Tick(.3+float64(n)*.05, .05, math.NaN(), math.NaN(), 0, false)
		if s.Player.Phase == AnimationEnd {
			seen[s.Player.Index] = true
		}
		if e.Cats[0].X != x {
			t.Fatal("recovery slides")
		}
	}
	if !seen[0] || !seen[1] || s.Action != "idle" {
		t.Fatal("recovery skipped", seen, s.Action)
	}
}
func TestMotionCancellationStillImmediate(t *testing.T) {
	e := motionEngine()
	e.Tick(0, 0, math.NaN(), math.NaN(), 0, false)
	e.Tick(.05, .05, math.NaN(), math.NaN(), 0, false)
	x := e.Cats[0].X
	e.Cats[0].Dragging = true
	f := e.Tick(.1, .05, math.NaN(), math.NaN(), 0, false)[0]
	if f.Action != "drag" || e.Cats[0].X != x {
		t.Fatal("drag failed to cancel motion")
	}
	e.Cats[0].Dragging = false
	e.Tick(.15, .05, math.NaN(), math.NaN(), 0, false)
	e.SetActivity(ActivityQuiet)
	x = e.Cats[0].X
	e.Tick(.2, .05, math.NaN(), math.NaN(), 0, false)
	if e.Cats[0].X != x {
		t.Fatal("quiet keeps travelling")
	}
}

func TestMotionTurnShowsRecoveryBeforeOppositeEntry(t *testing.T) {
	e := motionEngine()
	s := e.States[0]
	a := s.Player.Manifest.Actions["run_right"]
	a.End = []AnimationFrame{{Row: 0, Col: 2, DurationMS: 100}, {Row: 0, Col: 3, DurationMS: 100}}
	s.Player.Manifest.Actions["run_right"] = a
	e.Tick(0, 0, math.NaN(), math.NaN(), 0, false)
	e.Tick(.05, .05, math.NaN(), math.NaN(), 0, false)
	x := e.Cats[0].X
	s.Action = "run_left"
	s.TargetX = 100
	e.Cats[0].Direction = -1
	seen := map[int]bool{}
	switched := false
	for n := 0; n < 7; n++ {
		f := e.Tick(.1+float64(n)*.05, .05, math.NaN(), math.NaN(), 0, false)[0]
		if f.Action == "run_right" && s.Player.Phase == AnimationEnd {
			seen[s.Player.Index] = true
			if e.Cats[0].X != x {
				t.Fatal("turn recovery translated")
			}
		}
		if f.Action == "run_left" && !switched {
			switched = true
			if !seen[0] || !seen[1] || e.Cats[0].X != x || s.Player.Index != 0 {
				t.Fatal("turn cut recovery or borrowed movement", seen, e.Cats[0].X-x)
			}
		}
	}
	if !switched || e.Cats[0].X >= x {
		t.Fatal("turn never resumed")
	}
	// A direct pet must not wait for the turnaround recovery.
	s.Action = "run_right"
	s.TargetX = 1000
	e.Cats[0].Direction = 1
	e.Tick(.5, .05, math.NaN(), math.NaN(), 0, false)
	e.Pet(0, .51)
	f := e.Tick(.55, .05, math.NaN(), math.NaN(), 0, false)[0]
	if f.Action != "pet" || s.MotionTurning {
		t.Fatal("pet delayed by turn")
	}
}
