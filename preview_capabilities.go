//go:build !windows && motionpreview

package main

import (
	"crypto/sha256"
	"math"
)

type previewActionCapability struct {
	Action           string              `json:"action"`
	Authored         bool                `json:"authored"`
	RuntimeEligible  bool                `json:"runtime_eligible"`
	DistinctDrawings int                 `json:"distinct_rendered_frames"`
	StartMS          int                 `json:"start_ms"`
	LoopMS           int                 `json:"loop_ms"`
	EndMS            int                 `json:"end_ms"`
	Source           previewActionSource `json:"source"`
}
type previewCapabilities struct {
	Name                   string                    `json:"name"`
	Actions                []previewActionCapability `json:"actions"`
	AuthoredGazeDirections []int                     `json:"authored_gaze_directions"`
	UsableGazeDirections   []int                     `json:"usable_gaze_directions"`
	DistinctGazeDrawings   int                       `json:"distinct_gaze_drawings"`
}

func previewSequenceMS(seqs ...[]AnimationFrame) int {
	ms := 0
	for _, seq := range seqs {
		for _, f := range seq {
			ms += f.DurationMS
		}
	}
	return ms
}
func previewSequenceFrames(fps int, seqs ...[]AnimationFrame) int {
	return int(math.Ceil(float64(previewSequenceMS(seqs...)) * float64(fps) / 1000))
}
func previewCapabilityReport(cat previewCat) previewCapabilities {
	report := previewCapabilities{Name: cat.Spec.Name, AuthoredGazeDirections: []int{}, UsableGazeDirections: []int{}}
	for _, action := range []string{"idle", "run_left", "run_right", "pet", "drag", "play", "knead", "stretch", "greet", "sleep", "groom", "pounce"} {
		mood := "calm"
		if action == "pet" {
			mood = "happy"
		} else if action == "play" {
			mood = "playful"
		}
		cat.Player = NewAnimationPlayer(cat.Manifest)
		cat.Player.Play(action, mood)
		clip := cat.Manifest.Resolve(action, mood)
		authored := HasAuthoredAction(cat.Manifest, action, mood)
		eligible := authored
		switch action {
		case "knead", "stretch":
			eligible = HasCPUAction(cat.Manifest, action, mood)
		case "run_left", "run_right":
			eligible = HasRunAnimation(cat.Manifest, action, mood) || (cat.ExperimentalMovement && HasRunArtwork(cat.Manifest, action, mood))
		}
		pixels := map[[32]byte]bool{}
		if authored {
			for _, seq := range [][]AnimationFrame{clip.Start, clip.Loop, clip.End} {
				for _, f := range seq {
					pixels[sha256.Sum256(previewImageFrame(&cat, f).Pix)] = true
				}
			}
		}
		report.Actions = append(report.Actions, previewActionCapability{action, authored, eligible, len(pixels), previewSequenceMS(clip.Start), previewSequenceMS(clip.Loop), previewSequenceMS(clip.End), previewResolvedSource(cat, action, mood)})
	}
	pixels := map[[32]byte]bool{}
	for d := 0; d < 16; d++ {
		name := "gaze_" + itoaDirection(d)
		a, exists := cat.Manifest.Actions[name]
		if !HasAuthoredAction(cat.Manifest, name, "calm") {
			continue
		}
		report.UsableGazeDirections = append(report.UsableGazeDirections, d)
		if exists && !a.DemoFallback && len(a.Loop) > 0 {
			report.AuthoredGazeDirections = append(report.AuthoredGazeDirections, d)
			for _, f := range a.Loop {
				pixels[sha256.Sum256(previewImageFrame(&cat, f).Pix)] = true
			}
		}
	}
	report.DistinctGazeDrawings = len(pixels)
	return report
}
