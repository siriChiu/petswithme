package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestGazeDirections(t *testing.T) {
	for _, c := range []struct {
		x, y float64
		want int
	}{{0, -100, 0}, {100, -100, 2}, {100, 0, 4}, {100, 100, 6}, {0, 100, 8}, {-100, 100, 10}, {-100, 0, 12}, {-100, -100, 14}} {
		if got := GazeDirection(c.x, c.y); got != c.want {
			t.Fatalf("gaze %v: got %d want %d", c, got, c.want)
		}
	}
	for i := 0; i < 16; i++ {
		a := float64(i) * math.Pi / 8
		if d := GazeDirection(math.Sin(a)*100, -math.Cos(a)*100); d != i {
			t.Fatalf("direction %d got %d", i, d)
		}
	}
}
func TestClampNegativeMonitor(t *testing.T) {
	r := Rect{-1920, -200, 0, 880}
	x, y := ClampPosition(-9999, 9999, 144, 156, r)
	if x != -1920 || y != 724 {
		t.Fatalf("%v %v", x, y)
	}
	x, y = ClampPosition(0, 0, 500, 500, Rect{-20, 30, 100, 100})
	if x != -20 || y != 30 {
		t.Fatalf("oversized clamp %v %v", x, y)
	}
}
func TestConfigValidation(t *testing.T) {
	d := t.TempDir()
	c, e := LoadConfig(d)
	if e != nil || len(c.Cats) != 3 || !c.Cats[0].Demo {
		t.Fatal(c, e)
	}
	for _, s := range []string{`{"version":2,"cats":[]}`, `{"version":1,"cats":[{"name":"x","sprite":"../bad.png"}]}`, `{"version":1,"cats":[{"name":"x","sprite":"C:\\secret.png"}]}`, `{"version":1,"cats":[{"name":"x","oops":1}]}`, `{"version":1,"cats":[{"name":"x"}]} {}`} {
		os.WriteFile(filepath.Join(d, "cats.json"), []byte(s), 0600)
		if _, e := LoadConfig(d); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
func TestSettingsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "settings.json")
	for _, v := range []Settings{{true, 192}, {false, 96}, {true, 777}} {
		if e := SaveSettings(p, v); e != nil {
			t.Fatal(e)
		}
		got := LoadSettings(p)
		if got.Quiet != v.Quiet || got.Size != ValidSize(v.Size) {
			t.Fatal(got)
		}
	}
	os.WriteFile(p, []byte("broken"), 0600)
	if got := LoadSettings(p); got.Size != 144 {
		t.Fatal(got)
	}
}
func TestBundledAtlasAndPremultiplication(t *testing.T) {
	f, e := os.Open("assets/spritesheet-extended.png")
	if os.IsNotExist(e) {
		t.Skip("private demo art is intentionally absent from public source; synthetic atlas tests still run")
	}
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	a, e := DecodeAtlas(f)
	if e != nil {
		t.Fatal(e)
	}
	if a.CellW != 192 || a.CellH != 208 {
		t.Fatal(a.CellW, a.CellH)
	}
	for row, n := range rowFrames {
		for col := 0; col < n; col++ {
			p := a.FrameBGRA(row, col, 96, 104)
			opaque, transparent := 0, 0
			for i := 0; i < len(p); i += 4 {
				a := p[i+3]
				if a == 0 {
					transparent++
					if p[i] != 0 || p[i+1] != 0 || p[i+2] != 0 {
						t.Fatal("nonzero transparent color")
					}
				} else {
					opaque++
				}
				if p[i] > a || p[i+1] > a || p[i+2] > a {
					t.Fatal("not premultiplied")
				}
			}
			if opaque == 0 || transparent == 0 {
				t.Fatalf("row %d col %d missing alpha geometry", row, col)
			}
		}
	}
}
func TestRejectInvalidAtlas(t *testing.T) {
	for _, im := range []*image.NRGBA{image.NewNRGBA(image.Rect(0, 0, 64, 88)), image.NewNRGBA(image.Rect(0, 0, 65, 88))} {
		var b bytes.Buffer
		png.Encode(&b, im)
		if _, e := DecodeAtlas(&b); e == nil {
			t.Fatal("accepted blank/invalid PNG")
		}
	}
	solid := image.NewNRGBA(image.Rect(0, 0, 64, 88))
	for y := 0; y < 88; y++ {
		for x := 0; x < 64; x++ {
			solid.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, solid)
	if _, e := DecodeAtlas(&b); e == nil {
		t.Fatal("accepted opaque atlas")
	}
}
func TestLegacyBehaviorPriority(t *testing.T) {
	c := NewCat(0, 100, 108, Rect{0, 0, 1920, 1080})
	c.Dragging = true
	r, _ := c.Tick(1, .1, 9999, 9999, 0, true)
	if r != 3 {
		t.Fatal("drag not top priority")
	}
	c.Dragging = false
	c.Pet(2)
	r, _ = c.Tick(2.5, .1, 9999, 9999, 0, true)
	if r != 3 {
		t.Fatal("quiet prevents direct pet")
	}
	r, col := c.Tick(5, .1, 9999, 9999, 0, true)
	if r != 0 || col != 3 {
		t.Fatal("quiet did not settle")
	}
}
func TestWanderStaysOnMonitor(t *testing.T) {
	c := NewCat(2, 144, 156, Rect{-1920, 0, 0, 1040})
	for i := 0; i < 100000; i++ {
		now := float64(i) * .1
		c.Tick(now, .1, 99999, 99999, 0, false)
		if c.X < float64(c.Bounds.Left) || c.X > float64(c.Bounds.Right-c.W) || c.Y < float64(c.Bounds.Top) || c.Y > float64(c.Bounds.Bottom-c.H) {
			t.Fatalf("escaped at tick %d: %+v", i, c)
		}
	}
}
func TestDefaultConfigJSON(t *testing.T) {
	b, e := json.Marshal(DefaultConfig())
	if e != nil || len(b) == 0 {
		t.Fatal(e)
	}
}
