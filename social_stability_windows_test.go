//go:build windows && amd64

package main

import (
	"image"
	"image/color"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsSocialBlockDoesNotFlashMovementAndResumes(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	old := app
	fixtures := []*smokeFixture{}
	var oldProc uintptr
	changeProc := user32.NewProc("SetWindowLongPtrW")
	defer func() {
		killTimer.Call(app.Controller, 1)
		if oldProc != 0 {
			changeProc.Call(app.Controller, signed(-4), oldProc)
		}
		for _, p := range app.Pets {
			p.DIB.Close()
		}
		for i := len(fixtures) - 1; i >= 0; i-- {
			fixtures[i].close()
		}
		app = old
	}()
	for i := 0; i < 3; i++ {
		f, err := newSmokeFixture()
		if err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, f)
	}
	app.Controller = fixtures[0].probe
	var callErr error
	oldProc, _, callErr = changeProc.Call(app.Controller, signed(-4), syscall.NewCallback(windowProc))
	if oldProc == 0 {
		t.Fatal("install real controller procedure", callErr)
	}
	app.Start = time.Now()
	app.LastTime = 0
	app.LastIdleCheck = 1000
	app.Idle = 0
	app.Hidden = false
	app.Quitting = false
	app.ClickThrough = false
	app.TimerIntervalMS = 0
	app.Settings = NormalizeSettings(Settings{Size: 100, CPU: DefaultCPUSettings()})
	app.Settings.CPU.Enabled = false
	app.Frames = map[string][]byte{}
	app.CachedBytes = 0
	app.Windows = map[uintptr]*PetWindow{}
	app.Pets = nil
	app.Engine = flickerSocialEngine(3)
	app.Engine.SetCPUSettings(app.Settings.CPU)
	app.Engine.Cats[0].X = 100
	app.Engine.Cats[1].X = 206
	app.Engine.Cats[2].X = 430
	for _, c := range app.Engine.Cats {
		c.Y = 80
	}
	flickerSetFollow(app.Engine, 0, 2, 430)
	im := image.NewNRGBA(image.Rect(0, 0, 64, 88))
	for row, count := range rowFrames {
		for col := 0; col < count; col++ {
			for y := 1; y < 7; y++ {
				for x := 1; x < 7; x++ {
					im.SetNRGBA(col*8+x, row*8+y, color.NRGBA{uint8(40 + row*12), 144, 208, 255})
				}
			}
		}
	}
	atlas := &Atlas{Image: im, CellW: 8, CellH: 8}
	for i, f := range fixtures {
		p := &PetWindow{HWND: f.pet, Cat: app.Engine.Cats[i], Atlas: atlas, Manifest: app.Engine.States[i].Player.Manifest, Index: i, CanvasW: 8, CanvasH: 8}
		app.Pets = append(app.Pets, p)
		app.Windows[f.pet] = p
	}
	setInterval()
	wait := user32.NewProc("MsgWaitForMultipleObjects")
	samples := 0
	lastTime := 0.0
	cleared := false
	deadline := time.Now().Add(1800 * time.Millisecond)
	for time.Now().Before(deadline) {
		smokePumpMessages() // Dispatch WM_TIMER through the production windowProc.
		if app.Quitting {
			t.Fatal("native rendering failed")
		}
		if app.LastTime != lastTime {
			lastTime = app.LastTime
			samples++
			if !cleared && app.Engine.States[0].Player.Action != "idle" {
				t.Fatal("blocked native cat flashed movement", app.Engine.States[0].Player.Action)
			}
			for _, p := range app.Pets {
				visible, _, _ := smokeIsVisible.Call(p.HWND)
				if visible == 0 {
					t.Fatal("pet became hidden")
				}
				if p.DIB.Bits == nil {
					t.Fatal("no presented surface")
				}
				pixels := unsafe.Slice((*byte)(p.DIB.Bits), p.Cat.W*p.Cat.H*4)
				opaque := false
				for j := 3; j < len(pixels); j += 4 {
					opaque = opaque || pixels[j] != 0
				}
				if !opaque {
					t.Fatal("presented empty surface")
				}
			}
		}
		if !cleared && app.LastTime >= .7 {
			app.Engine.Cats[1].Y = 400
			cleared = true
		}
		wait.Call(0, 0, 0, 100, 0x04ff)
	}
	if samples < 10 || !cleared || app.Engine.Cats[0].X <= 110 {
		t.Fatal("native follower failed to resume", samples, app.Engine.Cats[0].X)
	}
	t.Logf("Real WM_TIMER/windowProc/layered rendering: %d updates, blocked cat held idle then resumed at x=%.2f; visible nonempty surfaces retained", samples, app.Engine.Cats[0].X)
}
