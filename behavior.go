package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
)

// AnimationManifest describes animation timing and source pixels only. Images
// remain shared, read-only Atlas values; every cat owns a separate player.
// Schema 1 supports the legacy atlas and optional pixel rectangles in that image.
// A fallback is an honest reuse of artwork, not a claim of additional drawings.
type AnimationManifest struct {
	ReferenceWidth      int                        `json:"referenceWidth,omitempty"`
	GazeDirections      int                        `json:"gazeDirections,omitempty"`
	GazeNearestAuthored bool                       `json:"gazeNearestAuthored,omitempty"`
	SchemaVersion       int                        `json:"schemaVersion"`
	Fallback            string                     `json:"fallback"`
	Anchor              AnimationAnchor            `json:"anchor"`
	GazeOrigin          *AnimationAnchor           `json:"gazeOrigin,omitempty"`
	Actions             map[string]AnimationAction `json:"actions"`
}
type AnimationAnchor struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type FrameRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// FrameCanvas preserves the original transparent logical canvas when only its
// nontransparent bounds are stored in the PNG. No artwork is scaled or warped.
type FrameCanvas struct {
	W int `json:"width"`
	H int `json:"height"`
	X int `json:"offsetX"`
	Y int `json:"offsetY"`
}
type AnimationFrame struct {
	Canvas *FrameCanvas `json:"canvas,omitempty"`

	Row        int              `json:"row,omitempty"`
	Col        int              `json:"col,omitempty"`
	Rect       *FrameRect       `json:"rect,omitempty"`
	Anchor     *AnimationAnchor `json:"anchor,omitempty"`
	DurationMS int              `json:"durationMs"`
}
type AnimationClip struct {
	Start []AnimationFrame `json:"start,omitempty"`
	Loop  []AnimationFrame `json:"loop,omitempty"`
	End   []AnimationFrame `json:"end,omitempty"`
}
type AnimationMovement struct {
	TrialSpeedRatio float64 `json:"trialSpeedRatio,omitempty"`
	StrideRatio     float64 `json:"strideRatio"`
	Verified        bool    `json:"verified,omitempty"`
}

type AnimationAction struct {
	AnimationClip
	Fallback     string                   `json:"fallback,omitempty"`
	Moods        map[string]AnimationClip `json:"moods,omitempty"`
	DemoFallback bool                     `json:"demoFallback,omitempty"`
	Movement     *AnimationMovement       `json:"movement,omitempty"`
}

func LoadAnimationManifest(r io.Reader, imageWidth, imageHeight int) (*AnimationManifest, error) {
	data, err := io.ReadAll(io.LimitReader(r, 1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1024*1024 {
		return nil, errors.New("animation manifest exceeds 1 MB")
	}
	var m AnimationManifest
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&m); err != nil {
		return nil, err
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("animation manifest must contain one JSON object")
	}
	if err = m.Validate(imageWidth, imageHeight); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *AnimationManifest) Validate(imageWidth, imageHeight int) error {
	if m == nil || m.SchemaVersion != 1 {
		return errors.New("animation manifest needs schemaVersion 1")
	}
	if m.ReferenceWidth != 0 && (m.ReferenceWidth < 16 || m.ReferenceWidth > 4096) {
		return errors.New("referenceWidth must be zero, or 16 to 4096 source pixels")
	}
	if m.GazeDirections != 0 && m.GazeDirections != 4 && m.GazeDirections != 8 && m.GazeDirections != 16 && m.GazeDirections != 32 {
		return errors.New("gazeDirections must be 4, 8, 16 or 32")
	}
	if len(m.Actions) == 0 || len(m.Actions) > 128 {
		return errors.New("animation manifest needs 1 to 128 actions")
	}
	if !finite(m.Anchor.X) || !finite(m.Anchor.Y) || m.Anchor.X < 0 || m.Anchor.X > 1 || m.Anchor.Y < 0 || m.Anchor.Y > 1 {
		return errors.New("animation anchor must be normalized between 0 and 1")
	}
	if p := m.GazeOrigin; p != nil && (!finite(p.X) || !finite(p.Y) || p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1) {
		return errors.New("gazeOrigin must be normalized between 0 and 1")
	}
	if _, ok := m.Actions[m.Fallback]; !ok {
		return errors.New("animation manifest fallback action is missing")
	}
	validateClip := func(action string, clip AnimationClip) error {
		if len(clip.Start)+len(clip.Loop)+len(clip.End) > 4096 {
			return fmt.Errorf("action %q has too many frames", action)
		}
		for _, seq := range [][]AnimationFrame{clip.Start, clip.Loop, clip.End} {
			for _, f := range seq {
				if f.Anchor != nil && (!finite(f.Anchor.X) || !finite(f.Anchor.Y) || f.Anchor.X < 0 || f.Anchor.X > 1 || f.Anchor.Y < 0 || f.Anchor.Y > 1) {
					return fmt.Errorf("action %q has an invalid frame anchor", action)
				}
				if f.DurationMS < 10 || f.DurationMS > 60000 {
					return fmt.Errorf("action %q durationMs must be 10 to 60000", action)
				}
				if c := f.Canvas; c != nil {
					if f.Rect == nil || c.W < 1 || c.H < 1 || c.W > 4096 || c.H > 4096 || c.X < 0 || c.Y < 0 || f.Rect.W > c.W || f.Rect.H > c.H || c.X > c.W-f.Rect.W || c.Y > c.H-f.Rect.H {
						return fmt.Errorf("action %q has an invalid logical frame canvas", action)
					}
				}
				if f.Rect != nil {
					q := f.Rect
					if q.X < 0 || q.Y < 0 || q.W <= 0 || q.H <= 0 || q.W > imageWidth || q.H > imageHeight || q.X > imageWidth-q.W || q.Y > imageHeight-q.H {
						return fmt.Errorf("action %q rectangle is outside its image", action)
					}
				} else if imageWidth < 8 || imageHeight < 11 || imageWidth%8 != 0 || imageHeight%11 != 0 || f.Row < 0 || f.Row >= len(rowFrames) || f.Col < 0 || f.Col >= rowFrames[f.Row] {
					return fmt.Errorf("action %q references a missing legacy atlas frame", action)
				}
			}
		}
		return nil
	}
	for name, a := range m.Actions {
		if a.Movement != nil && (!finite(a.Movement.TrialSpeedRatio) || (a.Movement.TrialSpeedRatio != 0 && (a.Movement.TrialSpeedRatio < .05 || a.Movement.TrialSpeedRatio > .5))) {
			return fmt.Errorf("action %q trialSpeedRatio must be zero or 0.05 to 0.5 canvas widths per second", name)
		}
		if a.Movement != nil && (!finite(a.Movement.StrideRatio) || a.Movement.StrideRatio <= 0 || a.Movement.StrideRatio > 2) {
			return fmt.Errorf("action %q strideRatio must be finite and between 0 (exclusive) and 2", name)
		}
		if name == "" || len(name) > 64 {
			return errors.New("action names must be 1 to 64 bytes")
		}
		if err := validateClip(name, a.AnimationClip); err != nil {
			return err
		}
		if a.Fallback == "" && len(a.Loop) == 0 {
			return fmt.Errorf("action %q needs loop frames or a fallback", name)
		}
		for mood, clip := range a.Moods {
			if mood == "" || len(clip.Loop) == 0 {
				return fmt.Errorf("action %q mood needs a name and loop frames", name)
			}
			if err := validateClip(name+"/"+mood, clip); err != nil {
				return err
			}
		}
		seen := map[string]bool{}
		key := name
		for {
			if seen[key] {
				return fmt.Errorf("action %q has a fallback cycle", name)
			}
			seen[key] = true
			current, ok := m.Actions[key]
			if !ok {
				return fmt.Errorf("action %q has unknown fallback %q", name, key)
			}
			if current.Fallback == "" {
				break
			}
			key = current.Fallback
		}
	}
	return nil
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Resolve selects mood-specific artwork, then action fallbacks, then the global
// fallback. Unknown behavior names therefore remain drawable.
func (m *AnimationManifest) Resolve(action, mood string) AnimationClip {
	if m == nil {
		return AnimationClip{Loop: []AnimationFrame{{DurationMS: 250}}}
	}
	seen := map[string]bool{}
	var start, end []AnimationFrame
	finish := func(c AnimationClip) AnimationClip {
		if start != nil {
			c.Start = start
		}
		if end != nil {
			c.End = end
		}
		return c
	}
	for tries := 0; tries <= len(m.Actions)+1; tries++ {
		if seen[action] {
			break
		}
		seen[action] = true
		a, ok := m.Actions[action]
		if !ok {
			action = m.Fallback
			continue
		}
		if c, ok := a.Moods[mood]; ok && len(c.Loop) > 0 {
			return finish(c)
		}
		if len(a.Loop) > 0 {
			return finish(a.AnimationClip)
		}
		// An alias may supply its own entry/exit while reusing a fallback loop.
		// The first (most specific) entry and exit always win.
		if start == nil && len(a.Start) > 0 {
			start = a.Start
		}
		if end == nil && len(a.End) > 0 {
			end = a.End
		}
		if a.Fallback != "" {
			action = a.Fallback
		} else {
			action = m.Fallback
		}
	}
	return finish(AnimationClip{Loop: []AnimationFrame{{DurationMS: 250}}})
}
func atlasLoop(row, ms int) AnimationClip {
	c := AnimationClip{}
	for col := 0; col < rowFrames[row]; col++ {
		c.Loop = append(c.Loop, AnimationFrame{Row: row, Col: col, DurationMS: ms})
	}
	return c
}
func DefaultAnimationManifest() *AnimationManifest {
	m := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{.5, 1}, Actions: map[string]AnimationAction{}}
	m.Actions["idle"] = AnimationAction{AnimationClip: atlasLoop(0, 250)}
	m.Actions["walk_right"] = AnimationAction{AnimationClip: atlasLoop(1, 100)}
	m.Actions["walk_left"] = AnimationAction{AnimationClip: atlasLoop(2, 100)}
	m.Actions["pet"] = AnimationAction{AnimationClip: atlasLoop(3, 200)}
	m.Actions["drag"] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{{Row: 3, Col: 1, DurationMS: 250}}}, DemoFallback: true}
	m.Actions["sleep"] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{{Row: 0, Col: 3, DurationMS: 1000}}}, DemoFallback: true}
	for _, a := range []string{"sit", "rest", "social_rest", "groom", "stretch"} {
		m.Actions[a] = AnimationAction{Fallback: "idle", DemoFallback: true}
	}
	m.Actions["greet"] = AnimationAction{Fallback: "pet", DemoFallback: true}
	play := atlasLoop(4, 140)
	play.Start = append([]AnimationFrame(nil), play.Loop[:1]...)
	play.End = append([]AnimationFrame(nil), play.Loop[len(play.Loop)-1:]...)
	m.Actions["play"] = AnimationAction{AnimationClip: play}
	m.Actions["curious"] = AnimationAction{AnimationClip: atlasLoop(8, 260)}
	m.Actions["waiting"] = AnimationAction{AnimationClip: atlasLoop(6, 220)}
	for direction := 0; direction < 16; direction++ {
		m.Actions[fmt.Sprintf("gaze_%d", direction)] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{{Row: 9 + direction/8, Col: direction % 8, DurationMS: 200}}}}
	}
	return m
}

type AnimationPhase string

const (
	AnimationStart AnimationPhase = "start"
	AnimationLoop  AnimationPhase = "loop"
	AnimationEnd   AnimationPhase = "end"
	AnimationDone  AnimationPhase = "done"
)

// AnimationPlayer is never shared between cats. Generation changes on every
// replacement, so callers can reject delayed completion work after an interrupt.
type AnimationPlayer struct {
	Manifest     *AnimationManifest
	Action, Mood string
	Phase        AnimationPhase
	Index        int
	ElapsedMS    float64
	Generation   uint64
	clip         AnimationClip
	doneReported bool
	LoopOnce     bool
}

func NewAnimationPlayer(m *AnimationManifest) *AnimationPlayer {
	p := &AnimationPlayer{Manifest: m}
	p.Play("idle", "calm")
	return p
}
func (p *AnimationPlayer) Play(action, mood string) uint64 {
	p.Action = action
	p.Mood = mood
	p.clip = p.Manifest.Resolve(action, mood)
	p.Index = 0
	p.ElapsedMS = 0
	p.Generation++
	p.doneReported = false
	p.LoopOnce = false
	p.Phase = AnimationStart
	if len(p.clip.Start) == 0 {
		p.Phase = AnimationLoop
	}
	return p.Generation
}
func (p *AnimationPlayer) Set(action, mood string) {
	if action != p.Action || mood != p.Mood || p.Phase == AnimationDone {
		p.Play(action, mood)
	}
}
func (p *AnimationPlayer) Stop() {
	if p.Phase == AnimationEnd || p.Phase == AnimationDone {
		return
	}
	p.Phase = AnimationEnd
	p.Index = 0
	p.ElapsedMS = 0
	if len(p.clip.End) == 0 {
		p.Phase = AnimationDone
	}
}
func (p *AnimationPlayer) sequence() []AnimationFrame {
	switch p.Phase {
	case AnimationStart:
		return p.clip.Start
	case AnimationEnd, AnimationDone:
		if len(p.clip.End) > 0 {
			return p.clip.End
		}
		return p.clip.Loop
	default:
		return p.clip.Loop
	}
}
func (p *AnimationPlayer) Frame() AnimationFrame {
	seq := p.sequence()
	if len(seq) == 0 {
		return AnimationFrame{DurationMS: 250}
	}
	return seq[min(p.Index, len(seq)-1)]
}

// Tick advances real durations, skips whole loop cycles in O(frame count), and
// reports completion exactly once. It never calls back into behavior state.
func (p *AnimationPlayer) Tick(dt float64) (AnimationFrame, bool) {
	if !finite(dt) || dt < 0 {
		dt = 0
	}
	p.ElapsedMS += dt * 1000
	for p.Phase != AnimationDone {
		seq := p.sequence()
		if len(seq) == 0 {
			p.Phase = AnimationDone
			break
		}
		if p.Phase == AnimationLoop && !p.LoopOnce {
			total := 0
			for _, f := range seq {
				total += max(10, f.DurationMS)
			}
			if p.ElapsedMS >= float64(total) {
				p.ElapsedMS = math.Mod(p.ElapsedMS, float64(total))
			}
		}
		duration := float64(max(10, seq[p.Index].DurationMS))
		// Subtracting successive seconds timestamps can leave an exact frame
		// boundary a few floating-point ulps short. Treat only up to ten-nanosecond
		// residue as the boundary, so splitting a hold cannot add a whole tick.
		const boundaryToleranceMS = 1e-5
		if p.ElapsedMS+boundaryToleranceMS < duration {
			break
		}
		p.ElapsedMS = math.Max(0, p.ElapsedMS-duration)
		p.Index++
		if p.Index < len(seq) {
			continue
		}
		p.Index = 0
		switch p.Phase {
		case AnimationLoop:
			if p.LoopOnce {
				// A single-shot action must reach its recovery instead of
				// restarting takeoff. Present the first recovery pose before
				// charging any delayed timer overflow to its duration.
				p.Phase = AnimationEnd
				p.ElapsedMS = 0
				if len(p.clip.End) == 0 {
					p.Phase = AnimationDone
				} else {
					return p.Frame(), false
				}
			}
		case AnimationStart:
			p.Phase = AnimationLoop
		case AnimationEnd:
			p.Phase = AnimationDone
			p.Index = max(0, len(p.clip.End)-1)
		}
	}
	completed := p.Phase == AnimationDone && !p.doneReported
	if completed {
		p.doneReported = true
		p.ElapsedMS = 0
	}
	return p.Frame(), completed
}

type BehaviorPriority int

const (
	PriorityIdle BehaviorPriority = iota
	PriorityWander
	PrioritySocial
	PriorityBusy
	PriorityPlay
	PriorityPet
	PriorityDrag
)

type BehaviorFrame struct {
	AnimationFrame
	Action     string
	Mood       string
	Anchor     AnimationAnchor
	Generation uint64
}
type CatBehavior struct {
	OneShot                                            bool
	MotionTurning                                      bool
	MotionSeconds                                      float64
	LastPresentedGeneration                            uint64
	LastPresentedPhase                                 AnimationPhase
	Action, Mood                                       string
	Priority                                           BehaviorPriority
	Until, NextDecision                                float64
	PendingDuration                                    float64
	Player                                             *AnimationPlayer
	WasDragging                                        bool
	Ending                                             bool
	Temperament                                        Temperament
	TargetX                                            float64
	HasDestination                                     bool
	LastChoice                                         string
	ActionCooldown                                     map[string]float64
	GazeDirection                                      int
	GazeActive                                         bool
	SocialMoving                                       bool
	AfterAction                                        string
	Autonomous                                         bool
	BusyElapsed, BusyNextStretch, BusyStretchRemaining float64
	BusyStretching, BusyStretchEnding                  bool
}
type BehaviorEngine struct {
	Cats                 []*Cat
	States               []*CatBehavior
	Manifest             *AnimationManifest
	Social               *SocialCoordinator
	Activity             ActivityLevel
	Load                 *CPULoadState
	ExperimentalMovement bool
}

func NewBehaviorEngine(cats []*Cat, m *AnimationManifest) *BehaviorEngine {
	if m == nil {
		m = DefaultAnimationManifest()
	}
	e := &BehaviorEngine{Cats: cats, Manifest: m, Social: NewSocialCoordinator(), Activity: ActivityNormal, Load: NewCPULoadState(DefaultCPUSettings())}
	for i := range cats {
		e.States = append(e.States, &CatBehavior{Action: "idle", Mood: "calm", Priority: PriorityIdle, NextDecision: 2 + float64(i)*1.3, Player: NewAnimationPlayer(m), Temperament: DefaultTemperament(i), ActionCooldown: map[string]float64{}})
		e.States[i].Player.Tick(float64(i) * .17)
	}
	return e
}
func (e *BehaviorEngine) valid(i int) bool { return i >= 0 && i < len(e.Cats) && e.Cats[i] != nil }

// SetManifest gives one cat independently supplied artwork and resets only that
// cat's animation cursor. The caller must validate the manifest when loading it.
func (e *BehaviorEngine) SetManifest(i int, m *AnimationManifest) bool {
	if !e.valid(i) || m == nil {
		return false
	}
	s := e.States[i]
	s.Player.Manifest = m
	s.MotionTurning = false
	s.OneShot = false
	s.PendingDuration = 0
	s.GazeActive = false
	s.SocialMoving = false
	s.Player.Play(s.Action, s.Mood)
	s.Ending = false
	s.HasDestination = false
	s.AfterAction = ""
	return true
}
func (e *BehaviorEngine) Cancel(i int, now float64) {
	if !e.valid(i) {
		return
	}
	e.Social.Cancel(i, now)
	s := e.States[i]
	s.Action = "idle"
	s.Priority = PriorityIdle
	s.GazeActive = false
	s.SocialMoving = false
	s.MotionTurning = false
	s.OneShot = false
	s.PendingDuration = 0
	s.Until = 0
	s.Ending = false
	s.NextDecision = now + 8
	s.HasDestination = false
	s.AfterAction = ""
	s.Autonomous = false
	s.BusyStretching = false
	s.BusyStretchEnding = false
	s.Player.Play("idle", s.Mood)
}
func (e *BehaviorEngine) Pet(i int, now float64) {
	if !e.valid(i) {
		return
	}
	if !e.Request(i, "pet", PriorityPet, now, 2) {
		return
	}
	e.Cats[i].Pet(now)
	e.States[i].Mood = "happy"
}
func (e *BehaviorEngine) Play(i int, now float64) {
	if !e.valid(i) {
		return
	}
	s := e.States[i]
	oldMood := s.Mood
	s.Mood = "playful"
	if !e.Request(i, "play", PriorityPlay, now, actionCycle(s.Player.Manifest, "play", s.Mood)) {
		s.Mood = oldMood
		return
	}
	s.OneShot = true
	s.Player.LoopOnce = true
}

// Request respects current live priorities. Dragging cannot be interrupted by a
// click or a menu command. Equal priority restarts deliberate repeated actions.
func (e *BehaviorEngine) Request(i int, action string, priority BehaviorPriority, now, duration float64) bool {
	if !e.valid(i) || e.Cats[i].Dragging || e.Cats[i].Pressed || !finite(now) || !finite(duration) || duration <= 0 {
		return false
	}
	s := e.States[i]
	if !HasAuthoredAction(s.Player.Manifest, action, s.Mood) {
		return false
	}
	if s.WasDragging {
		e.Cancel(i, now)
		s.WasDragging = false
	}
	if (s.OneShot || now < s.Until || s.Ending) && priority < s.Priority {
		return false
	}
	e.Social.Cancel(i, now)
	s.Action = action
	s.Priority = priority
	s.Until = now + duration
	s.PendingDuration = duration
	s.Ending = false
	s.HasDestination = false
	s.AfterAction = ""
	s.Autonomous = false
	s.BusyStretching = false
	s.BusyStretchEnding = false
	s.MotionTurning = false
	s.OneShot = false
	// A fresh release-click can arrive before the next timer sees Dragging=false.
	// It replaces the drag instead of being mistaken for stale pre-drag work.
	s.WasDragging = false
	s.NextDecision = s.Until + 8
	s.Player.Play(action, s.Mood)
	return true
}
func (e *BehaviorEngine) SetMood(i int, mood string) {
	if e.valid(i) && mood != "" {
		e.States[i].Mood = mood
	}
}
func (e *BehaviorEngine) Tick(now, dt, cursorX, cursorY, idleSeconds float64, quiet bool) []BehaviorFrame {
	if !finite(now) {
		now = 0
	}
	if !finite(dt) || dt < 0 {
		dt = 0
	}
	dt = math.Min(dt, .25)
	quiet = quiet || e.Activity == ActivityQuiet
	if e.Load != nil {
		e.Load.Expire(now)
	}
	blocked := make([]bool, len(e.Cats))
	busy := make([]bool, len(e.Cats))
	for i, c := range e.Cats {
		if c == nil {
			blocked[i] = true
			continue
		}
		s := e.States[i]
		// A requested animation's visible duration starts with its first
		// presentation, even if a modal UI or a delayed timer held that up.
		if s.PendingDuration > 0 && !c.Dragging && !c.Pressed {
			if s.Player.Phase == AnimationStart || s.Player.Phase == AnimationLoop {
				s.Until = now + s.PendingDuration
				s.NextDecision = s.Until + 8
			}
			s.PendingDuration = 0
		}
		// Advance only the action that was actually visible during the elapsed
		// interval. A newly requested action or exit must show its first pose.
		s.MotionSeconds = 0
		if s.Player.Generation == s.LastPresentedGeneration && !(s.Player.Phase == AnimationEnd && s.LastPresentedPhase != AnimationEnd) {
			s.MotionSeconds = s.Player.loopSeconds(dt)
			s.Player.Tick(dt)
		}
		busy[i] = e.cpuEligible(i, quiet)
		if e.Load == nil || !e.Load.Active {
			s.BusyElapsed = 0
			s.BusyNextStretch = 0
		}
		if !busy[i] {
			e.clearBusyAction(i, now)
		}
		if quiet && s.Autonomous {
			e.Cancel(i, now)
		}
		if c.Dragging {
			if !s.WasDragging {
				e.Cancel(i, now)
			}
			s.WasDragging = true
			s.Action = "drag"
			s.Priority = PriorityDrag
			s.Until = now + dt + 1
		} else if s.WasDragging {
			s.WasDragging = false
			e.Cancel(i, now)
			s.Mood = "calm"
		}
		// Waking from an autonomous sleep is a gentle transition. Direct
		// interaction still interrupts immediately through the normal priorities.
		if s.Action == "sleep" && s.Priority == PriorityIdle && !quiet && (idleSeconds < 180 || busy[i]) && !c.Dragging {
			s.Priority = PriorityPlay
			s.Until = now
			s.Ending = true
			s.Player.Stop()
		}
		if s.Priority >= PriorityPlay && ((s.OneShot && s.Player.Phase == AnimationDone) || (!s.OneShot && now >= s.Until)) && !c.Dragging {
			if !s.Ending && !s.OneShot {
				s.Player.Stop()
				s.Ending = true
			}
			if s.Player.Phase == AnimationDone {
				s.Priority = PriorityIdle
				s.OneShot = false
				s.Action = "idle"
				s.Until = 0
				s.Ending = false
				s.Mood = "calm"
				if s.AfterAction != "" && !quiet && idleSeconds < 180 {
					next := s.AfterAction
					s.AfterAction = ""
					if HasAuthoredAction(s.Player.Manifest, next, s.Mood) {
						s.Action = next
						s.Until = now + actionCycle(s.Player.Manifest, next, s.Mood)
						s.NextDecision = s.Until + e.dwell(i)
					}
				}
				s.AfterAction = ""
			}
		}
		// Drag release and completed direct actions may change mood. Re-evaluate
		// artwork in that final mood before letting CPU state claim the action.
		busy[i] = e.cpuEligible(i, quiet)
		if !busy[i] {
			e.clearBusyAction(i, now)
		}
		blocked[i] = c.Dragging || c.Pressed || (s.Priority >= PriorityPlay && (s.OneShot || now < s.Until || s.Ending))
	}
	e.configureSocial()
	socialBlocked := append([]bool(nil), blocked...)
	for i := range socialBlocked {
		socialBlocked[i] = socialBlocked[i] || busy[i]
	}
	intents := e.Social.Tick(e.Cats, socialBlocked, now, dt, quiet || idleSeconds >= 180)
	out := make([]BehaviorFrame, len(e.Cats))
	for i, c := range e.Cats {
		if c == nil {
			continue
		}
		s := e.States[i]
		action := s.Action
		finishingSocial := false
		switch {
		case c.Dragging:
			action = "drag"
		case blocked[i]:
			action = s.Action
			if c.Pressed && !c.Dragging {
				action = "idle"
			}
		case busy[i]:
			action = e.tickBusy(i, now, dt)
		case quiet || idleSeconds >= 180:
			s.Action = e.restingAction(i)
			if quiet && idleSeconds < 180 && finite(cursorX) && finite(cursorY) && hasGazeArtwork(s.Player.Manifest, s.Mood) {
				s.Action = "idle"
			}
			s.HasDestination = false
			s.Priority = PriorityIdle
			s.Until = 0
			s.NextDecision = now + 5
			s.Mood = "sleepy"
			action = s.Action
		default:
			if intent, ok := intents[i]; ok {
				s.Action = intent.Action
				s.Autonomous = true
				s.HasDestination = false
				s.Priority = PrioritySocial
				s.Until = now + .5
				action = e.socialAction(i, intent.Action)
				if intent.Action == "greet_end" {
					action = "greet"
					if s.Player.Action != action {
						s.Player.Play(action, s.Mood)
					}
					s.Player.Stop()
					finishingSocial = true
				}
				if intent.Move {
					direction := intent.TargetX - c.X
					action = e.movementAction(i, direction, intent.Action == "chase")
					// Check the reachable path before selecting a moving pose. A
					// blocked cat must not restart its entry on every other tick.
					path := math.Hypot(intent.TargetX-c.X, intent.TargetY-c.Y)
					rx, ry := e.moveTowardPosition(i, intent.TargetX, intent.TargetY, path)
					reachable := math.Hypot(rx-c.X, ry-c.Y)
					threshold := 1.0
					if !s.SocialMoving && (intent.Action == "follow" || intent.Action == "chase") {
						threshold = math.Max(4, math.Max(float64(c.W)*.02, e.movementSpeed(i, action)*.25))
					}
					if action == "" || reachable <= threshold {
						action = "idle"
						s.SocialMoving = false
					} else {
						s.SocialMoving = true
						distance := e.movementDistance(i, action)
						e.moveToward(i, intent.TargetX, intent.TargetY, distance)
					}
				} else {
					s.SocialMoving = false
				}
			} else {
				s.SocialMoving = false
				action = e.tickAutonomy(i, now, dt, cursorX, cursorY)
			}
		}
		// Gaze is a presentation of idle, not an autonomous action or a new
		// reservation. It cannot delay roaming or interrupt higher priorities.
		if action == "idle" && s.Priority == PriorityIdle && !s.Ending && idleSeconds < 180 {
			action = e.pointerGaze(i, cursorX, cursorY)
		} else {
			s.GazeActive = false
		}
		// A direction change may reuse an explicitly declared grounded recovery
		// before starting the opposite clip. Direct input still cancels at once.
		finishingMotion := false
		normalSocial := s.Priority == PrioritySocial && !blocked[i] && !busy[i] && !quiet && idleSeconds < 180
		if s.MotionTurning {
			if (!isLocomotion(action) && !normalSocial) || s.Ending || s.Player.Phase == AnimationDone {
				s.MotionTurning = false
			} else {
				action = s.Player.Action
				finishingMotion = true
			}
		} else if isLocomotion(s.Player.Action) && action != s.Player.Action && (isLocomotion(action) || normalSocial) && !s.Ending && s.Player.Phase != AnimationDone && len(s.Player.clip.End) > 0 {
			s.Player.Stop()
			s.MotionTurning = true
			if s.Priority == PriorityWander {
				s.Until += clipSeconds(s.Player.clip.End)
			}
			action = s.Player.Action
			finishingMotion = true
		}
		c.Mode = action
		if !s.Ending && !finishingSocial && !finishingMotion {
			s.Player.Set(action, s.Mood)
			if s.OneShot {
				s.Player.LoopOnce = true
			}
		}
		frame := s.Player.Frame()
		s.LastPresentedGeneration = s.Player.Generation
		s.LastPresentedPhase = s.Player.Phase
		anchor := s.Player.Manifest.Anchor
		if frame.Anchor != nil {
			anchor = *frame.Anchor
		}
		out[i] = BehaviorFrame{AnimationFrame: frame, Action: action, Mood: s.Mood, Anchor: anchor, Generation: s.Player.Generation}
	}
	return out
}

// moveToward clamps the path before a neighbor, including when a long timer tick
// would otherwise step through it. Cats on other monitors are never obstacles.
func (e *BehaviorEngine) moveToward(i int, x, y, speed float64) {
	e.Cats[i].X, e.Cats[i].Y = e.moveTowardPosition(i, x, y, speed)
}

// The same collision calculation is used for preflight and actual movement.
// Asking whether a path is open never moves the cat or its animation clock.
func (e *BehaviorEngine) moveTowardPosition(i int, x, y, speed float64) (float64, float64) {
	c := e.Cats[i]
	dx, dy := x-c.X, y-c.Y
	distance := math.Hypot(dx, dy)
	if distance == 0 || speed <= 0 {
		return c.X, c.Y
	}
	scale := math.Min(1, speed/distance)
	nx, ny := ClampPosition(c.X+dx*scale, c.Y+dy*scale, c.W, c.H, c.Bounds)
	gap := math.Max(6, float64(c.W)*.06)
	for j, o := range e.Cats {
		if i == j || o == nil || o.Bounds != c.Bounds {
			continue
		}
		if ny+float64(c.H) <= o.Y || ny >= o.Y+float64(o.H) {
			continue
		}
		if c.X < o.X+float64(o.W) && c.X+float64(c.W) > o.X {
			// Dragging can deliberately leave two windows overlapping. Autonomous
			// motion may separate them, but must not cross through the neighbor.
			center := c.X + float64(c.W)/2
			otherCenter := o.X + float64(o.W)/2
			if (nx > c.X && center <= otherCenter) || (nx < c.X && center >= otherCenter) {
				nx = c.X
			}
		}
		if nx > c.X && c.X+float64(c.W) <= o.X+gap {
			nx = math.Min(nx, o.X-float64(c.W)-gap)
			nx = math.Max(nx, c.X)
		}
		if nx < c.X && c.X >= o.X+float64(o.W)-gap {
			nx = math.Max(nx, o.X+float64(o.W)+gap)
			nx = math.Min(nx, c.X)
		}
		// Vertical movement is not part of autonomous behavior. Reject a diagonal
		// step that enters an occupied window rather than allowing corner tunneling.
		if ny != c.Y && nx < o.X+float64(o.W) && nx+float64(c.W) > o.X && ny < o.Y+float64(o.H) && ny+float64(c.H) > o.Y {
			ny = c.Y
		}
	}
	return ClampPosition(nx, ny, c.W, c.H, c.Bounds)
}

type SocialIntent struct {
	Action           string
	TargetX, TargetY float64
	Move             bool
}
type SocialPlan struct {
	ID                               uint64
	A, B, Leader                     int
	Phase                            string
	Started, PhaseUntil, Destination float64
	Bounds                           Rect
	Chase                            bool
}
type SocialCoordinator struct {
	Plans            map[uint64]*SocialPlan
	Reservations     map[int]uint64
	Cooldown         map[int]float64
	NextAttempt      float64
	generation       uint64
	Activity         ActivityLevel
	Eligible         func(int, int) bool
	Supports         func(int, string) bool
	GreetingDuration func(int) float64
	GreetingEnded    func(int) bool
	MovementEnded    func(int) bool
	Sociability      []float64
}

func NewSocialCoordinator() *SocialCoordinator {
	return &SocialCoordinator{Plans: map[uint64]*SocialPlan{}, Reservations: map[int]uint64{}, Cooldown: map[int]float64{}, NextAttempt: 8, Activity: ActivityNormal}
}
func (s *SocialCoordinator) Cancel(index int, now float64) {
	id, ok := s.Reservations[index]
	if !ok {
		return
	}
	p := s.Plans[id]
	if p == nil {
		delete(s.Reservations, index)
		return
	}
	delete(s.Plans, id)
	delete(s.Reservations, p.A)
	delete(s.Reservations, p.B)
	cooldown := 25.0
	if s.Activity == ActivityLively {
		cooldown = 12
	}
	s.Cooldown[p.A] = now + cooldown
	s.Cooldown[p.B] = now + cooldown
}
func (s *SocialCoordinator) Start(cats []*Cat, a, b int, now float64) bool {
	if a == b || a < 0 || b < 0 || a >= len(cats) || b >= len(cats) || cats[a] == nil || cats[b] == nil {
		return false
	}
	ca, cb := cats[a], cats[b]
	if s.Eligible != nil && !s.Eligible(a, b) {
		return false
	}
	_, ar := s.Reservations[a]
	_, br := s.Reservations[b]
	if ar || br || ca.Dragging || cb.Dragging || now < s.Cooldown[a] || now < s.Cooldown[b] || ca.Bounds != cb.Bounds {
		return false
	}
	if math.Abs((ca.Y+float64(ca.H))-(cb.Y+float64(cb.H))) > 8 {
		return false
	}
	if math.Abs(ca.X-cb.X) > math.Max(520, float64(max(ca.W, cb.W))*4) {
		return false
	}
	if ca.Bounds.Width() < ca.W+cb.W+int(socialGap(ca, cb)) {
		return false
	}
	if ca.X > cb.X {
		a, b = b, a
		ca, cb = cb, ca
	}
	// Do not invite cats on opposite sides of a third cat to walk through it.
	// A later dragged-in obstacle is handled by movement preflight as well.
	for i, other := range cats {
		if i == a || i == b || other == nil || other.Bounds != ca.Bounds {
			continue
		}
		if other.Y+float64(other.H) <= ca.Y || other.Y >= ca.Y+float64(ca.H) {
			continue
		}
		if other.X < cb.X && other.X+float64(other.W) > ca.X+float64(ca.W) {
			return false
		}
	}
	s.generation++
	p := &SocialPlan{ID: s.generation, A: a, B: b, Leader: b, Phase: "approach", Started: now, PhaseUntil: now + 10, Bounds: ca.Bounds}
	if s.Supports != nil && s.Supports(a, "run_left") && s.Supports(a, "run_right") && s.Supports(b, "run_left") && s.Supports(b, "run_right") {
		chance := .2
		if s.Activity == ActivityLively {
			chance = .6
		}
		p.Chase = ca.Seed.Float64() < chance
	}
	s.Plans[p.ID] = p
	s.Reservations[a] = p.ID
	s.Reservations[b] = p.ID
	return true
}
func socialGap(a, b *Cat) float64 { return math.Max(10, float64(max(a.W, b.W))*.12) }
func (s *SocialCoordinator) beginGreeting(p *SocialPlan, a, b *Cat, now float64) {
	p.Phase = "greet"
	p.PhaseUntil = now + 2.4
	if s.GreetingDuration != nil {
		p.PhaseUntil = now + math.Max(2.4, math.Max(s.GreetingDuration(p.A), s.GreetingDuration(p.B)))
	}
	if s.Supports != nil && (!s.Supports(p.A, "greet") || !s.Supports(p.B, "greet")) {
		s.beginFollowing(p, a, b, now)
	}
}
func (s *SocialCoordinator) beginFollowing(p *SocialPlan, a, b *Cat, now float64) {
	p.Phase = "follow"
	if p.Chase {
		p.Phase = "chase"
	}
	p.PhaseUntil = now + 3 + a.Seed.Float64()*4
	right := float64(b.Bounds.Right-b.W) - b.X
	left := a.X - float64(a.Bounds.Left)
	distance := float64(max(a.W, b.W)) * (.6 + a.Seed.Float64()*1.8)
	if left > right {
		p.Leader = p.A
		p.Destination = math.Max(float64(a.Bounds.Left), a.X-distance)
	} else {
		p.Leader = p.B
		p.Destination = math.Min(float64(b.Bounds.Right-b.W), b.X+distance)
	}
}
func (s *SocialCoordinator) Tick(cats []*Cat, blocked []bool, now, dt float64, quiet bool) map[int]SocialIntent {
	intents := map[int]SocialIntent{}
	isBlocked := func(i int) bool {
		return i < 0 || i >= len(cats) || cats[i] == nil || cats[i].Dragging || (i < len(blocked) && blocked[i])
	}
	ids := make([]uint64, 0, len(s.Plans))
	for id := range s.Plans {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		p := s.Plans[id]
		if p == nil {
			continue
		}
		if quiet || isBlocked(p.A) || isBlocked(p.B) || cats[p.A].Bounds != p.Bounds || cats[p.B].Bounds != p.Bounds || (s.Eligible != nil && !s.Eligible(p.A, p.B)) {
			s.Cancel(p.A, now)
			continue
		}
		a, b := cats[p.A], cats[p.B]
		gap := socialGap(a, b)
		distance := b.X - (a.X + float64(a.W))
		switch p.Phase {
		case "approach":
			if distance >= gap-2 && distance <= gap+5 {
				if s.MovementEnded != nil && (!s.MovementEnded(p.A) || !s.MovementEnded(p.B)) {
					p.Phase = "approach_end"
				} else {
					s.beginGreeting(p, a, b, now)
				}
			} else if now >= p.PhaseUntil {
				s.Cancel(p.A, now)
				continue
			} else {
				center := (a.X + float64(a.W) + b.X) / 2
				ax := clamp(center-gap/2-float64(a.W), float64(a.Bounds.Left), float64(a.Bounds.Right-a.W-b.W)-gap)
				bx := ax + float64(a.W) + gap
				intents[p.A] = SocialIntent{"approach", ax, a.Y, true}
				intents[p.B] = SocialIntent{"approach", bx, b.Y, true}
				continue
			}
		case "approach_end":
			if s.MovementEnded == nil || (s.MovementEnded(p.A) && s.MovementEnded(p.B)) {
				s.beginGreeting(p, a, b, now)
			}
		case "greet":
			if now >= p.PhaseUntil {
				if s.GreetingEnded != nil && (!s.GreetingEnded(p.A) || !s.GreetingEnded(p.B)) {
					p.Phase = "greet_end"
				} else {
					s.beginFollowing(p, a, b, now)
				}
			}
		case "greet_end":
			if s.GreetingEnded == nil || (s.GreetingEnded(p.A) && s.GreetingEnded(p.B)) {
				s.beginFollowing(p, a, b, now)
			}
		case "follow", "chase":
			if now >= p.PhaseUntil {
				p.Phase = "rest"
				p.PhaseUntil = now + 3 + a.Seed.Float64()*6
			}
		case "rest":
			if now >= p.PhaseUntil {
				s.Cancel(p.A, now)
				continue
			}
		}
		switch p.Phase {
		case "approach_end":
			intents[p.A] = SocialIntent{Action: "social_settle"}
			intents[p.B] = SocialIntent{Action: "social_settle"}
		case "greet", "greet_end":
			intents[p.A] = SocialIntent{Action: p.Phase}
			intents[p.B] = SocialIntent{Action: p.Phase}
		case "follow", "chase":
			if p.Leader == p.B {
				intents[p.B] = SocialIntent{p.Phase, p.Destination, b.Y, true}
				intents[p.A] = SocialIntent{p.Phase, b.X - float64(a.W) - gap, a.Y, true}
			} else {
				intents[p.A] = SocialIntent{p.Phase, p.Destination, a.Y, true}
				intents[p.B] = SocialIntent{p.Phase, a.X + float64(a.W) + gap, b.Y, true}
			}
		case "rest":
			intents[p.A] = SocialIntent{Action: "social_rest"}
			intents[p.B] = SocialIntent{Action: "social_rest"}
		}
	}
	if quiet {
		s.NextAttempt = now + 15
		return intents
	}
	if now >= s.NextAttempt {
		s.NextAttempt = now + 12
		order := make([]int, 0, len(cats))
		for i, c := range cats {
			if c != nil {
				order = append(order, i)
			}
		}
		if len(order) > 0 {
			rng := cats[order[0]].Seed
			rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
			delay := 8 + rng.Float64()*12
			if s.Activity == ActivityLively {
				delay *= .55
			}
			s.NextAttempt = now + delay
			started := false
			for ai, a := range order {
				if started || isBlocked(a) {
					continue
				}
				for _, b := range order[ai+1:] {
					affinity := .65
					if a < len(s.Sociability) && b < len(s.Sociability) {
						affinity = (s.Sociability[a] + s.Sociability[b]) / 2
					}
					if !isBlocked(b) && rng.Float64() < .15+.8*affinity && s.Start(cats, a, b, now) {
						started = true
						break
					}
				}
			}
		}
	}
	return intents
}

// ResolveMovement follows declared action aliases, but never fabricates stride
// calibration for a missing or static fallback animation.
func ResolveMovement(m *AnimationManifest, action string) *AnimationMovement {
	if m == nil {
		return nil
	}
	seen := map[string]bool{}
	for !seen[action] {
		seen[action] = true
		a, ok := m.Actions[action]
		if !ok {
			return nil
		}
		if a.Movement != nil {
			return a.Movement
		}
		if a.Fallback == "" {
			return nil
		}
		action = a.Fallback
	}
	return nil
}

// WalkPixelsPerSecond converts a measured source stride into rendered motion.
// Without an explicitly calibrated stride, preserve the legacy conservative
// rate. Callers and preview metadata must identify that fallback as uncalibrated.
func WalkPixelsPerSecond(m *AnimationManifest, action, mood string, width int) float64 {
	if width <= 0 || !HasWalkAnimation(m, action, mood) {
		return 0
	}
	movement := ResolveMovement(m, action)
	if movement == nil || !finite(movement.StrideRatio) || movement.StrideRatio <= 0 {
		return float64(width) * .23
	}
	clip := m.Resolve(action, mood)
	milliseconds := 0
	for _, f := range clip.Loop {
		milliseconds += f.DurationMS
	}
	if milliseconds <= 0 {
		return float64(width) * .23
	}
	return float64(width) * movement.StrideRatio / (float64(milliseconds) / 1000)
}

// HasWalkAnimation prevents a missing walk action from sliding a sitting/idle
// fallback across the desktop. It proves only distinct referenced frames, not
// anatomical gait quality; supplied artwork still needs motion review.
func HasWalkAnimation(m *AnimationManifest, action, mood string) bool {
	if m == nil {
		return false
	}
	name := action
	seen := map[string]bool{}
	var clip AnimationClip
	for {
		if seen[name] {
			return false
		}
		seen[name] = true
		a, ok := m.Actions[name]
		if !ok {
			return false
		}
		if v, ok := a.Moods[mood]; ok && len(v.Loop) > 0 {
			clip = v
			break
		}
		if len(a.Loop) > 0 {
			clip = a.AnimationClip
			break
		}
		if a.Fallback == "" {
			return false
		}
		name = a.Fallback
	}
	switch name {
	case "idle", "sleep", "sit", "rest", "social_rest", "pet", "drag", "greet":
		return false
	}
	unique := map[[6]int]bool{}
	for _, f := range clip.Loop {
		key := [6]int{f.Row, f.Col, -1, -1, 0, 0}
		if f.Rect != nil {
			key = [6]int{f.Rect.X, f.Rect.Y, f.Rect.W, f.Rect.H, 0, 0}
		}
		unique[key] = true
	}
	return len(unique) >= 2
}
