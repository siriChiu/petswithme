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
	im := image.NewNRGBA(image.Rect(0, 0, 256, 8))
	for d := 0; d < 32; d++ {
		for y := 2; y < 6; y++ {
			for x := 2; x < 6; x++ {
				im.SetNRGBA(d*8+x, y, color.NRGBA{R: uint8(32 + d*6), G: 120, B: 80, A: 255})
			}
		}
	}
	e := gazeFixture()
	m := &AnimationManifest{SchemaVersion: 1, GazeDirections: 32, Fallback: "idle", Anchor: AnimationAnchor{.5, 1}, Actions: map[string]AnimationAction{}}
	for d := 0; d < 32; d++ {
		m.Actions["gaze_"+itoaDirection(d)] = AnimationAction{AnimationClip: AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{d * 8, 0, 8, 8}, DurationMS: 200}}}}
	}
	m.Actions["idle"] = m.Actions["gaze_0"]
	e.SetManifest(0, m)
	c := e.Cats[0]
	c.X, c.Y = float64(f.x), float64(f.y)
	c.W, c.H = smokeWidth, smokeHeight
	pet := &PetWindow{HWND: f.pet, Cat: c, Atlas: &Atlas{Image: im, CellW: 8, CellH: 8}, Manifest: e.Manifest}
	defer pet.DIB.Close()
	ox, oy := GazeOrigin(c)
	for d := 0; d < 32; d++ {
		a := float64(d) * math.Pi / 16
		frame := e.Tick(float64(d), .05, ox+1000*math.Sin(a), oy-1000*math.Cos(a), 0, true)[0]
		if err := renderFrame(pet, frame); err != nil {
			t.Fatal(err)
		}
		pixels := unsafe.Slice((*byte)(pet.DIB.Bits), smokeWidth*smokeHeight*4)
		center := (smokeHeight/2*smokeWidth + smokeWidth/2) * 4
		if pixels[center+2] != uint8(32+d*6) {
			t.Fatalf("direction%d did not reach native DIB", d)
		}
	}
	// Check actual presented pixels around the idle/gaze boundary, too.
	// This must not flash the differently colored idle frame on alternate ticks.
	frame := e.Tick(40, .05, ox+100, oy, 0, true)[0]
	if err := renderFrame(pet, frame); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 20; n++ {
		x := ox + 7
		if n%2 == 0 {
			x = ox + 9
		}
		frame = e.Tick(40.05+float64(n)*.05, .05, x, oy, 0, true)[0]
		if err := renderFrame(pet, frame); err != nil {
			t.Fatal(err)
		}
		pixels := unsafe.Slice((*byte)(pet.DIB.Bits), smokeWidth*smokeHeight*4)
		center := (smokeHeight/2*smokeWidth + smokeWidth/2) * 4
		if pixels[center+2] != 80 {
			t.Fatal("pointer boundary flashed idle in the native surface", frame.Action)
		}
	}
	// A partial set must use real angular neighbors rather than fixed alias
	// bins, including when the input angle lies across the up-direction seam.
	m.GazeNearestAuthored = true
	for d := 1; d < 32; d += 2 {
		if d != 15 {
			m.Actions["gaze_"+itoaDirection(d)] = AnimationAction{Fallback: "gaze_" + itoaDirection((d+1)%32)}
		}
	}
	for i, tc := range []struct {
		degrees float64
		want    int
	}{{6, 0}, {12, 2}, {30, 2}, {164, 15}, {175, 16}, {359, 0}} {
		e.States[0].GazeActive = false
		a := tc.degrees * math.Pi / 180
		frame = e.Tick(60+float64(i), .05, ox+1000*math.Sin(a), oy-1000*math.Cos(a), 0, true)[0]
		if err := renderFrame(pet, frame); err != nil {
			t.Fatal(err)
		}
		pixels := unsafe.Slice((*byte)(pet.DIB.Bits), smokeWidth*smokeHeight*4)
		center := (smokeHeight/2*smokeWidth + smokeWidth/2) * 4
		if pixels[center+2] != uint8(32+tc.want*6) {
			t.Fatal("partial gaze angle selected wrong native pixels", tc.degrees, frame.Action)
		}
	}
	fg, _, _ := smokeGetForeground.Call()
	if fg == f.pet || fg == f.probe {
		t.Fatal("gaze stole focus")
	}
	t.Log("Gaze sweep: all 32 engine directions rendered through the real layered window; no cursor injection or focus steal")
}
