//go:build windows && amd64

package main

import (
	"testing"
	"unsafe"
)

// These bidirectional array bounds fail at compile time if Go's Windows x64
// struct layout ever differs from the ABI. They are checked by `go test -c`
// even on a non-Windows cross-compilation host.
var (
	_ [unsafe.Sizeof(WndClass{}) - 80]byte
	_ [80 - unsafe.Sizeof(WndClass{})]byte
	_ [unsafe.Sizeof(Message{}) - 48]byte
	_ [48 - unsafe.Sizeof(Message{})]byte
	_ [unsafe.Sizeof(NotifyData{}) - 976]byte
	_ [976 - unsafe.Sizeof(NotifyData{})]byte
	_ [unsafe.Sizeof(MonitorInfo{}) - 40]byte
	_ [40 - unsafe.Sizeof(MonitorInfo{})]byte
	_ [unsafe.Sizeof(BitmapInfo{}) - 40]byte
	_ [40 - unsafe.Sizeof(BitmapInfo{})]byte
	_ [unsafe.Sizeof(Blend{}) - 4]byte
	_ [4 - unsafe.Sizeof(Blend{})]byte
	_ [unsafe.Sizeof(LastInput{}) - 8]byte
	_ [8 - unsafe.Sizeof(LastInput{})]byte
	_ [unsafe.Sizeof(Point{}) - 8]byte
	_ [8 - unsafe.Sizeof(Point{})]byte
	_ [unsafe.Sizeof(Size{}) - 8]byte
	_ [8 - unsafe.Sizeof(Size{})]byte
	_ [unsafe.Sizeof(WinRect{}) - 16]byte
	_ [16 - unsafe.Sizeof(WinRect{})]byte
	_ [unsafe.Offsetof(WndClass{}.WndProc) - 8]byte
	_ [8 - unsafe.Offsetof(WndClass{}.WndProc)]byte
	_ [unsafe.Offsetof(WndClass{}.ClassName) - 64]byte
	_ [64 - unsafe.Offsetof(WndClass{}.ClassName)]byte
	_ [unsafe.Offsetof(Message{}.WParam) - 16]byte
	_ [16 - unsafe.Offsetof(Message{}.WParam)]byte
	_ [unsafe.Offsetof(Message{}.LParam) - 24]byte
	_ [24 - unsafe.Offsetof(Message{}.LParam)]byte
	_ [unsafe.Offsetof(Message{}.Pt) - 36]byte
	_ [36 - unsafe.Offsetof(Message{}.Pt)]byte
	_ [unsafe.Offsetof(NotifyData{}.HWND) - 8]byte
	_ [8 - unsafe.Offsetof(NotifyData{}.HWND)]byte
	_ [unsafe.Offsetof(NotifyData{}.Icon) - 32]byte
	_ [32 - unsafe.Offsetof(NotifyData{}.Icon)]byte
	_ [unsafe.Offsetof(NotifyData{}.Tip) - 40]byte
	_ [40 - unsafe.Offsetof(NotifyData{}.Tip)]byte
	_ [unsafe.Offsetof(NotifyData{}.GUID) - 952]byte
	_ [952 - unsafe.Offsetof(NotifyData{}.GUID)]byte
	_ [unsafe.Offsetof(NotifyData{}.Balloon) - 968]byte
	_ [968 - unsafe.Offsetof(NotifyData{}.Balloon)]byte
)

func TestWindowsAMD64ABILayout(t *testing.T) {
	// Keeping this as a named test documents that runtime execution on Windows
	// remains separate from the cross-host compile-time ABI checks above.
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Fatal("this build is intended for Windows x64")
	}
}
