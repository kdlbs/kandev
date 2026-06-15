//go:build darwin

package metrics

import (
	"testing"
)

func TestDarwinReadCPUTimes(t *testing.T) {
	got, err := readCPUTimes(NewCollector())
	if err != nil {
		t.Fatalf("readCPUTimes: %v", err)
	}
	if got.total == 0 {
		t.Fatal("expected total cpu ticks > 0")
	}
	if got.idle > got.total {
		t.Fatalf("idle=%d > total=%d", got.idle, got.total)
	}
}

func TestDarwinCPUPercentNonNegative(t *testing.T) {
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

func TestDarwinLoadAvg1(t *testing.T) {
	v, err := darwinLoadAvg1()
	if err != nil {
		t.Fatalf("darwinLoadAvg1: %v", err)
	}
	if v < 0 {
		t.Fatalf("loadavg=%v, want >= 0", v)
	}
}

func TestDarwinMemoryPercent(t *testing.T) {
	v, err := darwinMemoryPercent()
	if err != nil {
		t.Fatalf("darwinMemoryPercent: %v", err)
	}
	if v < 0 || v > 100 {
		t.Fatalf("memory percent=%v, want 0..100", v)
	}
}

func TestDarwinCPUTempUnavailable(t *testing.T) {
	if _, err := NewCollector().cpuTempValue(); err == nil {
		t.Fatal("expected cpu temperature to be unavailable on darwin")
	}
}
