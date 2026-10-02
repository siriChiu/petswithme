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
	sector := math.Atan2(dx, -dy) * float64(count) / (2 * math.Pi)
	if s.Player.Manifest.GazeNearestAuthored {
		var ok bool
		direction, ok = nearestAuthoredGaze(s.Player.Manifest, s.Mood, sector, s.GazeDirection, s.GazeActive)
		if !ok {
			s.GazeActive = false
			return "idle"
		}
		// Retain the previous sector for an extra 15% of a sector at a boundary.
		// This prevents pixel-scale cursor tremor from flashing between poses.
	} else if s.GazeActive {
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

func gazeSectorDelta(a, b float64, count int) float64 {
	return math.Mod(a-b+float64(count)*1.5, float64(count)) - float64(count)/2
}

// Partial packs opt in to nearest genuine angle selection. Fixed aliases must
// not move old angle boundaries or be counted as extra authored directions.
func nearestAuthoredGaze(m *AnimationManifest, mood string, sector float64, previous int, active bool) (int, bool) {
	count := ManifestGazeDirections(m)
	best, bestDistance, bestDelta := -1, math.Inf(1), math.Inf(-1)
	previousPresent := false
	for i := 0; i < count; i++ {
		a, ok := m.Actions["gaze_"+itoaDirection(i)]
		if !ok || a.DemoFallback {
			continue
		}
		clip, moodSpecific := a.Moods[mood]
		if (!moodSpecific || len(clip.Loop) == 0) && len(a.Loop) == 0 {
			continue
		}
		previousPresent = previousPresent || i == previous
		delta := gazeSectorDelta(float64(i), sector, count)
		distance := math.Abs(delta)
		// Exact midpoints prefer the clockwise endpoint, as uniform rounding
		// did. Once a pose is active, the hysteresis check below wins the tie.
		if distance < bestDistance-1e-9 || (math.Abs(distance-bestDistance) <= 1e-9 && delta > bestDelta) {
			best, bestDistance, bestDelta = i, distance, delta
		}
	}
	if best < 0 {
		return 0, false
	}
	if active && previousPresent && previous != best {
		gap := math.Abs(gazeSectorDelta(float64(previous), float64(best), count))
		previousDistance := math.Abs(gazeSectorDelta(float64(previous), sector, count))
		if previousDistance-bestDistance <= .3*gap {
			best = previous
		}
	}
	return best, true
}
