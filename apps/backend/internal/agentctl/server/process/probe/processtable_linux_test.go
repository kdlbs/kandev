//go:build linux

package probe

import (
	"testing"
	"time"
)

func TestLinuxBootTimeForClockDetectsTimeNamespaceOffset(t *testing.T) {
	now := time.Unix(1_790_000_000, 250_000_000)
	wallBootTime := time.Unix(1_780_000_000, 0)
	uptime := 20 * time.Minute
	pid1StartTime := now.Sub(wallBootTime) - uptime

	got := linuxBootTimeForClock(now, uptime, wallBootTime, pid1StartTime)
	if want := wallBootTime; !got.Equal(want) {
		t.Fatalf("time-namespace boot time = %s, want %s", got, want)
	}
}

func TestLinuxBootTimeForClockKeepsUptimeClockWhenNoNamespaceOffset(t *testing.T) {
	now := time.Unix(1_790_000_000, 250_000_000)
	uptime := 20 * time.Minute
	wallBootTime := now.Add(-uptime - 400*time.Millisecond)
	pid1StartTime := 10 * time.Minute

	got := linuxBootTimeForClock(now, uptime, wallBootTime, pid1StartTime)
	if want := now.Add(-uptime); !got.Equal(want) {
		t.Fatalf("boot time = %s, want uptime-derived %s", got, want)
	}
}
