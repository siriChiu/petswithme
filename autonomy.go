package main

import (
	"math"
	"strings"
)

func (e *BehaviorEngine) SetActivity(level ActivityLevel) bool {
	if !ValidActivity(level) {
		return false
	}
	e.Activity = level
	return true
}
func (e *BehaviorEngine) SetTemperament(i int, t Temperament) bool {
	if !e.valid(i) || t.Validate() != nil {
		return false
	}
	e.States[i].Temperament = t
	return true
}

// HasAuthoredAction deliberately does not treat an alias to a different behavior
// as new artwork. Gaze aliases may select a neighboring authored gaze direction.
func HasAuthoredAction(m *AnimationManifest, action, mood string) bool {
	if m == nil {
		return false
	}
	seen := map[string]bool{}
	name := action
	for !seen[name] {
		seen[name] = true
		a, ok := m.Actions[name]
		if !ok || a.DemoFallback {
			return false
		}
		if c, ok := a.Moods[mood]; ok && len(c.Loop) > 0 {
			return true
		}
		if len(a.Loop) > 0 {
			return true
		}
		if !strings.HasPrefix(action, "gaze_") || !strings.HasPrefix(a.Fallback, "gaze_") {
			return false
		}
		name = a.Fallback
	}
	return false
}
func HasRunAnimation(m *AnimationManifest, action, mood string) bool {
	if !HasRunArtwork(m, action, mood) {
		return false
	}
	a := m.Actions[action]
	return a.Movement != nil && a.Movement.Verified && finite(a.Movement.StrideRatio) && a.Movement.StrideRatio > 0
}

func HasRunArtwork(m *AnimationManifest, action, mood string) bool {
	if action != "run_left" && action != "run_right" {
		return false
	}
	if !HasAuthoredAction(m, action, mood) || !HasWalkAnimation(m, action, mood) {
		return false
	}
	// Different timing over the same walk rectangles is not a running drawing.
	walk := m.Resolve("walk_"+strings.TrimPrefix(action, "run_"), mood)
	walkFrames := map[[6]int]bool{}
	for _, f := range walk.Loop {
		walkFrames[frameIdentity(f)] = true
	}
	for _, f := range m.Resolve(action, mood).Loop {
		if !walkFrames[frameIdentity(f)] {
			return true
		}
	}
	return false
}
func (e *BehaviorEngine) SetExperimentalMovement(enabled bool) { e.ExperimentalMovement = enabled }
func (e *BehaviorEngine) canRun(i int, action string) bool {
	s := e.States[i]
	return HasRunAnimation(s.Player.Manifest, action, s.Mood) || (e.ExperimentalMovement && HasRunArtwork(s.Player.Manifest, action, s.Mood))
}
func (e *BehaviorEngine) movementSpeed(i int, action string) float64 {
	s, c := e.States[i], e.Cats[i]
	if strings.HasPrefix(action, "run_") && !HasRunAnimation(s.Player.Manifest, action, s.Mood) {
		if e.ExperimentalMovement && HasRunArtwork(s.Player.Manifest, action, s.Mood) {
			return float64(c.W) * .23
		}
		return 0
	}
	return LocomotionPixelsPerSecond(s.Player.Manifest, action, s.Mood, c.W)
}
func (e *BehaviorEngine) movementAction(i int, direction float64, preferRun bool) string {
	s := e.States[i]
	suffix := "right"
	if direction < 0 {
		suffix = "left"
	}
	run, walk := "run_"+suffix, "walk_"+suffix
	if preferRun && e.canRun(i, run) {
		return run
	}
	if HasAuthoredAction(s.Player.Manifest, walk, s.Mood) && HasWalkAnimation(s.Player.Manifest, walk, s.Mood) {
		return walk
	}
	if e.canRun(i, run) {
		return run
	}
	return ""
}
func frameIdentity(f AnimationFrame) [6]int {
	if f.Rect != nil {
		return [6]int{f.Rect.X, f.Rect.Y, f.Rect.W, f.Rect.H, -1, -1}
	}
	return [6]int{0, 0, 0, 0, f.Row, f.Col}
}
func LocomotionPixelsPerSecond(m *AnimationManifest, action, mood string, width int) float64 {
	if strings.HasPrefix(action, "run_") && !HasRunAnimation(m, action, mood) {
		return 0
	}
	return WalkPixelsPerSecond(m, action, mood, width)
}
func (e *BehaviorEngine) restingAction(i int) string {
	s := e.States[i]
	for _, action := range []string{"sleep", "sit", "rest", "idle"} {
		if HasAuthoredAction(s.Player.Manifest, action, s.Mood) {
			return action
		}
	}
	return "idle"
}
func (e *BehaviorEngine) dwell(i int) float64 {
	s, c := e.States[i], e.Cats[i]
	// Higher activity means shorter pauses, never faster animation playback.
	value := 3.0 + (1-s.Temperament.Energy)*5 + c.Seed.Float64()*7
	if e.Activity == ActivityLively {
		value *= .45
	}
	return value
}
func (e *BehaviorEngine) finishAutonomy(i int, now float64) {
	s := e.States[i]
	s.Action, s.Priority, s.Until = "idle", PriorityIdle, 0
	s.HasDestination = false
	s.NextDecision = now + e.dwell(i)
}

// The reachable interval ends before the next neighboring window. Choosing a
// destination within this interval avoids repeatedly walking into a blocker.
func (e *BehaviorEngine) roamInterval(i int) (float64, float64) {
	c := e.Cats[i]
	lo, hi := float64(c.Bounds.Left), float64(c.Bounds.Right-c.W)
	gap := math.Max(6, float64(c.W)*.06)
	for j, o := range e.Cats {
		if j == i || o == nil || o.Bounds != c.Bounds || c.Y+float64(c.H) <= o.Y || c.Y >= o.Y+float64(o.H) {
			continue
		}
		if c.X+float64(c.W)/2 <= o.X+float64(o.W)/2 {
			hi = math.Min(hi, math.Max(c.X, o.X-float64(c.W)-gap))
		} else {
			lo = math.Max(lo, math.Min(c.X, o.X+float64(o.W)+gap))
		}
	}
	return lo, math.Max(lo, hi)
}
func actionCycle(m *AnimationManifest, action, mood string) float64 {
	c := m.Resolve(action, mood)
	ms := 0
	for _, seq := range [][]AnimationFrame{c.Start, c.Loop} {
		for _, f := range seq {
			ms += f.DurationMS
		}
	}
	return math.Max(.1, float64(ms)/1000)
}

type autonomousChoice struct {
	action, family string
	weight         float64
	target         float64
	move           bool
}

func (e *BehaviorEngine) decideAutonomy(i int, now float64) {
	s, c := e.States[i], e.Cats[i]
	m := s.Player.Manifest
	choices := []autonomousChoice{}
	add := func(q autonomousChoice) {
		if now < s.ActionCooldown[q.family] || q.family == s.LastChoice {
			return
		}
		choices = append(choices, q)
	}
	lo, hi := e.roamInterval(i)
	minimum := math.Max(16, float64(c.W)*.18)
	for _, direction := range []float64{-1, 1} {
		suffix := "right"
		available := hi - c.X
		if direction < 0 {
			suffix = "left"
			available = c.X - lo
		}
		if available < minimum {
			continue
		}
		distance := minimum + c.Seed.Float64()*(available-minimum)
		// Long walks still have a bounded duration and rest between destinations.
		walk := "walk_" + suffix
		if HasAuthoredAction(m, walk, s.Mood) && HasWalkAnimation(m, walk, s.Mood) {
			add(autonomousChoice{walk, "roam", 1 + s.Temperament.Energy*3, c.X + direction*distance, true})
		}
		run := "run_" + suffix
		if e.canRun(i, run) {
			weight := s.Temperament.Energy * .8
			if e.Activity == ActivityLively {
				weight *= 2
			}
			add(autonomousChoice{run, "run", weight, c.X + direction*distance, true})
		}
	}
	for _, entry := range []struct {
		a string
		w float64
	}{
		{"sit", 1 + (1 - s.Temperament.Energy)}, {"stretch", .9}, {"groom", 1.2},
		{"pounce", s.Temperament.Energy}, {"play", s.Temperament.Energy},
		{"curious", .5 + s.Temperament.Curiosity*2}, {"waiting", .5}, {"rest", .8},
	} {
		if HasAuthoredAction(m, entry.a, s.Mood) {
			add(autonomousChoice{action: entry.a, family: entry.a, weight: entry.w})
		}
	}
	if len(choices) == 0 {
		// A genuine idle pause breaks repeats when the only available action is walk.
		s.LastChoice = "idle"
		e.finishAutonomy(i, now)
		return
	}
	total := 0.0
	for _, q := range choices {
		total += q.weight
	}
	selected := choices[len(choices)-1]
	pick := c.Seed.Float64() * total
	for _, q := range choices {
		pick -= q.weight
		if pick <= 0 {
			selected = q
			break
		}
	}
	s.Action = selected.action
	s.Autonomous = true
	s.LastChoice = selected.family
	s.ActionCooldown[selected.family] = now + 6 + c.Seed.Float64()*10
	s.HasDestination = selected.move
	s.TargetX = selected.target
	s.Priority = PriorityIdle
	duration := actionCycle(m, selected.action, s.Mood)
	if selected.action == "sit" || selected.action == "rest" {
		duration = 3 + c.Seed.Float64()*5
	}
	if selected.move {
		s.Priority = PriorityWander
		c.Direction = 1
		if selected.target < c.X {
			c.Direction = -1
		}
		speed := e.movementSpeed(i, selected.action)
		duration = math.Min(18, math.Abs(selected.target-c.X)/math.Max(1, speed)+.3)
	}
	s.Until = now + duration
	s.NextDecision = s.Until + e.dwell(i)
}
func (e *BehaviorEngine) tickAutonomy(i int, now, dt, cursorX, cursorY float64) string {
	s, c := e.States[i], e.Cats[i]
	if s.Priority == PrioritySocial || s.Action == "sleep" {
		e.finishAutonomy(i, now)
		s.Mood = "calm"
	}
	if s.Priority == PriorityWander {
		target := s.TargetX
		if !s.HasDestination {
			target = c.X + c.Direction*100
		}
		oldX := c.X
		if now >= s.Until || math.Abs(target-c.X) < .5 {
			e.finishAutonomy(i, now)
		} else {
			action := s.Action
			if action != "run_left" && action != "run_right" {
				action = "walk_right"
				if c.Direction < 0 {
					action = "walk_left"
				}
			}
			e.moveToward(i, target, c.Y, e.movementSpeed(i, action)*dt)
			if math.Abs(c.X-oldX) < .001 && dt > 0 {
				e.finishAutonomy(i, now)
			} else {
				s.Action = action
			}
		}
	}
	if s.Priority == PriorityIdle && s.Until > 0 && now >= s.Until {
		next := ""
		if s.Action == "sit" && HasAuthoredAction(s.Player.Manifest, "getup", s.Mood) {
			next = "getup"
		}
		if len(s.Player.Manifest.Resolve(s.Action, s.Mood).End) > 0 {
			s.AfterAction = next
			s.Ending = true
			s.Priority = PriorityPlay
			s.Player.Stop()
			return s.Action
		}
		e.finishAutonomy(i, now)
		if next != "" {
			s.Action = next
			s.Until = now + actionCycle(s.Player.Manifest, next, s.Mood)
			s.NextDecision = s.Until + e.dwell(i)
		}
	}
	if s.Priority == PriorityIdle && s.Until == 0 && now >= s.NextDecision {
		e.decideAutonomy(i, now)
	}
	return s.Action
}
func itoaDirection(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return "1" + string(rune('0'+i-10))
}

func (e *BehaviorEngine) configureSocial() {
	e.Social.Activity = e.Activity
	e.Social.Sociability = make([]float64, len(e.States))
	for i, s := range e.States {
		e.Social.Sociability[i] = s.Temperament.Sociability
	}
	e.Social.Supports = func(i int, action string) bool {
		if !e.valid(i) {
			return false
		}
		s := e.States[i]
		if strings.HasPrefix(action, "run_") {
			return e.canRun(i, action)
		}
		return HasAuthoredAction(s.Player.Manifest, action, s.Mood)
	}
	e.Social.Eligible = func(a, b int) bool {
		for _, i := range []int{a, b} {
			if !e.valid(i) {
				return false
			}
			if e.movementAction(i, -1, false) == "" || e.movementAction(i, 1, false) == "" {
				return false
			}
		}
		return true
	}
}
func (e *BehaviorEngine) socialAction(i int, action string) string {
	s := e.States[i]
	if action == "social_rest" {
		for _, candidate := range []string{"social_rest", "sit", "rest", "sleep"} {
			if HasAuthoredAction(s.Player.Manifest, candidate, s.Mood) {
				return candidate
			}
		}
		return "idle"
	}
	if HasAuthoredAction(s.Player.Manifest, action, s.Mood) {
		return action
	}
	return "idle"
}
