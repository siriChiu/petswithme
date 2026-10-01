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
	seen := map[FrameRect]bool{}
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
					if seen[rect] {
						continue
					}
					seen[rect] = true
					occupied := 0
					for y := rect.Y; y < rect.Y+rect.H; y++ {
						for x := rect.X; x < rect.X+rect.W; x++ {
							if a.Image.NRGBAAt(x, y).A >= 24 {
								occupied++
							}
						}
					}
					if occupied < 16 {
						return fmt.Errorf("action %q frame %d is empty or has fewer than 16 visible pixels", name, i)
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
