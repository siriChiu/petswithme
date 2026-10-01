//go:build windows

package main

import (
	"testing"
	"time"
	"unsafe"
)

func TestWindowsAggregateCPUSampler(t *testing.T) {
	if unsafe.Sizeof(cpuFileTime{}) != 8 || unsafe.Offsetof(cpuFileTime{}.High) != 4 {
		t.Fatal("FILETIME ABI mismatch")
	}
	if got := (cpuFileTime{Low: 0x12345678, High: 0x9abcdef0}).ticks(); got != 0x9abcdef012345678 {
		t.Fatal("FILETIME combination")
	}
	groups, _, _ := getActiveProcessorGroupCount.Call()
	first, ok := readSystemCPU()
	if groups != 1 {
		if ok {
			t.Fatal("partial processor group was called aggregate CPU")
		}
		t.Logf("CPU sampling intentionally unavailable on %d processor groups; ordinary behavior remains enabled", groups)
		return
	}
	if !ok {
		t.Fatal("GetSystemTimes failed on one processor group")
	}
	time.Sleep(50 * time.Millisecond)
	second, ok := readSystemCPU()
	if !ok {
		t.Fatal("second GetSystemTimes failed")
	}
	value, valid := CPUPercent(first, second)
	if !valid || value < 0 || value > 100 {
		t.Fatalf("invalid native CPU delta %+v -> %+v: %g %v", first, second, value, valid)
	}
	t.Logf("NATIVE_AGGREGATE_CPU_PASSED: one processor group, valid monotonic system counters, %.2f%% aggregate utilization", value)
}
