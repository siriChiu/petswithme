package main

import (
	"math"
	"testing"
)

func TestStationaryPressDoesNotDragOrWander(t *testing.T) {
	e := motionEngine()
	e.Tick(0, 0, math.NaN(), math.NaN(), 0, false)
	c := e.Cats[0]
	e.Cancel(0, .1)
	c.Pressed = true
	x, y := c.X, c.Y
	for n := 1; n <= 420; n++ {
		f := e.Tick(.1+float64(n)*.05, .05, math.NaN(), math.NaN(), 0, false)[0]
		if f.Action != "idle" || c.X != x || c.Y != y {
			t.Fatal("stationary press moved or showed pickup", f.Action)
		}
	}
	if e.Request(0, "play", PriorityPlay, 21.2, 1) {
		t.Fatal("command interrupted a held press")
	}
	c.Pressed = false
	e.Pet(0, 21.3)
	if f := e.Tick(21.35, .05, math.NaN(), math.NaN(), 0, false)[0]; f.Action != "pet" {
		t.Fatal("click release did not pet")
	}
}
func TestRealDragOutranksStationaryPress(t *testing.T) {
	e := motionEngine()
	c := e.Cats[0]
	e.Cancel(0, 0)
	c.Pressed = true
	c.Dragging = true
	if f := e.Tick(.05, .05, math.NaN(), math.NaN(), 0, false)[0]; f.Action != "drag" {
		t.Fatal("real drag did not show pickup")
	}
	c.Pressed = false
	c.Dragging = false
	if f := e.Tick(.1, .05, math.NaN(), math.NaN(), 0, false)[0]; f.Action != "idle" {
		t.Fatal("drag release retained press")
	}
}
