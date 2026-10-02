package main

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
