package agents

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/managedruntime"
)

type versionRun struct {
	output string
	err    error
}

// fakeOpenCodeNative scripts the version runs of one detector and records the
// retry waits, so no test runs a real executable or sleeps.
type fakeOpenCodeNative struct {
	mu    sync.Mutex
	runs  []versionRun
	calls []string
	waits []time.Duration
	now   time.Time
}

func newFakeOpenCodeDetector(fake *fakeOpenCodeNative, path string) *openCodeNativeDetector {
	d := newOpenCodeNativeDetector()
	d.lookPath = func(string) (string, error) { return path, nil }
	d.runVersion = func(_ context.Context, path string) ([]byte, error) {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		fake.calls = append(fake.calls, path)
		if len(fake.runs) == 0 {
			return nil, errors.New("unexpected version run")
		}
		run := fake.runs[0]
		fake.runs = fake.runs[1:]
		return []byte(run.output), run.err
	}
	d.sleep = func(_ context.Context, wait time.Duration) error {
		fake.waits = append(fake.waits, wait)
		return nil
	}
	d.now = func() time.Time { return fake.now }
	return d
}

var errExitStatus1 = errors.New("exit status 1")

func TestOpenCodeNativeDetectionRetriesTransientFailures(t *testing.T) {
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
		{err: errExitStatus1},
		{output: "Bun is warming up"},
		{output: "1.18.34\n"},
	}}
	got, found, err := newFakeOpenCodeDetector(fake, "/bin/opencode").detect(context.Background())
	if err != nil || !found {
		t.Fatalf("detect = %+v, found %v, err %v; want the third run's version", got, found, err)
	}
	if got.Version != "1.18.34" || got.Family != managedruntime.OpenCodeFamilyV1 {
		t.Fatalf("runtime = %+v, want v1 1.18.34", got)
	}
	if len(fake.calls) != 3 {
		t.Fatalf("version runs = %d, want 3", len(fake.calls))
	}
	if want := []time.Duration{300 * time.Millisecond, time.Second}; !equalDurations(fake.waits, want) {
		t.Fatalf("retry waits = %v, want %v", fake.waits, want)
	}
}

func TestOpenCodeNativeDetectionReportsBoundedOutputAfterRetries(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory to redact")
	}
	output := "error: cannot start runtime at " + home + "\x1b[0m\n" + strings.Repeat("x", 400)
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
		{output: output, err: errExitStatus1},
		{output: output, err: errExitStatus1},
		{output: output, err: errExitStatus1},
	}}
	_, found, err := newFakeOpenCodeDetector(fake, "/bin/opencode").detect(context.Background())
	if err == nil || !found {
		t.Fatalf("detect found %v, err %v; want a found executable and an error", found, err)
	}
	message := err.Error()
	if len(fake.calls) != 3 {
		t.Fatalf("version runs = %d, want 3", len(fake.calls))
	}
	if !strings.Contains(message, "read native OpenCode version: exit status 1") ||
		!strings.Contains(message, `output: "error: cannot start runtime at ~ [0m `) {
		t.Fatalf("error = %q, want the exit status and a sanitized output excerpt", message)
	}
	if strings.Contains(message, home) || strings.Contains(message, "\x1b") || strings.Contains(message, strings.Repeat("x", 201)) {
		t.Fatalf("error = %q, want home redacted, control bytes dropped and the excerpt bounded", message)
	}
}

func TestOpenCodeNativeDetectionMarksTimedOutRuns(t *testing.T) {
	d := newOpenCodeNativeDetector()
	d.lookPath = func(string) (string, error) { return "/bin/opencode", nil }
	d.versionTimeout = 10 * time.Millisecond
	d.backoff = nil
	d.runVersion = func(ctx context.Context, _ string) ([]byte, error) {
		<-ctx.Done()
		return nil, errExitStatus1
	}
	_, _, err := d.detect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exit status 1 (timed out after 10ms)") {
		t.Fatalf("error = %v, want the killed run marked as timed out", err)
	}
}

func TestOpenCodeNativeDetectionFallsBackToRecentDetectionOfTheSamePath(t *testing.T) {
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
		{output: "opencode 1.18.33"},
		{err: errExitStatus1}, {err: errExitStatus1}, {err: errExitStatus1},
	}}
	d := newFakeOpenCodeDetector(fake, "/bin/opencode")
	if _, _, err := d.detect(context.Background()); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	fake.now = fake.now.Add(9 * time.Minute)
	got, found, err := d.detect(context.Background())
	if err != nil || !found || got.Version != "1.18.33" {
		t.Fatalf("detect after a transient failure = %+v, found %v, err %v; want the cached 1.18.33", got, found, err)
	}
	if len(fake.calls) != 4 || !equalDurations(fake.waits, []time.Duration{300 * time.Millisecond, time.Second}) {
		t.Fatalf("runs = %d waits = %v, want all retries before the cached result", len(fake.calls), fake.waits)
	}
}

func TestOpenCodeNativeDetectionRetrySupersedesCachedVersion(t *testing.T) {
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
		{output: "opencode 1.18.33"},
		{err: errExitStatus1},
		{output: "opencode 2.0.18"},
	}}
	d := newFakeOpenCodeDetector(fake, "/bin/opencode")
	if _, _, err := d.detect(context.Background()); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	got, found, err := d.detect(context.Background())
	if err != nil || !found || got.Version != "2.0.18" || got.Family != managedruntime.OpenCodeFamilyV2 {
		t.Fatalf("detect = %+v, found %v, err %v; want the retry's v2 runtime", got, found, err)
	}
	if len(fake.calls) != 3 || !equalDurations(fake.waits, []time.Duration{300 * time.Millisecond}) {
		t.Fatalf("runs = %d waits = %v, want one retry before the updated version", len(fake.calls), fake.waits)
	}
}

func TestOpenCodeNativeDetectionRetryRejectsUnsupportedMajorAndClearsCache(t *testing.T) {
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
		{output: "opencode 1.18.33"},
		{err: errExitStatus1},
		{output: "opencode 3.0.0"},
		{err: errExitStatus1}, {err: errExitStatus1}, {err: errExitStatus1},
	}}
	d := newFakeOpenCodeDetector(fake, "/bin/opencode")
	if _, _, err := d.detect(context.Background()); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	if _, found, err := d.detect(context.Background()); !found || err == nil || err.Error() != "native OpenCode major 3 is not supported" {
		t.Fatalf("detect found %v, err %v; want the retry's unsupported major", found, err)
	}
	if len(fake.calls) != 3 || !equalDurations(fake.waits, []time.Duration{300 * time.Millisecond}) {
		t.Fatalf("runs = %d waits = %v, want to stop at the unsupported major", len(fake.calls), fake.waits)
	}
	if _, found, err := d.detect(context.Background()); !found || !errors.Is(err, errExitStatus1) {
		t.Fatalf("detect found %v, err %v; want failure after cache invalidation", found, err)
	}
}

func TestOpenCodeNativeDetectionDoesNotReuseCacheThatExpiresDuringRetries(t *testing.T) {
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
		{output: "opencode 1.18.33"},
		{err: errExitStatus1}, {err: errExitStatus1}, {err: errExitStatus1},
	}}
	d := newFakeOpenCodeDetector(fake, "/bin/opencode")
	if _, _, err := d.detect(context.Background()); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	fake.now = fake.now.Add(10*time.Minute - time.Second)
	d.sleep = func(_ context.Context, wait time.Duration) error {
		fake.waits = append(fake.waits, wait)
		fake.now = fake.now.Add(wait)
		return nil
	}
	if _, found, err := d.detect(context.Background()); !found || !errors.Is(err, errExitStatus1) {
		t.Fatalf("detect found %v, err %v; want failure after cache expiry", found, err)
	}
}

func TestOpenCodeNativeDetectionCacheExpiresAndIsPerPath(t *testing.T) {
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
		{output: "opencode 1.18.33"},
		{err: errExitStatus1}, {err: errExitStatus1}, {err: errExitStatus1},
	}}
	d := newFakeOpenCodeDetector(fake, "/bin/opencode")
	if _, _, err := d.detect(context.Background()); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	fake.now = fake.now.Add(11 * time.Minute)
	if _, _, err := d.detect(context.Background()); err == nil {
		t.Fatal("detect after the cache expired succeeded, want the failure")
	}

	other := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
		{output: "opencode 1.18.33"},
		{err: errExitStatus1}, {err: errExitStatus1}, {err: errExitStatus1},
	}}
	d = newFakeOpenCodeDetector(other, "/bin/opencode")
	if _, _, err := d.detect(context.Background()); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	d.lookPath = func(string) (string, error) { return "/other/opencode", nil }
	if _, _, err := d.detect(context.Background()); err == nil {
		t.Fatal("another executable answered from the first one's cache")
	}
}

func TestOpenCodeNativeDetectionDoesNotRetryDefiniteAnswers(t *testing.T) {
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{{output: "opencode 3.0.0"}}}
	_, found, err := newFakeOpenCodeDetector(fake, "/bin/opencode").detect(context.Background())
	if err == nil || !found || err.Error() != "native OpenCode major 3 is not supported" {
		t.Fatalf("detect found %v, err %v; want the unsupported major", found, err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("version runs = %d, want 1", len(fake.calls))
	}

	missing := newOpenCodeNativeDetector()
	missing.lookPath = func(name string) (string, error) {
		return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
	missing.runVersion = func(context.Context, string) ([]byte, error) {
		t.Fatal("version ran without an executable")
		return nil, nil
	}
	if got, found, err := missing.detect(context.Background()); err != nil || found || got != (OpenCodeNativeRuntime{}) {
		t.Fatalf("detect without an executable = %+v, found %v, err %v; want absent without error", got, found, err)
	}
}

func TestOpenCodeNativeDetectionStopsRetryingWhenTheCallerGivesUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{{err: errExitStatus1}, {output: "opencode 1.18.33"}}}
	d := newFakeOpenCodeDetector(fake, "/bin/opencode")
	d.sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}
	if _, _, err := d.detect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("detect error = %v, want caller cancellation", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("version runs = %d, want no retry after cancellation", len(fake.calls))
	}
}

func TestOpenCodeNativeDetectionDoesNotUseCacheAfterCancellation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cancelled bool
		run       versionRun
		wantCalls int
	}{
		{name: "before the probe", cancelled: true, wantCalls: 1},
		{name: "failed probe", run: versionRun{err: context.Canceled}, wantCalls: 2},
		{name: "successful probe", run: versionRun{output: "opencode 2.0.18"}, wantCalls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeOpenCodeNative{now: time.Unix(1_000, 0), runs: []versionRun{
				{output: "opencode 1.18.33"}, tc.run,
			}}
			d := newFakeOpenCodeDetector(fake, "/bin/opencode")
			if _, _, err := d.detect(context.Background()); err != nil {
				t.Fatalf("first detect: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			} else {
				runVersion := d.runVersion
				d.runVersion = func(ctx context.Context, path string) ([]byte, error) {
					output, err := runVersion(ctx, path)
					cancel()
					return output, err
				}
			}
			got, found, err := d.detect(ctx)
			if !errors.Is(err, context.Canceled) || !found || got != (OpenCodeNativeRuntime{}) {
				t.Fatalf("detect = %+v, found %v, err %v; want caller cancellation", got, found, err)
			}
			if len(fake.calls) != tc.wantCalls || len(fake.waits) != 0 {
				t.Fatalf("runs = %d waits = %v, want no work after cancellation", len(fake.calls), fake.waits)
			}
		})
	}
}

func equalDurations(got, want []time.Duration) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
