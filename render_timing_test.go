package main

import (
	"math"
	"testing"
)

func TestLowResourceTimerPolicy(t *testing.T) {
	s := NormalizeSettings(Settings{CPU: DefaultCPUSettings()})
	if RenderIntervalMS(s, false, false, false) != 50 || RenderIntervalMS(s, true, false, false) != 100 || RenderIntervalMS(s, false, true, false) != 16 || RenderIntervalMS(s, true, true, true) != 0 {
		t.Fatal("timer policy changed")
	}
	s.Activity = ActivityQuiet
	if RenderIntervalMS(s, false, false, false) != 250 {
		t.Fatal("quiet did not throttle")
	}
}
func TestMoreGeneratedFramesPreserveActionDuration(t *testing.T) {
	duration := func(count int) float64 {
		clip := AnimationClip{}
		for i := 0; i < count; i++ {
			clip.Loop = append(clip.Loop, AnimationFrame{Row: 0, Col: i % 6, DurationMS: 1200 / count})
		}
		m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Actions: map[string]AnimationAction{"idle": {AnimationClip: clip}}}
		p := NewAnimationPlayer(m)
		p.Tick(.7)
		return float64(p.Index)*float64(1200/count) + p.ElapsedMS
	}
	if math.Abs(duration(6)-duration(12)) > .001 {
		t.Fatal("additional poses accelerated the clip")
	}
}
