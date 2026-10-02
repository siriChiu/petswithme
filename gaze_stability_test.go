package main

import (
	"math"
	"testing"
)

func TestGazeDeadZoneTremorDoesNotFlashIdle(t *testing.T) {
	for _, size := range []int{96, 144, 230} {
		e := gazeFixture()
		c := e.Cats[0]
		c.W = size
		c.X = -1700
		c.Y = 450
		ox, oy := ManifestGazeOrigin(c, e.Manifest)
		radius := math.Max(8, float64(size)*.05)
		lowX, highX := math.Ceil(ox+radius)-1, math.Floor(ox+radius)+1
		first := e.Tick(0, 0, ox+radius+1, oy, 0, false)[0]
		if first.Action != "gaze_4" {
			t.Fatal("fixture did not look right")
		}
		for n := 1; n <= 60; n++ {
			x := lowX
			if n%2 == 0 {
				x = highX
			}
			f := e.Tick(float64(n)*.05, .05, x, oy, 0, false)[0]
			if f.Action != "gaze_4" {
				t.Fatalf("size%d: boundary tremor flashed %s at tick%d", size, f.Action, n)
			}
		}
		if f := e.Tick(3.1, .05, ox+radius*.5, oy, 0, false)[0]; f.Action != "idle" {
			t.Fatal("real move into face did not rest")
		}
		for n := 0; n < 10; n++ {
			if f := e.Tick(3.2+float64(n)*.05, .05, ox+radius*.8, oy, 0, false)[0]; f.Action != "idle" {
				t.Fatal("inside point restarted gaze")
			}
		}
		if f := e.Tick(4, .05, ox+radius+1, oy, 0, false)[0]; f.Action != "gaze_4" {
			t.Fatal("real move away did not restore gaze")
		}
	}
}
