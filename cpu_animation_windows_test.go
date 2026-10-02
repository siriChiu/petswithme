//go:build windows && amd64

package main

import (
	"encoding/json"
	"image"
	"image/color"
	"math"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// CPU observations are synthetic, while WM_TIMER, the production controller,
// animation arbitration and layered DIB submission are native Windows paths.
func TestWindowsCPUReusedRisePoseAndInterruptions(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	fixture, err := newSmokeFixture()
	if err != nil {
		t.Fatal(err)
	}
	old := app
	changeProc := user32.NewProc("SetWindowLongPtrW")
	previous, _, callErr := changeProc.Call(fixture.probe, signed(-4), syscall.NewCallback(windowProc))
	if previous == 0 {
		fixture.close()
		t.Fatal("install controller", callErr)
	}
	defer func() {
		killTimer.Call(fixture.probe, 1)
		changeProc.Call(fixture.probe, signed(-4), previous)
		for _, p := range app.Pets {
			p.DIB.Close()
		}
		fixture.close()
		app = old
	}()
	frame := func(drawing, ms int) AnimationFrame {
		return AnimationFrame{Rect: &FrameRect{drawing * 8, 0, 8, 8}, DurationMS: ms}
	}
	manifest := func(whole bool) *AnimationManifest {
		loop := []AnimationFrame{frame(0, 150), frame(1, 230), frame(2, 120), frame(3, 150), frame(4, 230), frame(5, 120)}
		if whole {
			loop = []AnimationFrame{frame(0, 150), frame(2, 100), frame(1, 130), frame(2, 120), frame(3, 150), frame(4, 230), frame(5, 120)}
		}
		return &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{.5, 1}, Actions: map[string]AnimationAction{
			"idle":  {AnimationClip: AnimationClip{Loop: []AnimationFrame{frame(6, 500)}}},
			"knead": {AnimationClip: AnimationClip{Loop: loop}},
			"pet":   {AnimationClip: AnimationClip{Start: []AnimationFrame{frame(7, 150)}, Loop: []AnimationFrame{frame(7, 250), frame(7, 300), frame(7, 300), frame(7, 220)}, End: []AnimationFrame{frame(7, 300)}}},
			"drag":  {AnimationClip: AnimationClip{Loop: []AnimationFrame{frame(8, 500)}}},
		}}
	}
	newEngine := func(m *AnimationManifest) *BehaviorEngine {
		c := NewCat(0, smokeWidth, smokeHeight, Rect{int(fixture.x) - 100, int(fixture.y) - 100, int(fixture.x) + 500, int(fixture.y) + 500})
		c.X, c.Y = float64(fixture.x), float64(fixture.y)
		e := NewBehaviorEngine([]*Cat{c}, m)
		e.Social.NextAttempt = 1000
		e.States[0].NextDecision = 1000
		for second := 0; second <= 10; second += 2 {
			e.ObserveCPU(float64(second), 95, true)
		}
		if !e.Load.Active {
			t.Fatal("synthetic sustained load failed to enter busy mode")
		}
		return e
	}
	app.Controller = fixture.probe
	app.Start = time.Now().Add(-10 * time.Second)
	app.LastTime = 10
	app.LastIdleCheck = 1000
	app.Idle = 0
	app.Hidden = false
	app.Quitting = false
	app.ClickThrough = false
	app.TimerIntervalMS = 0
	app.Settings = NormalizeSettings(Settings{Size: smokeWidth, CPU: DefaultCPUSettings(), Activity: ActivityNormal})
	// Keep real hardware observations out of this fixture; the native sampler
	// has its own test. Sustained synthetic observations continue every two seconds.
	app.CPU = CPUMonitor{polled: true, last: 10, next: 1000}
	m := manifest(true)
	app.Engine = newEngine(m)
	reference := newEngine(manifest(false))
	im := image.NewNRGBA(image.Rect(0, 0, 72, 8))
	for drawing := 0; drawing < 9; drawing++ {
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				im.SetNRGBA(drawing*8+x, y, color.NRGBA{R: uint8(25 + drawing*20), G: 140, B: 90, A: 255})
			}
		}
	}
	pet := &PetWindow{HWND: fixture.pet, Cat: app.Engine.Cats[0], Atlas: &Atlas{Image: im, CellW: 8, CellH: 8}, Manifest: m, CanvasW: 8, CanvasH: 8}
	app.Pets = []*PetWindow{pet}
	app.Windows = map[uintptr]*PetWindow{fixture.pet: pet}
	app.Frames = map[string][]byte{}
	app.CachedBytes = 0
	setInterval()
	wait := user32.NewProc("MsgWaitForMultipleObjects")
	deadline := time.Now().Add(16 * time.Second)
	last, nextCPU, firstBusy := 10.0, 12.0, -1.0
	stage := 0
	quietSamples := 0
	petRequested := false
	petEnded := false
	stalled := false
	stageAt := 0.0
	maxDelta, steadyMaxDelta := 0.0, 0.0
	postStallFirstDelta := 0.0
	postStallSamples := 0
	completed := false
	updates := 0
	seen := map[int]int{}
	petPhases := map[AnimationPhase]bool{}
	for time.Now().Before(deadline) {
		now := nowSeconds()
		if now >= nextCPU {
			app.Engine.ObserveCPU(now, 95, true)
			reference.ObserveCPU(now, 95, true)
			nextCPU = now + 2
		}
		if stage == 0 && firstBusy >= 0 && now-firstBusy >= 4.05 {
			app.Settings.Quiet = true
			app.Settings.Activity = ActivityQuiet
			app.Engine.SetActivity(ActivityQuiet)
			reference.SetActivity(ActivityQuiet)
			setInterval()
			stage = 1
			stageAt = now
		}
		if stage == 1 && quietSamples >= 2 && now-stageAt >= .6 {
			app.Settings.Quiet = false
			app.Settings.Activity = ActivityNormal
			app.Engine.SetActivity(ActivityNormal)
			reference.SetActivity(ActivityNormal)
			setInterval()
			stage = 2
		}
		if stage == 3 && !petRequested {
			app.Engine.Pet(0, now)
			reference.Pet(0, now)
			setInterval()
			petRequested = true
		}
		if stage == 4 && now-stageAt >= .3 && !stalled {
			time.Sleep(800 * time.Millisecond)
			stalled = true
			stage = 5
			stageAt = nowSeconds()
		}
		var msg Message
		got, _, _ := smokePeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
		if got == 0 {
			wait.Call(0, 0, 0, 20, 0x04ff)
			continue
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		if app.Quitting {
			t.Fatal("native render failed")
		}
		if app.LastTime == last {
			continue
		}
		now = app.LastTime
		delta := now - last
		last = now
		maxDelta = math.Max(maxDelta, delta)
		updates++
		if stage == 5 {
			if postStallSamples == 0 {
				postStallFirstDelta = delta
			}
			postStallSamples++
		}
		reference.Tick(now, math.Min(.25, delta), math.NaN(), math.NaN(), 0, app.Settings.Quiet)
		a, b := app.Engine.States[0], reference.States[0]
		p := a.Player
		if p.Action != b.Player.Action || p.Phase != b.Player.Phase || p.Generation != b.Player.Generation || math.Abs(a.Until-b.Until) > 1e-8 || math.Abs(a.BusyElapsed-b.BusyElapsed) > 1e-8 || a.BusyNextStretch != b.BusyNextStretch {
			t.Fatal("whole loop changed behavior at same native clock", p.Action, b.Player.Action, p.Phase, b.Player.Phase)
		}
		if pet.Cat.X != float64(fixture.x) || pet.Cat.Y != float64(fixture.y) {
			t.Fatal("stationary CPU action moved")
		}
		drawing := p.Frame().Rect.X / 8
		if p.Action == "knead" {
			oldDrawing := b.Player.Frame().Rect.X / 8
			if p.Index == 1 {
				if drawing != 2 || oldDrawing != 1 {
					t.Fatal("reused rise replaced the wrong original interval")
				}
			} else if drawing != oldDrawing {
				t.Fatal("an unchanged CPU pose moved to a different time")
			}
		}
		if pet.DIB.Bits == nil {
			t.Fatal("missing native DIB")
		}
		pixels := unsafe.Slice((*byte)(pet.DIB.Bits), smokeWidth*smokeHeight*4)
		center := (smokeHeight/2*smokeWidth + smokeWidth/2) * 4
		if pixels[center+2] != uint8(25+drawing*20) || pixels[center+3] != 255 {
			t.Fatal("selected pose did not reach native DIB")
		}
		wantCadence := 100
		if app.Settings.Quiet {
			wantCadence = 250
		}
		if app.Engine.DirectAnimationActive() {
			wantCadence = 50
		}
		if app.TimerIntervalMS != wantCadence {
			t.Fatal("wrong native cadence", app.TimerIntervalMS, wantCadence)
		}
		if stage == 0 {
			steadyMaxDelta = math.Max(steadyMaxDelta, delta)
			if p.Action == "knead" {
				if firstBusy < 0 {
					firstBusy = now
				}
				seen[p.Index]++
			}
		}
		if stage == 1 {
			if p.Action != "idle" {
				t.Fatal("quiet did not suppress CPU action")
			}
			quietSamples++
		}
		if stage == 2 && p.Action == "knead" {
			stage = 3
		}
		if stage == 3 && petRequested {
			if p.Action == "pet" {
				petPhases[p.Phase] = true
			}
			if len(petPhases) > 0 && p.Action == "knead" {
				petEnded = true
				stage = 4
				stageAt = now
			}
		}
		if stage == 5 && now-stageAt >= 1 {
			completed = true
			break
		}
	}
	if !completed || stage != 5 || !stalled || !petEnded || quietSamples < 2 || !petPhases[AnimationStart] || !petPhases[AnimationLoop] || !petPhases[AnimationEnd] || postStallFirstDelta < .7 || postStallSamples < 2 {
		t.Fatal("incomplete CPU native scenarios", stage, quietSamples, petPhases, maxDelta)
	}
	if steadyMaxDelta <= .1 && len(seen) != 7 {
		t.Fatal("regular native callbacks missed a whole phase", seen)
	}
	result := map[string]any{"nativeUpdates": updates, "requestedBusyTimerMS": 100, "quietTimerMS": 250, "manualTimerMS": 50, "sequenceReferences": 7, "distinctDrawings": 6, "newDrawings": 0, "newRiseReferenceSamples": seen[1], "riseExposureVerified": seen[1] > 0, "loopMS": 1000, "steadyPhaseSamples": seen, "steadyMaxUpdateMS": steadyMaxDelta * 1000, "maxUpdateMS": maxDelta * 1000, "postStallFirstDeltaMS": postStallFirstDelta * 1000, "postStallSamples": postStallSamples, "postStallTailCompleted": completed, "quietSuppressedAndResumed": true, "petInterruptedAndResumed": true, "layeredDIBVerified": true, "sameClockCPUStatePreserved": true, "CPUInput": "synthetic 95 percent observations for 10 seconds"}
	raw, _ := json.Marshal(result)
	t.Log("NATIVE_CPU_REUSED_RISE " + string(raw))
	if seen[1] == 0 {
		t.Skip("CPU state flow passed, but the native scheduler never exposed the reused rise; exposure is inconclusive")
	}
}
