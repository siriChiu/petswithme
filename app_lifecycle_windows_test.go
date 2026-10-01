//go:build windows && amd64

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

const lifecycleHelperEnvironment = "THREECAT_LIFECYCLE_TEST_HELPER"

var lifecycleGetWindowProcess = user32.NewProc("GetWindowThreadProcessId")

// This runs the real application main/message loop in an isolated subprocess.
// Artwork and preferences are synthetic, temporary, and never use the user's
// files. Commands are addressed to this child process's controller only; there
// is no keyboard/mouse injection or network access.
func TestWindowsAppLifecycleSubprocess(t *testing.T) {
	if os.Getenv(lifecycleHelperEnvironment) == "1" {
		return
	}
	if ok, reason := smokeInteractiveDesktop(); !ok {
		t.Skipf("Full app/tray lifecycle requires an interactive desktop: %s", reason)
	}
	if tray, _, _ := findWindow.Call(uintptr(unsafe.Pointer(utf("Shell_TrayWnd"))), 0); tray == 0 {
		t.Skip("Full app/tray lifecycle unavailable: this desktop has no Explorer notification-area host (Shell_TrayWnd)")
	}
	if existing, _, _ := findWindow.Call(uintptr(unsafe.Pointer(utf("ThreeCatCompanionController"))), 0); existing != 0 {
		t.Fatal("refusing to run lifecycle test while another Three Cat Companion instance is open")
	}
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	childExe := filepath.Join(dir, "companion-lifecycle.test.exe")
	if err := lifecycleCopyExecutable(executable, childExe); err != nil {
		t.Fatal(err)
	}
	if err := lifecycleWriteFixture(dir); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(dir, "test-profile")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, childExe, "-test.run=^TestWindowsAppLifecycleHelper$", "-test.v", "-test.timeout=17s")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), lifecycleHelperEnvironment+"=1", "APPDATA="+profile, "LOCALAPPDATA="+profile)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("real app did not finish within 20 seconds; child was terminated:\n%s", output)
	}
	if err != nil {
		t.Fatalf("real application lifecycle failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "REAL_APP_LIFECYCLE_PASSED") {
		t.Fatalf("child returned without proving full lifecycle:\n%s", output)
	}
	t.Logf("Real main/controller/tray/three-pet lifecycle passed:\n%s", output)
}

func lifecycleCopyExecutable(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func lifecycleWriteFixture(dir string) error {
	im := image.NewNRGBA(image.Rect(0, 0, 64, 88))
	for row, count := range rowFrames {
		for col := 0; col < count; col++ {
			for y := 2; y < 6; y++ {
				for x := 2; x < 6; x++ {
					im.SetNRGBA(col*8+x, row*8+y, color.NRGBA{R: 96, G: 180, B: 128, A: 255})
				}
			}
		}
	}
	f, err := os.Create(filepath.Join(dir, "synthetic-qa.png"))
	if err != nil {
		return err
	}
	encodeErr := png.Encode(f, im)
	closeErr := f.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	manifest := &AnimationManifest{SchemaVersion: 1, Fallback: "idle", Anchor: AnimationAnchor{X: .5, Y: 1}, Actions: map[string]AnimationAction{"idle": {AnimationClip: AnimationClip{Loop: []AnimationFrame{{Rect: &FrameRect{X: 0, Y: 0, W: 8, H: 8}, DurationMS: 200, Anchor: &AnimationAnchor{X: .5, Y: .9}}}}}}}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "synthetic-animation.json"), manifestJSON, 0600); err != nil {
		return err
	}
	cfg := Config{Version: 1}
	for i := 0; i < 3; i++ {
		spec := CatSpec{Name: fmt.Sprintf("Lifecycle square %d", i+1), Sprite: "synthetic-qa.png", Demo: false}
		if i == 1 {
			spec.Animations = "synthetic-animation.json"
		}
		cfg.Cats = append(cfg.Cats, spec)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "cats.json"), data, 0600)
}

func lifecycleOwnWindow(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	var pid uint32
	lifecycleGetWindowProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid == uint32(os.Getpid())
}

func lifecycleWait(ctx context.Context, description string, predicate func() bool) error {
	for {
		if predicate() {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s", description)
		case <-time.After(15 * time.Millisecond):
		}
	}
}

func lifecycleDrive(ctx context.Context) (err error) {
	var controller uintptr
	defer func() {
		// An assertion failure must still ask this child to quit. The parent
		// timeout is the final backstop for a modal startup failure or deadlock.
		if err != nil && lifecycleOwnWindow(controller) {
			postMessage.Call(controller, 0x0010, 0, 0)
		}
	}()
	if err = lifecycleWait(ctx, "this process's controller", func() bool {
		controller, _, _ = findWindow.Call(uintptr(unsafe.Pointer(utf("ThreeCatCompanionController"))), 0)
		return lifecycleOwnWindow(controller)
	}); err != nil {
		return err
	}
	pets := make([]uintptr, 3)
	if err = lifecycleWait(ctx, "three visible synthetic pets", func() bool {
		for i := range pets {
			pets[i], _, _ = findWindow.Call(uintptr(unsafe.Pointer(utf("ThreeCatCompanionPet"))), uintptr(unsafe.Pointer(utf(fmt.Sprintf("Lifecycle square %d", i+1)))))
			if !lifecycleOwnWindow(pets[i]) {
				return false
			}
			if visible, _, _ := smokeIsVisible.Call(pets[i]); visible == 0 {
				return false
			}
		}
		return true
	}); err != nil {
		return err
	}
	postCommand := func(id uintptr) error {
		if !lifecycleOwnWindow(controller) {
			return fmt.Errorf("controller disappeared before command %d", id)
		}
		if result, _, callErr := postMessage.Call(controller, 0x0111, id, 0); result == 0 {
			return fmt.Errorf("PostMessageW(WM_COMMAND %d): %v", id, callErr)
		}
		return nil
	}
	visible := func(want bool) bool {
		for _, pet := range pets {
			v, _, _ := smokeIsVisible.Call(pet)
			if (v != 0) != want {
				return false
			}
		}
		return true
	}
	settingsPath := filepath.Join(os.Getenv("APPDATA"), "ThreeCatCompanion", "settings.json")
	settingsAre := func(quiet bool, size int) bool {
		data, readErr := os.ReadFile(settingsPath)
		if readErr != nil {
			return false
		}
		var settings Settings
		return json.Unmarshal(data, &settings) == nil && settings.Quiet == quiet && settings.Size == size
	}
	if err = postCommand(101); err != nil {
		return err
	} // Quiet on; verifies a settings write.
	if err = lifecycleWait(ctx, "quiet mode saved", func() bool { return settingsAre(true, 144) }); err != nil {
		return err
	}
	if err = postCommand(100); err != nil {
		return err
	}
	if err = lifecycleWait(ctx, "all pets hidden", func() bool { return visible(false) }); err != nil {
		return err
	}
	if err = postCommand(100); err != nil {
		return err
	}
	if err = lifecycleWait(ctx, "all pets shown", func() bool { return visible(true) }); err != nil {
		return err
	}
	if err = postCommand(102); err != nil {
		return err
	} // Reset on current monitor, without moving the mouse.
	if err = postCommand(101); err != nil {
		return err
	} // Quiet off before Play.
	if err = lifecycleWait(ctx, "quiet mode disabled", func() bool { return settingsAre(false, 144) }); err != nil {
		return err
	}
	activityIs := func(want ActivityLevel) bool {
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			return false
		}
		var settings Settings
		return json.Unmarshal(data, &settings) == nil && settings.Activity == want && settings.Quiet == (want == ActivityQuiet)
	}
	if err = postCommand(107); err != nil {
		return err
	}
	if err = lifecycleWait(ctx, "lively activity saved", func() bool { return activityIs(ActivityLively) }); err != nil {
		return err
	}
	if err = postCommand(106); err != nil {
		return err
	}
	if err = lifecycleWait(ctx, "normal activity saved", func() bool { return activityIs(ActivityNormal) }); err != nil {
		return err
	}
	cpuEnabled := func(want bool) bool {
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			return false
		}
		var settings Settings
		return json.Unmarshal(data, &settings) == nil && settings.CPU.Enabled == want && settings.CPU.StretchEverySeconds == 300
	}
	if err = postCommand(108); err != nil {
		return err
	}
	if err = lifecycleWait(ctx, "CPU response disabled", func() bool { return cpuEnabled(false) }); err != nil {
		return err
	}
	if err = postCommand(108); err != nil {
		return err
	}
	if err = lifecycleWait(ctx, "CPU response enabled", func() bool { return cpuEnabled(true) }); err != nil {
		return err
	}
	experimentalIs := func(want bool) bool {
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			return false
		}
		var settings Settings
		return json.Unmarshal(data, &settings) == nil && settings.ExperimentalMovement == want
	}
	if err = postCommand(109); err != nil {
		return err
	}
	if err = lifecycleWait(ctx, "experimental movement enabled", func() bool { return experimentalIs(true) }); err != nil {
		return err
	}
	if err = postCommand(109); err != nil {
		return err
	}
	if err = lifecycleWait(ctx, "experimental movement disabled", func() bool { return experimentalIs(false) }); err != nil {
		return err
	}
	if err = postCommand(105); err != nil {
		return err
	} // Play all three.
	if err = postCommand(196); err != nil {
		return err
	} // Small size, distinguishable from startup default.
	if err = lifecycleWait(ctx, "size command persisted", func() bool { return settingsAre(false, 96) }); err != nil {
		return err
	}
	// All earlier posted commands have been processed before the size save.
	for _, pet := range pets {
		var rect WinRect
		if result, _, callErr := smokeGetWindowRect.Call(pet, uintptr(unsafe.Pointer(&rect))); result == 0 {
			return fmt.Errorf("GetWindowRect for resized pet: %v", callErr)
		}
		monitor, _, _ := monitorFromWindow.Call(pet, 2)
		mi := MonitorInfo{Size: uint32(unsafe.Sizeof(MonitorInfo{}))}
		if result, _, callErr := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&mi))); result == 0 {
			return fmt.Errorf("GetMonitorInfoW for resized pet: %v", callErr)
		}
		if rect.Left < mi.Work.Left || rect.Top < mi.Work.Top || rect.Right > mi.Work.Right || rect.Bottom > mi.Work.Bottom {
			return fmt.Errorf("pet escaped monitor work area after reset/size: %+v outside %+v", rect, mi.Work)
		}
	}
	return postCommand(104)
}

func TestWindowsAppLifecycleHelper(t *testing.T) {
	if os.Getenv(lifecycleHelperEnvironment) != "1" {
		return
	}
	if existing, _, _ := findWindow.Call(uintptr(unsafe.Pointer(utf("ThreeCatCompanionController"))), 0); existing != 0 {
		t.Fatal("another app instance appeared; refusing to send it any commands")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- lifecycleDrive(ctx) }()
	main() // Real startup, PNG/config/engine/tray setup, UI loop, and shutdown.
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if !app.Quitting || len(app.Pets) != 3 || app.Engine == nil {
		t.Fatalf("incomplete application lifecycle: quitting=%v pets=%d engine=%v", app.Quitting, len(app.Pets), app.Engine != nil)
	}
	if app.Hidden || app.Settings.Quiet || app.Settings.Activity != ActivityNormal || app.Settings.Size != 96 {
		t.Fatalf("commands did not leave expected state: hidden=%v settings=%+v", app.Hidden, app.Settings)
	}
	if !app.Settings.CPU.Enabled {
		t.Fatal("CPU settings were not restored")
	}
	if expected := filepath.Join(os.Getenv("APPDATA"), "ThreeCatCompanion", "settings.json"); app.SettingsPath != expected {
		t.Fatalf("preferences escaped test profile: %q, expected %q", app.SettingsPath, expected)
	}
	for _, pet := range app.Pets {
		if exists, _, _ := smokeIsWindow.Call(pet.HWND); exists != 0 {
			t.Fatal("pet window survived application quit")
		}
		if pet.DIB.DC != 0 || pet.DIB.Bitmap != 0 || pet.DIB.Bits != nil {
			t.Fatal("pet DIB survived application quit")
		}
		if pet.Spec.Demo || pet.Spec.Sprite != "synthetic-qa.png" {
			t.Fatal("application did not use synthetic-only configured art")
		}
	}
	if exists, _, _ := smokeIsWindow.Call(app.Controller); exists != 0 {
		t.Fatal("controller survived main return")
	}
	t.Log("REAL_APP_LIFECYCLE_PASSED: real main, tray setup, three synthetic pets, experimental movement toggle/CPU toggle/quiet/normal/lively/hide/show/reset/play/size/quit, isolated settings, window and DIB cleanup")
}
