package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestCPUPercentCounterMath(t *testing.T) {
	for _, test := range []struct {
		p, c  CPUCounters
		want  float64
		valid bool
	}{
		{CPUCounters{}, CPUCounters{100, 100, 0}, 0, true},
		{CPUCounters{}, CPUCounters{0, 100, 100}, 100, true},
		{CPUCounters{}, CPUCounters{40, 100, 100}, 80, true},
		{CPUCounters{}, CPUCounters{400, 1000, 1000}, 80, true},
		{CPUCounters{}, CPUCounters{}, 0, false},
		{CPUCounters{1, 2, 3}, CPUCounters{0, 3, 4}, 0, false},
		{CPUCounters{1, 2, 3}, CPUCounters{2, 1, 4}, 0, false},
		{CPUCounters{1, 2, 3}, CPUCounters{2, 3, 2}, 0, false},
		{CPUCounters{}, CPUCounters{101, 100, 100}, 0, false},
		{CPUCounters{}, CPUCounters{0, math.MaxUint64, 1}, 0, false},
		{CPUCounters{math.MaxUint64 - 500, math.MaxUint64 - 400, math.MaxUint64 - 300}, CPUCounters{math.MaxUint64 - 480, math.MaxUint64 - 300, math.MaxUint64 - 200}, 90, true},
	} {
		value, valid := CPUPercent(test.p, test.c)
		if valid != test.valid || valid && math.Abs(value-test.want) > 1e-9 {
			t.Fatalf("%+v => %g %v", test, value, valid)
		}
	}
}
func TestCPUSamplerBaselinesFailureAndLongGap(t *testing.T) {
	s := CPUCounterSampler{}
	if _, valid := s.Sample(0, CPUCounters{0, 100, 100}, true); valid {
		t.Fatal("first baseline gave usage")
	}
	if value, valid := s.Sample(2, CPUCounters{20, 200, 200}, true); !valid || value != 90 {
		t.Fatalf("usage %g %v", value, valid)
	}
	if _, valid := s.Sample(4, CPUCounters{10, 150, 150}, true); valid || s.known {
		t.Fatal("regression retained baseline")
	}
	if _, valid := s.Sample(6, CPUCounters{20, 200, 200}, true); valid {
		t.Fatal("recovery skipped baseline")
	}
	if _, valid := s.Sample(8, CPUCounters{}, false); valid || s.known {
		t.Fatal("failure retained counter")
	}
	s.Sample(10, CPUCounters{20, 200, 200}, true)
	if _, valid := s.Sample(100, CPUCounters{40, 400, 400}, true); valid {
		t.Fatal("counted unseen gap")
	}
	if _, valid := s.Sample(90, CPUCounters{50, 500, 500}, true); valid {
		t.Fatal("backward time accepted")
	}
	if _, valid := s.Sample(math.NaN(), CPUCounters{}, true); valid || s.known {
		t.Fatal("NaN time accepted")
	}
}
func TestCPUPollGateLimitsReadsAndPauses(t *testing.T) {
	m := CPUMonitor{}
	reads := 0
	reader := func() (CPUCounters, bool) {
		reads++
		return CPUCounters{uint64(reads), uint64(reads * 10), uint64(reads * 10)}, true
	}
	for tick := 0; tick < 10000; tick++ {
		m.Poll(float64(tick)/1000, true, false, reader)
	}
	if reads != 5 {
		t.Fatalf("1000Hz UI sampled %d times in 10 seconds", reads)
	}
	m.Poll(100, true, false, reader)
	if reads != 6 {
		t.Fatal("catch-up read burst")
	}
	for tick := 100; tick < 200; tick++ {
		m.Poll(float64(tick), true, true, reader)
	}
	if reads != 6 {
		t.Fatal("sampled while hidden")
	}
	if _, valid, observed := m.Poll(200, true, false, reader); valid || !observed {
		t.Fatal("show did not establish baseline")
	}
	m.Poll(202, false, false, reader)
	if reads != 7 {
		t.Fatal("sampled disabled")
	}
	if _, valid, _ := m.Poll(204, true, false, reader); valid {
		t.Fatal("enable reused disabled interval")
	}
}
func TestCPULoadSustainedSmoothingAndHysteresis(t *testing.T) {
	d := NewCPULoadState(DefaultCPUSettings())
	for now := 0.; now <= 8; now += 2 {
		d.Observe(now, 90, true)
		if d.Active {
			t.Fatal("entered too early")
		}
	}
	d.Observe(10, 90, true)
	if !d.Active {
		t.Fatal("did not enter at 10 seconds")
	}
	for now := 12.; now <= 40; now += 2 {
		d.Observe(now, 60, true)
		if !d.Active {
			t.Fatal("deadband chatter")
		}
	}
	firstLow := -1.0
	for now := 42.; now <= 70; now += 2 {
		d.Observe(now, 0, true)
		if firstLow < 0 && d.Smoothed < 50 {
			firstLow = now
		}
		if now-firstLow < 10 && firstLow >= 0 && !d.Active {
			t.Fatal("exited before sustained low window")
		}
	}
	if d.Active {
		t.Fatal("never exited")
	}
	d = NewCPULoadState(DefaultCPUSettings())
	d.Observe(0, 20, true)
	d.Observe(2, 100, true)
	if d.Smoothed >= 70 || d.Active {
		t.Fatal("one spike not smoothed")
	}
	d.Observe(4, 20, true)
	if d.entering {
		t.Fatal("broken high period retained")
	}
	d.Observe(20, 100, true)
	if d.Active {
		t.Fatal("long gap credited")
	}
	d.Observe(22, math.NaN(), true)
	if d.Active || d.known {
		t.Fatal("invalid value retained")
	}
}
func TestCPULoadExactThresholdsAndDisabled(t *testing.T) {
	d := NewCPULoadState(DefaultCPUSettings())
	for now := 0.; now <= 40; now += 2 {
		d.Observe(now, 70, true)
	}
	if d.Active {
		t.Fatal("equal enter threshold entered")
	}
	d.Active = true
	d.Smoothed = 50
	for now := 42.; now <= 70; now += 2 {
		d.Observe(now, 50, true)
	}
	if !d.Active {
		t.Fatal("equal exit threshold exited")
	}
	d.Expire(100)
	if d.Active {
		t.Fatal("stale busy retained")
	}
	config := DefaultCPUSettings()
	config.Enabled = false
	d = NewCPULoadState(config)
	for now := 0.; now <= 40; now += 2 {
		d.Observe(now, 100, true)
	}
	if d.Active || d.known {
		t.Fatal("disabled detector active")
	}
}
func cpuFixture() *AnimationManifest {
	m := DefaultAnimationManifest()
	makeClip := func(x int) AnimationClip {
		return AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{x, 0, 8, 8}, DurationMS: 100}, {Rect: &FrameRect{x + 8, 0, 8, 8}, DurationMS: 100}}}
	}
	m.Actions["knead"] = AnimationAction{AnimationClip: makeClip(0)}
	m.Actions["stretch"] = AnimationAction{AnimationClip: makeClip(32)}
	return m
}
func cpuEngine(n int) *BehaviorEngine {
	e := NewBehaviorEngine(behaviorCats(n), cpuFixture())
	e.Social.NextAttempt = math.Inf(1)
	for _, s := range e.States {
		s.NextDecision = math.Inf(1)
	}
	return e
}
func cpuTick(e *BehaviorEngine, step int, quiet bool) []BehaviorFrame {
	now := float64(step) / 10
	if step%20 == 0 {
		e.ObserveCPU(now, 90, true)
	}
	return e.Tick(now, .1, -10000, -10000, 1000, quiet)
}
func TestCPUKneadStationaryAndFiveMinuteStretch(t *testing.T) {
	e := cpuEngine(1)
	x, y := e.Cats[0].X, e.Cats[0].Y
	stretches := 0
	previous := ""
	firstStretch := -1.0
	for step := 0; step <= 9200; step++ {
		f := cpuTick(e, step, false)[0]
		if e.Cats[0].X != x || e.Cats[0].Y != y {
			t.Fatal("CPU kneading moved cat")
		}
		if step >= 100 && f.Action != "knead" && f.Action != "stretch" {
			t.Fatalf("CPU lost to idle sleep: %s at %g", f.Action, float64(step)/10)
		}
		if f.Action == "stretch" && previous != "stretch" {
			stretches++
			if firstStretch < 0 {
				firstStretch = float64(step) / 10
			}
		}
		previous = f.Action
	}
	if firstStretch < 309.8 || firstStretch > 310.2 || stretches != 3 {
		t.Fatalf("stretch count=%d first=%g", stretches, firstStretch)
	}
	if e.States[0].BusyStretching {
		t.Fatal("stretch never returned to knead")
	}
}
func TestCPUArtworkGates(t *testing.T) {
	m := cpuFixture()
	if !HasCPUAction(m, "knead", "calm") || !HasCPUAction(m, "stretch", "calm") {
		t.Fatal("authored clips unavailable")
	}
	for _, fake := range []AnimationAction{{Fallback: "idle"}, {AnimationClip: m.Actions["knead"].AnimationClip, DemoFallback: true}, m.Actions["idle"]} {
		m.Actions["knead"] = fake
		if HasCPUAction(m, "knead", "calm") {
			t.Fatal("fake knead accepted")
		}
	}
	e := NewBehaviorEngine(behaviorCats(1), DefaultAnimationManifest())
	e.States[0].NextDecision = math.Inf(1)
	for step := 0; step < 4000; step++ {
		f := cpuTick(e, step, false)[0]
		if f.Action == "knead" || f.Action == "stretch" {
			t.Fatal("missing art was fabricated")
		}
	}
	e = cpuEngine(1)
	delete(e.States[0].Player.Manifest.Actions, "stretch")
	for step := 0; step < 4000; step++ {
		f := cpuTick(e, step, false)[0]
		if step >= 100 && f.Action != "knead" {
			t.Fatal("missing stretch broke knead")
		}
	}
}
func TestCPUQuietDragAndManualPlayPause(t *testing.T) {
	e := cpuEngine(1)
	for step := 0; step < 500; step++ {
		cpuTick(e, step, false)
	}
	elapsed := e.States[0].BusyElapsed
	for step := 500; step < 700; step++ {
		cpuTick(e, step, true)
	}
	if math.Abs(e.States[0].BusyElapsed-elapsed) > 1e-6 {
		t.Fatal("quiet counted busy time")
	}
	e.Cats[0].Dragging = true
	for step := 700; step < 900; step++ {
		if cpuTick(e, step, false)[0].Action != "drag" {
			t.Fatal("CPU overrode drag")
		}
	}
	if math.Abs(e.States[0].BusyElapsed-elapsed) > 1e-6 {
		t.Fatal("drag counted busy time")
	}
	e.Cats[0].Dragging = false
	cpuTick(e, 900, false)
	e.Play(0, 90.1)
	if cpuTick(e, 901, false)[0].Action != "play" {
		t.Fatal("CPU overrode manual play")
	}
	e.Pet(0, 90.2)
	if cpuTick(e, 902, false)[0].Action != "pet" {
		t.Fatal("CPU overrode pet")
	}
}
func TestCPUExitResetAndNoCatchupStretch(t *testing.T) {
	e := cpuEngine(1)
	for step := 0; step < 3090; step++ {
		cpuTick(e, step, false)
	}
	if e.States[0].BusyStretching {
		t.Fatal("early stretch")
	}
	e.ResetCPU()
	e.Tick(1000, .1, -10000, -10000, 0, false)
	if e.States[0].BusyElapsed != 0 || e.States[0].BusyStretching {
		t.Fatal("hide/resume carried busy stretch")
	}
	for step := 10000; step < 10200; step++ {
		cpuTick(e, step, false)
	}
	if e.States[0].BusyStretching || e.States[0].BusyElapsed > 11 {
		t.Fatal("recovery caught up skipped interval")
	}
	e.ObserveCPU(1020, 0, false)
	e.Tick(1020, .1, 0, 0, 0, false)
	if e.States[0].Priority == PriorityBusy || e.States[0].BusyElapsed != 0 {
		t.Fatal("failed sample retained busy action")
	}
}
func TestCPUCancelsSocialAndIsPerCatGated(t *testing.T) {
	e := cpuEngine(2)
	e.Social.Start(e.Cats, 0, 1, 0)
	other := DefaultAnimationManifest()
	e.SetManifest(1, other)
	for step := 0; step <= 100; step++ {
		cpuTick(e, step, false)
	}
	if len(e.Social.Reservations) != 0 || e.Cats[0].Mode != "knead" || e.Cats[1].Mode == "knead" {
		t.Fatal("CPU/social/per-cat gate failed")
	}
}
func TestCPUSettingsLoadAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, data := range []string{`{"cpu":{"enabled":false}}`, `{"cpu":{"enabled":false,"enterPercent":0,"exitPercent":0,"enterSeconds":0,"exitSeconds":0,"stretchEverySeconds":0}}`, `{"quiet":false}`} {
		os.WriteFile(path, []byte(data), 0600)
		s := LoadSettings(path)
		want := data == `{"quiet":false}`
		if s.CPU.Enabled != want || s.CPU.StretchEverySeconds != 300 || s.CPU.Validate() != nil {
			t.Fatalf("bad migration %+v", s.CPU)
		}
	}
	for _, bad := range []CPUSettings{{true, 100, 50, 10, 10, 300}, {true, 70, 0, 10, 10, 300}, {true, 50, 70, 10, 10, 300}, {true, 70, 50, 0, 10, 300}, {true, math.NaN(), 50, 10, 10, 300}} {
		if bad.Validate() == nil {
			t.Fatal("bad settings accepted")
		}
	}
	s := DefaultCPUSettings()
	s.EnterPercent = 80
	s.ExitPercent = 40
	s.EnterSeconds = 20
	s.StretchEverySeconds = 120
	if s.Validate() != nil {
		t.Fatal("valid custom settings rejected")
	}
}

func TestCPUMoodGateRechecksAfterDirectAction(t *testing.T) {
	for _, drag := range []bool{false, true} {
		e := cpuEngine(1)
		m := e.States[0].Player.Manifest
		knead := m.Actions["knead"]
		m.Actions["knead"] = AnimationAction{Fallback: "idle", Moods: map[string]AnimationClip{"happy": knead.AnimationClip}}
		for now := 0.; now <= 10; now += 2 {
			e.ObserveCPU(now, 90, true)
		}
		s := e.States[0]
		s.Mood = "happy"
		if drag {
			e.Cats[0].Dragging = true
			e.Tick(10, .1, 0, 0, 0, false)
			e.Cats[0].Dragging = false
		} else {
			e.Pet(0, 8)
			s.Player.Phase = AnimationDone
		}
		f := e.Tick(10.1, .1, -10000, -10000, 0, false)[0]
		if s.Mood != "calm" || f.Action == "knead" || s.Priority == PriorityBusy {
			t.Fatalf("mood fallback accepted (drag=%v): %+v %s", drag, f, s.Mood)
		}
	}
}
func TestCPUStretchAllowsSingleHoldWithDistinctEntryRecovery(t *testing.T) {
	e := cpuEngine(1)
	m := e.States[0].Player.Manifest
	stretch := AnimationClip{Start: []AnimationFrame{{Rect: &FrameRect{32, 0, 8, 8}, DurationMS: 200}}, Loop: []AnimationFrame{{Rect: &FrameRect{40, 0, 8, 8}, DurationMS: 600}}, End: []AnimationFrame{{Rect: &FrameRect{48, 0, 8, 8}, DurationMS: 250}, {Rect: &FrameRect{56, 0, 8, 8}, DurationMS: 250}}}
	m.Actions["stretch"] = AnimationAction{AnimationClip: stretch}
	if !HasCPUAction(m, "stretch", "calm") {
		t.Fatal("real entry/hold/recovery rejected")
	}
	for now := 0.; now <= 10; now += 2 {
		e.ObserveCPU(now, 90, true)
	}
	cpuTick(e, 101, false)
	s := e.States[0]
	s.BusyElapsed = 300
	s.BusyNextStretch = 300
	seenEnd := map[int]bool{}
	for step := 102; step <= 121; step++ {
		f := cpuTick(e, step, false)[0]
		if f.Action == "stretch" && s.Player.Phase == AnimationEnd {
			seenEnd[s.Player.Index] = true
		}
	}
	if !seenEnd[0] || !seenEnd[1] || s.Action != "knead" {
		t.Fatalf("recovery skipped: %+v final %s", seenEnd, s.Action)
	}
}

func TestCPUStretchUsesDisplayedTimeAfterDelayedTick(t *testing.T) {
	e := cpuEngine(1)
	m := e.States[0].Player.Manifest
	c := m.Actions["stretch"]
	c.Start = []AnimationFrame{{Rect: &FrameRect{64, 0, 8, 8}, DurationMS: 1000}}
	c.Loop[0].DurationMS = 500
	c.Loop[1].DurationMS = 500
	c.End = []AnimationFrame{{Rect: &FrameRect{72, 0, 8, 8}, DurationMS: 300}}
	m.Actions["stretch"] = c
	for now := 0.; now <= 10; now += 2 {
		e.ObserveCPU(now, 90, true)
	}
	e.Tick(10.1, .1, -10000, -10000, 0, false)
	s := e.States[0]
	s.BusyElapsed = 300
	s.BusyNextStretch = 300
	e.Tick(10.2, .1, -10000, -10000, 0, false)
	e.ObserveCPU(14, 90, true)
	f := e.Tick(14, 3.8, -10000, -10000, 0, false)[0]
	if f.Action != "stretch" || s.BusyStretchEnding || s.Player.Phase != AnimationStart {
		t.Fatal("delayed UI truncated stretch entry")
	}
}

func TestCPUUnavailableStretchAliasDoesNotDisableRealKnead(t *testing.T) {
	m := cpuFixture()
	m.Actions["stretch"] = AnimationAction{Fallback: "knead", DemoFallback: true}
	if !HasCPUAction(m, "knead", "calm") || HasCPUAction(m, "stretch", "calm") {
		t.Fatal("missing stretch contaminated real knead capability")
	}
}
