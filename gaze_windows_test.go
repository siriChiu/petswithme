//go:build windows && amd64

package main

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"
	"unsafe"
)

// Reads the real documented screen cursor without moving it or injecting input.
func TestWindowsGazeReadsGlobalCursor(t *testing.T) {
	p, ok := readCurrentCursor()
	if !ok {
		t.Fatal("GetCursorPos did not return a screen sample")
	}
	for _, origin := range []*AnimationAnchor{nil, {X: .42, Y: .56}} {
		for d := 0; d < 16; d++ {
			e := gazeFixture()
			c := e.Cats[0]
			e.Manifest.GazeOrigin = origin
			ox, oy := ManifestGazeOrigin(c, e.Manifest)
			headX, headY := ox-c.X, oy-c.Y
			a := float64(d) * math.Pi / 8
			c.X = float64(p.X) - 1000*math.Sin(a) - headX
			c.Y = float64(p.Y) + 1000*math.Cos(a) - headY
			f := e.Tick(0, .05, float64(p.X), float64(p.Y), 0, false)[0]
			if f.Action != "gaze_"+itoaDirection(d) {
				t.Fatalf("native screen sample with origin%v selected%s instead of%d", origin, f.Action, d)
			}
		}
	}

}

func TestWindowsGazeSweepRendersWithoutFocus(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f, err := newSmokeFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	oldFrames, oldBytes := app.Frames, app.CachedBytes
	app.Frames = map[string][]byte{}
	app.CachedBytes = 0
	defer func() { app.Frames = oldFrames; app.CachedBytes = oldBytes }()
	im := image.NewNRGBA(image.Rect(0, 0, 64, 88))
	for d := 0; d < 16; d++ {
		for y := 2; y < 6; y++ {
			for x := 2; x < 6; x++ {
				im.SetNRGBA((d%8)*8+x, (9+d/8)*8+y, color.NRGBA{R: uint8(32 + d*10), G: 120, B: 80, A: 255})
			}
		}
	}
	e := gazeFixture()
	c := e.Cats[0]
	c.X, c.Y = float64(f.x), float64(f.y)
	c.W, c.H = smokeWidth, smokeHeight
	pet := &PetWindow{HWND: f.pet, Cat: c, Atlas: &Atlas{Image: im, CellW: 8, CellH: 8}, Manifest: e.Manifest}
	defer pet.DIB.Close()
	ox, oy := GazeOrigin(c)
	for d := 0; d < 16; d++ {
		a := float64(d) * math.Pi / 8
		frame := e.Tick(float64(d), .05, ox+1000*math.Sin(a), oy-1000*math.Cos(a), 0, true)[0]
		if err := renderFrame(pet, frame); err != nil {
			t.Fatal(err)
		}
		pixels := unsafe.Slice((*byte)(pet.DIB.Bits), smokeWidth*smokeHeight*4)
		center := (smokeHeight/2*smokeWidth + smokeWidth/2) * 4
		if pixels[center+2] != uint8(32+d*10) {
			t.Fatalf("direction%d did not reach native DIB", d)
		}
	}
	fg, _, _ := smokeGetForeground.Call()
	if fg == f.pet || fg == f.probe {
		t.Fatal("gaze stole focus")
	}
	t.Log("Gaze sweep: all 16 engine directions rendered through the real layered window; no cursor injection or focus steal")
}
