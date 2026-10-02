//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var interactionGetStyle = user32.NewProc("GetWindowLongPtrW")
var interactionSetStyle = user32.NewProc("SetWindowLongPtrW")

func setPetClickThrough(hwnd uintptr, enabled bool) error {
	style, _, err := interactionGetStyle.Call(hwnd, signed(-20))
	if style == 0 {
		return fmt.Errorf("無法讀取貓咪視窗：%v", err)
	}
	next := style &^ uintptr(0x20)
	if enabled {
		next |= 0x20
	}
	if old, _, err := interactionSetStyle.Call(hwnd, signed(-20), next); old == 0 {
		return fmt.Errorf("無法切換點穿：%v", err)
	}
	if ok, _, err := setWindowPos.Call(hwnd, 0, 0, 0, 0, 0, 0x0037); ok == 0 {
		interactionSetStyle.Call(hwnd, signed(-20), style)
		return fmt.Errorf("無法更新點穿視窗：%v", err)
	}
	return nil
}
func setClickThrough(enabled bool) error {
	old := app.ClickThrough
	for i, p := range app.Pets {
		if p.Down {
			p.Down = false
			p.Cat.Dragging = false
			p.Cat.Pressed = false
			releaseCapture.Call()
			if app.Engine != nil {
				app.Engine.Cancel(p.Index, nowSeconds())
			}
		}
		if err := setPetClickThrough(p.HWND, enabled); err != nil {
			for _, prior := range app.Pets[:i] {
				setPetClickThrough(prior.HWND, old)
			}
			return err
		}
	}
	app.ClickThrough = enabled
	app.ClickThroughUntil = 0
	if enabled {
		app.ClickThroughUntil = nowSeconds() + 300
	}
	updateTrayTip()
	setInterval()
	return nil
}
func expireClickThrough(now float64) {
	if app.ClickThrough && now >= app.ClickThroughUntil {
		if err := setClickThrough(false); err != nil {
			app.Hidden = true
			for _, p := range app.Pets {
				showWindow.Call(p.HWND, 0)
			}
			setInterval()
		}
	}
}
func updateTrayTip() {
	text := "三貓桌面陪伴 · 單擊顯示／隱藏，右鍵選單"
	if app.ClickThrough {
		text = "點穿中 · 右鍵圖示可恢復，5分鐘後自動恢復"
	}
	for i := range app.Tray.Tip {
		app.Tray.Tip[i] = 0
	}
	copy(app.Tray.Tip[:], syscall.StringToUTF16(text))
	notifyIcon.Call(1, uintptr(unsafe.Pointer(&app.Tray)))
}
func applyAppSettings(s Settings) error {
	s.ExperimentalMovement = app.Settings.ExperimentalMovement
	if err := SaveSettings(app.SettingsPath, s); err != nil {
		return fmt.Errorf("設定未儲存：%v", err)
	}
	app.Settings = s
	if app.Engine != nil {
		app.Engine.SetActivity(s.Activity)
		app.Engine.SetCPUSettings(s.CPU)
		app.Engine.SetExperimentalMovement(s.ExperimentalMovement)
	}
	resetCPUMonitor()
	app.Frames = map[string][]byte{}
	app.CachedBytes = 0
	for _, p := range app.Pets {
		updateBounds(p)
		p.LastKey = ""
	}
	setInterval()
	tick()
	return nil
}
