//go:build windows

package metrics

import "testing"

func TestWindowsReadCPUTimes(t *testing.T) {
	got, err := readCPUTimes(NewCollector())
	if err != nil {
		t.Fatalf("readCPUTimes: %v", err)
	}
	if got.total == 0 {
		t.Fatal("expected total cpu time > 0")
	}
	if got.idle > got.total {
		t.Fatalf("idle=%d > total=%d", got.idle, got.total)
	}
}

func TestWindowsCPUPercentNonNegative(t *testing.T) {
	c := NewCollector()
	if _, err := c.cpuPercent(); err != nil {
		t.Fatalf("first cpuPercent (baseline): %v", err)
	}
	v, err := c.cpuPercent()
	if err != nil {
		t.Fatalf("second cpuPercent: %v", err)
	}
	if v < 0 || v > 100 {
		t.Fatalf("cpu percent=%v, want 0..100", v)
	}
}

func TestWindowsMemoryPercent(t *testing.T) {
	v, err := NewCollector().memoryPercent()
	if err != nil {
		t.Fatalf("memoryPercent: %v", err)
	}
	if v < 0 || v > 100 {
		t.Fatalf("memory percent=%v, want 0..100", v)
	}
}

func TestWindowsLoadAvgUnavailable(t *testing.T) {
	if _, err := NewCollector().ioLoadValue(); err == nil {
		t.Fatal("expected load average to be unavailable on windows")
	}
}

func TestWindowsCPUTempUnavailable(t *testing.T) {
	if _, err := NewCollector().cpuTempValue(); err == nil {
		t.Fatal("expected cpu temperature to be unavailable on windows")
	}
}
