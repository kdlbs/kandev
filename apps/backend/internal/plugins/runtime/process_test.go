package runtime

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// fakeSpawnedProcess is a controllable spawnedProcess for unit-testing
// process's supervision decisions without a real subprocess.
type fakeSpawnedProcess struct {
	mu      sync.Mutex
	pingErr error
	exited  bool
	killed  bool
}

func (f *fakeSpawnedProcess) Ping() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pingErr
}

func (f *fakeSpawnedProcess) Exited() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exited
}

func (f *fakeSpawnedProcess) Kill() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.killed = true
}

func (f *fakeSpawnedProcess) Remote() *pluginsdk.RemotePlugin { return nil }

func (f *fakeSpawnedProcess) isKilled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.killed
}

// statusRecorder records every (id, healthy) pair passed to
// onStatusChange, for assertion.
type statusRecorder struct {
	mu    sync.Mutex
	calls []bool
}

func (r *statusRecorder) record(_ string, healthy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, healthy)
}

func (r *statusRecorder) snapshot() []bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]bool, len(r.calls))
	copy(out, r.calls)
	return out
}

// noSleep is a sleepFn that never actually blocks: it returns true
// immediately unless stopCh is already closed, in which case it returns
// false. Used by every test in this file so supervision-decision logic
// runs with zero real waiting.
func noSleep(stopCh <-chan struct{}, _ time.Duration) bool {
	select {
	case <-stopCh:
		return false
	default:
		return true
	}
}

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

func newTestProcess(t *testing.T, spawnFn func() (spawnedProcess, error), rec *statusRecorder) *process {
	t.Helper()
	p := newProcess("test-plugin", testLogger(t), spawnFn, rec.record)
	p.sleepFn = noSleep
	p.maxConsecutiveFails = 3
	p.restartBackoff = []time.Duration{0, 0, 0, 0, 0}
	p.maxRestartAttempts = 5
	return p
}

func TestProcess_Tick_HealthyPingIsANoOp(t *testing.T) {
	rec := &statusRecorder{}
	fake := &fakeSpawnedProcess{}
	p := newTestProcess(t, nil, rec)
	p.current = fake

	if gaveUp := p.tick(); gaveUp {
		t.Fatal("tick() = gave up, want false for a healthy ping")
	}
	if fake.isKilled() {
		t.Fatal("tick() killed the process on a healthy ping")
	}
	if len(rec.snapshot()) != 0 {
		t.Fatalf("onStatusChange calls = %v, want none for a healthy ping", rec.snapshot())
	}
}

func TestProcess_Tick_FailureBelowThresholdDoesNotRestart(t *testing.T) {
	rec := &statusRecorder{}
	fake := &fakeSpawnedProcess{pingErr: errors.New("boom")}
	p := newTestProcess(t, nil, rec)
	p.current = fake
	p.maxConsecutiveFails = 3

	p.tick()
	if gaveUp := p.tick(); gaveUp {
		t.Fatal("tick() = gave up, want false before reaching the failure threshold")
	}

	if fake.isKilled() {
		t.Fatal("tick() killed the process before reaching the failure threshold")
	}
	if len(rec.snapshot()) != 0 {
		t.Fatalf("onStatusChange calls = %v, want none below the failure threshold", rec.snapshot())
	}
	if p.failures != 2 {
		t.Fatalf("p.failures = %d, want 2", p.failures)
	}
}

func TestProcess_Tick_ThresholdReachedRestartsAndNotifiesUnhealthyThenHealthy(t *testing.T) {
	rec := &statusRecorder{}
	failing := &fakeSpawnedProcess{pingErr: errors.New("boom")}
	healthy := &fakeSpawnedProcess{}
	spawnCalls := 0
	spawnFn := func() (spawnedProcess, error) {
		spawnCalls++
		return healthy, nil
	}

	p := newTestProcess(t, spawnFn, rec)
	p.current = failing
	p.maxConsecutiveFails = 1

	if gaveUp := p.tick(); gaveUp {
		t.Fatal("tick() = gave up, want false: the restart should have succeeded")
	}

	if !failing.isKilled() {
		t.Fatal("tick() did not kill the failing process before restarting")
	}
	if spawnCalls != 1 {
		t.Fatalf("spawnFn called %d times, want 1", spawnCalls)
	}
	if _, ok := p.remote(); !ok {
		t.Fatal("remote() ok = false after a successful restart, want true")
	}
	if p.restartCount() != 1 {
		t.Fatalf("p.restartCount() = %d, want 1", p.restartCount())
	}
	if want := []bool{false, true}; !equalBoolSlices(rec.snapshot(), want) {
		t.Fatalf("onStatusChange calls = %v, want %v", rec.snapshot(), want)
	}
}

func TestProcess_Tick_ExitedTriggersImmediateRestartRegardlessOfFailureCount(t *testing.T) {
	rec := &statusRecorder{}
	exited := &fakeSpawnedProcess{exited: true}
	healthy := &fakeSpawnedProcess{}
	spawnFn := func() (spawnedProcess, error) { return healthy, nil }

	p := newTestProcess(t, spawnFn, rec)
	p.current = exited
	p.maxConsecutiveFails = 3 // would not be reached by ping failures alone

	if gaveUp := p.tick(); gaveUp {
		t.Fatal("tick() = gave up, want false: the restart should have succeeded")
	}
	if want := []bool{false, true}; !equalBoolSlices(rec.snapshot(), want) {
		t.Fatalf("onStatusChange calls = %v, want %v (immediate restart on exit)", rec.snapshot(), want)
	}
}

func TestProcess_HandleFailureAndRestart_GivesUpAfterMaxAttempts(t *testing.T) {
	rec := &statusRecorder{}
	spawnCalls := 0
	spawnFn := func() (spawnedProcess, error) {
		spawnCalls++
		return nil, errors.New("spawn failed")
	}

	p := newTestProcess(t, spawnFn, rec)
	p.current = &fakeSpawnedProcess{}
	p.maxRestartAttempts = 3
	p.restartBackoff = []time.Duration{0, 0, 0}

	if gaveUp := p.handleFailureAndRestart(); !gaveUp {
		t.Fatal("handleFailureAndRestart() = false, want true after exhausting every attempt")
	}
	if spawnCalls != 3 {
		t.Fatalf("spawnFn called %d times, want 3 (maxRestartAttempts)", spawnCalls)
	}
	if want := []bool{false}; !equalBoolSlices(rec.snapshot(), want) {
		t.Fatalf("onStatusChange calls = %v, want %v (only the initial degradation, no false recovery)", rec.snapshot(), want)
	}
	if _, ok := p.remote(); ok {
		t.Fatal("remote() ok = true after giving up, want false")
	}
}

func TestProcess_HandleFailureAndRestart_StopDuringBackoffSkipsSpawn(t *testing.T) {
	rec := &statusRecorder{}
	spawnCalls := 0
	spawnFn := func() (spawnedProcess, error) {
		spawnCalls++
		return &fakeSpawnedProcess{}, nil
	}

	p := newTestProcess(t, spawnFn, rec)
	p.current = &fakeSpawnedProcess{}
	close(p.stopCh) // simulate Stop() racing the backoff wait
	p.sleepFn = sleepOrStop

	if gaveUp := p.handleFailureAndRestart(); gaveUp {
		t.Fatal("handleFailureAndRestart() = true, want false: stopping is not the same as giving up")
	}
	if spawnCalls != 0 {
		t.Fatalf("spawnFn called %d times, want 0: stop should short-circuit before any spawn attempt", spawnCalls)
	}
}

func equalBoolSlices(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
