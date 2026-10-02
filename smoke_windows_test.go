//go:build windows && amd64

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// These integration tests use only synthetic squares. They do not load cat art,
// launch the app, touch preferences, install a tray icon, or inject user input.
var (
	smokeUnregisterClass          = user32.NewProc("UnregisterClassW")
	smokeIsWindow                 = user32.NewProc("IsWindow")
	smokeIsVisible                = user32.NewProc("IsWindowVisible")
	smokeGetWindowRect            = user32.NewProc("GetWindowRect")
	smokeGetWindowLong            = user32.NewProc("GetWindowLongPtrW")
	smokeGetForeground            = user32.NewProc("GetForegroundWindow")
	smokeWindowFromPoint          = user32.NewProc("WindowFromPoint")
	smokePeekMessage              = user32.NewProc("PeekMessageW")
	smokeOpenInputDesktop         = user32.NewProc("OpenInputDesktop")
	smokeCloseDesktop             = user32.NewProc("CloseDesktop")
	smokeGetProcessWindowStation  = user32.NewProc("GetProcessWindowStation")
	smokeGetUserObjectInformation = user32.NewProc("GetUserObjectInformationW")
	smokeSetThreadDPI             = user32.NewProc("SetThreadDpiAwarenessContext")
)

const (
	smokeWidth      = 96
	smokeHeight     = 96
	smokeNoActivate = uintptr(0x08000000)
	smokeToolWindow = uintptr(0x00000080)
	smokeTopmost    = uintptr(0x00000008)
	smokeLayered    = uintptr(0x00080000)
)

type smokeFixture struct {
	instance, probe, pet, previousDPI uintptr
	className                         *uint16
	dib                               DIB
	x, y                              int32
}

func smokeWndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0021: // WM_MOUSEACTIVATE
		return 3 // MA_NOACTIVATE
	case 0x0084: // WM_NCHITTEST; zero-alpha exclusion is supplied by Windows.
		return 1 // HTCLIENT
	}
	result, _, _ := defWindow.Call(hwnd, uintptr(msg), wp, lp)
	return result
}

func (f *smokeFixture) close() {
	if f.pet != 0 {
		destroyWindow.Call(f.pet)
		f.pet = 0
	}
	if f.probe != 0 {
		destroyWindow.Call(f.probe)
		f.probe = 0
	}
	f.dib.Close()
	if f.className != nil {
		smokeUnregisterClass.Call(uintptr(unsafe.Pointer(f.className)), f.instance)
	}
	if f.previousDPI != 0 {
		smokeSetThreadDPI.Call(f.previousDPI)
	}
}

func newSmokeFixture() (_ *smokeFixture, err error) {
	f := &smokeFixture{}
	defer func() {
		if err != nil {
			f.close()
		}
	}()
	f.instance, _, _ = moduleHandle.Call(0)
	if f.instance == 0 {
		return nil, fmt.Errorf("GetModuleHandleW returned NULL")
	}
	f.previousDPI, _, err = smokeSetThreadDPI.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2
	if f.previousDPI == 0 {
		return nil, fmt.Errorf("SetThreadDpiAwarenessContext: %v", err)
	}
	f.className = syscall.StringToUTF16Ptr(fmt.Sprintf("ThreeCatCompanionSmoke_%d", time.Now().UnixNano()))
	wc := WndClass{Size: uint32(unsafe.Sizeof(WndClass{})), WndProc: syscall.NewCallback(smokeWndProc), Instance: f.instance, ClassName: f.className}
	if result, _, callErr := registerClass.Call(uintptr(unsafe.Pointer(&wc))); result == 0 {
		return nil, fmt.Errorf("RegisterClassExW: %v", callErr)
	}
	monitor, _, callErr := monitorFromPoint.Call(0, 2)
	if monitor == 0 {
		return nil, fmt.Errorf("MonitorFromPoint: %v", callErr)
	}
	mi := MonitorInfo{Size: uint32(unsafe.Sizeof(MonitorInfo{}))}
	if result, _, callErr := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&mi))); result == 0 {
		return nil, fmt.Errorf("GetMonitorInfoW: %v", callErr)
	}
	if mi.Work.Right-mi.Work.Left < smokeWidth || mi.Work.Bottom-mi.Work.Top < smokeHeight {
		return nil, fmt.Errorf("monitor work area is too small for 96x96 test: %+v", mi.Work)
	}
	f.x = mi.Work.Left + (mi.Work.Right-mi.Work.Left-smokeWidth)/2
	f.y = mi.Work.Top + (mi.Work.Bottom-mi.Work.Top-smokeHeight)/2
	styles := smokeNoActivate | smokeToolWindow | smokeTopmost
	f.probe, _, callErr = createWindow.Call(styles, uintptr(unsafe.Pointer(f.className)), 0, 0x80000000, signed(int(f.x)), signed(int(f.y)), smokeWidth, smokeHeight, 0, 0, f.instance, 0)
	if f.probe == 0 {
		return nil, fmt.Errorf("CreateWindowExW(probe): %v", callErr)
	}
	f.pet, _, callErr = createWindow.Call(styles|smokeLayered, uintptr(unsafe.Pointer(f.className)), 0, 0x80000000, signed(int(f.x)), signed(int(f.y)), smokeWidth, smokeHeight, 0, 0, f.instance, 0)
	if f.pet == 0 {
		return nil, fmt.Errorf("CreateWindowExW(layered pet): %v", callErr)
	}
	if err = f.dib.Resize(smokeWidth, smokeHeight); err != nil {
		return nil, err
	}
	frame, err := smokeSyntheticFrame()
	if err != nil {
		return nil, err
	}
	copy(unsafe.Slice((*byte)(f.dib.Bits), smokeWidth*smokeHeight*4), frame)
	position, source := Point{f.x, f.y}, Point{}
	size := Size{smokeWidth, smokeHeight}
	blend := Blend{ConstantAlpha: 255, AlphaFormat: 1}
	if result, _, callErr := updateLayered.Call(f.pet, 0, uintptr(unsafe.Pointer(&position)), uintptr(unsafe.Pointer(&size)), f.dib.DC, uintptr(unsafe.Pointer(&source)), 0, uintptr(unsafe.Pointer(&blend)), 2); result == 0 {
		return nil, fmt.Errorf("UpdateLayeredWindow(ULW_ALPHA): %v", callErr)
	}
	showWindow.Call(f.probe, 4) // SW_SHOWNOACTIVATE
	showWindow.Call(f.pet, 4)
	if result, _, callErr := setWindowPos.Call(f.pet, ^uintptr(0), signed(int(f.x)), signed(int(f.y)), smokeWidth, smokeHeight, 0x0010); result == 0 {
		return nil, fmt.Errorf("SetWindowPos(HWND_TOPMOST, SWP_NOACTIVATE): %v", callErr)
	}
	smokePumpMessages()
	return f, nil
}

func smokeSyntheticFrame() ([]byte, error) {
	im := image.NewNRGBA(image.Rect(0, 0, 64, 88))
	for row, count := range rowFrames {
		for col := 0; col < count; col++ {
			for y := 2; y < 6; y++ {
				for x := 2; x < 6; x++ {
					im.SetNRGBA(col*8+x, row*8+y, color.NRGBA{R: 72, G: 120, B: 208, A: 255})
				}
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, im); err != nil {
		return nil, err
	}
	atlas, err := DecodeAtlas(&encoded)
	if err != nil {
		return nil, err
	}
	return atlas.FrameBGRA(0, 0, smokeWidth, smokeHeight), nil
}

func smokePumpMessages() {
	var msg Message
	for {
		result, _, _ := smokePeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
		if result == 0 {
			return
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func TestWindowsSmokeLayeredWindowLifecycle(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f, err := newSmokeFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()                                             // Destroy and unregister on the same locked OS thread.
	styles, _, _ := smokeGetWindowLong.Call(f.pet, signed(-20)) // GWL_EXSTYLE
	want := smokeLayered | smokeNoActivate | smokeToolWindow | smokeTopmost
	if styles&want != want {
		t.Fatalf("extended styles %#x do not contain %#x", styles, want)
	}
	foreground, _, _ := smokeGetForeground.Call()
	if foreground == f.pet || foreground == f.probe {
		t.Fatal("non-activating windows took foreground focus")
	}
	var rect WinRect
	if result, _, callErr := smokeGetWindowRect.Call(f.pet, uintptr(unsafe.Pointer(&rect))); result == 0 {
		t.Fatalf("GetWindowRect: %v", callErr)
	}
	if rect.Left != f.x || rect.Top != f.y || rect.Right-rect.Left != smokeWidth || rect.Bottom-rect.Top != smokeHeight {
		t.Fatalf("unexpected physical window rectangle: %+v at (%d,%d)", rect, f.x, f.y)
	}
	if visible, _, _ := smokeIsVisible.Call(f.pet); visible == 0 {
		t.Fatal("pet was not shown")
	}
	showWindow.Call(f.pet, 0)
	if visible, _, _ := smokeIsVisible.Call(f.pet); visible != 0 {
		t.Fatal("pet remained visible after hide")
	}
	showWindow.Call(f.pet, 4)
	if visible, _, _ := smokeIsVisible.Call(f.pet); visible == 0 {
		t.Fatal("pet was not shown again")
	}
	foreground, _, _ = smokeGetForeground.Call()
	if foreground == f.pet || foreground == f.probe {
		t.Fatal("showing the pet again took foreground focus")
	}
	pet := f.pet
	if result, _, callErr := destroyWindow.Call(pet); result == 0 {
		t.Fatalf("DestroyWindow: %v", callErr)
	}
	f.pet = 0
	if exists, _, _ := smokeIsWindow.Call(pet); exists != 0 {
		t.Fatal("window still exists after DestroyWindow")
	}
	f.dib.Close()
	if f.dib.DC != 0 || f.dib.Bitmap != 0 || f.dib.Bits != nil {
		t.Fatal("DIB resources were not cleared")
	}
	t.Log("Native Windows API lifecycle passed: PNG→premultiplied BGRA→DIB→UpdateLayeredWindow, styles, physical bounds, no activation, hide/show, destruction")
}

func smokeInteractiveDesktop() (bool, string) {
	station, _, callErr := smokeGetProcessWindowStation.Call()
	if station == 0 {
		return false, fmt.Sprintf("GetProcessWindowStation: %v", callErr)
	}
	var flags struct{ Inherit, Reserved, Flags uint32 }
	var needed uint32
	if result, _, callErr := smokeGetUserObjectInformation.Call(station, 1, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags), uintptr(unsafe.Pointer(&needed))); result == 0 {
		return false, fmt.Sprintf("cannot inspect window station: %v", callErr)
	}
	if flags.Flags&1 == 0 {
		return false, "process window station is not visible (non-interactive Windows session)"
	}
	desktop, _, callErr := smokeOpenInputDesktop.Call(0, 0, 1) // DESKTOP_READOBJECTS
	if desktop == 0 {
		return false, fmt.Sprintf("no accessible input desktop: %v", callErr)
	}
	smokeCloseDesktop.Call(desktop)
	return true, ""
}

func TestWindowsSmokePerPixelAlphaHitTesting(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if ok, reason := smokeInteractiveDesktop(); !ok {
		t.Skipf("Interactive per-pixel hit test not run: %s. Native lifecycle test runs separately without this skip.", reason)
	}
	f, err := newSmokeFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	pointWindow := func(x, y int32) uintptr {
		packed := uint64(uint32(x)) | uint64(uint32(y))<<32
		hwnd, _, _ := smokeWindowFromPoint.Call(uintptr(packed))
		return hwnd
	}
	if got := pointWindow(f.x+smokeWidth/2, f.y+smokeHeight/2); got != f.pet {
		t.Fatalf("opaque synthetic square should hit pet %#x; got %#x (probe %#x)", f.pet, got, f.probe)
	}
	if got := pointWindow(f.x+6, f.y+6); got != f.probe {
		t.Fatalf("zero-alpha area should pass through to probe %#x; got %#x (pet %#x)", f.probe, got, f.pet)
	}
	showWindow.Call(f.pet, 0)
	if got := pointWindow(f.x+smokeWidth/2, f.y+smokeHeight/2); got != f.probe {
		t.Fatalf("hidden pet should expose probe %#x; got %#x", f.probe, got)
	}
	t.Log("Native Windows WindowFromPoint distinguishes opaque pixels, transparent pixels, and hidden pet without input injection")
}
