package orchestrator

// step_entry_dispatch_atomic_test.go covers AC-B6: when engineDecisions is
// wired the way production actually wires it — workflowadapters.NewDecisionAdapter
// over the real workflow repository, sharing the same writer DB as the task
// repository (backendapp/main.go:1501, backendapp/storage.go) — clear_decisions
// must go through dispatchClearDecisionsAtomic's one-transaction path, not the
// generic callback.Execute + CompleteStepEntryMarker sequence the other
// step_entry_dispatch_test.go fixtures exercise with a fakeDecisionStore.
//
// This test cannot observe the transaction boundary directly (that is
// step_entries_test.go's job, at the repository layer), but it does prove the
// capability-detection wiring actually engages against a real
// *workflowadapters.DecisionAdapter — a wrong type assertion, a mismatched
// entryID/position, or a step_id mixup would surface here as decisions never
// clearing or the marker never completing, exactly like it would in
// production.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/repository"
	workflowadapters "github.com/kandev/kandev/internal/workflow/adapters"
	"github.com/kandev/kandev/internal/workflow/engine"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/workflow/stepentry"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newSharedDBReviewLoopFixture is newReviewLoopFixture's sibling: it wires
// svc.engineDecisions to a real *workflowadapters.DecisionAdapter backed by a
// real *workflowrepository.Repository sharing the task repository's writer
// *sqlx.DB, matching production's backendapp/storage.go wiring exactly.
func newSharedDBReviewLoopFixture(t *testing.T) *reviewLoopFixture {
	t.Helper()
	sg, nameToID := buildWorkflowFromJSON(t, reviewLoopWorkflowJSON)

	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	taskRepo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	workflowRepo, err := workflowrepository.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("create workflow repository: %v", err)
	}

	seedSession(t, taskRepo, "t1", "s1", nameToID["Work"])
	setSessionExecID(t, taskRepo, "s1", "exec-1")

	agentMgr := &mockAgentManager{repoForExecutionLookup: taskRepo, isAgentRunning: true}
	svc := createEngineService(t, taskRepo, sg, agentMgr)

	runQueue := &stepEntryFakeRunQueue{}
	participants := &fakeParticipantStore{participants: []engine.ParticipantInfo{
		{ID: "participant-1", StepID: nameToID["Review"], Role: "reviewer", AgentProfileID: "agent-reviewer-1"},
	}}
	svc.SetEngineRunQueue(runQueue)
	svc.SetEngineParticipantStore(participants)
	svc.SetEngineDecisionStore(workflowadapters.NewDecisionAdapter(workflowRepo))

	mockRepo := svc.taskRepo.(*mockTaskRepo)
	seedMockTaskState(mockRepo, "t1", v1.TaskStateInProgress)

	return &reviewLoopFixture{
		svc: svc, repo: taskRepo, agentMgr: agentMgr,
		runQueue: runQueue, nameToID: nameToID, taskRepoMk: mockRepo,
	}
}

// TestProcessOnEnter_ClearDecisions_RealDecisionAdapter_ClearsAndCompletesAtomically
// covers AC-B6 end to end: with engineDecisions wired as the real production
// adapter, an existing decision from a prior round is cleared and the
// clear_decisions marker lands done — the same observable outcome the
// fallback path produces, but now driven through
// Repository.ClearStepDecisionsAndCompleteMarker's single transaction rather
// than two independent calls.
func TestProcessOnEnter_ClearDecisions_RealDecisionAdapter_ClearsAndCompletesAtomically(t *testing.T) {
	ctx := context.Background()
	f := newSharedDBReviewLoopFixture(t)

	reviewStepID := f.nameToID["Review"]
	decisions := workflowadapters.NewDecisionAdapter(mustWorkflowRepoFromDecisionStore(t, f.svc))
	if err := decisions.RecordStepDecision(ctx, engine.DecisionInfo{
		ID: "dec-stale", TaskID: "t1", StepID: reviewStepID, ParticipantID: "participant-0", Decision: "approve",
	}); err != nil {
		t.Fatalf("seed stale decision: %v", err)
	}

	if !f.fireOnTurnComplete(t, ctx) {
		t.Fatalf("expected a transition from Work -> Review, got none")
	}
	assertStepByName(t, ctx, f.repo, "s1", "Review", f.nameToID)

	if got := f.runQueue.callCount(); got != 1 {
		t.Fatalf("queued run count = %d, want exactly 1", got)
	}

	remaining, err := decisions.ListStepDecisions(ctx, "t1", reviewStepID)
	if err != nil {
		t.Fatalf("ListStepDecisions: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining decisions = %d, want 0 (AC-B6 atomic clear_decisions)", len(remaining))
	}

	// The run-queued assertion above only proves dispatchOnEnterActions
	// continued past clear_decisions — it does not directly prove
	// ClearStepDecisionsAndCompleteMarker's own marker-write half of its one
	// transaction actually committed "done" rather than, say, leaving the
	// marker in_progress while a downstream code path incorrectly continued
	// anyway. entryID 1: the fresh per-test database's very first allocated
	// workflow_step_entries row (same reasoning as
	// step_entry_dispatch_cas_loser_test.go's sibling tests); position 0 is
	// clear_decisions, the first action reviewLoopWorkflowJSON's Review step
	// declares.
	const entryID, clearDecisionsPosition = int64(1), 0
	state, cause, found, err := f.repo.GetStepEntryMarkerState(ctx, entryID, clearDecisionsPosition)
	if err != nil {
		t.Fatalf("GetStepEntryMarkerState: %v", err)
	}
	if !found {
		t.Fatalf("expected a clear_decisions marker to exist for entry %d position %d", entryID, clearDecisionsPosition)
	}
	if state != stepentry.MarkerDone {
		t.Fatalf("clear_decisions marker state = %q (cause=%q), want %q (AC-B6: the atomic path must complete the marker inside the same transaction as the delete)", state, cause, stepentry.MarkerDone)
	}
}

// mustWorkflowRepoFromDecisionStore extracts the *workflowadapters.DecisionAdapter
// wired by newSharedDBReviewLoopFixture, so the test can seed/read decisions
// through the identical repository instance the atomic path writes through.
func mustWorkflowRepoFromDecisionStore(t *testing.T, svc *Service) workflowadapters.WorkflowRepo {
	t.Helper()
	adapter, ok := svc.engineDecisions.(*workflowadapters.DecisionAdapter)
	if !ok {
		t.Fatalf("svc.engineDecisions is %T, want *workflowadapters.DecisionAdapter", svc.engineDecisions)
	}
	return adapter.Repo
}
