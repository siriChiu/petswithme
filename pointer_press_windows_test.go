//go:build windows && amd64

package main

import (
	"math"
	"runtime"
	"testing"
	"time"
)

func TestWindowsClicksWaitForActualDragMovement(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	fixture, err := newSmokeFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.close()
	old := app
	defer func() { releaseCapture.Call(); killTimer.Call(fixture.probe, 1); app = old }()
	app.Hidden = false
	app.Quitting = false
	app.ClickThrough = false
	app.Start = time.Now()
	app.Controller = fixture.probe
	app.Settings = NormalizeSettings(Settings{Size: 96, CPU: DefaultCPUSettings()})
	c := NewCat(0, 96, 96, Rect{0, 0, 2000, 1200})
	c.X, c.Y = float64(fixture.x), float64(fixture.y)
	manifest := DefaultAnimationManifest()
	app.Engine = NewBehaviorEngine([]*Cat{c}, manifest)
	app.Engine.Social.NextAttempt = 1000
	pet := &PetWindow{HWND: fixture.pet, Cat: c, Manifest: manifest, Index: 0, CanvasW: 8, CanvasH: 8}
	app.Pets = []*PetWindow{pet}
	app.Windows = map[uintptr]*PetWindow{fixture.pet: pet}
	windowProc(fixture.pet, 0x201, 0, 0)
	if !pet.Down || !c.Pressed || c.Dragging {
		t.Fatal("press immediately became pickup")
	}
	x, y := c.X, c.Y
	dragMove(pet, Point{pet.Press.X + 3, pet.Press.Y + 1})
	frame := app.Engine.Tick(.016, .016, math.NaN(), math.NaN(), 0, false)[0]
	if frame.Action == "drag" || c.X != x || c.Y != y || pet.DidDrag {
		t.Fatal("sub-threshold click moved or showed drag")
	}
	windowProc(fixture.pet, 0x202, 0, 0)
	if pet.Down || c.Pressed || c.Dragging || app.Engine.States[0].Action != "pet" {
		t.Fatal("click release was not a clean pet")
	}
	windowProc(fixture.pet, 0x203, 0, 0)
	frame = app.Engine.Tick(.08, .016, math.NaN(), math.NaN(), 0, false)[0]
	if frame.Action != "play" || !app.Engine.States[0].OneShot {
		t.Fatal("double-click did not become one jump")
	}
	windowProc(fixture.pet, 0x201, 0, 0)
	dragMove(pet, Point{pet.Press.X + 8, pet.Press.Y})
	frame = app.Engine.Tick(.12, .016, math.NaN(), math.NaN(), 0, false)[0]
	if !pet.DidDrag || !c.Dragging || frame.Action != "drag" {
		t.Fatal("actual movement did not begin pickup")
	}
	windowProc(fixture.pet, 0x1f, 0, 0)
	frame = app.Engine.Tick(.15, .016, math.NaN(), math.NaN(), 0, false)[0]
	if pet.Down || c.Pressed || c.Dragging || frame.Action != "idle" {
		t.Fatal("cancelled capture left a press or drag")
	}
	windowProc(fixture.pet, 0x201, 0, 0)
	toggleHidden()
	if pet.Down || c.Pressed || c.Dragging {
		t.Fatal("hiding retained pressed state")
	}
	toggleHidden()
	windowProc(fixture.pet, 0x201, 0, 0)
	if err := setClickThrough(true); err != nil {
		t.Fatal(err)
	}
	if pet.Down || c.Pressed || c.Dragging {
		t.Fatal("clickthrough retained pressed state")
	}
	if err := setClickThrough(false); err != nil {
		t.Fatal(err)
	}
	foreground, _, _ := smokeGetForeground.Call()
	if foreground == fixture.pet {
		t.Fatal("pointer controls activated pet")
	}
	t.Log("Native click, double-click, threshold drag, capture cancel, hide and clickthrough preserve gesture state without a pickup flash")
}
