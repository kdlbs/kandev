package statussummary

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// alternatingRejectProjectorStore models a second, unsynchronized writer (a
// boot reconciliation pass or an HTTP rebuild request) racing the live
// projector on the same row. It rejects every odd-numbered attempt across the
// whole store by bumping the stored revision out from under the caller, and
// accepts every even-numbered attempt. Because the projector's per-task lock
// makes every attempt within one HandleEvent call (including its own
// rebase-and-retry sequence) contiguous in this global order, this
// deterministically forces at least one genuine CAS rejection under
// concurrency without ever requiring more than one retry per event - keeping
// the test itself non-flaky while still exercising real contention.
type alternatingRejectProjectorStore struct {
	*projectorTestStore
	counter int64
}

func (s *alternatingRejectProjectorStore) CompareAndUpdateTaskStatusSummary(
	ctx context.Context,
	stored *StoredTaskStatusSummary,
) (bool, error) {
	n := atomic.AddInt64(&s.counter, 1)
	if n%2 == 1 {
		s.mu.Lock()
		bumped := StoredTaskStatusSummary{TaskID: stored.TaskID, WorkspaceID: stored.WorkspaceID}
		if previous := s.rows[stored.TaskID]; previous != nil {
			bumped = *previous
		}
		bumped.Summary.Revision++
		s.rows[stored.TaskID] = &bumped
		s.mu.Unlock()
		return false, nil
	}
	return s.projectorTestStore.CompareAndUpdateTaskStatusSummary(ctx, stored)
}

// TestProjectorConcurrentSessionEventsConvergeWithoutExhaustedCAS drives many
// goroutines applying distinct session observations for one task while a
// simulated external writer (boot reconciliation/HTTP rebuild analog) races
// the live projector on the same row. It asserts zero exhausted-CAS handler
// errors, that the jittered backoff hook actually fires under contention, and
// that the final summary reflects every concurrently applied session -
// proving a genuine CAS loss reloads/rebases authoritative state instead of
// silently dropping concurrent work.
func TestProjectorConcurrentSessionEventsConvergeWithoutExhaustedCAS(t *testing.T) {
	const taskID = "task-concurrent-cas"
	base := newProjectorTestStore()
	base.rows[taskID] = &StoredTaskStatusSummary{
		TaskID:      taskID,
		WorkspaceID: "workspace-1",
		Summary:     TaskStatusSummary{Revision: 1},
	}
	store := &alternatingRejectProjectorStore{projectorTestStore: base}

	var registryMu sync.Mutex
	registry := make(map[string]RebuildSession)
	registerSession := func(id string) {
		registryMu.Lock()
		registry[id] = RebuildSession{ID: id, State: sessionStateRunning, ActiveSubagentCount: 1}
		registryMu.Unlock()
	}
	loadSessions := func(context.Context, string) (SessionObservationSnapshot, error) {
		registryMu.Lock()
		defer registryMu.Unlock()
		sessions := make([]RebuildSession, 0, len(registry))
		for _, session := range registry {
			sessions = append(sessions, session)
		}
		return SessionObservationSnapshot{Sessions: sessions, ActivityObserved: true}, nil
	}

	var backoffCalls int64
	projector := NewProjector(ProjectorConfig{
		Store:                   store,
		LoadSessionObservations: loadSessions,
		RetryBackoff: func(context.Context, int) error {
			atomic.AddInt64(&backoffCalls, 1)
			return nil
		},
	})

	const sessionCount = 24
	var wg sync.WaitGroup
	errs := make(chan error, sessionCount)
	for i := 0; i < sessionCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sessionID := fmt.Sprintf("session-%02d", i)
			registerSession(sessionID)
			err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskSessionStateChanged, "test", map[string]interface{}{
				"task_id":               taskID,
				"workspace_id":          "workspace-1",
				"session_id":            sessionID,
				"new_state":             sessionStateRunning,
				"foreground_activity":   activityGenerating,
				"active_subagent_count": 1,
			}))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent HandleEvent returned error (want zero exhausted-CAS handler errors): %v", err)
		}
	}

	if atomic.LoadInt64(&backoffCalls) == 0 {
		t.Fatal("expected the CAS-retry backoff hook to fire at least once under contention")
	}

	got := base.summary(taskID)
	if got == nil {
		t.Fatal("expected a persisted summary after concurrent contention")
	}
	if got.ActiveSubagentCount != sessionCount {
		t.Fatalf("ActiveSubagentCount = %d, want %d (every concurrently applied session preserved)",
			got.ActiveSubagentCount, sessionCount)
	}
	if got.ForegroundActivity != activityGenerating {
		t.Fatalf("ForegroundActivity = %q, want %q", got.ForegroundActivity, activityGenerating)
	}
}

// TestProjectorCoalescesConcurrentEquivalentPendingRefreshes drives many
// goroutines that all apply the exact same session observation for one task
// concurrently. Because the projector serializes per-task application and
// treats a semantically-equal derived summary as an already-applied no-op,
// only the first genuinely-changing call should persist or publish; every
// other concurrent duplicate must coalesce into that same outcome instead of
// producing its own store write or event.
func TestProjectorCoalescesConcurrentEquivalentPendingRefreshes(t *testing.T) {
	const taskID = "task-coalesce"
	store := newProjectorTestStore()
	eventBus := bus.NewMemoryEventBus(logger.Default())
	defer eventBus.Close()
	var updates atomic.Int64
	if _, err := eventBus.Subscribe(events.TaskStatusSummaryUpdated, func(context.Context, *bus.Event) error {
		updates.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	projector := NewProjector(ProjectorConfig{
		Store:    store,
		EventBus: eventBus,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := projector.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer projector.Close()

	const callers = 20
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := projector.HandleEvent(context.Background(), bus.NewEvent(events.TaskSessionStateChanged, "test", map[string]interface{}{
				"task_id":               taskID,
				"workspace_id":          "workspace-1",
				"session_id":            "session-shared",
				"new_state":             sessionStateRunning,
				"is_primary":            true,
				"foreground_activity":   activityGenerating,
				"active_subagent_count": 3,
			}))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent equivalent HandleEvent returned error: %v", err)
		}
	}

	// Publish is synchronous in the in-memory bus, so by the time every
	// goroutine's HandleEvent call has returned, every publish it triggered
	// has already been delivered.
	if got := updates.Load(); got != 1 {
		t.Fatalf("SummaryUpdated publish count = %d, want exactly 1 (coalesced)", got)
	}
	got := store.summary(taskID)
	if got == nil || got.ActiveSubagentCount != 3 || got.ForegroundActivity != activityGenerating {
		t.Fatalf("summary = %+v, want single coalesced application", got)
	}
	if got.Revision != 1 {
		t.Fatalf("revision = %d, want 1 (only the first change persisted)", got.Revision)
	}
}

func TestDefaultCASRetryBackoffRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := defaultCASRetryBackoff(ctx, 5)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("cancelled backoff took %s, want near-instant", elapsed)
	}
}

func TestCASRetryJitterStaysWithinBounds(t *testing.T) {
	base := 40 * time.Millisecond
	for i := 0; i < 200; i++ {
		jitter := casRetryJitter(base)
		if jitter < 0 || jitter > base/4 {
			t.Fatalf("jitter = %s, want within [0, %s]", jitter, base/4)
		}
	}
}

func TestDefaultCASRetryBackoffDelayGrowsAndCaps(t *testing.T) {
	for attempt := 0; attempt < 6; attempt++ {
		start := time.Now()
		if err := defaultCASRetryBackoff(context.Background(), attempt); err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", attempt, err)
		}
		elapsed := time.Since(start)
		maxAllowed := casRetryMaxDelay + casRetryMaxDelay/4 + 30*time.Millisecond // scheduling slack
		if elapsed > maxAllowed {
			t.Fatalf("attempt %d backoff took %s, want <= %s", attempt, elapsed, maxAllowed)
		}
	}
}
