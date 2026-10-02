package main

import "math"

// GazeOrigin is in the same physical, global screen coordinates as GetCursorPos
// and the pet window. The upper-body origin avoids looking down at a cursor
// beside the face just because the transparent window extends beneath it.
func GazeOrigin(c *Cat) (float64, float64) {
	return c.X + float64(c.W)*.5, c.Y + float64(c.H)*.3
}

// A taller animation canvas need not move the cat's head. Packs may declare
// that head point independently of the floor anchor and transparent padding.
func ManifestGazeOrigin(c *Cat, m *AnimationManifest) (float64, float64) {
	if m != nil && m.GazeOrigin != nil {
		return c.X + float64(c.W)*m.GazeOrigin.X, c.Y + float64(c.H)*m.GazeOrigin.Y
	}
	return GazeOrigin(c)
}

func ManifestGazeDirections(m *AnimationManifest) int {
	if m != nil && (m.GazeDirections == 4 || m.GazeDirections == 8 || m.GazeDirections == 32) {
		return m.GazeDirections
	}
	return 16
}

func hasGazeArtwork(m *AnimationManifest, mood string) bool {
	for direction := 0; direction < ManifestGazeDirections(m); direction++ {
		if HasAuthoredAction(m, "gaze_"+itoaDirection(direction), mood) {
			return true
		}
	}
	return false
}

func (e *BehaviorEngine) pointerGaze(i int, cursorX, cursorY float64) string {
	s, c := e.States[i], e.Cats[i]
	ox, oy := ManifestGazeOrigin(c, s.Player.Manifest)
	dx, dy := cursorX-ox, cursorY-oy
	radius := math.Max(8, float64(c.W)*.05)
	// Use separate enter/exit radii as well as angular hysteresis. A pointer
	// resting on the face boundary must not flash between idle and a gaze pose.
	if s.GazeActive {
		radius *= .75
	}
	if !finite(cursorX) || !finite(cursorY) || !finite(ox) || !finite(oy) || math.Hypot(dx, dy) < radius {
		s.GazeActive = false
		return "idle"
	}
	count := ManifestGazeDirections(s.Player.Manifest)
	direction := GazeDirectionCount(dx, dy, count)
	// Retain the previous sector for an extra 15% of a sector at a boundary.
	// This prevents pixel-scale cursor tremor from flashing between poses.
	if s.GazeActive {
		sector := math.Atan2(dx, -dy) * float64(count) / (2 * math.Pi)
		delta := math.Mod(sector-float64(s.GazeDirection)+float64(count)*1.5, float64(count)) - float64(count)/2
		if math.Abs(delta) <= .65 {
			direction = s.GazeDirection
		}
	}
	action := "gaze_" + itoaDirection(direction)
	if !HasAuthoredAction(s.Player.Manifest, action, s.Mood) {
		s.GazeActive = false
		return "idle"
	}
	s.GazeDirection, s.GazeActive = direction, true
	return action
}
