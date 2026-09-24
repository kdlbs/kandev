package statussummary

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// perTaskAlternatingRejectStore models the same "second, unsynchronized
// writer" contention as alternatingRejectProjectorStore (a boot
// reconciliation pass or an HTTP rebuild request racing the live projector),
// but keeps a per-task counter instead of one global counter. The projector's
// per-task lock only guarantees that one task's own CAS attempts stay
// contiguous in a shared *global* ordering when no other task's calls can
// interleave between them; with many tasks contending concurrently on one
// global counter, an unrelated task's accepted call can land between two of
// this task's attempts and break the "reject-then-accept" guarantee within
// maxPendingPersistAttempts. Keying the alternation per task preserves that
// guarantee at scale while still deterministically forcing a genuine CAS
// rejection (and therefore a real retry/rebase) for every task, every cycle.
type perTaskAlternatingRejectStore struct {
	*projectorTestStore
	counterMu sync.Mutex
	counters  map[string]int64
}

func (s *perTaskAlternatingRejectStore) CompareAndUpdateTaskStatusSummary(
	ctx context.Context,
	stored *StoredTaskStatusSummary,
) (bool, error) {
	s.counterMu.Lock()
	if s.counters == nil {
		s.counters = make(map[string]int64)
	}
	s.counters[stored.TaskID]++
	n := s.counters[stored.TaskID]
	s.counterMu.Unlock()
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

// TestProjectorSustainedLoadAcrossManyTasksConvergesWithoutExhaustedCAS is the
// Task 07 statussummary half of the deterministic load-validation fixture
// (plan.md Task 07, "Document operations and validate sustained load"). It
// scales TestProjectorConcurrentSessionEventsConvergeWithoutExhaustedCAS's
// single-task contention harness up to the same 25-canonical-identity, ten
// simulated-cycle shape as the github package's
// TestLoadValidation_TenSimulatedMinutes_NoWatchCreationLoopBoundedLookups,
// standing in for the many simultaneous task/coordinator boards a real
// deployment runs concurrently.
//
// Each of ten simulated poll cycles fires one concurrent burst of session
// observations across 25 tasks (4 sessions per task per cycle = 100
// concurrent HandleEvent calls per cycle, 1000 total) while the shared
// alternatingRejectProjectorStore keeps forcing every other
// CompareAndUpdateTaskStatusSummary call to lose its CAS race - modeling a
// second, unsynchronized writer (a boot reconciliation pass or an HTTP
// rebuild request) racing the live projector throughout the run, not just in
// a single burst. It asserts zero exhausted-CAS handler errors across the
// entire sustained run and that every task's final summary reflects its last
// cycle's session count, proving sustained contention never drops or
// misapplies concurrent work.
func TestProjectorSustainedLoadAcrossManyTasksConvergesWithoutExhaustedCAS(t *testing.T) {
	const (
		taskCount         = 25
		simulatedCycles   = 10
		sessionsPerCycle  = 4
		workspaceID       = "workspace-load"
		activeSubagentCnt = 1
	)

	base := newProjectorTestStore()
	taskIDs := make([]string, taskCount)
	for i := 0; i < taskCount; i++ {
		taskID := fmt.Sprintf("task-load-%02d", i)
		taskIDs[i] = taskID
		base.rows[taskID] = &StoredTaskStatusSummary{
			TaskID:      taskID,
			WorkspaceID: workspaceID,
			Summary:     TaskStatusSummary{Revision: 1},
		}
	}
	store := &perTaskAlternatingRejectStore{projectorTestStore: base}

	var registryMu sync.Mutex
	registry := make(map[string]map[string]RebuildSession) // taskID -> sessionID -> session
	registerSession := func(taskID, sessionID string) {
		registryMu.Lock()
		if registry[taskID] == nil {
			registry[taskID] = make(map[string]RebuildSession)
		}
		registry[taskID][sessionID] = RebuildSession{ID: sessionID, State: sessionStateRunning, ActiveSubagentCount: activeSubagentCnt}
		registryMu.Unlock()
	}
	loadSessions := func(_ context.Context, taskID string) (SessionObservationSnapshot, error) {
		registryMu.Lock()
		defer registryMu.Unlock()
		sessions := make([]RebuildSession, 0, len(registry[taskID]))
		for _, session := range registry[taskID] {
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

	for cycle := 0; cycle < simulatedCycles; cycle++ {
		var wg sync.WaitGroup
		errs := make(chan error, taskCount*sessionsPerCycle)
		for _, taskID := range taskIDs {
			for s := 0; s < sessionsPerCycle; s++ {
				wg.Add(1)
				go func(taskID string, cycle, s int) {
					defer wg.Done()
					sessionID := fmt.Sprintf("session-%d-%02d", cycle, s)
					registerSession(taskID, sessionID)
					err := projector.HandleEvent(context.Background(), bus.NewEvent(
						events.TaskSessionStateChanged, "test", map[string]interface{}{
							"task_id":               taskID,
							"workspace_id":          workspaceID,
							"session_id":            sessionID,
							"new_state":             sessionStateRunning,
							"foreground_activity":   activityGenerating,
							"active_subagent_count": activeSubagentCnt,
						}))
					errs <- err
				}(taskID, cycle, s)
			}
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("cycle %d: concurrent HandleEvent returned error (want zero exhausted-CAS handler errors across the sustained run): %v",
					cycle, err)
			}
		}
	}

	if atomic.LoadInt64(&backoffCalls) == 0 {
		t.Fatal("expected the CAS-retry backoff hook to fire at least once across the sustained run")
	}

	// Every task must reflect the cumulative session count from every cycle:
	// registerSession keys by (cycle, s), so each task accumulates
	// simulatedCycles*sessionsPerCycle distinct running sessions overall.
	wantActive := simulatedCycles * sessionsPerCycle
	for _, taskID := range taskIDs {
		got := base.summary(taskID)
		if got == nil {
			t.Fatalf("task %s: expected a persisted summary after sustained contention", taskID)
		}
		if got.ActiveSubagentCount != wantActive {
			t.Fatalf("task %s: ActiveSubagentCount = %d, want %d (every concurrent session across all cycles preserved)",
				taskID, got.ActiveSubagentCount, wantActive)
		}
		if got.ForegroundActivity != activityGenerating {
			t.Fatalf("task %s: ForegroundActivity = %q, want %q", taskID, got.ForegroundActivity, activityGenerating)
		}
	}
	t.Logf("sustained load: %d tasks x %d cycles x %d sessions/cycle = %d concurrent HandleEvent calls, %d CAS backoff retries",
		taskCount, simulatedCycles, sessionsPerCycle, taskCount*simulatedCycles*sessionsPerCycle, atomic.LoadInt64(&backoffCalls))
}
