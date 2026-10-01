//go:build windows

package main

import (
	"bytes"
	"embed"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

//go:embed assets/*
var embeddedAssets embed.FS
var user32 = syscall.NewLazyDLL("user32.dll")
var gdi32 = syscall.NewLazyDLL("gdi32.dll")
var shell32 = syscall.NewLazyDLL("shell32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var (
	registerClass     = user32.NewProc("RegisterClassExW")
	createWindow      = user32.NewProc("CreateWindowExW")
	defWindow         = user32.NewProc("DefWindowProcW")
	getMessage        = user32.NewProc("GetMessageW")
	translateMessage  = user32.NewProc("TranslateMessage")
	dispatchMessage   = user32.NewProc("DispatchMessageW")
	postQuit          = user32.NewProc("PostQuitMessage")
	destroyWindow     = user32.NewProc("DestroyWindow")
	showWindow        = user32.NewProc("ShowWindow")
	setWindowPos      = user32.NewProc("SetWindowPos")
	setTimer          = user32.NewProc("SetTimer")
	killTimer         = user32.NewProc("KillTimer")
	getCursorPos      = user32.NewProc("GetCursorPos")
	getLastInput      = user32.NewProc("GetLastInputInfo")
	setCapture        = user32.NewProc("SetCapture")
	releaseCapture    = user32.NewProc("ReleaseCapture")
	updateLayered     = user32.NewProc("UpdateLayeredWindow")
	getDC             = user32.NewProc("GetDC")
	releaseDC         = user32.NewProc("ReleaseDC")
	loadCursor        = user32.NewProc("LoadCursorW")
	loadIcon          = user32.NewProc("LoadIconW")
	setDPIContext     = user32.NewProc("SetProcessDpiAwarenessContext")
	getDPI            = user32.NewProc("GetDpiForWindow")
	monitorFromPoint  = user32.NewProc("MonitorFromPoint")
	monitorFromWindow = user32.NewProc("MonitorFromWindow")
	getMonitorInfo    = user32.NewProc("GetMonitorInfoW")
	createMenu        = user32.NewProc("CreatePopupMenu")
	appendMenu        = user32.NewProc("AppendMenuW")
	trackMenu         = user32.NewProc("TrackPopupMenu")
	destroyMenu       = user32.NewProc("DestroyMenu")
	setForeground     = user32.NewProc("SetForegroundWindow")
	messageBox        = user32.NewProc("MessageBoxW")
	registerMessage   = user32.NewProc("RegisterWindowMessageW")
	postMessage       = user32.NewProc("PostMessageW")
	findWindow        = user32.NewProc("FindWindowW")
	createDIB         = gdi32.NewProc("CreateDIBSection")
	createDC          = gdi32.NewProc("CreateCompatibleDC")
	selectObject      = gdi32.NewProc("SelectObject")
	deleteObject      = gdi32.NewProc("DeleteObject")
	deleteDC          = gdi32.NewProc("DeleteDC")
	notifyIcon        = shell32.NewProc("Shell_NotifyIconW")
	moduleHandle      = kernel32.NewProc("GetModuleHandleW")
	getTick           = kernel32.NewProc("GetTickCount")
	createMutex       = kernel32.NewProc("CreateMutexW")
	closeHandle       = kernel32.NewProc("CloseHandle")
)

type Point struct{ X, Y int32 }
type Size struct{ CX, CY int32 }
type WinRect struct{ Left, Top, Right, Bottom int32 }
type WndClass struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSmall                          uintptr
}
type Message struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      Point
	Private uint32
}
type BitmapInfo struct {
	Size                   uint32
	Width, Height          int32
	Planes, BitCount       uint16
	Compression, SizeImage uint32
	XPels, YPels           int32
	ClrUsed, ClrImportant  uint32
}
type Blend struct{ Op, Flags, ConstantAlpha, AlphaFormat byte }
type MonitorInfo struct {
	Size          uint32
	Monitor, Work WinRect
	Flags         uint32
}
type LastInput struct{ Size, Time uint32 }
type NotifyData struct {
	Size                uint32
	HWND                uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Timeout             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                [16]byte
	Balloon             uintptr
}
type DIB struct {
	DC, Bitmap, Old uintptr
	Bits            unsafe.Pointer
	W, H            int
}
type PetWindow struct {
	HWND             uintptr
	Cat              *Cat
	Atlas            *Atlas
	Spec             CatSpec
	Manifest         *AnimationManifest
	Index            int
	CanvasW, CanvasH int
	DIB              DIB
	LastKey          string
	LastX, LastY     int
	Down, DidDrag    bool
	Press, Offset    Point
}

var getMouseButtonState = user32.NewProc("GetAsyncKeyState")
var getSystemMetrics = user32.NewProc("GetSystemMetrics")

var app struct {
	Controller, Instance, Mutex, Icon uintptr
	Tray                              NotifyData
	TaskbarMessage                    uint32
	Pets                              []*PetWindow
	Windows                           map[uintptr]*PetWindow
	Frames                            map[string][]byte
	CachedBytes                       int
	Engine                            *BehaviorEngine
	CPU                               CPUMonitor
	Settings                          Settings
	SettingsPath, Folder              string
	Hidden, Quitting                  bool
	ClickThrough                      bool
	ClickThroughUntil, TrayLastClick  float64
	Start                             time.Time
	LastTime, LastIdleCheck, Idle     float64
}

const wmTray = 0x8001
const wmWake = 0x8002
const appTitle = "Three Cat Companion"

func utf(s string) *uint16 { return syscall.StringToUTF16Ptr(s) }
func signed(v int) uintptr { return uintptr(v) }
func showError(s string) {
	messageBox.Call(0, uintptr(unsafe.Pointer(utf(s))), uintptr(unsafe.Pointer(utf(appTitle))), 0x10)
}
func readCurrentCursor() (Point, bool) {
	var p Point
	ok, _, _ := getCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p, ok != 0
}
func currentCursor() Point { p, _ := readCurrentCursor(); return p }
func workArea(hwnd uintptr, p Point) Rect {
	var m uintptr
	if hwnd != 0 {
		m, _, _ = monitorFromWindow.Call(hwnd, 2)
	} else {
		packed := uint64(uint32(p.X)) | (uint64(uint32(p.Y)) << 32)
		m, _, _ = monitorFromPoint.Call(uintptr(packed), 2)
	}
	mi := MonitorInfo{Size: uint32(unsafe.Sizeof(MonitorInfo{}))}
	ok, _, _ := getMonitorInfo.Call(m, uintptr(unsafe.Pointer(&mi)))
	if ok == 0 {
		return Rect{0, 0, 1024, 768}
	}
	return Rect{int(mi.Work.Left), int(mi.Work.Top), int(mi.Work.Right), int(mi.Work.Bottom)}
}
func dpi(hwnd uintptr) int {
	v, _, _ := getDPI.Call(hwnd)
	if v < 48 || v > 768 {
		return 96
	}
	return int(v)
}
func (d *DIB) Close() {
	if d.DC != 0 {
		selectObject.Call(d.DC, d.Old)
		deleteObject.Call(d.Bitmap)
		deleteDC.Call(d.DC)
		*d = DIB{}
	}
}
func (d *DIB) Resize(w, h int) error {
	if d.W == w && d.H == h {
		return nil
	}
	d.Close()
	dc, _, _ := createDC.Call(0)
	if dc == 0 {
		return fmt.Errorf("CreateCompatibleDC failed")
	}
	bi := BitmapInfo{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	bmp, _, e := createDIB.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		deleteDC.Call(dc)
		return fmt.Errorf("CreateDIBSection failed: %v", e)
	}
	old, _, _ := selectObject.Call(dc, bmp)
	*d = DIB{dc, bmp, old, bits, w, h}
	return nil
}
func renderFrame(p *PetWindow, frame BehaviorFrame) error {
	c := p.Cat
	x, y := int(math.Round(c.X)), int(math.Round(c.Y))
	key := fmt.Sprintf("%t:%p:%d:%d:%v:%v:%g:%g:%d:%d", frame.Action == "drag", p.Atlas, frame.Row, frame.Col, frame.Rect, frame.Canvas, frame.Anchor.X, frame.Anchor.Y, c.W, c.H)
	if key == p.LastKey && x == p.LastX && y == p.LastY {
		return nil
	}
	if e := p.DIB.Resize(c.W, c.H); e != nil {
		return e
	}
	if key != p.LastKey {
		pixels, ok := app.Frames[key]
		if !ok {
			pixels = RenderBehaviorPixels(p.Atlas, frame, c.W, c.H)
			if app.CachedBytes+len(pixels) > 32*1024*1024 {
				app.Frames = map[string][]byte{}
				app.CachedBytes = 0
			}
			if len(pixels) <= 32*1024*1024 {
				app.Frames[key] = pixels
				app.CachedBytes += len(pixels)
			}
		}
		copy(unsafe.Slice((*byte)(p.DIB.Bits), c.W*c.H*4), pixels)
	}
	dest := Point{int32(x), int32(y)}
	source := Point{}
	size := Size{int32(c.W), int32(c.H)}
	blend := Blend{ConstantAlpha: 255, AlphaFormat: 1}
	ok, _, e := updateLayered.Call(p.HWND, 0, uintptr(unsafe.Pointer(&dest)), uintptr(unsafe.Pointer(&size)), p.DIB.DC, uintptr(unsafe.Pointer(&source)), 0, uintptr(unsafe.Pointer(&blend)), 2)
	if ok == 0 {
		return fmt.Errorf("UpdateLayeredWindow failed: %v", e)
	}
	p.LastKey = key
	p.LastX = x
	p.LastY = y
	return nil
}
func nowSeconds() float64 { return time.Since(app.Start).Seconds() }
func setInterval() {
	dragging := false
	for _, p := range app.Pets {
		dragging = dragging || p.Down
	}
	busy := app.Engine != nil && app.Engine.Load != nil && app.Engine.Load.Active
	ms := RenderIntervalMS(app.Settings, busy, dragging, app.Hidden)
	if ms == 0 {
		killTimer.Call(app.Controller, 1)
		return
	}
	setTimer.Call(app.Controller, 1, uintptr(ms), 0)
}

func save() {
	if e := SaveSettings(app.SettingsPath, app.Settings); e != nil { /* Preferences are optional; the app still works in a restricted profile. */
	}
}
func updateBounds(p *PetWindow) {
	c := p.Cat
	c.Bounds = workArea(p.HWND, currentCursor())
	scale := dpi(p.HWND)
	w := app.Settings.Size * scale / 96
	h := w * p.CanvasH / p.CanvasW
	w, h = FitSize(w, h, c.Bounds)
	if w != c.W || h != c.H {
		c.W = w
		c.H = h
		p.LastKey = ""
	}
	c.X, c.Y = ClampPosition(c.X, c.Y, c.W, c.H, c.Bounds)
}
func reset() {
	resetCPUMonitor()
	p := currentCursor()
	r := workArea(0, p)
	for i, w := range app.Pets {
		if app.Engine != nil {
			app.Engine.Cancel(i, nowSeconds())
		}
		c := w.Cat
		c.Bounds = r
		c.X, c.Y = ClampPosition(float64(r.Left+30+i*(c.W+16)), float64(r.Bottom-c.H), c.W, c.H, r)
		c.Mode = "idle"
		c.NextDecision = nowSeconds() + 5 + float64(i)*3
		w.LastKey = ""
		setWindowPos.Call(w.HWND, ^uintptr(0), signed(int(c.X)), signed(int(c.Y)), uintptr(c.W), uintptr(c.H), 0x0010)
		updateBounds(w)
	}
	app.Hidden = false
	for _, w := range app.Pets {
		if e := renderFrame(w, InitialBehaviorFrame(w.Manifest)); e != nil {
			quit()
			showError(e.Error())
			return
		}
		showWindow.Call(w.HWND, 4)
	}
	setInterval()
}
func toggleHidden() {
	expireClickThrough(nowSeconds())
	resetCPUMonitor()
	app.Hidden = !app.Hidden
	for _, p := range app.Pets {
		v := uintptr(0)
		if !app.Hidden {
			v = 4
		}
		showWindow.Call(p.HWND, v)
	}
	app.LastTime = nowSeconds()
	setInterval()
}
func tick() {
	if app.Hidden || app.Quitting {
		return
	}
	now := nowSeconds()
	expireClickThrough(now)
	if app.Hidden {
		return
	}
	dt := math.Min(.2, now-app.LastTime)
	app.LastTime = now
	p, cursorValid := readCurrentCursor()
	if now-app.LastIdleCheck >= 1 {
		li := LastInput{Size: 8}
		if v, _, _ := getLastInput.Call(uintptr(unsafe.Pointer(&li))); v != 0 {
			t, _, _ := getTick.Call()
			app.Idle = float64(uint32(t)-li.Time) / 1000
		}
		app.LastIdleCheck = now
	}
	for _, w := range app.Pets {
		if w.Down {
			button := uintptr(1)
			if swapped, _, _ := getSystemMetrics.Call(23); swapped != 0 {
				button = 2
			}
			down, _, _ := getMouseButtonState.Call(button)
			if down&0x8000 == 0 {
				finishDrag(w)
			} else if cursorValid {
				dragMove(w, p)
			}
		}
	}
	if app.Engine == nil {
		return
	}
	pollCPU(now)
	cursorX, cursorY := float64(p.X), float64(p.Y)
	if !cursorValid {
		cursorX, cursorY = math.NaN(), math.NaN()
	}
	frames := app.Engine.Tick(now, dt, cursorX, cursorY, app.Idle, app.Settings.Quiet)
	for i, w := range app.Pets {
		if e := renderFrame(w, frames[i]); e != nil {
			quit()
			showError("桌面繪圖發生錯誤，程式將結束。\n" + e.Error())
			return
		}
	}

}
func addTray() bool {
	app.Tray = NotifyData{Size: uint32(unsafe.Sizeof(NotifyData{})), HWND: app.Controller, ID: 1, Flags: 1 | 2 | 4, Callback: wmTray, Icon: app.Icon}
	copy(app.Tray.Tip[:], syscall.StringToUTF16("三貓桌面陪伴 · 單擊顯示／隱藏，右鍵設定"))
	ok, _, _ := notifyIcon.Call(0, uintptr(unsafe.Pointer(&app.Tray)))
	return ok != 0
}
func menu() {
	h, _, _ := createMenu.Call()
	if h == 0 {
		return
	}
	defer destroyMenu.Call(h)
	add := func(id int, s string, check bool) {
		flags := uintptr(0)
		if check {
			flags = 8
		}
		appendMenu.Call(h, flags, uintptr(id), uintptr(unsafe.Pointer(utf(s))))
	}
	label := "隱藏貓咪"
	if app.Hidden {
		label = "顯示貓咪"
	}
	add(100, label, false)
	add(110, "設定：大小／活動／CPU", false)
	add(111, "暫時整隻點穿（5分鐘，從圖示可恢復）", app.ClickThrough)
	add(101, "安靜（停止自主活動）", app.Settings.Activity == ActivityQuiet)
	add(106, "一般活動", app.Settings.Activity == ActivityNormal)
	add(107, "活潑（更常探索與互動）", app.Settings.Activity == ActivityLively)
	add(108, "隨 CPU 負載踏踏／伸懶腰（需動作素材）", app.Settings.CPU.Enabled)
	add(109, "試玩小跑（未校準，可能滑步）", app.Settings.ExperimentalMovement)
	add(102, "把貓咪帶回滑鼠所在螢幕", false)
	add(105, "一起玩一下", false)
	appendMenu.Call(h, 0x800, 0, 0)
	add(196, "小貓咪", app.Settings.Size == 96)
	add(244, "中貓咪", app.Settings.Size == 144)
	add(292, "大貓咪", app.Settings.Size == 192)
	appendMenu.Call(h, 0x800, 0, 0)
	add(103, "關於與操作說明", false)
	add(104, "結束程式", false)
	p := currentCursor()
	setForeground.Call(app.Controller)
	id, _, _ := trackMenu.Call(h, 0x0100|0x0002, signed(int(p.X)), signed(int(p.Y)), 0, app.Controller, 0)
	postMessage.Call(app.Controller, 0, 0, 0)
	command(int(id))
}
func changeActivity(level ActivityLevel) {
	if !ValidActivity(level) {
		return
	}
	app.Settings.Activity = level
	app.Settings.Quiet = level == ActivityQuiet
	if app.Engine != nil {
		app.Engine.SetActivity(level)
	}
	save()
	setInterval()
	tick()
}
func command(id int) {
	switch id {
	case 100:
		toggleHidden()
	case 101:
		level := ActivityQuiet
		if app.Settings.Quiet {
			level = ActivityNormal
		}
		changeActivity(level)
	case 106:
		changeActivity(ActivityNormal)
	case 107:
		changeActivity(ActivityLively)
	case 108:
		app.Settings.CPU.Enabled = !app.Settings.CPU.Enabled
		if app.Engine != nil {
			app.Engine.SetCPUSettings(app.Settings.CPU)
		}
		resetCPUMonitor()
		save()
		setInterval()
		tick()
	case 109:
		app.Settings.ExperimentalMovement = !app.Settings.ExperimentalMovement
		if app.Engine != nil {
			app.Engine.SetExperimentalMovement(app.Settings.ExperimentalMovement)
		}
		save()
		tick()
	case 110:
		if err := openNativeSettings(app.Settings, applyAppSettings); err != nil {
			showError(err.Error())
		}
	case 111:
		if err := setClickThrough(!app.ClickThrough); err != nil {
			showError(err.Error())
		}
	case 102:
		reset()
	case 105:
		if app.Engine != nil {
			for i := range app.Pets {
				app.Engine.Cancel(i, nowSeconds())
				app.Engine.Play(i, nowSeconds())
			}
		}
		tick()
	case 196, 244, 292:
		app.Settings.Size = id - 100
		app.Frames = map[string][]byte{}
		app.CachedBytes = 0
		for _, p := range app.Pets {
			updateBounds(p)
			p.LastKey = ""
		}
		save()
		tick()
	case 103:
		specs := make([]CatSpec, len(app.Pets))
		for i, p := range app.Pets {
			specs[i] = p.Spec
		}
		messageBox.Call(app.Controller, uintptr(unsafe.Pointer(utf("三貓桌面陪伴 "+appVersion+"\n\n點一下：摸摸／揮手回應\n點兩下：玩一下\n按住拖曳：移動貓咪\n右鍵貓咪或右下角圖示：選單\n系統閒置約 3 分鐘：打盹\n活動選單：安靜／一般／活潑\n活潑會更常探索，仍可摸摸和拖曳\n新動作需有對應素材；跑步不會拿散步加速代替\n\n"+CharacterSummary(specs)+"\n\n完全離線，不擷取畫面、文字或按鍵。\n只讀滑鼠位置、拖曳時的滑鼠按鈕、系統閒置秒數與總體 CPU 計時。\nCPU 高負載踏踏：預設 70% 持續 10 秒，每 5 分鐘伸懶腰；需專屬動作素材。\n僅表示電腦負載，不代表使用者工作狀態，可在選單關閉。\n沒有自動開機啟動、廣告或更新下載。\n\n已通過 Windows 原生透明視窗與基本生命週期測試；多螢幕操作與最終素材仍待實機驗證。"))), uintptr(unsafe.Pointer(utf(appTitle))), 0x40)
	case 104:
		quit()
	}
}
func quit() {
	if app.Quitting {
		return
	}
	app.Quitting = true
	closeNativeSettings()
	killTimer.Call(app.Controller, 1)
	notifyIcon.Call(2, uintptr(unsafe.Pointer(&app.Tray)))
	for _, p := range app.Pets {
		destroyWindow.Call(p.HWND)
		p.DIB.Close()
	}
	postQuit.Call(0)
}
func windowProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if hwnd == app.Controller && app.TaskbarMessage != 0 && msg == app.TaskbarMessage {
		if !addTray() {
			quit()
			showError("無法重新建立通知區圖示。請重新開啟程式。")
		}
		return 0
	}
	if hwnd == app.Controller {
		switch msg {
		case 0x0218: // WM_POWERBROADCAST
			if wp == 4 || wp == 7 || wp == 18 {
				resetCPUMonitor()
				app.LastTime = nowSeconds()
				setInterval()
				return 1
			}
		case 0x0111:
			command(int(wp & 0xffff))
			return 0
		case 0x0113:
			tick()
			return 0
		case wmTray:
			switch uint32(lp) {
			case 0x0205, 0x007B:
				menu()
			case 0x0202:
				now := nowSeconds()
				if app.TrayLastClick == 0 || now-app.TrayLastClick >= .35 {
					app.TrayLastClick = now
					toggleHidden()
				}
			}
			return 0
		case wmWake:
			if app.Hidden {
				toggleHidden()
			}
			reset()
			return 0
		case 0x007E, 0x001A:
			for _, p := range app.Pets {
				updateBounds(p)
				p.LastKey = ""
			}
			tick()
			return 0
		case 0x0010:
			quit()
			return 0
		case 0x0011:
			return 1
		case 0x0016:
			if wp != 0 {
				quit()
			}
			return 0
		}
	}
	p := app.Windows[hwnd]
	if p != nil {
		switch msg {
		case 0x0021:
			return 3 // MA_NOACTIVATE: never steal typing focus.
		case 0x0084:
			if app.ClickThrough {
				return ^uintptr(0)
			}
			return 1 // Native layered-window alpha hit testing excludes zero-alpha pixels.
		case 0x0201:
			q := currentCursor()
			p.Down = true
			p.Cat.Dragging = true
			if app.Engine != nil {
				app.Engine.Cancel(p.Index, nowSeconds())
			}
			p.DidDrag = false
			p.Press = q
			p.Offset = Point{q.X - int32(p.Cat.X), q.Y - int32(p.Cat.Y)}
			setCapture.Call(hwnd)
			setInterval()
			return 0
		case 0x0200:
			if p.Down {
				dragMove(p, currentCursor())
			}
			return 0
		case 0x0202:
			if p.Down {
				finishDrag(p)
			}
			return 0
		case 0x001F:
			p.Down = false
			p.Cat.Dragging = false
			releaseCapture.Call()
			setInterval()
			return 0
		case 0x0215:
			p.Down = false
			p.Cat.Dragging = false
			setInterval()
			return 0
		case 0x0203:
			if app.Engine != nil {
				app.Engine.Cancel(p.Index, nowSeconds())
				app.Engine.Play(p.Index, nowSeconds())
			}
			return 0
		case 0x0205:
			menu()
			return 0
		case 0x02E0:
			c := p.Cat
			oldW, oldH := c.W, c.H
			scale := int(wp & 0xffff)
			if scale < 48 || scale > 768 {
				scale = 96
			}
			w := app.Settings.Size * scale / 96
			h := w * p.CanvasH / p.CanvasW
			if p.Down {
				c.Bounds = workArea(0, currentCursor())
			} else if lp != 0 {
				r := (*WinRect)(unsafe.Pointer(lp))
				c.Bounds = workArea(0, Point{r.Left + (r.Right-r.Left)/2, r.Top + (r.Bottom-r.Top)/2})
			} else {
				c.Bounds = workArea(hwnd, currentCursor())
			}
			w, h = FitSize(w, h, c.Bounds)
			c.W, c.H = w, h
			if p.Down {
				p.Offset.X = int32(float64(p.Offset.X) * float64(w) / float64(oldW))
				p.Offset.Y = int32(float64(p.Offset.Y) * float64(h) / float64(oldH))
				dragMove(p, currentCursor())
			} else if lp != 0 {
				r := (*WinRect)(unsafe.Pointer(lp))
				c.X, c.Y = ClampPosition(float64(r.Left), float64(r.Top), w, h, c.Bounds)
			}
			p.LastKey = ""
			return 0
		case 0x0010:
			toggleHidden()
			return 0
		}
	}
	v, _, _ := defWindow.Call(hwnd, uintptr(msg), wp, lp)
	return v
}
func dragMove(p *PetWindow, q Point) {
	if abs32(q.X-p.Press.X)+abs32(q.Y-p.Press.Y) > 5 {
		p.DidDrag = true
		p.Cat.Dragging = true
	}
	if p.DidDrag {
		p.Cat.Bounds = workArea(0, q)
		p.Cat.X, p.Cat.Y = ClampPosition(float64(q.X-p.Offset.X), float64(q.Y-p.Offset.Y), p.Cat.W, p.Cat.H, p.Cat.Bounds)
	}
}
func finishDrag(p *PetWindow) {
	dragged := p.DidDrag
	p.Down = false
	p.Cat.Dragging = false
	releaseCapture.Call()
	updateBounds(p)
	if app.Engine != nil {
		app.Engine.Pet(p.Index, nowSeconds())
	} else {
		p.Cat.Pet(nowSeconds())
	}
	if dragged {
		p.Cat.PetUntil = nowSeconds() + .5
	}
	setInterval()
}
func abs32(n int32) int32 {
	if n < 0 {
		return -n
	}
	return n
}
func main() {
	runtime.LockOSThread()
	app.Start = time.Now()
	app.Windows = map[uintptr]*PetWindow{}
	app.Frames = map[string][]byte{}
	app.Instance, _, _ = moduleHandle.Call(0)
	if setDPIContext.Find() == nil {
		setDPIContext.Call(^uintptr(3))
	}
	var err error
	exe, err := os.Executable()
	if err != nil {
		showError(err.Error())
		return
	}
	app.Folder = filepath.Dir(exe)
	app.Mutex, _, err = createMutex.Call(0, 0, uintptr(unsafe.Pointer(utf("Local\\ThreeCatCompanion-2026"))))
	if app.Mutex == 0 {
		showError("無法啟動單一執行個體。\n" + err.Error())
		return
	}
	defer closeHandle.Call(app.Mutex)
	if err == syscall.Errno(183) {
		h, _, _ := findWindow.Call(uintptr(unsafe.Pointer(utf("ThreeCatCompanionController"))), 0)
		if h != 0 {
			postMessage.Call(h, wmWake, 0, 0)
		}
		return
	}
	cfg, e := LoadConfig(app.Folder)
	if e != nil {
		showError("cats.json 設定無法讀取，請修正後再開啟。\n" + e.Error())
		return
	}
	var base *Atlas
	needDemo := false
	for _, s := range cfg.Cats {
		if s.Sprite == "" {
			needDemo = true
		}
	}
	if needDemo {
		demoPNG, _ := embeddedAssets.ReadFile("assets/spritesheet-extended.png")
		base, e = DecodeAtlas(bytes.NewReader(demoPNG))
		if e != nil {
			showError("未找到有效示範素材。請使用含素材的私人 ZIP，或在 cats.json 設定自己的合法素材。\n" + e.Error())
			return
		}
	}
	confDir, e := os.UserConfigDir()
	if e != nil {
		confDir = app.Folder
	}
	app.SettingsPath = filepath.Join(confDir, "ThreeCatCompanion", "settings.json")
	app.Settings = loadSettingsAtSize(app.SettingsPath, cfg.DefaultSize, cfg.ExperimentalMovement)
	cursor, _, _ := loadCursor.Call(0, 32512)
	app.Icon, _, _ = loadIcon.Call(0, 32512)
	proc := syscall.NewCallback(windowProc)
	for _, name := range []string{"ThreeCatCompanionController", "ThreeCatCompanionPet"} {
		wc := WndClass{Style: 8, Size: uint32(unsafe.Sizeof(WndClass{})), WndProc: proc, Instance: app.Instance, Cursor: cursor, Icon: app.Icon, ClassName: utf(name)}
		if v, _, e := registerClass.Call(uintptr(unsafe.Pointer(&wc))); v == 0 {
			showError("無法建立視窗類別。\n" + e.Error())
			return
		}
	}
	app.Controller, _, e = createWindow.Call(0, uintptr(unsafe.Pointer(utf("ThreeCatCompanionController"))), uintptr(unsafe.Pointer(utf(appTitle))), 0, 0, 0, 0, 0, 0, 0, app.Instance, 0)
	if app.Controller == 0 {
		showError("無法建立控制視窗。\n" + e.Error())
		return
	}
	defer destroyWindow.Call(app.Controller)
	taskbar, _, _ := registerMessage.Call(uintptr(unsafe.Pointer(utf("TaskbarCreated"))))
	app.TaskbarMessage = uint32(taskbar)
	if !addTray() {
		showError("無法建立右下角通知區圖示。為避免無法結束，程式不會啟動。")
		return
	}
	defer notifyIcon.Call(2, uintptr(unsafe.Pointer(&app.Tray)))
	cache := map[string]*Atlas{"": base}
	r := workArea(0, currentCursor())
	for i, spec := range cfg.Cats {
		assetKey := spec.Sprite + "|" + spec.Animations
		atlas := cache[assetKey]
		if spec.Sprite == "" {
			atlas = base
		}
		if atlas == nil {
			f, e := OpenLocalAsset(app.Folder, spec.Sprite)
			if e != nil {
				showError(spec.Name + " 圖片無法開啟。\n" + e.Error())
				quit()
				return
			}
			atlas, e = DecodeSprite(f, spec.Animations == "")
			f.Close()
			if e != nil {
				showError(spec.Name + " 圖片格式錯誤。\n" + e.Error())
				quit()
				return
			}
			cache[assetKey] = atlas
		}
		manifest := DefaultAnimationManifest()
		canvasW, canvasH := atlas.CellW, atlas.CellH
		if spec.Animations != "" {
			f, err := OpenLocalAsset(app.Folder, spec.Animations)
			if err != nil {
				quit()
				showError(err.Error())
				return
			}
			manifest, e = LoadAnimationManifest(f, atlas.Image.Bounds().Dx(), atlas.Image.Bounds().Dy())
			f.Close()
			if e != nil {
				quit()
				showError("動作設定錯誤：" + e.Error())
				return
			}
			if e := ValidateManifestPixels(manifest, atlas); e != nil {
				quit()
				showError("動畫畫格錯誤：" + e.Error())
				return
			}
			canvasW, canvasH = ManifestCanvas(manifest, atlas)
		}
		w := app.Settings.Size
		h := w * canvasH / canvasW
		w, h = FitSize(w, h, r)
		c := NewCat(i, w, h, r)
		// Unit simulations use NewCat's fixed seed; each real launch varies choices.
		c.Seed = rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)*7919))
		p := &PetWindow{Cat: c, Atlas: atlas, Spec: spec, Manifest: manifest, Index: i, CanvasW: canvasW, CanvasH: canvasH, LastX: -99999, LastY: -99999}
		p.HWND, _, e = createWindow.Call(0x00080000|0x00000080|0x08000000|0x00000008, uintptr(unsafe.Pointer(utf("ThreeCatCompanionPet"))), uintptr(unsafe.Pointer(utf(spec.Name))), 0x80000000, signed(int(c.X)), signed(int(c.Y)), uintptr(w), uintptr(h), 0, 0, app.Instance, 0)
		if p.HWND == 0 {
			showError("無法建立貓咪視窗。\n" + e.Error())
			quit()
			return
		}
		app.Pets = append(app.Pets, p)
		app.Windows[p.HWND] = p
		updateBounds(p)
		if e = renderFrame(p, InitialBehaviorFrame(manifest)); e != nil {
			showError(e.Error())
			quit()
			return
		}
		showWindow.Call(p.HWND, 4)
	}
	cats := make([]*Cat, len(app.Pets))
	for i, p := range app.Pets {
		cats[i] = p.Cat
	}
	app.Engine = NewBehaviorEngine(cats, DefaultAnimationManifest())
	app.Engine.SetActivity(app.Settings.Activity)
	app.Engine.SetCPUSettings(app.Settings.CPU)
	app.Engine.SetExperimentalMovement(app.Settings.ExperimentalMovement)
	for i, p := range app.Pets {
		app.Engine.SetManifest(i, p.Manifest)
		if p.Spec.Temperament != nil {
			app.Engine.SetTemperament(i, *p.Spec.Temperament)
		}
	}
	setInterval()
	var msg Message
	for {
		ret, _, e := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) == -1 {
			showError("Windows 訊息迴圈錯誤。\n" + e.Error())
			quit()
			break
		}
		if ret == 0 {
			break
		}
		if nativeSettingsMessage(&msg) {
			continue
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	for _, p := range app.Pets {
		p.DIB.Close()
	}
}
