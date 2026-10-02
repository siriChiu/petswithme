//go:build windows && amd64

package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Exercise the real native timer and layered-window path with short authored
// poses. This measures visibility at each cadence; it deliberately does not
// promise that a 60ms drawing survives a 250ms presentation interval.
func TestWindowsIdleShortFramesAtNormalAndQuietCadence(t *testing.T) {
	profiles := []struct {
		name                                               string
		durations, drawings, legacyDurations, shortIndices []int
		closedIndex, legacyClosedIndex, cycleMS            int
	}{
		{"opening_only", []int{1200, 180, 180, 60, 100, 600, 400}, []int{0, 1, 2, 6, 3, 4, 5}, []int{1200, 180, 180, 160, 600, 400}, []int{3}, 2, 2, 2720},
		{"two_sided", []int{1200, 120, 60, 180, 60, 100, 600, 400}, []int{0, 1, 6, 2, 6, 3, 4, 5}, []int{1200, 180, 180, 160, 600, 400}, []int{2, 4}, 3, 2, 2720},
		{"long_cycle", []int{1800, 700, 90, 60, 100, 60, 120, 700}, []int{0, 1, 2, 6, 3, 6, 4, 5}, []int{1800, 700, 150, 100, 180, 700}, []int{3, 5}, 4, 3, 3630},
	}
	for _, profile := range profiles {
		for _, quiet := range []bool{false, true} {
			name := "normal"
			if quiet {
				name = "quiet"
			}
			t.Run(profile.name+"/"+name, func(t *testing.T) {
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
					t.Fatal("install real controller", callErr)
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
				durations := profile.durations
				drawings := profile.drawings
				loop := []AnimationFrame{}
				im := image.NewNRGBA(image.Rect(0, 0, 56, 8))
				for i := 0; i < 7; i++ {
					for y := 0; y < 8; y++ {
						for x := 0; x < 8; x++ {
							im.SetNRGBA(i*8+x, y, color.NRGBA{R: uint8(40 + i*25), G: 140, B: 80, A: 255})
						}
					}
				}
				for i, ms := range durations {
					loop = append(loop, AnimationFrame{Rect: &FrameRect{drawings[i] * 8, 0, 8, 8}, DurationMS: ms})
				}
				m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{.5, 1}, Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: loop}}}}
				legacyLoop := []AnimationFrame{}
				for i, ms := range profile.legacyDurations {
					legacyLoop = append(legacyLoop, AnimationFrame{Rect: &FrameRect{i * 8, 0, 8, 8}, DurationMS: ms})
				}
				legacy := NewAnimationPlayer(&AnimationManifest{Fallback: "idle", Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: legacyLoop}}}})
				c := NewCat(0, smokeWidth, smokeHeight, Rect{int(fixture.x) - 100, int(fixture.y) - 100, int(fixture.x) + 500, int(fixture.y) + 500})
				c.X = float64(fixture.x)
				c.Y = float64(fixture.y)
				app.Controller = fixture.probe
				app.Start = time.Now()
				app.LastTime = 0
				app.LastIdleCheck = 1000
				app.Idle = 0
				app.Hidden = false
				app.Quitting = false
				app.ClickThrough = false
				app.TimerIntervalMS = 0
				app.Settings = NormalizeSettings(Settings{Size: smokeWidth, Quiet: quiet, CPU: DefaultCPUSettings()})
				if quiet {
					app.Settings.Activity = ActivityQuiet
				} else {
					app.Settings.Activity = ActivityNormal
				}
				app.Settings.CPU.Enabled = false
				app.Engine = NewBehaviorEngine([]*Cat{c}, m)
				app.Engine.SetActivity(app.Settings.Activity)
				app.Engine.SetCPUSettings(app.Settings.CPU)
				app.Engine.States[0].NextDecision = 1000
				app.Engine.Social.NextAttempt = 1000
				app.Frames = map[string][]byte{}
				app.CachedBytes = 0
				pet := &PetWindow{HWND: fixture.pet, Cat: c, Atlas: &Atlas{Image: im, CellW: 8, CellH: 8}, Manifest: m, CanvasW: 8, CanvasH: 8}
				app.Pets = []*PetWindow{pet}
				app.Windows = map[uintptr]*PetWindow{fixture.pet: pet}
				setInterval()
				expectedInterval := 50
				if quiet {
					expectedInterval = 250
				}
				if app.TimerIntervalMS != expectedInterval {
					t.Fatal("unexpected idle cadence", app.TimerIntervalMS)
				}
				first, last := -1.0, 0.0
				logicalMS := 0.0
				minDelta, maxDelta := 1000.0, 0.0
				samples := 0
				visible := map[int]map[int]int{}
				legacyClosed := map[int]int{}
				wait := user32.NewProc("MsgWaitForMultipleObjects")
				deadline := time.Now().Add(22 * time.Second)
				for logicalMS < float64(4*profile.cycleMS) && time.Now().Before(deadline) {
					// Observe after each dispatch, not after draining the queue: a
					// drained queue can contain several independently rendered timers.
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
					if app.LastTime != last {
						now := app.LastTime
						if first < 0 {
							first = now
						} else {
							delta := (now - last) * 1000
							minDelta = math.Min(minDelta, delta)
							maxDelta = math.Max(maxDelta, delta)
							clampedDT := math.Min(.25, now-last)
							logicalMS += clampedDT * 1000
							legacy.Tick(clampedDT)
						}
						last = now
						samples++
						state := app.Engine.States[0]
						player := state.Player
						if player.Action != "idle" || player.Phase != AnimationLoop {
							t.Fatal("unexpected idle interruption", player.Action, player.Phase)
						}
						if (player.Index == profile.closedIndex) != (legacy.Index == profile.legacyClosedIndex) {
							t.Fatal("complete closed-eye interval changed at the same native clock", logicalMS, player.Index, legacy.Index)
						}
						if c.X != float64(fixture.x) || c.Y != float64(fixture.y) {
							t.Fatal("stationary idle moved")
						}
						if pet.DIB.Bits == nil {
							t.Fatal("missing presented DIB")
						}
						pixels := unsafe.Slice((*byte)(pet.DIB.Bits), c.W*c.H*4)
						center := (c.H/2*c.W + c.W/2) * 4
						expectedRed := uint8(40 + drawings[player.Index]*25)
						if pixels[center+2] != expectedRed || pixels[center+3] != 255 {
							t.Fatal("selected pose did not reach native layered pixels", player.Index, pixels[center:center+4])
						}
						cycle := int(math.Floor((logicalMS + 1e-7) / float64(profile.cycleMS)))
						if cycle < 4 {
							if visible[cycle] == nil {
								visible[cycle] = map[int]int{}
							}
							visible[cycle][player.Index]++
							if legacy.Index == profile.legacyClosedIndex {
								legacyClosed[cycle]++
							}
						}
					}
				}
				if logicalMS < float64(4*profile.cycleMS) || samples < 30 {
					t.Fatal("native timer did not complete four cycles", samples, logicalMS)
				}
				shown := 0
				counts := []map[string]int{}
				for cycle := 0; cycle < 4; cycle++ {
					countsForCycle := map[string]int{"cycle": cycle, "completeClosedSamples": visible[cycle][profile.closedIndex], "legacyPlayerClosedSamplesAtSameClock": legacyClosed[cycle]}
					for occurrence, index := range profile.shortIndices {
						n := visible[cycle][index]
						if n > 0 {
							shown++
						}
						countsForCycle[fmt.Sprintf("shortOccurrence%dSamples", occurrence)] = n
					}
					counts = append(counts, countsForCycle)
				}
				// Low-cadence omissions are measured, not a failure that should cause the
				// engine to stretch authored durations or change the quiet-mode policy.
				result := map[string]any{"mode": name, "requestedIntervalMS": expectedInterval, "nativeUpdates": samples, "wallSeconds": last - first, "logicalElapsedMS": logicalMS, "minUpdateMS": minDelta, "maxUpdateMS": maxDelta, "shortOccurrencesShown": shown, "shortOccurrencesTotal": 4 * len(profile.shortIndices), "perCycle": counts, "authoredLoopMS": profile.cycleMS, "profile": profile.name, "layeredPixelsVerified": true}
				b, _ := json.Marshal(result)
				t.Log("NATIVE_IDLE_CADENCE " + string(b))
			})
		}
	}
}
