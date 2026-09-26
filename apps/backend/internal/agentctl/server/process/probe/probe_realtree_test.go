package probe

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
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

	got, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart, "")
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

	settled, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart, "")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if settled != ResultSettled {
		t.Fatalf("before the new descendant: got %q, want %q", settled, ResultSettled)
	}

	time.Sleep(50 * time.Millisecond)
	startRealChild(t, false) // post-turn-start descendant

	live, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart, "")
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

	got, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart, "")
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
		got, err := probeWithReader(fixedReader, os.Getpid(), turnStart, "")
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

// skipUnlessOrphanAttributionSupported gates the three orphan-attribution
// cases below to Linux, the one platform whose reader implements
// environmentReader (AC-DW-ORPHAN-002.1: Darwin keeps the descendant-only
// answer and cannot attribute a reparented workload at all).
func skipUnlessOrphanAttributionSupported(t *testing.T) {
	t.Helper()
	skipUnlessRealTreeSupported(t)
	if runtime.GOOS != "linux" {
		t.Skip("probe: environment-read attribution is Linux-only")
	}
}

// uniqueTestSessionID returns a session id that cannot collide with any
// other process's real KANDEV_SESSION_ID on the host running this test.
func uniqueTestSessionID(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("probe-realtree-test-%d-%d", os.Getpid(), time.Now().UnixNano())
}

// envWithSessionID returns a copy of this process's environment with any
// existing KANDEV_SESSION_ID replaced by sessionID — this test binary may
// itself be running inside a Kandev-managed session, and appending rather
// than replacing would leave two entries where the ambient one, not ours,
// decides the match.
func envWithSessionID(sessionID string) []string {
	base := os.Environ()
	env := make([]string, 0, len(base)+1)
	for _, kv := range base {
		if strings.HasPrefix(kv, "KANDEV_SESSION_ID=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "KANDEV_SESSION_ID="+sessionID)
}

// spawnReparentedWorkload launches a wrapper shell that backgrounds a
// long-lived workload and exits immediately without waiting for it,
// reproducing the pattern the system design measured: the workload is
// reparented to init while the wrapper's own exit is what cmd.Output
// blocks on, so by the time this function returns the reparenting has
// already happened synchronously in the kernel. The background job's own
// stdio is redirected away from the wrapper's pipe — left inherited, the
// still-running workload would hold that pipe's write end open and
// cmd.Output would block on it for the workload's whole lifetime instead of
// returning once the wrapper shell exits. The workload inherits sessionID
// via its environment. The caller's t.Cleanup kills it.
func spawnReparentedWorkload(t *testing.T, sessionID string) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", "sleep 30 </dev/null >/dev/null 2>&1 & echo $!")
	cmd.Env = envWithSessionID(sessionID)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("spawn reparented workload: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse workload pid from %q: %v", out, err)
	}
	t.Cleanup(func() {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	})
	return pid
}

// Required E2E case 1 (system design "E2E decision"): a workload whose
// parent shell exits before the sample must still read live, attributed by
// KANDEV_SESSION_ID even though it left the descendant tree.
func TestProbeRealTree_ReparentedWorkloadAttributedBySessionID(t *testing.T) {
	skipUnlessOrphanAttributionSupported(t)

	sessionID := uniqueTestSessionID(t)
	turnStart := time.Now()
	time.Sleep(20 * time.Millisecond)
	spawnReparentedWorkload(t, sessionID)

	got, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart, sessionID)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got != ResultLive {
		t.Errorf("got %q, want %q", got, ResultLive)
	}
}

// Required E2E case 2: a workload left over from an earlier turn must not
// be attributed, even though it carries this session's identity.
func TestProbeRealTree_PreTurnReparentedWorkload_Settled(t *testing.T) {
	skipUnlessOrphanAttributionSupported(t)

	sessionID := uniqueTestSessionID(t)
	spawnReparentedWorkload(t, sessionID)
	time.Sleep(50 * time.Millisecond)
	turnStart := time.Now()

	got, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart, sessionID)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
}

// Required E2E case 3 (AC-DW-ORPHAN-001.11): re-validating an unchanged,
// still-running candidate must succeed after a delay long enough to move
// the wall clock. This must be a real-process-tree case: fakeProcessTableReader
// replays stored processInfo values verbatim, so a re-read through it would
// return an identical start time and pass even against an implementation
// that re-derives the start time from a fresh /proc/uptime read each time.
func TestProbeRealTree_ReparentedWorkloadRevalidatesAfterDelay(t *testing.T) {
	skipUnlessOrphanAttributionSupported(t)

	sessionID := uniqueTestSessionID(t)
	turnStart := time.Now()
	time.Sleep(20 * time.Millisecond)
	spawnReparentedWorkload(t, sessionID)

	// The delay gives a re-deriving implementation room to drift: each
	// linuxBootTime() call re-anchors to time.Now() against /proc/uptime,
	// so two derivations of one unchanged process's start time disagree by
	// roughly however long this sleep runs.
	time.Sleep(200 * time.Millisecond)

	got, err := ProbeBackgroundWorkloads(os.Getpid(), turnStart, sessionID)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got != ResultLive {
		t.Errorf("got %q, want %q — re-validation must compare the platform's invariant start-time datum, never a value re-derived from the current wall clock", got, ResultLive)
	}
}
