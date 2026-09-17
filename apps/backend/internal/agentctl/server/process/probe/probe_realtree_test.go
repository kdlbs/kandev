package probe

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// Real-process-tree coverage for AC-70/70a/71/72/80, driven against this
// test binary's own real subprocess tree rather than a synthetic table.
// Gated at runtime (not by build tag) per round-5 F12's disposition, so the
// suite still compiles and shows up as explicitly skipped in CI output:
// Linux runs for real in CI; Darwin only when a developer runs it locally
// on a Mac (this repo's CI has no macOS runner).
func skipUnlessRealTreeSupported(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("probe: no real-process-tree reader implemented for this platform")
	}
}

// startRealChild spawns a real, long-lived child of the current test
// process and returns it; the caller's t.Cleanup kills and reaps it. `sleep`
// is hermetic (no repo binary dependency) and present on both platforms
// this suite runs on.
func startRealChild(t *testing.T, ownProcessGroup bool) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	if ownProcessGroup {
		withOwnProcessGroup(cmd)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn real child: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd
}

// AC-70: a descendant that predates the recorded turn start by a wide
// margin settles.
func TestProbeRealTree_AllDescendantsPreTurn_Settled(t *testing.T) {
	skipUnlessRealTreeSupported(t)

	startRealChild(t, false)
	time.Sleep(50 * time.Millisecond) // clear the platform's start-time resolution
	turnStart := time.Now()

	got, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
}

// AC-70a / AC-72: settled with only a pre-turn descendant, then live once a
// new descendant starts after the recorded turn start.
func TestProbeRealTree_NewDescendantAfterTurnStart_Live(t *testing.T) {
	skipUnlessRealTreeSupported(t)

	startRealChild(t, false) // pre-turn descendant
	time.Sleep(50 * time.Millisecond)
	turnStart := time.Now()

	settled, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if settled != ResultSettled {
		t.Fatalf("before the new descendant: got %q, want %q", settled, ResultSettled)
	}

	time.Sleep(50 * time.Millisecond)
	startRealChild(t, false) // post-turn-start descendant

	live, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if live != ResultLive {
		t.Errorf("after the new descendant: got %q, want %q", live, ResultLive)
	}
}

// AC-71: §L found process-group membership unusable as the predicate — a
// real descendant placed in its own process group must still be found via
// the ppid chain.
func TestProbeRealTree_DescendantInOwnProcessGroupStillCounted(t *testing.T) {
	skipUnlessRealTreeSupported(t)

	turnStart := time.Now()
	time.Sleep(50 * time.Millisecond)
	startRealChild(t, true) // own process group, started after turnStart

	got, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got != ResultLive {
		t.Errorf("got %q, want %q", got, ResultLive)
	}
}

// AC-80: turnStart is recorded immediately after the real child's Start()
// returns, so the child's actual OS-reported start time lands a few
// microseconds BEFORE turnStart — well within the same truncated-resolution
// bucket, given Linux's ~10ms clock-tick resolution. This must still count
// as live: an untruncated "descendant start >= turnStart" comparison would
// wrongly call it settled, which is exactly the bug round-5 F3's truncated
// comparison exists to prevent. Linux-only: Darwin's 1µs resolution leaves
// no realistic margin, so the same construction is flaky there rather than
// meaningful — TestProbeWithReader_TruncationBoundary in probe_test.go is
// the deterministic, platform-independent form of this assertion and is
// what covers AC-80 on Darwin.
func TestProbeRealTree_TruncationBoundary(t *testing.T) {
	skipUnlessRealTreeSupported(t)
	if runtime.GOOS != "linux" {
		t.Skip("probe: only Linux's ~10ms resolution gives this construction a non-flaky truncation margin")
	}

	reader := platformProcessTableReader()
	for attempt := 0; attempt < 10; attempt++ {
		child := startRealChild(t, false)
		table, err := reader.ReadProcessTable()
		if err != nil {
			t.Fatalf("read process table: %v", err)
		}
		var childStart time.Time
		for _, entry := range table {
			if entry.PID == child.Process.Pid {
				childStart = entry.StartTime
				break
			}
		}
		if childStart.IsZero() {
			continue
		}
		// Derive the raw turn start from the observed OS start time. This
		// removes scheduler timing from the assertion while keeping the raw
		// value after the child and the truncated values in one bucket.
		turnStart := childStart.Add(time.Nanosecond)
		if childStart.Truncate(reader.Resolution()) != turnStart.Truncate(reader.Resolution()) {
			continue
		}
		// Keep the observed snapshot fixed for the assertion. A second Linux
		// /proc/uptime read can shift the wall-clock anchor by a few
		// microseconds and move a tick-aligned child across the bucket.
		fixedReader := fakeProcessTableReader{resolution: reader.Resolution(), table: table}
		got, err := probeWithReader(fixedReader, os.Getpid(), turnStart)
		if err != nil {
			t.Fatalf("probe: %v", err)
		}
		if got != ResultLive {
			t.Errorf("got %q, want %q", got, ResultLive)
		}
		return
	}
	t.Fatal("could not place a real child and turn start in the same Linux clock-tick bucket")
}
