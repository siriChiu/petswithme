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

// Run a denser pet clip through real WM_TIMER and the layered DIB. A reference
// engine retains the original clip and receives the same event times and native
// deltas. Extra drawings must not change requests, recovery or idle deadlines.
func TestWindowsPetInbetweenPreservesInteractionFlow(t *testing.T) {
	for _, scenario := range []struct {
		name                 string
		quiet, repeat, delay bool
	}{
		{"normal", false, false, false}, {"quiet_manual", true, false, false},
		{"repeated", false, true, false}, {"delayed_timer", false, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			fixture, err := newSmokeFixture()
			if err != nil {
				t.Fatal(err)
			}
			old := app
			changeProc := user32.NewProc("SetWindowLongPtrW")
			oldProc, _, callErr := changeProc.Call(fixture.probe, signed(-4), syscall.NewCallback(windowProc))
			if oldProc == 0 {
				fixture.close()
				t.Fatal("install controller", callErr)
			}
			defer func() {
				killTimer.Call(fixture.probe, 1)
				changeProc.Call(fixture.probe, signed(-4), oldProc)
				for _, p := range app.Pets {
					p.DIB.Close()
				}
				fixture.close()
				app = old
			}()
			frame := func(drawing, ms int) AnimationFrame {
				return AnimationFrame{Rect: &FrameRect{drawing * 8, 0, 8, 8}, DurationMS: ms}
			}
			manifest := func(dense bool) *AnimationManifest {
				loop := []AnimationFrame{frame(2, 250), frame(3, 300), frame(4, 300), frame(5, 220)}
				if dense {
					loop = []AnimationFrame{frame(2, 250), frame(3, 200), frame(7, 100), frame(4, 300), frame(5, 220)}
				}
				return &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{.5, 1}, Actions: map[string]AnimationAction{
					"idle": {AnimationClip: AnimationClip{Loop: []AnimationFrame{frame(0, 500)}}},
					"pet":  {AnimationClip: AnimationClip{Start: []AnimationFrame{frame(1, 150)}, Loop: loop, End: []AnimationFrame{frame(6, 300)}}},
				}}
			}
			im := image.NewNRGBA(image.Rect(0, 0, 64, 8))
			for i := 0; i < 8; i++ {
				for y := 0; y < 8; y++ {
					for x := 0; x < 8; x++ {
						im.SetNRGBA(i*8+x, y, color.NRGBA{R: uint8(30 + i*25), G: 140, B: 80, A: 255})
					}
				}
			}
			app.Controller = fixture.probe
			app.Start = time.Now()
			app.LastTime = 0
			app.LastIdleCheck = 1000
			app.Idle = 0
			app.Hidden = false
			app.Quitting = false
			app.ClickThrough = false
			app.TimerIntervalMS = 0
			app.Settings = NormalizeSettings(Settings{Size: smokeWidth, Quiet: scenario.quiet, CPU: DefaultCPUSettings()})
			app.Settings.CPU.Enabled = false
			app.Settings.Activity = ActivityNormal
			if scenario.quiet {
				app.Settings.Activity = ActivityQuiet
			}
			newEngine := func(m *AnimationManifest) *BehaviorEngine {
				c := NewCat(0, smokeWidth, smokeHeight, Rect{int(fixture.x) - 100, int(fixture.y) - 100, int(fixture.x) + 500, int(fixture.y) + 500})
				c.X = float64(fixture.x)
				c.Y = float64(fixture.y)
				e := NewBehaviorEngine([]*Cat{c}, m)
				e.SetActivity(app.Settings.Activity)
				e.SetCPUSettings(app.Settings.CPU)
				e.States[0].NextDecision = 1000
				e.Social.NextAttempt = 1000
				return e
			}
			dense := manifest(true)
			app.Engine = newEngine(dense)
			reference := newEngine(manifest(false))
			pet := &PetWindow{HWND: fixture.pet, Cat: app.Engine.Cats[0], Atlas: &Atlas{Image: im, CellW: 8, CellH: 8}, Manifest: dense, CanvasW: 8, CanvasH: 8}
			app.Pets = []*PetWindow{pet}
			app.Windows = map[uintptr]*PetWindow{fixture.pet: pet}
			app.Frames = map[string][]byte{}
			app.CachedBytes = 0
			app.Engine.Pet(0, 0)
			reference.Pet(0, 0)
			setInterval()
			wait := user32.NewProc("MsgWaitForMultipleObjects")
			deadline := time.Now().Add(8 * time.Second)
			last := 0.0
			samples, newSamples := 0, 0
			repeated, stalled, finished := false, false, false
			endAt, idleAt := -1.0, -1.0
			maxDelta := 0.0
			seen := map[AnimationPhase]bool{}
			for time.Now().Before(deadline) {
				eventTime := nowSeconds()
				if scenario.repeat && !repeated && eventTime >= .8 {
					app.Engine.Pet(0, eventTime)
					reference.Pet(0, eventTime)
					setInterval()
					repeated = true
				}
				if scenario.delay && !stalled && eventTime >= .9 {
					time.Sleep(800 * time.Millisecond)
					stalled = true
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
					t.Fatal("native rendering failed")
				}
				if app.LastTime == last {
					continue
				}
				now := app.LastTime
				delta := now - last
				maxDelta = math.Max(maxDelta, delta)
				last = now
				samples++
				reference.Tick(now, math.Min(.25, delta), math.NaN(), math.NaN(), 0, scenario.quiet)
				a, b := app.Engine.States[0], reference.States[0]
				p := a.Player
				if p.Action != b.Player.Action || p.Phase != b.Player.Phase || a.Until != b.Until || a.Ending != b.Ending || p.Generation != b.Player.Generation {
					t.Fatalf("dense clip changed flow at %.3f: action %s/%s phase %s/%s deadline %.3f/%.3f", now, p.Action, b.Player.Action, p.Phase, b.Player.Phase, a.Until, b.Until)
				}
				if pet.Cat.X != float64(fixture.x) || pet.Cat.Y != float64(fixture.y) {
					t.Fatal("pet moved root")
				}
				f := p.Frame()
				drawing := f.Rect.X / 8
				if drawing == 7 && b.Player.Frame().Rect.X/8 != 3 {
					t.Fatal("new drawing replaced a pose outside original loop1")
				}
				if drawing != 7 && drawing != b.Player.Frame().Rect.X/8 {
					t.Fatal("unchanged drawing was reordered")
				}
				if pet.DIB.Bits == nil {
					t.Fatal("no submitted DIB")
				}
				pixels := unsafe.Slice((*byte)(pet.DIB.Bits), smokeWidth*smokeHeight*4)
				center := (smokeHeight/2*smokeWidth + smokeWidth/2) * 4
				if pixels[center+2] != uint8(30+drawing*25) || pixels[center+3] != 255 {
					t.Fatal("selected drawing did not reach native layered pixels")
				}
				if drawing == 7 {
					newSamples++
				}
				if p.Action == "pet" {
					seen[p.Phase] = true
					if app.TimerIntervalMS != 50 {
						t.Fatal("manual pet lost50ms cadence", app.TimerIntervalMS)
					}
					if p.Phase == AnimationEnd && endAt < 0 {
						endAt = now
					}
				} else if endAt >= 0 && p.Action == "idle" {
					idleAt = now
					finished = true
					want := 50
					if scenario.quiet {
						want = 250
					}
					if app.TimerIntervalMS != want {
						t.Fatal("idle cadence did not recover", app.TimerIntervalMS)
					}
					break
				}
			}
			if !finished || !seen[AnimationStart] || !seen[AnimationLoop] || !seen[AnimationEnd] {
				t.Fatal("incomplete actual interaction", finished, seen, newSamples)
			}
			if scenario.repeat && !repeated {
				t.Fatal("repeat not exercised")
			}
			if scenario.delay && (!stalled || maxDelta < .7) {
				t.Fatal("native timer delay not exercised", maxDelta)
			}
			// WM_TIMER can legally skip a short drawing; report its exposure rather
			// than misclassifying scheduler jitter as a flow regression.
			result := map[string]any{"scenario": scenario.name, "nativeUpdates": samples, "newDrawingSamples": newSamples, "endAt": endAt, "idleAt": idleAt, "maxUpdateMS": maxDelta * 1000, "requestedPetTimerMS": 50, "quietIdleTimerMS": 250, "loopMS": 1070, "originalFlowAtSameClock": true, "layeredPixelsVerified": true}
			raw, _ := json.Marshal(result)
			t.Log("NATIVE_PET_FLOW " + string(raw))
		})
	}
}
