package main

import (
	"fmt"
	"testing"
)

func TestPlayerFractionalClockMatchesIntegerFrameBoundaries(t *testing.T) {
	for _, origin := range []float64{0, 3600, 86400, 1e6, 1 << 21, 1 << 25} {
		for _, durations := range [][]int{{250, 200, 100, 300, 220}, {250, 200, 100, 200, 100, 220}, {1200, 120, 60, 180, 60, 100, 600, 400}, {120, 120, 120, 120, 120, 120, 120, 120}} {
			t.Run(fmt.Sprintf("origin%.0f/%v", origin, durations), func(t *testing.T) {
				seq := []AnimationFrame{}
				total := 0
				for i, ms := range durations {
					seq = append(seq, AnimationFrame{Row: 0, Col: i, DurationMS: ms})
					total += ms
				}
				m := &AnimationManifest{Fallback: "idle", Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: seq}}}}
				p := NewAnimationPlayer(m)
				last := origin
				for step := 1; step <= 10000; step++ {
					now := origin + float64(step)*.05
					p.Tick(now - last)
					last = now
					remaining := (step * 50) % total
					want := 0
					for remaining >= durations[want] {
						remaining -= durations[want]
						want++
					}
					if p.Index != want {
						t.Fatalf("split durations%v at%dms: index%d instead of%d, residual%.15fms", durations, step*50, p.Index, want, p.ElapsedMS)
					}
					if p.ElapsedMS < 0 {
						t.Fatal("negative elapsed residue", p.ElapsedMS)
					}
				}
			})
		}
	}
}

func TestPlayerDoesNotAdvanceMeaningfullyEarly(t *testing.T) {
	m := &AnimationManifest{Fallback: "idle", Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: []AnimationFrame{{Row: 0, Col: 0, DurationMS: 100}, {Row: 0, Col: 1, DurationMS: 100}}, End: []AnimationFrame{{Row: 1, Col: 0, DurationMS: 100}}}}}}
	p := NewAnimationPlayer(m)
	p.Tick(.099999)
	if p.Index != 0 {
		t.Fatal("advanced one microsecond early")
	}
	p.Tick(.000001)
	if p.Index != 1 {
		t.Fatal("did not advance at100ms")
	}
	p.Stop()
	_, done := p.Tick(.099999)
	if done || p.Phase != AnimationEnd {
		t.Fatal("recovery finished early")
	}
	_, done = p.Tick(.000001)
	if !done || p.Phase != AnimationDone {
		t.Fatal("recovery did not complete at100ms")
	}
	_, done = p.Tick(0)
	if done {
		t.Fatal("completion reported twice")
	}
}
