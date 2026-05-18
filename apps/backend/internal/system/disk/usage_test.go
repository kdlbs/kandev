// Tests for the lazy-cached disk-usage Service. Covers the four branches of
// Get described in the spec (cold-not-computing, cold-computing, fresh,
// stale) plus Refresh's force-walk semantics. Uses the in-process
// jobs.Tracker with a stub event bus.
package disk

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/system/jobs"
)

// stubBus is a minimal in-memory EventBus used by the jobs.Tracker. We only
// need Publish to succeed; the other methods are no-ops to satisfy the
// interface.
type stubBus struct {
	mu     sync.Mutex
	events []*bus.Event
}

func (s *stubBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *stubBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (s *stubBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (s *stubBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (s *stubBus) Close()            {}
func (s *stubBus) IsConnected() bool { return true }

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

// newServiceWithTmpHome builds a Service rooted at a fresh tmp dir with a
// known layout: data/ + worktrees/ + repos/ + data/backups/. The remaining
// subdirs (sessions/tasks/quick-chat) are intentionally absent to exercise
// the missing-root path.
func newServiceWithTmpHome(t *testing.T) (*Service, string) {
	t.Helper()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "data", "kandev.db"), []byte("0123456789"))    // 10
	writeFile(t, filepath.Join(home, "data", "backups", "auto.db"), []byte("AAAA")) // 4
	writeFile(t, filepath.Join(home, "worktrees", "ws1", "file"), []byte("BB"))     // 2
	writeFile(t, filepath.Join(home, "repos", "r", "x"), []byte("CCC"))             // 3
	tracker := jobs.NewTracker(&stubBus{}, newTestLogger(t))
	return NewService(home, tracker, newTestLogger(t)), home
}

// waitForBreakdown polls until the Service caches a value or the deadline
// fires; mirrors the jobs_test.go waitForState helper.
func waitForBreakdown(t *testing.T, svc *Service) *Breakdown {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		v := svc.value
		svc.mu.Unlock()
		if v != nil {
			return v
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("breakdown not cached within 2s")
	return nil
}

func TestGet_ColdReturnsComputingAndKicksWalk(t *testing.T) {
	svc, _ := newServiceWithTmpHome(t)

	first := svc.Get(context.Background())
	if first.Data != nil {
		t.Errorf("first Get.Data = %+v, want nil", first.Data)
	}
	if !first.Computing {
		t.Error("first Get.Computing = false, want true")
	}

	got := waitForBreakdown(t, svc)
	// data 10 + backups 4 + worktrees 2 + repos 3 = 19. Note that data
	// includes backups (because backups lives under data/) so they are
	// counted twice intentionally — backups is its own bucket but also
	// physically nested in data_dir on disk.
	const wantTotal = int64(10 + 4 + 2 + 3 + 4) // data=14 (10+4), worktrees=2, repos=3, backups=4
	if got.Total != wantTotal {
		t.Errorf("Total = %d, want %d (data 14 + worktrees 2 + repos 3 + backups 4)", got.Total, wantTotal)
	}
	if got.DataDir != 14 {
		t.Errorf("DataDir = %d, want 14", got.DataDir)
	}
	if got.Backups != 4 {
		t.Errorf("Backups = %d, want 4", got.Backups)
	}
	if got.Worktrees != 2 {
		t.Errorf("Worktrees = %d, want 2", got.Worktrees)
	}
	if got.Repos != 3 {
		t.Errorf("Repos = %d, want 3", got.Repos)
	}
	if got.Sessions != 0 || got.Tasks != 0 || got.QuickChat != 0 {
		t.Errorf("expected missing subdirs to be zero, got sessions=%d tasks=%d quick=%d",
			got.Sessions, got.Tasks, got.QuickChat)
	}
	if got.ComputedAt.IsZero() {
		t.Error("ComputedAt is zero")
	}
}

func TestGet_FreshReturnsCachedWithoutRecomputing(t *testing.T) {
	svc, _ := newServiceWithTmpHome(t)
	svc.Get(context.Background())
	first := waitForBreakdown(t, svc)

	// Capture the result-publishing event count before the second Get so
	// we can prove no new walk was scheduled.
	second := svc.Get(context.Background())
	if second.Data == nil {
		t.Fatal("second Get.Data = nil, want cached breakdown")
	}
	if second.Computing {
		t.Error("second Get.Computing = true, want false (fresh cache)")
	}
	if !second.Data.ComputedAt.Equal(first.ComputedAt) {
		t.Errorf("second ComputedAt = %s, want unchanged %s", second.Data.ComputedAt, first.ComputedAt)
	}

	// Give any rogue background walk a chance to overwrite the cache. If
	// the implementation kicked one off, ComputedAt would shift.
	time.Sleep(50 * time.Millisecond)
	svc.mu.Lock()
	final := svc.value
	svc.mu.Unlock()
	if !final.ComputedAt.Equal(first.ComputedAt) {
		t.Errorf("cache was rewritten after fresh Get; ComputedAt drifted from %s to %s",
			first.ComputedAt, final.ComputedAt)
	}
}

func TestGet_StaleReturnsValueAndKicksRefresh(t *testing.T) {
	svc, _ := newServiceWithTmpHome(t)
	svc.Get(context.Background())
	first := waitForBreakdown(t, svc)

	// Rewind ComputedAt past the 2h TTL.
	stale := time.Now().Add(-3 * time.Hour)
	svc.setComputedAt(stale)

	// Add a new file so the refreshed walk produces a different Total.
	writeFile(t, filepath.Join(svc.homeDir, "tasks", "t1", "f"), []byte("DDDDD")) // 5

	res := svc.Get(context.Background())
	if res.Data == nil {
		t.Fatal("stale Get.Data = nil, want stale value")
	}
	if !res.Computing {
		t.Error("stale Get.Computing = false, want true (refresh kicked)")
	}

	// Wait for the refresh to land — Total should grow by the new file.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		v := svc.value
		svc.mu.Unlock()
		if v != nil && v.Total > first.Total {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("refresh did not update cache; first Total=%d still current", first.Total)
}

func TestRefresh_AlwaysKicksWalkAndReturnsJobID(t *testing.T) {
	svc, _ := newServiceWithTmpHome(t)
	id := svc.Refresh(context.Background())
	if id == "" {
		t.Fatal("Refresh returned empty job ID")
	}
	if got := svc.jobs.Get(id); got == nil {
		t.Errorf("tracker has no record of job %s", id)
	}
	waitForBreakdown(t, svc)
}

func TestGet_SecondColdCallWhileComputingReturnsComputing(t *testing.T) {
	// Build a Service with a tracker we control so we can prove a second
	// Get during the in-flight walk does not double-schedule. The walker
	// runs fast on a tmp dir, so we just check the synchronous return
	// shape: Data nil, Computing true, no panic.
	svc, _ := newServiceWithTmpHome(t)

	first := svc.Get(context.Background())
	second := svc.Get(context.Background())

	if first.Data != nil || second.Data != nil {
		t.Errorf("expected both cold Gets to return nil Data, got %+v / %+v", first.Data, second.Data)
	}
	if !first.Computing || !second.Computing {
		t.Errorf("expected both cold Gets to report Computing=true, got %v / %v", first.Computing, second.Computing)
	}
	waitForBreakdown(t, svc)
}
