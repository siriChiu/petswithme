package main

import "math"

func isLocomotion(action string) bool {
	return action == "run_left" || action == "run_right" || action == "walk_left" || action == "walk_right"
}
func clipSeconds(frames []AnimationFrame) float64 {
	total := 0
	for _, f := range frames {
		total += max(10, f.DurationMS)
	}
	return float64(total) / 1000
}

// loopSeconds returns the portion of an upcoming interval spent in the moving
// loop. Authored start/end poses have no inferred displacement or speed curve.
func (p *AnimationPlayer) loopSeconds(dt float64) float64 {
	if !finite(dt) || dt <= 0 || !isLocomotion(p.Action) {
		return 0
	}
	switch p.Phase {
	case AnimationLoop:
		return dt
	case AnimationStart:
		remaining := clipSeconds(p.clip.Start[p.Index:]) - p.ElapsedMS/1000
		return math.Max(0, dt-math.Max(0, remaining))
	default:
		return 0
	}
}
