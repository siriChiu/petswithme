//go:build windows && amd64

package main

import (
	"encoding/json"
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

var deadlineMsgWait = user32.NewProc("MsgWaitForMultipleObjects")
var deadlineProcessTimes = kernel32.NewProc("GetProcessTimes")

type deadlineMeasurement struct {
	Adaptive             bool      `json:"adaptive"`
	Cats                 int       `json:"cats"`
	ElapsedSeconds       float64   `json:"elapsedSeconds"`
	Updates              int       `json:"updates"`
	UpdatesPerSecond     float64   `json:"updatesPerSecond"`
	HoldsMS              []float64 `json:"holdsMs"`
	MeanHoldMS           float64   `json:"meanHoldMs"`
	HoldRMSEMS           float64   `json:"holdRmseMs"`
	MaxHoldErrorMS       float64   `json:"maxHoldErrorMs"`
	SkippedPoses         int       `json:"skippedPoses"`
	ProcessCPUSeconds    float64   `json:"processCpuSeconds"`
	SingleCoreCPUPercent float64   `json:"singleCoreCpuPercent"`
	TickWallMS           float64   `json:"tickWallMs"`
}

func deadlineCPU(t *testing.T) float64 {
	t.Helper()
	var c, x, k, u cpuFileTime
	ok, _, err := deadlineProcessTimes.Call(^uintptr(0), uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u)))
	if ok == 0 {
		t.Fatal(err)
	}
	return float64(k.ticks()+u.ticks()) / 1e7
}
func measureNativeDeadlines(t *testing.T, adaptive bool, count int) deadlineMeasurement {
	t.Helper()
	old := app
	oldMode := frameDeadlineScheduling
	fixtures := []*smokeFixture{}
	defer func() {
		killTimer.Call(app.Controller, 1)
		for _, p := range app.Pets {
			p.DIB.Close()
		}
		for _, f := range fixtures {
			f.close()
		}
		app = old
		frameDeadlineScheduling = oldMode
	}()
	for i := 0; i < count; i++ {
		f, err := newSmokeFixture()
		if err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, f)
	}
	app.Controller = fixtures[0].probe
	app.Start = time.Now()
	app.LastTime = 0
	app.LastIdleCheck = 0
	app.Idle = 0
	app.Hidden = false
	app.Quitting = false
	app.ClickThrough = false
	app.TimerIntervalMS = 0
	app.TimerBaseMS = 0
	app.TimerDeadline = 0
	app.Frames = map[string][]byte{}
	app.CachedBytes = 0
	app.Settings = NormalizeSettings(Settings{Size: 192, CPU: DefaultCPUSettings()})
	app.Settings.CPU.Enabled = false
	im := image.NewNRGBA(image.Rect(0, 0, 64, 8))
	clip := AnimationClip{}
	for i := 0; i < 8; i++ {
		for y := 1; y < 7; y++ {
			for x := 1; x < 7; x++ {
				im.SetNRGBA(i*8+x, y, color.NRGBA{uint8(64 + i*16), 144, 208, 255})
			}
		}
		clip.Loop = append(clip.Loop, AnimationFrame{Rect: &FrameRect{X: i * 8, W: 8, H: 8}, DurationMS: 120})
	}
	m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{X: .5, Y: 1}, Actions: map[string]AnimationAction{"idle": {AnimationClip: clip}, "walk_left": {AnimationClip: clip, Movement: &AnimationMovement{StrideRatio: 44.08 * .96 / 230, Verified: true}}}}
	atlas := &Atlas{Image: im, CellW: 8, CellH: 8}
	cats := []*Cat{}
	app.Pets = nil
	for i, f := range fixtures {
		c := NewCat(i, 230, 172, Rect{0, 0, 4000, 2000})
		c.X = float64(f.x)
		c.Y = float64(f.y)
		cats = append(cats, c)
		app.Pets = append(app.Pets, &PetWindow{HWND: f.pet, Cat: c, Atlas: atlas, Manifest: m, Index: i, CanvasW: 8, CanvasH: 8})
	}
	app.Engine = NewBehaviorEngine(cats, m)
	app.Engine.SetCPUSettings(app.Settings.CPU)
	app.Engine.Social.NextAttempt = 1000
	for i := range cats {
		app.Engine.Request(i, "walk_left", PriorityWander, 0, 60)
		app.Engine.States[i].HasDestination = true
		app.Engine.States[i].TargetX = 0
		cats[i].Direction = -1
		app.Engine.States[i].NextDecision = 1000
		app.Engine.States[i].Player.ElapsedMS = float64(i * 40)
	}
	frameDeadlineScheduling = adaptive
	setInterval()
	start := time.Now()
	const warmup = 1.2
	const duration = 4.5
	lastIndex := make([]int, count)
	lastChange := make([]float64, count)
	cpuStart, measurementStart := 0.0, 0.0
	result := deadlineMeasurement{Adaptive: adaptive, Cats: count}
	var msg Message
	for time.Since(start).Seconds() < duration {
		for {
			ok, _, _ := smokePeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
			if ok == 0 {
				break
			}
			if msg.HWND == app.Controller && msg.Message == 0x113 && msg.WParam == 1 {
				begin := time.Now()
				tick()
				elapsed := time.Since(start).Seconds()
				cost := time.Since(begin).Seconds() * 1000
				if app.Quitting {
					t.Fatal("render failed")
				}
				if elapsed >= warmup && measurementStart == 0 {
					measurementStart = elapsed
					cpuStart = deadlineCPU(t)
				}
				if measurementStart > 0 {
					result.Updates++
					result.TickWallMS += cost
				}
				for i, s := range app.Engine.States {
					if s.Player.Index != lastIndex[i] {
						if lastChange[i] >= warmup {
							result.HoldsMS = append(result.HoldsMS, (elapsed-lastChange[i])*1000)
							delta := (s.Player.Index - lastIndex[i] + 8) % 8
							if delta > 1 {
								result.SkippedPoses += delta - 1
							}
						}
						lastIndex[i] = s.Player.Index
						lastChange[i] = elapsed
					}
				}
			} else {
				translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
				dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
			}
		}
		wait, _, err := deadlineMsgWait.Call(0, 0, 0, 100, 0x04ff)
		if wait == 0xffffffff {
			t.Fatal(err)
		}
	}
	result.ElapsedSeconds = time.Since(start).Seconds() - measurementStart
	result.ProcessCPUSeconds = deadlineCPU(t) - cpuStart
	result.SingleCoreCPUPercent = 100 * result.ProcessCPUSeconds / result.ElapsedSeconds
	result.UpdatesPerSecond = float64(result.Updates) / result.ElapsedSeconds
	for _, v := range result.HoldsMS {
		result.MeanHoldMS += v
		err := v - 120
		result.HoldRMSEMS += err * err
		result.MaxHoldErrorMS = math.Max(result.MaxHoldErrorMS, math.Abs(err))
	}
	if len(result.HoldsMS) < 8 {
		t.Fatal("not enough native timer samples", len(result.HoldsMS))
	}
	result.MeanHoldMS /= float64(len(result.HoldsMS))
	result.HoldRMSEMS = math.Sqrt(result.HoldRMSEMS / float64(len(result.HoldsMS)))
	if result.UpdatesPerSecond > 70 {
		t.Fatal("unexpected high timer rate", result.UpdatesPerSecond)
	}
	return result
}
func TestWindowsFrameDeadlineCadenceExperiment(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	results := []deadlineMeasurement{}
	// Paired orders reduce a one-time warmup or VM load effect. No private art,
	// input injection, timer-resolution changes, preferences or tray are involved.
	for _, trial := range []struct {
		adaptive bool
		cats     int
	}{{false, 1}, {true, 1}, {true, 1}, {false, 1}, {false, 3}, {true, 3}} {
		results = append(results, measureNativeDeadlines(t, trial.adaptive, trial.cats))
	}
	b, _ := json.Marshal(results)
	t.Log("FRAME_DEADLINE_MEASUREMENTS=" + string(b))
}

func TestWindowsFrameDeadlineKeepsEarlierPendingTimer(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f, err := newSmokeFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	old := app
	oldMode := frameDeadlineScheduling
	defer func() { killTimer.Call(f.probe, 1); app = old; frameDeadlineScheduling = oldMode }()
	app.Controller = f.probe
	app.Start = time.Now()
	app.Settings = NormalizeSettings(Settings{CPU: DefaultCPUSettings()})
	app.Engine = deadlineFixture()
	app.Pets = nil
	app.Hidden = false
	app.Engine.States[0].Player.ElapsedMS = 50
	frameDeadlineScheduling = true
	now := nowSeconds()
	app.LastTime = now - .03
	app.TimerBaseMS = 50
	app.TimerIntervalMS = 50
	app.TimerDeadline = now + .02
	if ok, _, err := setTimer.Call(f.probe, 1, 20, 0); ok == 0 {
		t.Fatal(err)
	}
	earlier := app.TimerDeadline
	setInterval()
	if app.TimerDeadline != earlier || app.TimerIntervalMS != 50 {
		t.Fatal("callback postponed already armed timer", earlier, app.TimerDeadline)
	}
	now = nowSeconds()
	app.LastTime = now - .045
	app.TimerDeadline = now + .005
	earlier = app.TimerDeadline
	setInterval()
	if app.TimerDeadline != earlier {
		t.Fatal("minimum-delay clamp postponed near-due timer")
	}
	t.Log("A non-tick callback preserves the existing earlier native timer, including a wake less than10ms away")
}
