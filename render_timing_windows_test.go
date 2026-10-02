//go:build windows && amd64

package main

import (
	"math"
	"runtime"
	"testing"
	"time"
)

func TestWindowsQuietInteractionTimerRestoresIdleCadence(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	fixture, err := newSmokeFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.close()
	old := app
	defer func() { killTimer.Call(fixture.probe, 1); app = old }()
	app.Controller = fixture.probe
	app.Start = time.Now()
	app.TimerIntervalMS = 0
	app.Hidden = false
	app.Quitting = false
	app.Settings = NormalizeSettings(Settings{Quiet: true, Activity: ActivityQuiet, CPU: DefaultCPUSettings()})
	app.Engine = jumpFixture()
	app.Engine.SetActivity(ActivityQuiet)
	p := &PetWindow{HWND: fixture.pet, Cat: app.Engine.Cats[0]}
	app.Pets = []*PetWindow{p}
	setInterval()
	if app.TimerIntervalMS != 250 {
		t.Fatal("quiet timer not installed")
	}
	app.Engine.Play(0, 0)
	setInterval()
	if app.TimerIntervalMS != 50 {
		t.Fatal("manual play did not raise timer")
	}
	seen := map[int]bool{}
	for n := 0; n < 25; n++ {
		f := app.Engine.Tick(float64(n)*.05, .05, math.NaN(), math.NaN(), 0, true)[0]
		s := app.Engine.States[0]
		if f.Action == "play" && s.Player.Phase == AnimationLoop {
			seen[s.Player.Index] = true
		}
		setInterval()
	}
	if len(seen) != 5 || app.TimerIntervalMS != 250 {
		t.Fatal("native timer did not cover poses and return to quiet", seen, app.TimerIntervalMS)
	}
	p.Down = true
	setInterval()
	if app.TimerIntervalMS != 16 {
		t.Fatal("drag priority lost")
	}
	app.Hidden = true
	setInterval()
	if app.TimerIntervalMS != 0 {
		t.Fatal("hidden timer still running")
	}
	t.Log("Native timer uses 50ms for a requested quiet-mode jump, restores 250ms after landing, prioritizes drag and stops when hidden")
}
