//go:build !windows && motionpreview

package main

// This optional headless tool renders only pixels from an explicitly supplied
// private pack. It is not part of the Windows executable and has no embedded or
// generated artwork fallback.

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	previewCanvasWidth  = 900
	previewCanvasHeight = 360
	previewColumns      = 3
	previewFramePattern = "frame_%06d.png"
	previewGaitHeight   = 660
	previewGaitLane     = 220
	previewGaitMargin   = 40
)

var previewBackground = color.RGBA{R: 244, G: 239, B: 231, A: 255}

type previewOptions struct {
	Root           string
	Out            string
	FPS            int
	Width          int
	ActionDuration time.Duration
	GazeDuration   time.Duration
	GaitOnly       bool
	GaitActions    string
	Actions        string
}

type previewCat struct {
	Spec                 CatSpec
	Atlas                *Atlas
	Manifest             *AnimationManifest
	Player               *AnimationPlayer
	Width                int
	Height               int
	ExperimentalMovement bool
}

type previewCatMetadata struct {
	Name       string `json:"name"`
	Sprite     string `json:"sprite"`
	Animations string `json:"animations"`
	Column     int    `json:"column"`
	Width      int    `json:"render_width"`
	Height     int    `json:"render_height"`
	Left       int    `json:"left"`
	Top        int    `json:"top"`
}

type previewActionSource struct {
	Name           string   `json:"name"`
	ResolvedAction string   `json:"resolved_action"`
	FallbackChain  []string `json:"fallback_chain"`
	GlobalFallback bool     `json:"global_fallback"`
	DemoFallback   bool     `json:"demo_fallback"`
}

type previewScene struct {
	Scene        string                `json:"scene"`
	Action       string                `json:"action"`
	Mood         string                `json:"mood"`
	StartFrame   int                   `json:"start_frame"`
	EndFrame     int                   `json:"end_frame_exclusive"`
	StartSeconds float64               `json:"start_seconds"`
	EndSeconds   float64               `json:"end_seconds"`
	Sources      []previewActionSource `json:"sources"`
	Gait         []previewGaitSource   `json:"gait,omitempty"`
	StopFrame    int                   `json:"stop_frame,omitempty"`
}

type previewGaitSource struct {
	Name                 string   `json:"name"`
	Lane                 int      `json:"lane"`
	PixelsPerSecond      float64  `json:"pixels_per_second"`
	RootPixelsPerSample  float64  `json:"root_pixels_per_sample"`
	MaxSpriteHoldSeconds float64  `json:"max_sprite_hold_seconds"`
	MaxRootTravelPerHold float64  `json:"max_root_travel_per_sprite_hold"`
	CycleSeconds         float64  `json:"cycle_seconds"`
	LoopFrames           int      `json:"loop_frames"`
	DeclaredStrideRatio  *float64 `json:"declared_stride_ratio,omitempty"`
	EffectiveStrideRatio float64  `json:"effective_stride_ratio"`
	MovementSource       string   `json:"movement_source,omitempty"`
	MovementFallback     bool     `json:"movement_fallback"`
	LegacySpeedFallback  bool     `json:"legacy_speed_fallback"`
	Calibrated           bool     `json:"calibrated"`
	Status               string   `json:"status"`
	Warning              string   `json:"warning,omitempty"`
	AutonomousEligible   bool     `json:"eligible_for_autonomous_motion"`
	StartX               float64  `json:"start_x"`
	EndX                 float64  `json:"end_x_at_scene_boundary"`
	Top                  int      `json:"top"`
	BaselineY            int      `json:"baseline_y"`
}

type previewFramePosition struct {
	Name           string           `json:"name"`
	Lane           int              `json:"lane"`
	X              float64          `json:"x"`
	Y              int              `json:"y"`
	RenderedX      int              `json:"rendered_x"`
	RenderedY      int              `json:"rendered_y"`
	Phase          AnimationPhase   `json:"phase"`
	AnimationIndex int              `json:"animation_index"`
	ElapsedMS      float64          `json:"elapsed_ms"`
	SourceFrame    AnimationFrame   `json:"source_frame"`
	Anchor         AnimationAnchor  `json:"anchor"`
	Transform      *SpriteTransform `json:"transform,omitempty"`
}

type previewFramePositions struct {
	Frame        int                    `json:"frame"`
	Seconds      float64                `json:"seconds"`
	SceneSeconds float64                `json:"scene_seconds"`
	Action       string                 `json:"action"`
	Cats         []previewFramePosition `json:"cats"`
}

type previewMetadata struct {
	Version                        int                     `json:"version"`
	CanvasWidth                    int                     `json:"canvas_width"`
	CanvasHeight                   int                     `json:"canvas_height"`
	Background                     string                  `json:"background"`
	FPS                            int                     `json:"fps"`
	RequestedActionDurationSeconds float64                 `json:"requested_action_duration_seconds"`
	RequestedGazeDurationSeconds   float64                 `json:"requested_gaze_duration_seconds"`
	FramePattern                   string                  `json:"frame_pattern"`
	FirstFrame                     int                     `json:"first_frame"`
	FrameCount                     int                     `json:"frame_count"`
	DurationSeconds                float64                 `json:"duration_seconds"`
	Notes                          []string                `json:"notes"`
	Cats                           []previewCatMetadata    `json:"cats"`
	Scenes                         []previewScene          `json:"scenes"`
	GaitOnly                       bool                    `json:"gait_only,omitempty"`
	Capabilities                   []previewCapabilities   `json:"capabilities"`
	Frames                         []previewFramePositions `json:"frames,omitempty"`
}

func main() {
	if err := previewMain(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "motionpreview:", err)
		os.Exit(1)
	}
}

func previewMain(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("motionpreview", flag.ContinueOnError)
	fs.SetOutput(stderr)
	opts := previewOptions{}
	fs.StringVar(&opts.Root, "root", "", "private pack directory containing cats.json (required)")
	fs.StringVar(&opts.Out, "out", "", "new or empty output directory for PNG sequence and metadata.json (required)")
	fs.IntVar(&opts.FPS, "fps", 10, "output frames per second (1 to 120; gait-only defaults to 20)")
	fs.IntVar(&opts.Width, "width", 144, "cat canvas width in pixels at 100% display scaling (1 to 280)")
	fs.DurationVar(&opts.ActionDuration, "action-duration", 3*time.Second, "duration of each action, for example 3s")
	fs.DurationVar(&opts.GazeDuration, "gaze-duration", 4800*time.Millisecond, "duration of the complete 16-direction gaze sweep, for example 4.8s")
	fs.BoolVar(&opts.GaitOnly, "gait-only", false, "review translated right/left walks at app speed in three 900x220 ground-marked lanes")
	fs.StringVar(&opts.GaitActions, "gait-actions", "", "gait-only actions: comma-separated walk_right,walk_left,run_right,run_left; trial runs remain diagnostic only")
	fs.StringVar(&opts.Actions, "actions", "", "comma-separated action comparison; shows complete entry, at least one loop, then complete exit for every supplied cat")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: go run -tags motionpreview . --root PRIVATE_PACK --out EMPTY_OUTPUT [options]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if opts.GaitOnly {
		fpsSupplied := false
		fs.Visit(func(f *flag.Flag) { fpsSupplied = fpsSupplied || f.Name == "fps" })
		if !fpsSupplied {
			opts.FPS = 20 // Match the application's normal 50ms timer.
		}
	}
	meta, err := renderMotionPreview(opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Rendered %d frames at %d fps (%.3fs), %d supplied cats, %dx%d canvas\n", meta.FrameCount, meta.FPS, meta.DurationSeconds, len(meta.Cats), meta.CanvasWidth, meta.CanvasHeight)
	fmt.Fprintf(stdout, "PNG sequence: %s\nMetadata: %s\n", filepath.Join(opts.Out, previewFramePattern), filepath.Join(opts.Out, "metadata.json"))
	for _, scene := range meta.Scenes {
		for _, gait := range scene.Gait {
			if gait.Warning != "" {
				fmt.Fprintf(stderr, "Gait warning: %s / %s: %s\n", gait.Name, scene.Action, gait.Warning)
			}
		}
	}
	return nil
}

func validatePreviewOptions(opts previewOptions) error {
	if opts.Actions != "" {
		if opts.GaitOnly || opts.GaitActions != "" {
			return errors.New("--actions cannot be combined with gait mode")
		}
		names := strings.Split(opts.Actions, ",")
		if len(names) > 32 {
			return errors.New("--actions supports at most 32 scenes")
		}
		for _, name := range names {
			if len(name) == 0 || len(name) > 64 || strings.IndexFunc(name, func(r rune) bool { return r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9') }) >= 0 {
				return fmt.Errorf("invalid action name %q", name)
			}
		}
	}

	if opts.GaitActions != "" {
		if !opts.GaitOnly {
			return errors.New("--gait-actions requires --gait-only")
		}
		actions := strings.Split(opts.GaitActions, ",")
		if len(actions) > 4 {
			return errors.New("--gait-actions supports at most four actions")
		}
		for _, action := range actions {
			if action != "walk_left" && action != "walk_right" && action != "run_left" && action != "run_right" {
				return fmt.Errorf("unsupported gait action %q", action)
			}
		}
	}
	if opts.Root == "" || opts.Out == "" {
		return errors.New("--root and --out are required; no demo artwork is substituted")
	}
	if opts.FPS < 1 || opts.FPS > 120 {
		return errors.New("--fps must be 1 to 120")
	}
	if opts.Width < 1 || opts.Width > previewCanvasWidth/previewColumns-20 {
		return errors.New("--width must be 1 to 280 to fit the three-column canvas")
	}
	if opts.ActionDuration <= 0 || opts.ActionDuration > 60*time.Second {
		return errors.New("--action-duration must be greater than zero and at most 60s")
	}
	if opts.GazeDuration <= 0 || opts.GazeDuration > 60*time.Second {
		return errors.New("--gaze-duration must be greater than zero and at most 60s")
	}
	return nil
}

func loadPreviewCats(root string, width int) ([]previewCat, error) {
	// LoadConfig normally permits an absent cats.json as a demo default. This
	// tool explicitly requires the file and opens every pack file through an
	// os.Root, so relative paths and symlinks cannot escape the supplied pack.
	pack, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open private pack: %w", err)
	}
	defer pack.Close()
	f, err := pack.Open("cats.json")
	if err != nil {
		return nil, fmt.Errorf("private cats.json is required: %w", err)
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		return nil, fmt.Errorf("invalid cats.json: %w", err)
	}
	cats := make([]previewCat, 0, len(cfg.Cats))
	for i, spec := range cfg.Cats {
		if spec.Demo || spec.Sprite == "" || spec.Animations == "" {
			return nil, fmt.Errorf("cat %d (%q) requires a private sprite and animation manifest with demo=false; no demo artwork is substituted", i+1, spec.Name)
		}
		f, err := pack.Open(spec.Sprite)
		if err != nil {
			return nil, fmt.Errorf("cat %d (%q) sprite %q: %w", i+1, spec.Name, spec.Sprite, err)
		}
		atlas, decodeErr := DecodeSprite(f, false)
		closeErr := f.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("cat %d (%q) invalid sprite %q: %w", i+1, spec.Name, spec.Sprite, decodeErr)
		}
		if closeErr != nil {
			return nil, closeErr
		}
		f, err = pack.Open(spec.Animations)
		if err != nil {
			return nil, fmt.Errorf("cat %d (%q) animation manifest %q: %w", i+1, spec.Name, spec.Animations, err)
		}
		manifest, decodeErr := LoadAnimationManifest(f, atlas.Image.Bounds().Dx(), atlas.Image.Bounds().Dy())
		closeErr = f.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("cat %d (%q) invalid animation manifest %q: %w", i+1, spec.Name, spec.Animations, decodeErr)
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if err = ValidateManifestPixels(manifest, atlas); err != nil {
			return nil, fmt.Errorf("cat %d (%q) invalid animation pixels: %w", i+1, spec.Name, err)
		}
		cw, ch := ManifestCanvas(manifest, atlas)
		height := max(1, width*ch/cw)
		if height > previewCanvasHeight-40 {
			return nil, fmt.Errorf("cat %d (%q) is %dx%d at --width %d and does not fit the preview; reduce --width", i+1, spec.Name, width, height, width)
		}
		cats = append(cats, previewCat{Spec: spec, Atlas: atlas, Manifest: manifest, Player: NewAnimationPlayer(manifest), Width: width, Height: height, ExperimentalMovement: cfg.ExperimentalMovement})
	}
	return cats, nil
}

func previewResolvedSource(cat previewCat, action, mood string) previewActionSource {
	s := previewActionSource{Name: cat.Spec.Name, FallbackChain: []string{}}
	seen := map[string]bool{}
	for !seen[action] {
		seen[action] = true
		s.FallbackChain = append(s.FallbackChain, action)
		a, ok := cat.Manifest.Actions[action]
		if !ok {
			s.GlobalFallback = true
			action = cat.Manifest.Fallback
			continue
		}
		s.DemoFallback = s.DemoFallback || a.DemoFallback
		if clip, ok := a.Moods[mood]; ok && len(clip.Loop) > 0 {
			s.ResolvedAction = action
			return s
		}
		if len(a.Loop) > 0 {
			s.ResolvedAction = action
			return s
		}
		action = a.Fallback
		if action == "" {
			s.GlobalFallback = true
			action = cat.Manifest.Fallback
		}
	}
	return s // Validation prevents fallback cycles and missing terminal loops.
}

func previewSchedule(opts previewOptions, cats []previewCat) []previewScene {
	framesPerAction := max(1, int(math.Ceil(opts.ActionDuration.Seconds()*float64(opts.FPS))))
	scenes := []previewScene{}
	nextFrame := 0
	add := func(scene, action, mood string, count int) {
		s := previewScene{Scene: scene, Action: action, Mood: mood, StartFrame: nextFrame, EndFrame: nextFrame + count, StartSeconds: float64(nextFrame) / float64(opts.FPS), EndSeconds: float64(nextFrame+count) / float64(opts.FPS)}
		for _, cat := range cats {
			s.Sources = append(s.Sources, previewResolvedSource(cat, action, mood))
		}
		scenes = append(scenes, s)
		nextFrame += count
	}
	if opts.Actions != "" {
		for _, action := range strings.Split(opts.Actions, ",") {
			mood := "calm"
			if action == "pet" {
				mood = "happy"
			} else if action == "play" {
				mood = "playful"
			}
			active, exit := framesPerAction, 0
			for _, cat := range cats {
				clip := cat.Manifest.Resolve(action, mood)
				active = max(active, previewSequenceFrames(opts.FPS, clip.Start, clip.Loop))
				exit = max(exit, previewSequenceFrames(opts.FPS, clip.End))
			}
			start := nextFrame
			add(action, action, mood, active+exit+1)
			scenes[len(scenes)-1].StopFrame = start + active
		}
		return scenes
	}
	if opts.GaitOnly {
		actions := opts.GaitActions
		if actions == "" {
			actions = "walk_right,walk_left"
		}
		for _, action := range strings.Split(actions, ",") {
			add(action, action, "calm", framesPerAction)
		}
		return scenes
	}
	for _, action := range []string{"idle", "walk_right", "walk_left", "pet", "drag", "play", "sleep"} {
		mood := "calm"
		if action == "pet" {
			mood = "happy"
		} else if action == "play" {
			mood = "playful"
		}
		add(action, action, mood, framesPerAction)
	}
	// At low FPS or short durations, extend only this sweep so that no direction
	// is silently dropped. Metadata records the exact frame-aligned schedule.
	gazeFrames := max(16, int(math.Ceil(opts.GazeDuration.Seconds()*float64(opts.FPS))))
	for direction := 0; direction < 16; direction++ {
		count := (direction+1)*gazeFrames/16 - direction*gazeFrames/16
		add("gaze", fmt.Sprintf("gaze_%d", direction), "calm", count)
	}
	return scenes
}

func previewCatPosition(column, width, height int) image.Point {
	// Use a stable common bottom edge, just as the desktop window retains its
	// bounds while FrameRectBGRA registers individual source-frame anchors.
	return image.Pt(column*previewCanvasWidth/previewColumns+(previewCanvasWidth/previewColumns-width)/2, previewCanvasHeight-20-height)
}

func previewImage(cat *previewCat) *image.RGBA {
	return previewImageFrame(cat, cat.Player.Frame())
}

func previewImageFrame(cat *previewCat, frame AnimationFrame) *image.RGBA {
	anchor := cat.Manifest.Anchor
	if frame.Anchor != nil {
		anchor = *frame.Anchor
	}
	bgra := RenderBehaviorPixels(cat.Atlas, BehaviorFrame{AnimationFrame: frame, Anchor: anchor, Action: cat.Player.Action}, cat.Width, cat.Height)
	// image.RGBA, like the Windows DIB, stores premultiplied channels. Swap
	// B/R directly; unpremultiplying or using NRGBA would create alpha halos.
	im := image.NewRGBA(image.Rect(0, 0, cat.Width, cat.Height))
	for i := 0; i < len(bgra); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = bgra[i+2], bgra[i+1], bgra[i], bgra[i+3]
	}
	return im
}

func previewMovementSource(m *AnimationManifest, action string) (*AnimationMovement, string) {
	seen := map[string]bool{}
	for !seen[action] {
		seen[action] = true
		a, ok := m.Actions[action]
		if !ok {
			return nil, ""
		}
		if a.Movement != nil {
			return a.Movement, action
		}
		if a.Fallback == "" {
			return nil, ""
		}
		action = a.Fallback
	}
	return nil, ""
}

func previewLoopHasDistinctPixels(cat *previewCat, clip AnimationClip) bool {
	if len(clip.Loop) < 2 {
		return false
	}
	first := previewImageFrame(cat, clip.Loop[0])
	for _, frame := range clip.Loop[1:] {
		if !bytes.Equal(first.Pix, previewImageFrame(cat, frame).Pix) {
			return true
		}
	}
	return false
}

func previewGaitInfo(cat *previewCat, action, mood string, lane, frameCount, fps int) (previewGaitSource, error) {
	clip := cat.Manifest.Resolve(action, mood)
	info := previewGaitSource{Name: cat.Spec.Name, Lane: lane, PixelsPerSecond: WalkPixelsPerSecond(cat.Manifest, action, mood, cat.Width), LoopFrames: len(clip.Loop), BaselineY: (lane+1)*previewGaitLane - 24}
	experimental := cat.ExperimentalMovement && strings.HasPrefix(action, "run_") && !HasRunAnimation(cat.Manifest, action, mood) && HasRunArtwork(cat.Manifest, action, mood)
	if experimental {
		info.PixelsPerSecond = float64(cat.Width) * .23
	}
	info.Top = info.BaselineY - cat.Height
	if cat.Height > previewGaitLane-40 {
		return info, fmt.Errorf("cat %q is %dpx tall and does not fit a %dpx gait lane; reduce --width", cat.Spec.Name, cat.Height, previewGaitLane)
	}
	for _, frame := range clip.Loop {
		info.CycleSeconds += float64(frame.DurationMS) / 1000
		info.MaxSpriteHoldSeconds = math.Max(info.MaxSpriteHoldSeconds, float64(frame.DurationMS)/1000)
	}
	for _, frame := range clip.Start {
		info.MaxSpriteHoldSeconds = math.Max(info.MaxSpriteHoldSeconds, float64(frame.DurationMS)/1000)
	}
	if !finite(info.PixelsPerSecond) || info.PixelsPerSecond < 0 {
		return info, fmt.Errorf("cat %q has invalid application movement speed for %s", cat.Spec.Name, action)
	}
	movement, movementSource := previewMovementSource(cat.Manifest, action)
	info.MovementSource = movementSource
	info.MovementFallback = movement != nil && movementSource != action
	hasWalk := HasWalkAnimation(cat.Manifest, action, mood)
	info.AutonomousEligible = HasAuthoredAction(cat.Manifest, action, mood) && hasWalk
	if strings.HasPrefix(action, "run_") {
		info.AutonomousEligible = HasRunAnimation(cat.Manifest, action, mood) || experimental
	}
	info.LegacySpeedFallback = movement == nil && hasWalk
	info.EffectiveStrideRatio = info.PixelsPerSecond * info.CycleSeconds / float64(cat.Width)
	info.RootPixelsPerSample = info.PixelsPerSecond / float64(fps)
	info.MaxRootTravelPerHold = info.PixelsPerSecond * info.MaxSpriteHoldSeconds
	if movement != nil {
		ratio := movement.StrideRatio
		info.DeclaredStrideRatio = &ratio
		info.Status = "trial_stride"
		info.Calibrated = movement.Verified
		info.Warning = "Trial stride metadata; visual paw-plant calibration has not been verified"
		if movement.Verified {
			info.Status = "declared_verified_stride"
			info.Warning = "Stride verification is declared by the pack; this renderer does not independently certify anatomical gait"
		}
	} else {
		info.Status = "uncalibrated_legacy_speed"
		info.Warning = "No stride metadata; using the application's uncalibrated legacy movement speed"
	}
	if experimental {
		info.Calibrated = false
		info.Status = "experimental_stylized_motion"
		info.Warning = "Explicit experimental movement enabled: conservative trial speed, not calibrated; foot sliding may occur"
	}
	source := previewResolvedSource(*cat, action, mood)
	if !previewLoopHasDistinctPixels(cat, clip) {
		info.Status = "missing_walk_animation"
		info.Warning = "Walk loop has no distinct rendered poses"
		info.Calibrated = false
	} else if !hasWalk {
		info.Status = "missing_walk_animation"
		info.Warning = fmt.Sprintf("Walk resolves to %q without an eligible walk animation", source.ResolvedAction)
		info.Calibrated = false
	}
	if info.Status == "missing_walk_animation" {
		if info.PixelsPerSecond == 0 {
			info.Warning += "; the application suppresses root translation"
		} else {
			info.Warning += "; the application still translates these distinct source references, shown for diagnosis only"
		}
	}
	if strings.HasPrefix(action, "run_") && !info.AutonomousEligible {
		info.Warning += "; this run is diagnostic only and is not eligible for autonomous application movement"
	}
	info.StartX = previewGaitMargin
	direction := 1.0
	if strings.HasSuffix(action, "_left") {
		info.StartX = float64(previewCanvasWidth - previewGaitMargin - cat.Width)
		direction = -1
	}
	info.EndX = info.StartX + direction*info.PixelsPerSecond*float64(frameCount)/float64(fps)
	if info.EndX < previewGaitMargin || info.EndX+float64(cat.Width) > previewCanvasWidth-previewGaitMargin {
		return info, fmt.Errorf("cat %q %s travels %.2fpx at %.2fpx/s and cannot fit the gait lane without clipping; reduce --action-duration or --width", cat.Spec.Name, action, math.Abs(info.EndX-info.StartX), info.PixelsPerSecond)
	}
	return info, nil
}

func previewDrawGround(canvas *image.RGBA, cats int) {
	ground := image.NewUniform(color.RGBA{R: 169, G: 159, B: 145, A: 255})
	separator := image.NewUniform(color.RGBA{R: 222, G: 214, B: 204, A: 255})
	for lane := 0; lane < cats; lane++ {
		y := (lane+1)*previewGaitLane - 24
		draw.Draw(canvas, image.Rect(20, y, previewCanvasWidth-20, y+1), ground, image.Point{}, draw.Src)
		for x := previewGaitMargin; x <= previewCanvasWidth-previewGaitMargin; x += 20 {
			draw.Draw(canvas, image.Rect(x, y, x+1, y+7), ground, image.Point{}, draw.Src)
		}
		if lane > 0 {
			draw.Draw(canvas, image.Rect(20, lane*previewGaitLane, previewCanvasWidth-20, lane*previewGaitLane+1), separator, image.Point{}, draw.Src)
		}
	}
}

func renderMotionPreview(opts previewOptions) (*previewMetadata, error) {
	if err := validatePreviewOptions(opts); err != nil {
		return nil, err
	}
	cats, err := loadPreviewCats(opts.Root, opts.Width)
	if err != nil {
		return nil, err
	}
	meta := &previewMetadata{Version: 1, CanvasWidth: previewCanvasWidth, CanvasHeight: previewCanvasHeight, Background: "#f4efe7", FPS: opts.FPS, RequestedActionDurationSeconds: opts.ActionDuration.Seconds(), RequestedGazeDurationSeconds: opts.GazeDuration.Seconds(), FramePattern: previewFramePattern, FirstFrame: 0,
		Notes: []string{"All artwork is read from the supplied private pack; no generated or demo artwork is substituted.", "Cats are rendered in place at the requested width using the application renderer; this is an animation preview, not a native Windows interaction recording.", "Scene end frames are exclusive. The gaze sweep has at least 16 frames so all directions appear.", "Each action starts at its entry phase. Declared entry frames and loops are shown; forced interruptions start the next scene without playing exit phases."}}
	if opts.GaitOnly {
		meta.GaitOnly = true
		meta.CanvasHeight = previewGaitHeight
		meta.Notes = []string{"All artwork is read from the supplied private pack; baseline and tick marks are diagnostic UI guides only.", "Cats translate using the application stride calculation, without speed adjustment, clipping, clamping, or wrapping. Trial runs are diagnostic only: eligible_for_autonomous_motion reports the stricter runtime scheduling gate. This is a deterministic gait review, not a native Windows recording.", "A declared stride ratio is a pack calibration claim, not proof of correct gait. Missing or static walk artwork is never marked calibrated. Legacy movement without stride metadata is uncalibrated.", "Source frames, effective anchors, animation cursors, exact positions and integer drawn positions are recorded for every sample. Ground ticks are 20px apart.", "Root movement continues while sprite poses are held, which can cause within-hold sawtooth foot drift. max_root_travel_per_sprite_hold and root_pixels_per_sample expose that sampling effect separately; perfect foot planting is not asserted.", "Gait-only CLI defaults to 20fps, matching the application's normal 50ms timer. An explicit --fps overrides the sampling rate without changing movement speed.", "Right and left scenes restart their entry phase from opposite lane ends. Scene end frames are exclusive; end_x_at_scene_boundary includes the unsampled final frame interval."}
	}
	if opts.Actions != "" {
		meta.Notes = append(meta.Notes[:3], "Custom action comparisons finish entry and at least one loop, then play all exit frames. Shorter exits hold their final recovery pose until the next scene. Runtime capability gates are reported separately from fallback rendering.")
	}
	for _, cat := range cats {
		meta.Capabilities = append(meta.Capabilities, previewCapabilityReport(cat))
	}
	meta.Scenes = previewSchedule(opts, cats)
	if opts.GaitOnly {
		for i := range meta.Scenes {
			scene := &meta.Scenes[i]
			for lane := range cats {
				gait, err := previewGaitInfo(&cats[lane], scene.Action, scene.Mood, lane, scene.EndFrame-scene.StartFrame, opts.FPS)
				if err != nil {
					return nil, err
				}
				scene.Gait = append(scene.Gait, gait)
			}
		}
	}
	for column, cat := range cats {
		pos := previewCatPosition(column, cat.Width, cat.Height)
		if opts.GaitOnly {
			gait := meta.Scenes[0].Gait[column]
			pos = image.Pt(int(math.Round(gait.StartX)), gait.Top)
		}
		meta.Cats = append(meta.Cats, previewCatMetadata{Name: cat.Spec.Name, Sprite: cat.Spec.Sprite, Animations: cat.Spec.Animations, Column: column, Width: cat.Width, Height: cat.Height, Left: pos.X, Top: pos.Y})
	}
	meta.FrameCount = meta.Scenes[len(meta.Scenes)-1].EndFrame
	meta.DurationSeconds = float64(meta.FrameCount) / float64(opts.FPS)
	entries, err := os.ReadDir(opts.Out)
	if err == nil && len(entries) != 0 {
		return nil, errors.New("--out must be new or empty; existing files will not be overwritten")
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read output directory: %w", err)
	}
	if err := os.MkdirAll(opts.Out, 0700); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	for _, scene := range meta.Scenes {
		for i := range cats {
			cats[i].Player.Play(scene.Action, scene.Mood)
		}
		for frame := scene.StartFrame; frame < scene.EndFrame; frame++ {
			canvas := image.NewRGBA(image.Rect(0, 0, meta.CanvasWidth, meta.CanvasHeight))
			draw.Draw(canvas, canvas.Bounds(), image.NewUniform(previewBackground), image.Point{}, draw.Src)
			positions := previewFramePositions{Frame: frame, Seconds: float64(frame) / float64(opts.FPS), SceneSeconds: float64(frame-scene.StartFrame) / float64(opts.FPS), Action: scene.Action}
			if opts.GaitOnly {
				previewDrawGround(canvas, len(cats))
			}
			for column := range cats {
				cat := &cats[column]
				if opts.Actions != "" && frame == scene.StopFrame {
					cat.Player.Stop()
				}
				im := previewImage(cat)
				pos := previewCatPosition(column, cat.Width, cat.Height)
				if opts.GaitOnly {
					gait := scene.Gait[column]
					direction := 1.0
					if strings.HasSuffix(scene.Action, "_left") {
						direction = -1
					}
					x := gait.StartX + direction*gait.PixelsPerSecond*positions.SceneSeconds
					pos = image.Pt(int(math.Round(x)), gait.Top)
				}
				if opts.GaitOnly || opts.Actions != "" {
					sourceFrame := cat.Player.Frame()
					anchor := cat.Manifest.Anchor
					if sourceFrame.Anchor != nil {
						anchor = *sourceFrame.Anchor
					}
					var transform *SpriteTransform
					if sourceFrame.Rect != nil {
						t := FrameRectTransform(*sourceFrame.Rect, anchor, cat.Width, cat.Height, scene.Action != "drag")
						transform = &t
					}
					rootX := float64(pos.X)
					if opts.GaitOnly {
						direction := 1.0
						if strings.HasSuffix(scene.Action, "_left") {
							direction = -1
						}
						gait := scene.Gait[column]
						rootX = gait.StartX + direction*gait.PixelsPerSecond*positions.SceneSeconds
					}
					positions.Cats = append(positions.Cats, previewFramePosition{Name: cat.Spec.Name, Lane: column, X: rootX, Y: pos.Y, RenderedX: pos.X, RenderedY: pos.Y, Phase: cat.Player.Phase, AnimationIndex: cat.Player.Index, ElapsedMS: cat.Player.ElapsedMS, SourceFrame: sourceFrame, Anchor: anchor, Transform: transform})
				}
				draw.Draw(canvas, im.Bounds().Add(pos), im, im.Bounds().Min, draw.Over)
				cat.Player.Tick(1 / float64(opts.FPS))
			}
			if opts.GaitOnly || opts.Actions != "" {
				meta.Frames = append(meta.Frames, positions)
			}
			path := filepath.Join(opts.Out, fmt.Sprintf(previewFramePattern, frame))
			if err := writePreviewPNG(path, canvas); err != nil {
				return nil, fmt.Errorf("write frame %d: %w", frame, err)
			}
		}
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	path := filepath.Join(opts.Out, "metadata.json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("create metadata: %w", err)
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return nil, fmt.Errorf("write metadata: %w", writeErr)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return meta, nil
}

func writePreviewPNG(path string, im image.Image) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(f, im)
	closeErr := f.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}
