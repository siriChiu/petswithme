package main

import "fmt"

type ActivityLevel string

const (
	ActivityQuiet  ActivityLevel = "quiet"
	ActivityNormal ActivityLevel = "normal"
	ActivityLively ActivityLevel = "lively"
)

func ValidActivity(a ActivityLevel) bool {
	return a == ActivityQuiet || a == ActivityNormal || a == ActivityLively
}

// Temperament changes choice frequency, not animation playback or stride speed.
// These are local play preferences rather than inferred traits of real animals.
type Temperament struct {
	Energy      float64 `json:"energy"`
	Sociability float64 `json:"sociability"`
	Curiosity   float64 `json:"curiosity"`
}

func (t Temperament) Validate() error {
	for name, v := range map[string]float64{"energy": t.Energy, "sociability": t.Sociability, "curiosity": t.Curiosity} {
		if !finite(v) || v < 0 || v > 1 {
			return fmt.Errorf("temperament %s must be between 0 and 1", name)
		}
	}
	return nil
}

func DefaultTemperament(index int) Temperament {
	defaults := []Temperament{{.65, .65, .5}, {.45, .5, .8}, {.8, .4, .6}}
	if index < 0 {
		index = 0
	}
	return defaults[index%len(defaults)]
}

func NormalizeSettings(s Settings) Settings {
	if !ValidActivity(s.Activity) {
		s.Activity = ActivityNormal
		if s.Quiet {
			s.Activity = ActivityQuiet
		}
	}
	s.Quiet = s.Activity == ActivityQuiet
	s.Size = ValidSize(s.Size)
	return s
}
