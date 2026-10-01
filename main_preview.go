//go:build !windows && motionpreview

package main

// This optional headless tool renders only pixels from an explicitly supplied
// private pack. It is not part of the Windows executable and has no embedded or
// generated artwork fallback.

import (
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
	"time"
)

const (
	previewCanvasWidth  = 900
	previewCanvasHeight = 360
	previewColumns      = 3
	previewFramePattern = "frame_%06d.png"
)

var previewBackground = color.RGBA{R: 244, G: 239, B: 231, A: 255}

type previewOptions struct {
	Root           string
	Out            string
	FPS            int
	Width          int
	ActionDuration time.Duration
	GazeDuration   time.Duration
}

type previewCat struct {
	Spec     CatSpec
	Atlas    *Atlas
	Manifest *AnimationManifest
	Player   *AnimationPlayer
	Width    int
	Height   int
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
}

type previewMetadata struct {
	Version                        int                  `json:"version"`
	CanvasWidth                    int                  `json:"canvas_width"`
	CanvasHeight                   int                  `json:"canvas_height"`
	Background                     string               `json:"background"`
	FPS                            int                  `json:"fps"`
	RequestedActionDurationSeconds float64              `json:"requested_action_duration_seconds"`
	RequestedGazeDurationSeconds   float64              `json:"requested_gaze_duration_seconds"`
	FramePattern                   string               `json:"frame_pattern"`
	FirstFrame                     int                  `json:"first_frame"`
	FrameCount                     int                  `json:"frame_count"`
	DurationSeconds                float64              `json:"duration_seconds"`
	Notes                          []string             `json:"notes"`
	Cats                           []previewCatMetadata `json:"cats"`
	Scenes                         []previewScene       `json:"scenes"`
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
	fs.IntVar(&opts.FPS, "fps", 10, "output frames per second (1 to 120)")
	fs.IntVar(&opts.Width, "width", 144, "cat canvas width in pixels at 100% display scaling (1 to 280)")
	fs.DurationVar(&opts.ActionDuration, "action-duration", 3*time.Second, "duration of each action, for example 3s")
	fs.DurationVar(&opts.GazeDuration, "gaze-duration", 4800*time.Millisecond, "duration of the complete 16-direction gaze sweep, for example 4.8s")
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
	meta, err := renderMotionPreview(opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Rendered %d frames at %d fps (%.3fs), %d supplied cats, %dx%d canvas\n", meta.FrameCount, meta.FPS, meta.DurationSeconds, len(meta.Cats), meta.CanvasWidth, meta.CanvasHeight)
	fmt.Fprintf(stdout, "PNG sequence: %s\nMetadata: %s\n", filepath.Join(opts.Out, previewFramePattern), filepath.Join(opts.Out, "metadata.json"))
	return nil
}

func validatePreviewOptions(opts previewOptions) error {
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
		cats = append(cats, previewCat{Spec: spec, Atlas: atlas, Manifest: manifest, Player: NewAnimationPlayer(manifest), Width: width, Height: height})
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
	frame := cat.Player.Frame()
	anchor := cat.Manifest.Anchor
	if frame.Anchor != nil {
		anchor = *frame.Anchor
	}
	var bgra []byte
	if frame.Rect != nil {
		bgra = cat.Atlas.FrameRectBGRA(*frame.Rect, anchor, cat.Width, cat.Height)
	} else {
		bgra = cat.Atlas.FrameBGRA(frame.Row, frame.Col, cat.Width, cat.Height)
	}
	// image.RGBA, like the Windows DIB, stores premultiplied channels. Swap
	// B/R directly; unpremultiplying or using NRGBA would create alpha halos.
	im := image.NewRGBA(image.Rect(0, 0, cat.Width, cat.Height))
	for i := 0; i < len(bgra); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = bgra[i+2], bgra[i+1], bgra[i], bgra[i+3]
	}
	return im
}

func renderMotionPreview(opts previewOptions) (*previewMetadata, error) {
	if err := validatePreviewOptions(opts); err != nil {
		return nil, err
	}
	cats, err := loadPreviewCats(opts.Root, opts.Width)
	if err != nil {
		return nil, err
	}
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
	meta := &previewMetadata{Version: 1, CanvasWidth: previewCanvasWidth, CanvasHeight: previewCanvasHeight, Background: "#f4efe7", FPS: opts.FPS, RequestedActionDurationSeconds: opts.ActionDuration.Seconds(), RequestedGazeDurationSeconds: opts.GazeDuration.Seconds(), FramePattern: previewFramePattern, FirstFrame: 0,
		Notes: []string{"All artwork is read from the supplied private pack; no generated or demo artwork is substituted.", "Cats are rendered in place at the requested width using the application renderer; this is an animation preview, not a native Windows interaction recording.", "Scene end frames are exclusive. The gaze sweep has at least 16 frames so all directions appear.", "Each action starts at its entry phase. Declared entry frames and loops are shown; forced interruptions start the next scene without playing exit phases."}}
	for column, cat := range cats {
		pos := previewCatPosition(column, cat.Width, cat.Height)
		meta.Cats = append(meta.Cats, previewCatMetadata{Name: cat.Spec.Name, Sprite: cat.Spec.Sprite, Animations: cat.Spec.Animations, Column: column, Width: cat.Width, Height: cat.Height, Left: pos.X, Top: pos.Y})
	}
	meta.Scenes = previewSchedule(opts, cats)
	meta.FrameCount = meta.Scenes[len(meta.Scenes)-1].EndFrame
	meta.DurationSeconds = float64(meta.FrameCount) / float64(opts.FPS)
	for _, scene := range meta.Scenes {
		for i := range cats {
			cats[i].Player.Play(scene.Action, scene.Mood)
		}
		for frame := scene.StartFrame; frame < scene.EndFrame; frame++ {
			canvas := image.NewRGBA(image.Rect(0, 0, previewCanvasWidth, previewCanvasHeight))
			draw.Draw(canvas, canvas.Bounds(), image.NewUniform(previewBackground), image.Point{}, draw.Src)
			for column := range cats {
				cat := &cats[column]
				im := previewImage(cat)
				pos := previewCatPosition(column, cat.Width, cat.Height)
				draw.Draw(canvas, im.Bounds().Add(pos), im, im.Bounds().Min, draw.Over)
				cat.Player.Tick(1 / float64(opts.FPS))
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
