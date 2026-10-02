//go:build windows && amd64

package main

import (
	"image"
	"image/color"
	"runtime"
	"testing"
	"unsafe"
)

func TestWindowsPackedCanvasIsPartOfFrameCacheKey(t *testing.T) {
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
	im := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 2; y < 6; y++ {
		for x := 2; x < 6; x++ {
			im.SetNRGBA(x, y, color.NRGBA{200, 100, 80, 255})
		}
	}
	c := NewCat(0, 96, 96, Rect{0, 0, 2000, 1200})
	c.X, c.Y = float64(f.x), float64(f.y)
	p := &PetWindow{HWND: f.pet, Cat: c, Atlas: &Atlas{Image: im}}
	defer p.DIB.Close()
	frame := BehaviorFrame{AnimationFrame: AnimationFrame{Rect: &FrameRect{0, 0, 8, 8}, Canvas: &FrameCanvas{16, 16, 0, 0}, DurationMS: 100}, Anchor: AnimationAnchor{.5, 1}, Action: "idle"}
	if err := renderFrame(p, frame); err != nil {
		t.Fatal(err)
	}
	first := p.LastKey
	frame.Canvas = &FrameCanvas{16, 16, 8, 8}
	if err := renderFrame(p, frame); err != nil {
		t.Fatal(err)
	}
	if p.LastKey == first {
		t.Fatal("canvas offset omitted from cache key")
	}
	pixels := unsafe.Slice((*byte)(p.DIB.Bits), 96*96*4)
	if pixels[(72*96+72)*4+3] != 255 || pixels[(24*96+24)*4+3] != 0 {
		t.Fatal("cache reused wrong canvas placement")
	}
}
