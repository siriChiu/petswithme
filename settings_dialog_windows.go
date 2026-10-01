//go:build windows

package main

import (
	"fmt"
	"math"
	"strconv"
	"syscall"
	"unsafe"
)

var (
	settingsSend          = user32.NewProc("SendMessageW")
	settingsText          = user32.NewProc("GetWindowTextW")
	settingsSetText       = user32.NewProc("SetWindowTextW")
	settingsClient        = user32.NewProc("GetClientRect")
	settingsDialogMessage = user32.NewProc("IsDialogMessageW")
	settingsSetFocus      = user32.NewProc("SetFocus")
	settingsCreateFont    = gdi32.NewProc("CreateFontW")
	settingsAdjust        = user32.NewProc("AdjustWindowRectEx")
)

type settingControl struct {
	hwnd       uintptr
	x, y, w, h int
}

var settingsUI struct {
	hwnd, font uintptr
	controls   map[int]settingControl
	base       Settings
	apply      func(Settings) error
	registered bool
	scale      float64
	errorText  string
}

func nativeSettingsMessage(msg *Message) bool {
	if settingsUI.hwnd == 0 {
		return false
	}
	ok, _, _ := settingsDialogMessage.Call(settingsUI.hwnd, uintptr(unsafe.Pointer(msg)))
	return ok != 0
}
func closeNativeSettings() {
	if settingsUI.hwnd != 0 {
		destroyWindow.Call(settingsUI.hwnd)
	}
}
func settingsField(id int) string {
	c := settingsUI.controls[id]
	buf := make([]uint16, 96)
	settingsText.Call(c.hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}
func settingsNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
func settingsSelect(id int) int {
	v, _, _ := settingsSend.Call(settingsUI.controls[id].hwnd, 0x147, 0, 0)
	return int(int32(v))
}
func readNativeSettings() (Settings, error) {
	sizes := []int{96, 144, 192}
	activities := []ActivityLevel{ActivityQuiet, ActivityNormal, ActivityLively}
	si, ai := settingsSelect(10), settingsSelect(11)
	if si < 0 || si >= len(sizes) || ai < 0 || ai >= len(activities) {
		return settingsUI.base, fmt.Errorf("請選擇大小和活動程度")
	}
	checked, _, _ := settingsSend.Call(settingsUI.controls[12].hwnd, 0xf0, 0, 0)
	return ParseSettingsForm(settingsUI.base, SettingsForm{sizes[si], activities[ai], checked == 1, settingsField(13), settingsField(14), settingsField(15), settingsField(16), settingsField(17)})
}
func applyNativeSettings() error {
	s, err := readNativeSettings()
	if err != nil {
		return err
	}
	if settingsUI.apply != nil {
		if err = settingsUI.apply(s); err != nil {
			return err
		}
	}
	settingsUI.base = s
	return nil
}
func layoutNativeSettings() {
	if settingsUI.hwnd == 0 {
		return
	}
	var r WinRect
	settingsClient.Call(settingsUI.hwnd, uintptr(unsafe.Pointer(&r)))
	scale := math.Min(float64(r.Right-r.Left)/460, float64(r.Bottom-r.Top)/390)
	if scale <= 0 {
		return
	}
	if settingsUI.font == 0 || math.Abs(scale-settingsUI.scale) > .01 {
		font, _, _ := settingsCreateFont.Call(signed(-int(math.Round(15*scale))), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(utf("Microsoft JhengHei UI"))))
		if font != 0 {
			old := settingsUI.font
			settingsUI.font = font
			settingsUI.scale = scale
			for _, c := range settingsUI.controls {
				settingsSend.Call(c.hwnd, 0x30, font, 1)
			}
			if old != 0 {
				deleteObject.Call(old)
			}
		}
	}
	for id, c := range settingsUI.controls {
		h := c.h
		if id == 10 || id == 11 {
			h = 140
		}
		setWindowPos.Call(c.hwnd, 0, signed(int(math.Round(float64(c.x)*scale))), signed(int(math.Round(float64(c.y)*scale))), uintptr(max(1, int(math.Round(float64(c.w)*scale)))), uintptr(max(1, int(math.Round(float64(h)*scale)))), 0x0014)
	}
}
func openNativeSettings(current Settings, apply func(Settings) error) error {
	if settingsUI.hwnd != 0 {
		showWindow.Call(settingsUI.hwnd, 9)
		setForeground.Call(settingsUI.hwnd)
		return nil
	}
	instance, _, _ := moduleHandle.Call(0)
	if !settingsUI.registered {
		cursor, _, _ := loadCursor.Call(0, 32512)
		wc := WndClass{Size: uint32(unsafe.Sizeof(WndClass{})), WndProc: syscall.NewCallback(settingsWndProc), Instance: instance, Cursor: cursor, Background: 16, ClassName: utf("ThreeCatCompanionSettings")}
		if ok, _, err := registerClass.Call(uintptr(unsafe.Pointer(&wc))); ok == 0 {
			return fmt.Errorf("無法建立設定視窗：%v", err)
		}
		settingsUI.registered = true
	}
	settingsUI.controls = map[int]settingControl{}
	settingsUI.base = current
	settingsUI.apply = apply
	settingsUI.errorText = ""
	settingsUI.scale = 0
	area := workArea(0, currentCursor())
	scale := math.Min(float64(dpi(app.Controller))/96, math.Min(float64(area.Width()-20)/480, float64(area.Height()-20)/430))
	scale = math.Max(.3, scale)
	rect := WinRect{Right: int32(math.Round(460 * scale)), Bottom: int32(math.Round(390 * scale))}
	const style = 0x00CA0000
	const exstyle = 0x00010001
	settingsAdjust.Call(uintptr(unsafe.Pointer(&rect)), style, 0, exstyle)
	w, h := int(rect.Right-rect.Left), int(rect.Bottom-rect.Top)
	w = min(w, area.Width())
	h = min(h, area.Height())
	x, y := area.Left+(area.Width()-w)/2, area.Top+(area.Height()-h)/2
	hwnd, _, err := createWindow.Call(exstyle, uintptr(unsafe.Pointer(utf("ThreeCatCompanionSettings"))), uintptr(unsafe.Pointer(utf("三貓桌面陪伴 · 設定"))), style, signed(x), signed(y), uintptr(w), uintptr(h), 0, 0, instance, 0)
	if hwnd == 0 {
		return fmt.Errorf("無法開啟設定：%v", err)
	}
	settingsUI.hwnd = hwnd
	add := func(id int, class, text string, flags uintptr, x, y, w, h int) error {
		ex := uintptr(0)
		if class == "EDIT" {
			ex = 0x200
		}
		control, _, err := createWindow.Call(ex, uintptr(unsafe.Pointer(utf(class))), uintptr(unsafe.Pointer(utf(text))), 0x50000000|flags, 0, 0, 1, 1, hwnd, uintptr(id), instance, 0)
		if control == 0 {
			return fmt.Errorf("無法建立設定欄位：%v", err)
		}
		settingsUI.controls[id] = settingControl{control, x, y, w, h}
		if class == "EDIT" {
			settingsSend.Call(control, 0xc5, 24, 0)
		}
		return nil
	}
	entries := []struct {
		id          int
		class, text string
		flags       uintptr
		x, y, w, h  int
	}{
		{30, "STATIC", "貓咪大小", 0, 20, 20, 145, 24}, {10, "COMBOBOX", "", 0x00210003, 170, 16, 260, 30},
		{31, "STATIC", "活動程度", 0, 20, 60, 145, 24}, {11, "COMBOBOX", "", 0x00210003, 170, 56, 260, 30},
		{12, "BUTTON", "隨總體 CPU 負載踏踏／伸懶腰", 0x10003, 20, 101, 410, 28},
		{32, "STATIC", "開始門檻 (%)", 0, 20, 145, 120, 24}, {13, "EDIT", settingsNumber(current.CPU.EnterPercent), 0x10080, 140, 140, 70, 28},
		{33, "STATIC", "結束門檻 (%)", 0, 240, 145, 120, 24}, {14, "EDIT", settingsNumber(current.CPU.ExitPercent), 0x10080, 360, 140, 70, 28},
		{34, "STATIC", "開始持續 (秒)", 0, 20, 185, 120, 24}, {15, "EDIT", settingsNumber(current.CPU.EnterSeconds), 0x10080, 140, 180, 70, 28},
		{35, "STATIC", "結束持續 (秒)", 0, 240, 185, 120, 24}, {16, "EDIT", settingsNumber(current.CPU.ExitSeconds), 0x10080, 360, 180, 70, 28},
		{36, "STATIC", "忙碌多久後伸懶腰 (秒)", 0, 20, 225, 275, 24}, {17, "EDIT", settingsNumber(current.CPU.StretchEverySeconds), 0x10080, 305, 220, 125, 28},
		{37, "STATIC", "高負載／安靜時會降低更新率，動作不會加速。\nCPU預設70%／50%、10秒持續、300秒伸展。", 0, 20, 260, 410, 45},
		{20, "STATIC", "", 0, 20, 307, 410, 37},
		{1, "BUTTON", "套用並關閉", 0x10001, 180, 347, 135, 30}, {2, "BUTTON", "取消", 0x10000, 330, 347, 100, 30}}
	for _, c := range entries {
		if err := add(c.id, c.class, c.text, c.flags, c.x, c.y, c.w, c.h); err != nil {
			closeNativeSettings()
			return err
		}
	}
	for _, v := range []string{"小 (96)", "中 (144)", "大 (192)"} {
		settingsSend.Call(settingsUI.controls[10].hwnd, 0x143, 0, uintptr(unsafe.Pointer(utf(v))))
	}
	si := 1
	if current.Size == 96 {
		si = 0
	} else if current.Size == 192 {
		si = 2
	}
	settingsSend.Call(settingsUI.controls[10].hwnd, 0x14e, uintptr(si), 0)
	for _, v := range []string{"安靜：停止自主活動，可看滑鼠", "一般：平衡活動與休息", "活潑：較常探索與互動"} {
		settingsSend.Call(settingsUI.controls[11].hwnd, 0x143, 0, uintptr(unsafe.Pointer(utf(v))))
	}
	ai := 1
	if current.Activity == ActivityQuiet {
		ai = 0
	} else if current.Activity == ActivityLively {
		ai = 2
	}
	settingsSend.Call(settingsUI.controls[11].hwnd, 0x14e, uintptr(ai), 0)
	check := uintptr(0)
	if current.CPU.Enabled {
		check = 1
	}
	settingsSend.Call(settingsUI.controls[12].hwnd, 0xf1, check, 0)
	layoutNativeSettings()
	showWindow.Call(hwnd, 5)
	setForeground.Call(hwnd)
	settingsSetFocus.Call(settingsUI.controls[10].hwnd)
	return nil
}
func settingsWndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x111:
		switch wp & 0xffff {
		case 1:
			if err := applyNativeSettings(); err != nil {
				settingsUI.errorText = err.Error()
				settingsSetText.Call(settingsUI.controls[20].hwnd, uintptr(unsafe.Pointer(utf(err.Error()))))
			} else {
				closeNativeSettings()
			}
			return 0
		case 2:
			closeNativeSettings()
			return 0
		}
	case 0x10:
		destroyWindow.Call(hwnd)
		return 0
	case 0x2:
		if settingsUI.font != 0 {
			deleteObject.Call(settingsUI.font)
		}
		settingsUI.hwnd = 0
		settingsUI.font = 0
		settingsUI.controls = nil
		settingsUI.apply = nil
		return 0
	case 0x5:
		layoutNativeSettings()
		return 0
	case 0x2e0:
		if lp != 0 {
			r := (*WinRect)(unsafe.Pointer(lp))
			setWindowPos.Call(hwnd, 0, signed(int(r.Left)), signed(int(r.Top)), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x0014)
		}
		layoutNativeSettings()
		return 0
	}
	v, _, _ := defWindow.Call(hwnd, uintptr(msg), wp, lp)
	return v
}
