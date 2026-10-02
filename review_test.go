package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewConfigRejectsNULWindowTitle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cats.json"), []byte(`{"version":1,"cats":[{"name":"cat\u0000title"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("embedded NUL must be rejected before Win32 UTF-16 conversion")
	}
}

func TestReviewConfigUnicodeNameBoundary(t *testing.T) {
	dir := t.TempDir()
	for _, count := range []int{40, 41} {
		cfg := Config{Version: 1, Cats: []CatSpec{{Name: "  " + strings.Repeat("貓", count) + "  "}}}
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cats.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := LoadConfig(dir)
		if count == 40 {
			if err != nil {
				t.Fatal(err)
			}
			if got.Cats[0].Name != strings.Repeat("貓", count) || !got.Cats[0].Demo {
				t.Fatal(got)
			}
		} else if err == nil {
			t.Fatal("accepted 41-rune name")
		}
	}
}

func TestReviewBGRAPixelOrderingAndHitThreshold(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 16, 22))
	im.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 128})
	im.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 23})
	im.SetNRGBA(0, 1, color.NRGBA{R: 255, G: 128, B: 0, A: 24})
	im.SetNRGBA(1, 1, color.NRGBA{R: 12, G: 34, B: 56, A: 255})
	a := &Atlas{Image: im, CellW: 2, CellH: 2}
	want := []byte{25, 50, 100, 128, 0, 0, 0, 0, 0, 12, 24, 24, 56, 34, 12, 255}
	if got := a.FrameBGRA(0, 0, 2, 2); !bytes.Equal(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	// Nearest-neighbor upscaling must preserve geometry and alpha exactly.
	got := a.FrameBGRA(0, 0, 4, 4)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			i := (y*4 + x) * 4
			j := ((y/2)*2 + x/2) * 4
			if !bytes.Equal(got[i:i+4], want[j:j+4]) {
				t.Fatalf("bad scaled pixel (%d,%d)", x, y)
			}
		}
	}
}

func TestReviewAtlasRequiresEveryUsedFrame(t *testing.T) {
	makeAtlas := func() *image.NRGBA {
		im := image.NewNRGBA(image.Rect(0, 0, 64, 88))
		for row, count := range rowFrames {
			for col := 0; col < count; col++ {
				for y := 1; y <= 4; y++ {
					for x := 1; x <= 4; x++ {
						im.SetNRGBA(col*8+x, row*8+y, color.NRGBA{R: 180, G: 100, B: 70, A: 255})
					}
				}
			}
		}
		return im
	}
	encode := func(im *image.NRGBA) *bytes.Buffer {
		var b bytes.Buffer
		if err := png.Encode(&b, im); err != nil {
			t.Fatal(err)
		}
		return &b
	}
	im := makeAtlas()
	if _, err := DecodeAtlas(encode(im)); err != nil {
		t.Fatal("valid atlas rejected:", err)
	}
	// The last gaze frame is used, even though it's distant from idle frames.
	for y := 80; y < 88; y++ {
		for x := 56; x < 64; x++ {
			im.SetNRGBA(x, y, color.NRGBA{})
		}
	}
	if _, err := DecodeAtlas(encode(im)); err == nil {
		t.Fatal("accepted atlas missing row 10 frame 7")
	}
}
