package launcher

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/startup"
)

// TestFormatDurationEstimate covers AC-PLATFORM-STARTUP-PROGRESS-003.10: 60s
// or less renders in whole seconds rounded up with a floor of one second, a
// longer duration renders in whole minutes rounded up, never sub-second.
func TestFormatDurationEstimate(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{ms: 1, want: "1s"},
		{ms: 999, want: "1s"},
		{ms: 1000, want: "1s"},
		{ms: 1001, want: "2s"},
		{ms: 60000, want: "60s"},
		{ms: 60001, want: "2m"},
		{ms: 90000, want: "2m"},
		{ms: 120000, want: "2m"},
	}
	for _, tc := range cases {
		if got := formatDurationEstimate(tc.ms); got != tc.want {
			t.Errorf("formatDurationEstimate(%d) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}

// TestFormatStartupLineNoStep covers AC-PLATFORM-STARTUP-PROGRESS-003.13-
// adjacent behavior for the launcher: with no active step the line carries
// only phase and elapsed time.
func TestFormatStartupLineNoStep(t *testing.T) {
	snapshot := startup.Snapshot{Phase: startup.OpeningDatabase, ElapsedMS: 1500, PhaseElapsedMS: 500}
	got := formatStartupLine(snapshot)
	want := "Opening database (elapsed 1.5s; phase 0.5s)"
	if got != want {
		t.Fatalf("formatStartupLine() = %q, want %q", got, want)
	}
}

// TestFormatStartupLineOpaqueStepStatesProgressUnavailable covers F74/
// AC-PLATFORM-STARTUP-PROGRESS-001.5: an opaque step never carries done or
// total, and the launcher line states plainly that it cannot report
// progress rather than showing a bar at any position.
func TestFormatStartupLineOpaqueStepStatesProgressUnavailable(t *testing.T) {
	snapshot := startup.Snapshot{
		Phase: startup.InitializingServices, ElapsedMS: 2000, PhaseElapsedMS: 1000,
		Step: &startup.StepSnapshot{LabelKey: "startup.step.prompt_seq", Measure: startup.MeasureOpaque},
	}
	got := formatStartupLine(snapshot)
	want := "Initializing services (elapsed 2.0s; phase 1.0s): Prompt sequence numbers (progress not available)"
	if got != want {
		t.Fatalf("formatStartupLine() = %q, want %q", got, want)
	}
}

// TestFormatStartupLineCountingStepWithNoTotal covers the design's
// database.backup example: a `counting` step carries a done count and a
// rate with no total, and the line must gain both rather than waiting for a
// `counted` step.
func TestFormatStartupLineCountingStepWithNoTotal(t *testing.T) {
	done := int64(42)
	rate := 3.5
	snapshot := startup.Snapshot{
		Phase: startup.BackingUpDatabase, ElapsedMS: 5000, PhaseElapsedMS: 5000,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.database_backup", Measure: startup.MeasureCounting,
			Done: &done, RatePerSecond: &rate,
		},
	}
	got := formatStartupLine(snapshot)
	want := "Backing up database (elapsed 5.0s; phase 5.0s): Database backup: 42 (3.5/s)"
	if got != want {
		t.Fatalf("formatStartupLine() = %q, want %q", got, want)
	}
}

// TestFormatStartupLineCountedStepWithEstimate covers the full counted case:
// done, total, rate, and estimate all present.
func TestFormatStartupLineCountedStepWithEstimate(t *testing.T) {
	done, total := int64(12), int64(50)
	rate := 2.0
	eta := int64(19000)
	snapshot := startup.Snapshot{
		Phase: startup.RecoveringSessions, ElapsedMS: 6000, PhaseElapsedMS: 6000,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.session_recovery", Measure: startup.MeasureCounted,
			Done: &done, Total: &total, RatePerSecond: &rate, ETAMS: &eta,
		},
	}
	got := formatStartupLine(snapshot)
	want := "Recovering sessions (elapsed 6.0s; phase 6.0s): Session recovery: 12/50 (2.0/s), ~19s remaining"
	if got != want {
		t.Fatalf("formatStartupLine() = %q, want %q", got, want)
	}
}

// TestFormatStartupLineStalledStepStatesDuration covers F75/
// AC-PLATFORM-STARTUP-PROGRESS-004.6: a stalled step states how long it has
// gone without progress.
func TestFormatStartupLineStalledStepStatesDuration(t *testing.T) {
	done, total := int64(3), int64(10)
	sinceAdvance := int64(125000)
	snapshot := startup.Snapshot{
		Phase: startup.RecoveringSessions, ElapsedMS: 130000, PhaseElapsedMS: 130000,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.session_recovery", Measure: startup.MeasureCounted,
			Done: &done, Total: &total, SinceAdvanceMS: &sinceAdvance, Stalled: true,
		},
	}
	got := formatStartupLine(snapshot)
	want := "Recovering sessions (elapsed 130.0s; phase 130.0s): Session recovery: 3/10, stalled for 3m"
	if got != want {
		t.Fatalf("formatStartupLine() = %q, want %q", got, want)
	}
}

// TestFormatStartupLineNonStalledStepOmitsStallStatement is the negative
// case for the same AC: a step short of the stall threshold prints no
// stall statement at all.
func TestFormatStartupLineNonStalledStepOmitsStallStatement(t *testing.T) {
	done, total := int64(3), int64(10)
	sinceAdvance := int64(500)
	snapshot := startup.Snapshot{
		Phase: startup.RecoveringSessions, ElapsedMS: 1000, PhaseElapsedMS: 1000,
		Step: &startup.StepSnapshot{
			LabelKey: "startup.step.session_recovery", Measure: startup.MeasureCounted,
			Done: &done, Total: &total, SinceAdvanceMS: &sinceAdvance, Stalled: false,
		},
	}
	got := formatStartupLine(snapshot)
	if strings.Contains(got, "stalled") {
		t.Fatalf("formatStartupLine() = %q, want no stall statement below the threshold", got)
	}
}

// TestReadinessExitNamesLastObservedStep covers AC-PLATFORM-STARTUP-
// PROGRESS-003.6: when the backend exits during startup, the launcher names
// the last observed step as well as the last observed phase.
func TestReadinessExitNamesLastObservedStep(t *testing.T) {
	var child toggledChild
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"starting","startup":{"phase":"recovering_sessions","elapsed_ms":1234,"phase_elapsed_ms":1000,"step":{"id":"sessions.recovery","label_key":"startup.step.session_recovery","measure":"counted","unit":"sessions","elapsed_ms":1000,"done":3,"total":10,"stalled":false}}}`))
		child.code.Store(3)
		child.exited.Store(true)
	}))
	defer srv.Close()
	err := waitForReady(context.Background(), srv.URL, &child)
	if err == nil || !strings.Contains(err.Error(), "Recovering sessions") {
		t.Fatalf("missing startup phase in error: %v", err)
	}
	if !strings.Contains(err.Error(), "Session recovery") {
		t.Fatalf("error = %v, want it to name the last observed step", err)
	}
}

// TestWaitForReadyReprintsOnReportFloorWithoutOtherChanges covers
// AC-PLATFORM-STARTUP-PROGRESS-003.5's floor trigger: with phase, seq, and
// stall state all unchanged, a line is written again only once
// startupProgressReportFloor has elapsed since the last one, and not on
// every intervening poll. Stubs probeReadyStatusFn rather than using a real
// httptest.Server: real HTTP I/O sits outside synctest's fake-time bubble
// and would prevent the 15-second floor from fast-forwarding (see
// internal/system/updates/service_test.go's TestService_StartPoller_
// TicksEvery6h for the same convention).
func TestWaitForReadyReprintsOnReportFloorWithoutOtherChanges(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		oldProbe := probeReadyStatusFn
		t.Cleanup(func() { probeReadyStatusFn = oldProbe })

		start := time.Now()
		probeReadyStatusFn = func(context.Context, string) (bool, startup.Snapshot) {
			if time.Since(start) >= 40*time.Second {
				return true, startup.Snapshot{}
			}
			return false, startup.Snapshot{Phase: startup.RecoveringSessions, Seq: 1, ElapsedMS: 1, PhaseElapsedMS: 1}
		}

		var err error
		output := captureStderr(t, func() {
			err = waitForReady(context.Background(), "http://probe.invalid", fakeChild{})
		})
		if err != nil {
			t.Fatalf("waitForReady() = %v, want nil", err)
		}
		lines := strings.Count(output, "Recovering sessions")
		if lines != 3 {
			t.Fatalf("stderr output = %q, want exactly 3 progress lines (initial + two floor reprints at 15s and 30s), got %d", output, lines)
		}
	})
}

// captureStderr swaps os.Stderr for the duration of fn and returns what was
// written to it. The launcher test package never runs tests in parallel, so
// this global swap is safe.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() = %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = write
	defer func() { os.Stderr = oldStderr }()

	fn()
	if err := write.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	var output bytes.Buffer
	if _, err := io.Copy(&output, read); err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	if err := read.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return output.String()
}

// TestWaitForReadyReprintsOnSeqChangeWithoutPhaseChange covers
// AC-PLATFORM-STARTUP-PROGRESS-003.5's sequence-number trigger: a step
// beginning or ending bumps `seq` without necessarily changing phase, and
// that alone must produce an immediate line rather than waiting for the
// 15-second floor or a phase change.
func TestWaitForReadyReprintsOnSeqChangeWithoutPhaseChange(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		switch calls {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"startup":{"phase":"recovering_sessions","seq":1,"elapsed_ms":1,"phase_elapsed_ms":1}}`))
		case 2:
			// Same phase, same seq: within the 15s floor, no reprint expected.
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"startup":{"phase":"recovering_sessions","seq":1,"elapsed_ms":2,"phase_elapsed_ms":2}}`))
		case 3:
			// seq bumped (a step ended) with no phase change: must reprint.
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"startup":{"phase":"recovering_sessions","seq":2,"elapsed_ms":3,"phase_elapsed_ms":3}}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var err error
	output := captureStderr(t, func() {
		err = waitForReady(ctx, srv.URL, fakeChild{})
	})
	if err != nil {
		t.Fatalf("waitForReady() = %v, want nil", err)
	}
	lines := strings.Count(output, "Recovering sessions")
	if lines != 2 {
		t.Fatalf("stderr output = %q, want exactly 2 progress lines (initial + seq change), got %d", output, lines)
	}
}
