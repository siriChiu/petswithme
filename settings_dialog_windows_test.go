//go:build windows && amd64

package main

import (
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsNativeSettingsApplyCancelAndBounds(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	base := NormalizeSettings(Settings{Size: 144, CPU: DefaultCPUSettings(), ExperimentalMovement: true})
	calls := 0
	var saved Settings
	if err := openNativeSettings(base, func(s Settings) error { calls++; saved = s; return nil }); err != nil {
		t.Fatal(err)
	}
	defer closeNativeSettings()
	h := settingsUI.hwnd
	if h == 0 || len(settingsUI.controls) != 19 {
		t.Fatalf("incomplete controls:%d", len(settingsUI.controls))
	}
	if err := openNativeSettings(base, nil); err != nil || settingsUI.hwnd != h {
		t.Fatal("reopening duplicated settings")
	}
	var outer WinRect
	smokeGetWindowRect.Call(h, uintptr(unsafe.Pointer(&outer)))
	area := workArea(h, currentCursor())
	if outer.Left < int32(area.Left) || outer.Top < int32(area.Top) || outer.Right > int32(area.Right) || outer.Bottom > int32(area.Bottom) {
		t.Fatal("settings escaped work area")
	}
	for _, c := range settingsUI.controls {
		var r WinRect
		smokeGetWindowRect.Call(c.hwnd, uintptr(unsafe.Pointer(&r)))
		if r.Left < outer.Left || r.Top < outer.Top || r.Right > outer.Right || r.Bottom > outer.Bottom {
			t.Fatalf("control outside dialog:%+v outer%+v", r, outer)
		}
	}
	settingsSetText.Call(settingsUI.controls[13].hwnd, uintptr(unsafe.Pointer(utf("40"))))
	settingsSetText.Call(settingsUI.controls[14].hwnd, uintptr(unsafe.Pointer(utf("70"))))
	settingsSend.Call(settingsUI.controls[1].hwnd, 0xf5, 0, 0)
	if calls != 0 || settingsUI.hwnd == 0 || settingsUI.errorText == "" {
		t.Fatal("invalid settings mutated or closed")
	}
	settingsSetText.Call(settingsUI.controls[13].hwnd, uintptr(unsafe.Pointer(utf("85"))))
	settingsSetText.Call(settingsUI.controls[14].hwnd, uintptr(unsafe.Pointer(utf("45"))))
	settingsSend.Call(settingsUI.controls[10].hwnd, 0x14e, 2, 0)
	settingsSend.Call(settingsUI.controls[11].hwnd, 0x14e, 2, 0)
	settingsSend.Call(settingsUI.controls[1].hwnd, 0xf5, 0, 0)
	if calls != 1 || settingsUI.hwnd != 0 || saved.Size != 192 || saved.Activity != ActivityLively || saved.CPU.EnterPercent != 85 || saved.CPU.ExitPercent != 45 || !saved.ExperimentalMovement {
		t.Fatalf("apply failed:%+v calls%d hwnd%x", saved, calls, settingsUI.hwnd)
	}
	if err := openNativeSettings(saved, func(s Settings) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	settingsSetText.Call(settingsUI.controls[13].hwnd, uintptr(unsafe.Pointer(utf("75"))))
	settingsSend.Call(settingsUI.controls[2].hwnd, 0xf5, 0, 0)
	if calls != 1 || settingsUI.hwnd != 0 || settingsUI.font != 0 {
		t.Fatal("cancel changed settings or leaked native resources")
	}
	t.Log("Native settings: valid apply, invalid hysteresis blocked, cancel/reopen, bounded controls, font/window cleanup")
}
func TestWindowsWholePetClickThroughAndExpiry(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f, err := newSmokeFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	old := app
	defer func() { killTimer.Call(f.probe, 1); app = old }()
	app.Start = time.Now()
	app.Controller = f.probe
	app.Settings = NormalizeSettings(Settings{CPU: DefaultCPUSettings()})
	app.Pets = []*PetWindow{{HWND: f.pet, Cat: NewCat(0, 96, 96, Rect{0, 0, 1000, 1000})}}
	app.ClickThrough = false
	app.Hidden = false
	app.Engine = nil
	hit := func() uintptr {
		p := uint64(uint32(f.x+48)) | uint64(uint32(f.y+48))<<32
		v, _, _ := smokeWindowFromPoint.Call(uintptr(p))
		return v
	}
	if hit() != f.pet {
		t.Fatal("synthetic pet not hittable before opt-in")
	}
	if err := setClickThrough(true); err != nil {
		t.Fatal(err)
	}
	if hit() != f.probe || !app.ClickThrough {
		t.Fatal("whole pet failed to pass click through")
	}
	expireClickThrough(nowSeconds() + 301)
	if hit() != f.pet || app.ClickThrough {
		t.Fatal("temporary clickthrough did not restore")
	}
	if err := setClickThrough(true); err != nil {
		t.Fatal(err)
	}
	if err := setClickThrough(false); err != nil {
		t.Fatal(err)
	}
	if hit() != f.pet {
		t.Fatal("manual recovery failed")
	}
	app.TrayLastClick = 0
	windowProc(app.Controller, wmTray, 0, 0x0202)
	if !app.Hidden {
		t.Fatal("single tray click did not hide")
	}
	windowProc(app.Controller, wmTray, 0, 0x0202)
	if !app.Hidden {
		t.Fatal("double-click sequence flickered visibility")
	}
	app.TrayLastClick = -10
	windowProc(app.Controller, wmTray, 0, 0x0202)
	if app.Hidden {
		t.Fatal("next single tray click did not show")
	}
	t.Log("Native whole-pet clickthrough, manual restore, five-minute expiry and single-click tray visibility passed without input injection")
}
