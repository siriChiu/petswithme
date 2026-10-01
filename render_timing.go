package main

// Existing low-resource timer policy. More generated poses do not accelerate
// the duration-based player, and unchanged cached frames are not repainted.
func RenderIntervalMS(s Settings, busy, dragging, hidden bool) int {
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
	if dragging {
		ms = 16
	}
	return ms
}
