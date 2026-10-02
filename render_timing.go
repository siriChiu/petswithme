package main

import "math"

// Kept isolated on the experiment branch until native timing is measured.
var frameDeadlineScheduling = true

// Existing low-resource timer policy. More generated poses do not accelerate
// the duration-based player, and unchanged cached frames are not repainted.
func RenderIntervalMS(s Settings, busy, dragging, hidden, interacting bool) int {
	if hidden {
		return 0
	}
	ms := 50
	if busy {
		ms = 100
	}
	if s.Quiet || s.Activity == ActivityQuiet {
		ms = 250
	}
	// A requested short animation keeps its poses visible even in quiet mode.
	// Return to the lower idle cadence once the complete recovery has finished.
	if interacting {
		ms = 50
	}
	if dragging {
		ms = 16
	}
	return ms
}

func (e *BehaviorEngine) DirectAnimationActive() bool {
	if e != nil {
		for _, s := range e.States {
			if s != nil && !s.Autonomous && s.Priority >= PriorityPlay {
				return true
			}
		}
	}
	return false
}

// FrameDeadlineMS preserves the normal movement interval, but may deliver the
// next authored pose earlier. Quiet, busy and drag timer policies keep their
// existing resource limits. No clip duration, image or movement speed changes.
func FrameDeadlineMS(e *BehaviorEngine, baseMS int, sinceTickMS float64) int {
	if baseMS != 50 || e == nil {
		return baseMS
	}
	if !finite(sinceTickMS) || sinceTickMS < 0 {
		sinceTickMS = 0
	}
	// A setting/interaction callback may ask between ticks. Its request must
	// not restart a fresh 50ms interval and postpone the movement update.
	delay := max(10, int(math.Ceil(float64(baseMS)-sinceTickMS)))
	for _, s := range e.States {
		if s == nil || s.Player == nil || s.Player.Phase == AnimationDone {
			continue
		}
		p := s.Player
		if p.Phase == AnimationLoop && len(p.sequence()) <= 1 && !p.LoopOnce {
			continue // A static pose needs no extra repaint deadline.
		}
		remaining := float64(p.Frame().DurationMS) - p.ElapsedMS
		if !finite(remaining) {
			continue
		}
		if p.Generation == s.LastPresentedGeneration {
			remaining -= sinceTickMS
		}
		// SetTimer has a documented 10 ms minimum. Do not spin for a near-due
		// frame, change system timer resolution, or skip the authored pose.
		delay = min(delay, max(10, int(math.Ceil(remaining))))
	}
	return delay
}
