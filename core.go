package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

const appVersion = "0.5.0-preview"

var rowFrames = [11]int{6, 8, 8, 4, 5, 8, 6, 6, 5, 8, 8}

type Rect struct{ Left, Top, Right, Bottom int }

func (r Rect) Width() int  { return r.Right - r.Left }
func (r Rect) Height() int { return r.Bottom - r.Top }
func clamp(v, lo, hi float64) float64 {
	if hi < lo {
		return lo
	}
	return math.Max(lo, math.Min(hi, v))
}
func ClampPosition(x, y float64, w, h int, r Rect) (float64, float64) {
	return clamp(x, float64(r.Left), float64(r.Right-w)), clamp(y, float64(r.Top), float64(r.Bottom-h))
}

// FitSize preserves the sprite aspect and guarantees the window fits the work area.
func FitSize(w, h int, r Rect) (int, int) {
	w = max(1, w)
	h = max(1, h)
	ratio := math.Min(1, math.Min(float64(max(1, r.Width()))/float64(w), float64(max(1, r.Height()))/float64(h)))
	return max(1, int(float64(w)*ratio)), max(1, int(float64(h)*ratio))
}
func GazeDirection(dx, dy float64) int { return GazeDirectionCount(dx, dy, 16) }
func GazeDirectionCount(dx, dy float64, count int) int {
	if count != 4 && count != 8 && count != 16 && count != 32 {
		count = 16
	}
	a := math.Atan2(dx, -dy)
	if a < 0 {
		a += 2 * math.Pi
	}
	return int(math.Round(a/(2*math.Pi/float64(count)))) % count
}

type CatSpec struct {
	Name        string       `json:"name"`
	Sprite      string       `json:"sprite"`
	Animations  string       `json:"animations,omitempty"`
	Demo        bool         `json:"demo"`
	Temperament *Temperament `json:"temperament,omitempty"`
}
type Config struct {
	Version              int       `json:"version"`
	Cats                 []CatSpec `json:"cats"`
	ExperimentalMovement bool      `json:"experimentalMovement,omitempty"`
	DefaultSize          int       `json:"defaultSize,omitempty"`
}

func DefaultConfig() Config {
	return Config{Version: 1, Cats: []CatSpec{{Name: "Demo cat 1", Demo: true}, {Name: "Demo cat 2", Demo: true}, {Name: "Demo cat 3", Demo: true}}}
}
func LoadConfig(dir string) (Config, error) {
	c := DefaultConfig()
	data, e := readBoundedFile(filepath.Join(dir, "cats.json"), 64*1024)
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if len(data) > 64*1024 {
		return c, errors.New("cats.json is too large")
	}
	var custom Config
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if e = d.Decode(&custom); e != nil {
		return c, e
	}
	if d.Decode(new(any)) != io.EOF {
		return c, errors.New("cats.json must contain one JSON object")
	}
	if custom.Version != 1 || len(custom.Cats) < 1 || len(custom.Cats) > 3 {
		return c, errors.New("cats.json needs version 1 and 1 to 3 cats")
	}
	for i := range custom.Cats {
		s := &custom.Cats[i]
		if s.Temperament != nil {
			if err := s.Temperament.Validate(); err != nil {
				return c, fmt.Errorf("cat %d: %w", i+1, err)
			}
		}
		s.Name = strings.TrimSpace(s.Name)
		if s.Name == "" || strings.ContainsRune(s.Name, 0) || len([]rune(s.Name)) > 40 {
			return c, fmt.Errorf("cat %d needs a name of 1 to 40 characters", i+1)
		}
		if s.Sprite != "" {
			p := filepath.Clean(s.Sprite)
			if !filepath.IsLocal(p) || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) || strings.Contains(p, ":") {
				return c, fmt.Errorf("cat %d sprite must be a relative path inside the app folder", i+1)
			}
			s.Sprite = p
		} else {
			s.Demo = true
		}
		if s.Animations != "" {
			p := filepath.Clean(s.Animations)
			if !filepath.IsLocal(p) || strings.Contains(p, ":") {
				return c, fmt.Errorf("cat %d animation manifest must be a local relative path", i+1)
			}
			s.Animations = p
		}
	}
	return custom, nil
}

type Settings struct {
	Quiet                bool          `json:"quiet"`
	Size                 int           `json:"size"`
	Activity             ActivityLevel `json:"activity,omitempty"`
	CPU                  CPUSettings   `json:"cpu"`
	ExperimentalMovement bool          `json:"experimentalMovement"`
}

func ValidSize(s int) int {
	if s == 96 || s == 144 || s == 192 {
		return s
	}
	return 144
}
func SaveSettings(path string, s Settings) error {
	s = NormalizeSettings(s)
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	tmp := path + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
func LoadSettings(path string, experimentalDefault ...bool) Settings {
	return loadSettingsAtSize(path, 144, experimentalDefault...)
}
func loadSettingsAtSize(path string, size int, experimentalDefault ...bool) Settings {
	s := Settings{Size: ValidSize(size), CPU: DefaultCPUSettings(), ExperimentalMovement: len(experimentalDefault) > 0 && experimentalDefault[0]}
	if b, e := readBoundedFile(path, 4096); e == nil {
		_ = json.Unmarshal(b, &s)
	}
	return NormalizeSettings(s)
}

type Atlas struct {
	Image        *image.NRGBA
	CellW, CellH int
}

func DecodeAtlas(r io.Reader) (*Atlas, error) { return DecodeSprite(r, true) }
func DecodeSprite(r io.Reader, legacy bool) (*Atlas, error) {
	b, e := io.ReadAll(io.LimitReader(r, 24*1024*1024+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 24*1024*1024 {
		return nil, errors.New("sprite PNG exceeds 24 MB")
	}
	c, e := png.DecodeConfig(strings.NewReader(string(b)))
	if e != nil {
		return nil, e
	}
	if c.Width < 1 || c.Height < 1 || c.Width > 4096 || c.Height > 5632 || c.Width*c.Height > 8_000_000 || (legacy && (c.Width%8 != 0 || c.Height%11 != 0 || c.Width < 64 || c.Height < 88)) {
		return nil, errors.New("sprite must be an 8-column, 11-row PNG atlas, at most 4096 x 5632")
	}
	im, e := png.Decode(strings.NewReader(string(b)))
	if e != nil {
		return nil, e
	}
	dst := image.NewNRGBA(im.Bounds())
	draw.Draw(dst, dst.Bounds(), im, im.Bounds().Min, draw.Src)
	transparent := false
	for i := 3; i < len(dst.Pix); i += 4 {
		if dst.Pix[i] == 0 {
			transparent = true
			break
		}
	}
	if !transparent {
		return nil, errors.New("sprite needs real transparent pixels")
	}
	if !legacy {
		return &Atlas{dst, max(1, c.Width/8), max(1, c.Height/11)}, nil
	}
	a := &Atlas{dst, c.Width / 8, c.Height / 11}
	if a.CellW < a.CellH/3 || a.CellH < a.CellW/3 {
		return nil, errors.New("sprite cell aspect ratio must stay between 1:3 and 3:1")
	}
	for row, n := range rowFrames {
		for col := 0; col < n; col++ {
			opaque := 0
			for y := row * a.CellH; y < (row+1)*a.CellH; y++ {
				for x := col * a.CellW; x < (col+1)*a.CellW; x++ {
					if dst.NRGBAAt(x, y).A > 24 {
						opaque++
					}
				}
			}
			if opaque < 16 {
				return nil, fmt.Errorf("sprite row %d frame %d is empty", row, col)
			}
		}
	}
	return a, nil
}

// FrameBGRA produces a top-down, premultiplied Windows DIB. Pixels below the
// hit-test threshold become fully transparent, including invisible RGB data.
func (a *Atlas) FrameBGRA(row, col, w, h int) []byte {
	row = max(0, min(10, row))
	col = max(0, min(rowFrames[row]-1, col))
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		sy := row*a.CellH + min(a.CellH-1, y*a.CellH/h)
		for x := 0; x < w; x++ {
			sx := col*a.CellW + min(a.CellW-1, x*a.CellW/w)
			c := a.Image.NRGBAAt(sx, sy)
			i := (y*w + x) * 4
			if c.A < 24 {
				continue
			}
			aa := uint32(c.A)
			out[i] = byte(uint32(c.B) * aa / 255)
			out[i+1] = byte(uint32(c.G) * aa / 255)
			out[i+2] = byte(uint32(c.R) * aa / 255)
			out[i+3] = c.A
		}
	}
	return out
}

type Cat struct {
	X, Y                                         float64
	Mode                                         string
	ModeUntil, NextDecision, PetUntil, GazeUntil float64
	Direction                                    float64
	Phase                                        float64
	W, H                                         int
	Bounds                                       Rect
	Dragging                                     bool
	Seed                                         *rand.Rand
}

func NewCat(i, w, h int, r Rect) *Cat {
	c := &Cat{W: w, H: h, Bounds: r, Mode: "idle", NextDecision: 8 + float64(i)*3, Seed: rand.New(rand.NewSource(int64(701 + i*97))), Phase: float64(i) * .17}
	c.X, c.Y = ClampPosition(float64(r.Left+40+i*(w+20)), float64(r.Bottom-h), w, h, r)
	return c
}
func (c *Cat) Pet(now float64) {
	c.PetUntil = now + 2
	c.GazeUntil = now + 4
	c.Mode = "idle"
	c.NextDecision = now + 8 + c.Seed.Float64()*10
}
func (c *Cat) Tick(now, dt, cursorX, cursorY, idleSeconds float64, quiet bool) (int, int) {
	if c.Dragging {
		return 3, 1
	}
	if now < c.PetUntil {
		return 3, int((now+c.Phase)*5) % 4
	}
	if quiet {
		c.Mode = "sleep"
		return 0, 3
	}
	if idleSeconds >= 180 {
		c.Mode = "sleep"
		return 0, 3
	}
	if c.Mode == "sleep" {
		c.Mode = "idle"
		c.NextDecision = now + 5
	}
	dx, dy := cursorX-(c.X+float64(c.W)/2), cursorY-(c.Y+float64(c.H)/2)
	near := dx*dx+dy*dy < 420*420
	if c.Mode == "walk" {
		if now >= c.ModeUntil || near {
			c.Mode = "idle"
			c.NextDecision = now + 10 + c.Seed.Float64()*20
		} else {
			speed := float64(c.W) * .23
			c.X += c.Direction * speed * dt
			c.X, c.Y = ClampPosition(c.X, c.Y, c.W, c.H, c.Bounds)
			if c.X <= float64(c.Bounds.Left) || c.X >= float64(c.Bounds.Right-c.W) {
				c.Direction *= -1
			}
			row := 1
			if c.Direction < 0 {
				row = 2
			}
			return row, int((now+c.Phase)*10) % 8
		}
	}
	if near || now < c.GazeUntil {
		if math.Abs(dx)+math.Abs(dy) > 15 {
			d := GazeDirection(dx, dy)
			return 9 + d/8, d % 8
		}
	}
	if now >= c.NextDecision {
		c.Mode = "walk"
		c.ModeUntil = now + 2 + c.Seed.Float64()*3
		c.Direction = 1
		if c.Seed.Intn(2) == 0 {
			c.Direction = -1
		}
		c.NextDecision = now + 15
		return 0, 0
	}
	return 0, int((now+c.Phase)*4) % 6
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e == nil && int64(len(b)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return b, e
}

// FrameRectBGRA scales and registers a manifest rectangle around its normalized
// anchor. Its aspect is preserved, and transparent margins stay click-through.
type SpriteTransform struct {
	ScaledW int `json:"scaled_width"`
	ScaledH int `json:"scaled_height"`
	OffsetX int `json:"offset_x"`
	OffsetY int `json:"offset_y"`
}

func FrameRectTransform(r FrameRect, anchor AnimationAnchor, w, h int, grounded bool) SpriteTransform {
	scale := math.Min(float64(w)/float64(r.W), float64(h)/float64(r.H))
	sw, sh := max(1, int(float64(r.W)*scale)), max(1, int(float64(r.H)*scale))
	ox, oy := int(float64(w-sw)*anchor.X), int(float64(h-sh)*anchor.Y)
	if grounded {
		ox = int(math.Round(float64(w)/2 - float64(sw)*anchor.X))
		oy = int(math.Round(float64(h) - float64(sh)*anchor.Y))
	}
	return SpriteTransform{sw, sh, ox, oy}
}
func (a *Atlas) FrameRectBGRA(r FrameRect, anchor AnimationAnchor, w, h int) []byte {
	return a.frameRectTransformed(r, w, h, FrameRectTransform(r, anchor, w, h, false))
}
func (a *Atlas) frameRectTransformed(r FrameRect, w, h int, t SpriteTransform) []byte {
	return a.frameRectCanvasTransformed(r, nil, w, h, t)
}
func FrameLogicalRect(f AnimationFrame) FrameRect {
	if f.Canvas != nil {
		return FrameRect{W: f.Canvas.W, H: f.Canvas.H}
	}
	if f.Rect != nil {
		return *f.Rect
	}
	return FrameRect{W: 1, H: 1}
}
func AnimationFrameTransform(f AnimationFrame, anchor AnimationAnchor, w, h int, grounded bool) SpriteTransform {
	return FrameRectTransform(FrameLogicalRect(f), anchor, w, h, grounded)
}
func (a *Atlas) frameRectCanvasTransformed(r FrameRect, canvas *FrameCanvas, w, h int, t SpriteTransform) []byte {
	logicalW, logicalH, cropX, cropY := r.W, r.H, 0, 0
	if canvas != nil {
		logicalW, logicalH, cropX, cropY = canvas.W, canvas.H, canvas.X, canvas.Y
	}

	out := make([]byte, w*h*4)
	for y := 0; y < t.ScaledH; y++ {
		dy := t.OffsetY + y
		if dy < 0 || dy >= h {
			continue
		}
		sy := y*logicalH/t.ScaledH - cropY
		if sy < 0 || sy >= r.H {
			continue
		}
		for x := 0; x < t.ScaledW; x++ {
			dx := t.OffsetX + x
			if dx < 0 || dx >= w {
				continue
			}
			sx := x*logicalW/t.ScaledW - cropX
			if sx < 0 || sx >= r.W {
				continue
			}
			c := a.Image.NRGBAAt(r.X+sx, r.Y+sy)
			if c.A < 24 {
				continue
			}
			i := (dy*w + dx) * 4
			alpha := uint32(c.A)
			out[i] = byte(uint32(c.B) * alpha / 255)
			out[i+1] = byte(uint32(c.G) * alpha / 255)
			out[i+2] = byte(uint32(c.R) * alpha / 255)
			out[i+3] = c.A
		}
	}
	return out
}

// Grounded actions place the source reference floor on the window's bottom.
// Dragging is airborne and preserves the full canvas, including dangling tails.
func RenderBehaviorPixels(a *Atlas, f BehaviorFrame, w, h int) []byte {
	if f.Rect == nil {
		return a.FrameBGRA(f.Row, f.Col, w, h)
	}
	return a.frameRectCanvasTransformed(*f.Rect, f.Canvas, w, h, AnimationFrameTransform(f.AnimationFrame, f.Anchor, w, h, f.Action != "drag"))
}

func ManifestCanvas(m *AnimationManifest, a *Atlas) (int, int) {
	w, h := 0, 0
	for _, action := range m.Actions {
		clips := []AnimationClip{action.AnimationClip}
		for _, clip := range action.Moods {
			clips = append(clips, clip)
		}
		for _, clip := range clips {
			for _, seq := range [][]AnimationFrame{clip.Start, clip.Loop, clip.End} {
				for _, f := range seq {
					if f.Rect != nil {
						logical := FrameLogicalRect(f)
						w = max(w, logical.W)
						h = max(h, logical.H)
					} else {
						w = max(w, a.CellW)
						h = max(h, a.CellH)
					}
				}
			}
		}
	}
	return max(1, w), max(1, h)
}
