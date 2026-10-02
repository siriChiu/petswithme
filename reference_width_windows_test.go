//go:build windows && amd64

package main

import (
	"image"
	"image/color"
	"runtime"
	"testing"
	"unsafe"
)

func TestWindowsReferenceWidthKeepsBodySizeInsideWiderWindow(t *testing.T) {
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
	im := image.NewNRGBA(image.Rect(0, 0, 40, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 40; x++ {
			im.SetNRGBA(x, y, color.NRGBA{200, 100, 80, 255})
		}
	}
	frame := BehaviorFrame{AnimationFrame: AnimationFrame{Rect: &FrameRect{0, 0, 40, 80}, Canvas: &FrameCanvas{384, 288, 172, 208}, DurationMS: 100}, Anchor: AnimationAnchor{.5, 1}, Action: "idle"}
	m := DefaultAnimationManifest()
	m.ReferenceWidth = 320
	w, h := ManifestDisplaySize(m, 384, 288, 192, 96)
	c := NewCat(0, w, h, Rect{0, 0, 2000, 1200})
	c.X, c.Y = float64(f.x), float64(f.y)
	p := &PetWindow{HWND: f.pet, Cat: c, Atlas: &Atlas{Image: im}, Manifest: m}
	defer p.DIB.Close()
	if err := renderFrame(p, frame); err != nil {
		t.Fatal(err)
	}
	var r WinRect
	smokeGetWindowRect.Call(f.pet, uintptr(unsafe.Pointer(&r)))
	if int(r.Right-r.Left) != 230 || int(r.Bottom-r.Top) != 172 {
		t.Fatal("unexpected physical reference window", r)
	}
	pixels := unsafe.Slice((*byte)(p.DIB.Bits), w*h*4)
	minX, maxX, minY, maxY := w, -1, h, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if pixels[(y*w+x)*4+3] > 0 {
				minX = min(minX, x)
				maxX = max(maxX, x)
				minY = min(minY, y)
				maxY = max(maxY, y)
			}
		}
	}
	if maxX-minX+1 < 23 || maxX-minX+1 > 25 || maxY-minY+1 < 47 || maxY-minY+1 > 49 {
		t.Fatal("body rescaled unexpectedly", minX, maxX, minY, maxY)
	}
	foreground, _, _ := smokeGetForeground.Call()
	if foreground == f.pet {
		t.Fatal("reference resizing stole focus")
	}
	t.Log("Native384canvas keeps40x80source body at approximately24x48px inside230x172window")
}
