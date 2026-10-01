package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ValidateManifestPixels checks the actual decoded pixels of every declared
// frame. Geometry-only validation cannot catch a mistakenly blank source slot.
func ValidateManifestPixels(m *AnimationManifest, a *Atlas) error {
	if m == nil || a == nil || a.Image == nil {
		return fmt.Errorf("animation pack is missing its image or manifest")
	}
	if e := m.Validate(a.Image.Bounds().Dx(), a.Image.Bounds().Dy()); e != nil {
		return e
	}
	type stats struct{ count, minX, minY, maxX, maxY int }
	cache := map[FrameRect]stats{}
	canvasW, canvasH := ManifestCanvas(m, a)
	dragGrounded := m.Fallback == "drag"
	for start := range m.Actions {
		if start == "drag" {
			continue
		}
		seen := map[string]bool{}
		for name := start; name != "" && !seen[name]; {
			seen[name] = true
			if name == "drag" {
				dragGrounded = true
				break
			}
			name = m.Actions[name].Fallback
		}
	}
	names := make([]string, 0, len(m.Actions))
	for name := range m.Actions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		action := m.Actions[name]
		clips := []AnimationClip{action.AnimationClip}
		for _, clip := range action.Moods {
			clips = append(clips, clip)
		}
		for _, clip := range clips {
			for _, seq := range [][]AnimationFrame{clip.Start, clip.Loop, clip.End} {
				for i, frame := range seq {
					rect := FrameRect{frame.Col * a.CellW, frame.Row * a.CellH, a.CellW, a.CellH}
					if frame.Rect != nil {
						rect = *frame.Rect
					}
					st, exists := cache[rect]
					if !exists {
						st = stats{minX: rect.W, minY: rect.H}
						for y := 0; y < rect.H; y++ {
							for x := 0; x < rect.W; x++ {
								if a.Image.NRGBAAt(rect.X+x, rect.Y+y).A >= 24 {
									st.count++
									st.minX = min(st.minX, x)
									st.minY = min(st.minY, y)
									st.maxX = max(st.maxX, x+1)
									st.maxY = max(st.maxY, y+1)
								}
							}
						}
						cache[rect] = st
					}
					if st.count < 16 {
						return fmt.Errorf("action %q frame %d is empty or has fewer than 16 visible pixels", name, i)
					}
					if frame.Rect != nil && (name != "drag" || dragGrounded) {
						anchor := m.Anchor
						if frame.Anchor != nil {
							anchor = *frame.Anchor
						}
						t := AnimationFrameTransform(frame, anchor, canvasW, canvasH, true)
						logical := FrameLogicalRect(frame)
						cx, cy := 0, 0
						if frame.Canvas != nil {
							cx, cy = frame.Canvas.X, frame.Canvas.Y
						}
						left := float64((st.minX+cx)*t.ScaledW)/float64(logical.W) + float64(t.OffsetX)
						right := float64((st.maxX+cx)*t.ScaledW)/float64(logical.W) + float64(t.OffsetX)
						top := float64((st.minY+cy)*t.ScaledH)/float64(logical.H) + float64(t.OffsetY)
						bottom := float64((st.maxY+cy)*t.ScaledH)/float64(logical.H) + float64(t.OffsetY)
						if left < 0 || top < 0 || right > float64(canvasW) || bottom > float64(canvasH) {
							return fmt.Errorf("action %q frame %d would clip visible pixels when its ground anchor is aligned; correct its anchor or registration", name, i)
						}
					}
				}
			}
		}
	}
	return nil
}

// InitialBehaviorFrame honors the first frame's anchor just as normal playback
// does, avoiding a one-frame position jump during startup or Reset.
func InitialBehaviorFrame(m *AnimationManifest) BehaviorFrame {
	f := NewAnimationPlayer(m).Frame()
	anchor := m.Anchor
	if f.Anchor != nil {
		anchor = *f.Anchor
	}
	return BehaviorFrame{AnimationFrame: f, Anchor: anchor, Action: "idle", Mood: "calm"}
}

func CharacterSummary(specs []CatSpec) string {
	if len(specs) == 0 {
		return "尚未載入角色素材。"
	}
	text := "角色素材："
	for _, s := range specs {
		label := "自訂素材"
		if s.Demo || s.Sprite == "" {
			label = "示範外觀"
		}
		text += "\n• " + s.Name + "（" + label + "）"
	}
	return text + "\n\n缺少的動作會使用素材包指定的回退動畫。"
}

// OpenLocalAsset refuses devices, directories and links that escape the pack.
// This keeps runtime loading aligned with the documented app-folder boundary.
func OpenLocalAsset(root, relative string) (*os.File, error) {
	if !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("asset path must stay inside the app folder")
	}
	base, e := filepath.EvalSymlinks(root)
	if e != nil {
		return nil, e
	}
	base, e = filepath.Abs(base)
	if e != nil {
		return nil, e
	}
	path, e := filepath.EvalSymlinks(filepath.Join(base, relative))
	if e != nil {
		return nil, e
	}
	rel, e := filepath.Rel(base, path)
	if e != nil || !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("asset link escapes the app folder")
	}
	info, e := os.Stat(path)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("asset must be a regular file")
	}
	return os.Open(path)
}
