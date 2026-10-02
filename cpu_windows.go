//go:build windows

package main

import "unsafe"

var getSystemTimes = kernel32.NewProc("GetSystemTimes")
var getActiveProcessorGroupCount = kernel32.NewProc("GetActiveProcessorGroupCount")

type cpuFileTime struct{ Low, High uint32 }

func (f cpuFileTime) ticks() uint64 { return uint64(f.High)<<32 | uint64(f.Low) }

func readSystemCPU() (CPUCounters, bool) {
	// GetSystemTimes only covers the caller's primary group on a multi-group
	// machine. Do not label that partial reading as whole-machine utilization.
	groups, _, _ := getActiveProcessorGroupCount.Call()
	if groups != 1 {
		return CPUCounters{}, false
	}
	var idle, kernel, user cpuFileTime
	ok, _, _ := getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if ok == 0 {
		return CPUCounters{}, false
	}
	return CPUCounters{idle.ticks(), kernel.ticks(), user.ticks()}, true
}
func resetCPUMonitor() {
	app.CPU.Reset()
	if app.Engine != nil {
		app.Engine.ResetCPU()
	}
}
func pollCPU(now float64) {
	if app.Engine == nil {
		return
	}
	old := app.Engine.Load != nil && app.Engine.Load.Active
	value, valid, observed := app.CPU.Poll(now, app.Settings.CPU.Enabled, app.Hidden, readSystemCPU)
	if observed {
		app.Engine.ObserveCPU(now, value, valid)
	}
	if old != (app.Engine.Load != nil && app.Engine.Load.Active) {
		setInterval()
	}
}
