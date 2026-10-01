package main

import (
	"errors"
	"math"
)

const cpuPollSeconds = 2.0
const cpuMaxSampleGap = 6.0
const cpuSmoothingSeconds = 4.0

type CPUSettings struct {
	Enabled             bool    `json:"enabled"`
	EnterPercent        float64 `json:"enterPercent"`
	ExitPercent         float64 `json:"exitPercent"`
	EnterSeconds        float64 `json:"enterSeconds"`
	ExitSeconds         float64 `json:"exitSeconds"`
	StretchEverySeconds float64 `json:"stretchEverySeconds"`
}

func DefaultCPUSettings() CPUSettings { return CPUSettings{true, 70, 50, 10, 10, 300} }
func (s CPUSettings) Validate() error {
	if !finite(s.EnterPercent) || !finite(s.ExitPercent) || s.EnterPercent <= 0 || s.EnterPercent >= 100 || s.ExitPercent <= 0 || s.ExitPercent >= s.EnterPercent {
		return errors.New("CPU thresholds need 0 < exitPercent < enterPercent < 100")
	}
	for _, v := range []float64{s.EnterSeconds, s.ExitSeconds, s.StretchEverySeconds} {
		if !finite(v) || v < 1 || v > 86400 {
			return errors.New("CPU durations must be 1 to 86400 seconds")
		}
	}
	return nil
}
func NormalizeCPUSettings(s CPUSettings) CPUSettings {
	if s.Validate() != nil {
		enabled := s.Enabled
		s = DefaultCPUSettings()
		s.Enabled = enabled
	}
	return s
}

// Kernel includes idle on Windows. Values are cumulative unsigned 100ns ticks,
// not wall-clock duration and not any process's private counters.
type CPUCounters struct{ Idle, Kernel, User uint64 }
type CPUCounterSampler struct {
	previous CPUCounters
	last     float64
	known    bool
}

func (s *CPUCounterSampler) Reset() { *s = CPUCounterSampler{} }
func CPUPercent(previous, current CPUCounters) (float64, bool) {
	if current.Idle < previous.Idle || current.Kernel < previous.Kernel || current.User < previous.User {
		return 0, false
	}
	idle, kernel, user := current.Idle-previous.Idle, current.Kernel-previous.Kernel, current.User-previous.User
	if ^uint64(0)-kernel < user {
		return 0, false
	}
	total := kernel + user
	if total == 0 || idle > kernel || idle > total {
		return 0, false
	}
	return float64(total-idle) / float64(total) * 100, true
}
func (s *CPUCounterSampler) Sample(now float64, c CPUCounters, ok bool) (float64, bool) {
	if !ok || !finite(now) || now < 0 {
		s.Reset()
		return 0, false
	}
	if !s.known || now <= s.last || now-s.last > cpuMaxSampleGap {
		s.previous, s.last, s.known = c, now, true
		return 0, false
	}
	value, valid := CPUPercent(s.previous, c)
	if !valid {
		s.Reset()
		return 0, false
	}
	s.previous, s.last = c, now
	return value, valid
}

// Poll gate is shared by native use and deterministic tests. It never catches
// up missed reads after a delayed UI timer and never samples while hidden.
type CPUMonitor struct {
	Sampler    CPUCounterSampler
	next, last float64
	polled     bool
}

func (m *CPUMonitor) Reset() { *m = CPUMonitor{} }
func (m *CPUMonitor) Poll(now float64, enabled, hidden bool, read func() (CPUCounters, bool)) (percent float64, valid, observed bool) {
	if !enabled || hidden {
		m.Reset()
		return 0, false, false
	}
	if !finite(now) || now < 0 {
		m.Reset()
		return 0, false, true
	}
	if m.polled && now >= m.last && now < m.next {
		return 0, false, false
	}
	if read == nil {
		m.Reset()
		return 0, false, true
	}
	if m.polled && now < m.last {
		m.Reset()
	}
	m.polled = true
	m.last = now
	m.next = now + cpuPollSeconds
	c, ok := read()
	percent, valid = m.Sampler.Sample(now, c, ok)
	return percent, valid, true
}

// CPULoadState smooths aggregate observations and applies separate enter/exit
// thresholds with sustained windows. Missing observations break continuity.
type CPULoadState struct {
	Settings              CPUSettings
	Active                bool
	Smoothed              float64
	known                 bool
	last                  float64
	enterSince, exitSince float64
	entering, exiting     bool
}

func NewCPULoadState(s CPUSettings) *CPULoadState {
	return &CPULoadState{Settings: NormalizeCPUSettings(s)}
}
func (s *CPULoadState) Reset() { config := s.Settings; *s = CPULoadState{Settings: config} }
func (s *CPULoadState) Expire(now float64) {
	if !finite(now) || !s.Settings.Enabled || (s.known && (now < s.last || now-s.last > cpuMaxSampleGap)) {
		s.Reset()
	}
}
func (s *CPULoadState) Observe(now, percent float64, valid bool) {
	if !s.Settings.Enabled || !valid || !finite(now) || now < 0 || !finite(percent) || percent < 0 || percent > 100 {
		s.Reset()
		return
	}
	if s.known && (now <= s.last || now-s.last > cpuMaxSampleGap) {
		s.Reset()
	}
	if !s.known {
		s.Smoothed = percent
		s.known = true
	} else {
		alpha := 1 - math.Exp(-(now-s.last)/cpuSmoothingSeconds)
		s.Smoothed += alpha * (percent - s.Smoothed)
	}
	s.last = now
	if !s.Active {
		s.exiting = false
		if s.Smoothed > s.Settings.EnterPercent {
			if !s.entering {
				s.entering = true
				s.enterSince = now
			}
			if now-s.enterSince >= s.Settings.EnterSeconds {
				s.Active = true
				s.entering = false
			}
		} else {
			s.entering = false
		}
	} else {
		s.entering = false
		if s.Smoothed < s.Settings.ExitPercent {
			if !s.exiting {
				s.exiting = true
				s.exitSince = now
			}
			if now-s.exitSince >= s.Settings.ExitSeconds {
				s.Active = false
				s.exiting = false
			}
		} else {
			s.exiting = false
		}
	}
}
func (e *BehaviorEngine) SetCPUSettings(settings CPUSettings) {
	e.Load = NewCPULoadState(settings)
	for _, s := range e.States {
		s.BusyElapsed = 0
		s.BusyNextStretch = e.Load.Settings.StretchEverySeconds
		s.BusyStretching = false
		s.BusyStretchEnding = false
	}
}
func (e *BehaviorEngine) ObserveCPU(now, percent float64, valid bool) {
	if e.Load == nil {
		e.Load = NewCPULoadState(DefaultCPUSettings())
	}
	e.Load.Observe(now, percent, valid)
}
func (e *BehaviorEngine) ResetCPU() {
	if e.Load != nil {
		e.Load.Reset()
	}
	for _, s := range e.States {
		s.BusyElapsed = 0
		s.BusyNextStretch = 0
		s.BusyStretching = false
		s.BusyStretchEnding = false
	}
}
func (e *BehaviorEngine) cpuEligible(i int, quiet bool) bool {
	if quiet || !e.valid(i) || e.Load == nil || !e.Load.Active {
		return false
	}
	s := e.States[i]
	return HasCPUAction(s.Player.Manifest, "knead", s.Mood)
}

// Directly repeating an idle/pet loop under a new name is not a new CPU action.
// Distinct references are only a structural gate; visual review is still needed.
func HasCPUAction(m *AnimationManifest, action, mood string) bool {
	if (action != "knead" && action != "stretch") || !HasAuthoredAction(m, action, mood) {
		return false
	}
	clip := m.Resolve(action, mood)
	unique := map[[6]int]bool{}
	sequences := [][]AnimationFrame{clip.Loop}
	if action == "stretch" {
		sequences = [][]AnimationFrame{clip.Start, clip.Loop, clip.End}
	}
	for _, seq := range sequences {
		for _, f := range seq {
			unique[frameIdentity(f)] = true
		}
	}
	if len(unique) < 2 {
		return false
	}
	for _, other := range []string{"idle", "pet", "play", "waiting", "curious", "walk_left", "walk_right", "sit", "sleep", "rest", "social_rest", "groom", "pounce", "run_left", "run_right", "knead", "stretch"} {
		if other == action {
			continue
		}
		if _, exists := m.Actions[other]; !exists {
			continue
		}
		if !HasAuthoredAction(m, other, mood) {
			continue
		}
		refs := map[[6]int]bool{}
		for _, f := range m.Resolve(other, mood).Loop {
			refs[frameIdentity(f)] = true
		}
		all := true
		for ref := range unique {
			if !refs[ref] {
				all = false
				break
			}
		}
		if all {
			return false
		}
	}
	return true
}
func (e *BehaviorEngine) clearBusyAction(i int, now float64) {
	s := e.States[i]
	s.BusyStretching = false
	s.BusyStretchEnding = false
	if s.Priority == PriorityBusy {
		e.Cancel(i, now)
	}
}
func (e *BehaviorEngine) tickBusy(i int, now, dt float64) string {
	s := e.States[i]
	// Direct interactions pause elapsed busy animation time. They never queue a
	// stale stretch to play after a drag or after load has ended.
	if s.Priority != PriorityBusy {
		e.Social.Cancel(i, now)
		s.Action = "knead"
		s.Priority = PriorityBusy
		s.Until = 0
		s.Ending = false
		s.HasDestination = false
		s.AfterAction = ""
		s.Autonomous = true
		s.BusyStretching = false
		s.BusyStretchEnding = false
	}
	s.BusyElapsed += dt
	if s.BusyNextStretch <= 0 {
		s.BusyNextStretch = e.Load.Settings.StretchEverySeconds
	}
	if s.BusyStretching {
		if !HasCPUAction(s.Player.Manifest, "stretch", s.Mood) {
			s.BusyStretching = false
			s.BusyStretchEnding = false
			s.Ending = false
		}
		if s.BusyStretching && s.BusyStretchRemaining <= 1e-9 && !s.BusyStretchEnding {
			s.Player.Stop()
			s.Ending = true
			s.BusyStretchEnding = true
		}
		if s.BusyStretching && !s.BusyStretchEnding {
			s.BusyStretchRemaining -= dt
		}
		if s.BusyStretching && s.BusyStretchEnding && s.Player.Phase == AnimationDone {
			s.BusyStretching = false
			s.BusyStretchEnding = false
			s.Ending = false
		}
	}
	if !s.BusyStretching && s.BusyElapsed >= s.BusyNextStretch {
		s.BusyNextStretch = s.BusyElapsed + e.Load.Settings.StretchEverySeconds
		if HasCPUAction(s.Player.Manifest, "stretch", s.Mood) {
			s.BusyStretching = true
			s.BusyStretchEnding = false
			s.Ending = false
			s.BusyStretchRemaining = actionCycle(s.Player.Manifest, "stretch", s.Mood) - dt
			s.Player.Play("stretch", s.Mood)
		}
	}
	s.Action = "knead"
	if s.BusyStretching {
		s.Action = "stretch"
	}
	return s.Action
}
