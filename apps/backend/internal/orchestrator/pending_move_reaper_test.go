package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// watcherAgentReady builds the agent.ready event handleAgentReady receives
// when the review session's turn ends.
func watcherAgentReady(sessionID string) watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID:           "task-1",
		SessionID:        sessionID,
		AgentExecutionID: "ae-review",
		AgentProfileID:   profileReview,
	}
}

// staleQueuedAt is comfortably past PendingMoveTTL. The live rows that
// motivated this work had been armed for nine days.
func staleQueuedAt() time.Time {
	return time.Now().UTC().Add(-messagequeue.PendingMoveTTL - 9*24*time.Hour)
}

// TestPendingMove_ExpiredMoveIsNotReplayed is the replay-time regression guard
// and the direct reproduction of the reported defect: a move armed days ago
// must not relocate the card when its session next reaches turn-end.
//
// The scenario's In Review step normally carries an unconditional
// on_turn_complete transition. It is cleared here so the assertion isolates
// one thing: the *pending move* did not fire. Whether the ordinary workflow
// evaluation would then have moved the card is a separate behavior, covered by
// the scenario's own tests.
func TestPendingMove_ExpiredMoveIsNotReplayed(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	sc.stepGetter.steps[stepInReviewID].Events.OnTurnComplete = nil

	// Re-arm the move the scenario queued, aged past the TTL. The hand-off
	// prompt the scenario queued alongside it stays as it was.
	sc.svc.messageQueue.SetPendingMove(sc.ctx, sc.reviewSessionID, &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
		QueuedAt:       staleQueuedAt(),
	})

	sc.svc.handleAgentReady(sc.ctx, watcherAgentReady(sc.reviewSessionID))

	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.WorkflowStepID != stepInReviewID {
		t.Fatalf("expired pending move relocated the card: workflow_step_id = %q, want %q",
			task.WorkflowStepID, stepInReviewID)
	}

	if move, exists := sc.svc.messageQueue.GetPendingMove(sc.ctx, sc.reviewSessionID); exists {
		t.Fatalf("expired move still armed after replay: %+v", move)
	}

	// The hand-off prompt was authored for the move's target step. With the
	// move dropped, leaving it queued would misdeliver it to the source step's
	// agent on some later turn.
	for _, entry := range sc.svc.messageQueue.GetStatus(sc.ctx, sc.reviewSessionID).Entries {
		if entry.QueuedBy == messagequeue.QueuedByMoveTask {
			t.Fatalf("hand-off prompt for the expired move survived: %+v", entry)
		}
	}
}

// TestPendingMove_FreshMoveStillReplays guards the happy path against the new
// TTL check: a move armed moments ago must still apply exactly as before.
func TestPendingMove_FreshMoveStillReplays(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	sc.svc.messageQueue.SetPendingMove(sc.ctx, sc.reviewSessionID, &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
		QueuedAt:       time.Now().UTC().Add(-time.Minute),
	})

	sc.svc.handleAgentReady(sc.ctx, watcherAgentReady(sc.reviewSessionID))

	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.WorkflowStepID != stepInProgressID {
		t.Fatalf("fresh pending move did not apply: workflow_step_id = %q, want %q",
			task.WorkflowStepID, stepInProgressID)
	}
}

func TestDiscardStalePendingMove(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()

	t.Run("nil move is not discarded", func(t *testing.T) {
		if svc.discardStalePendingMove(ctx, "task-1", "sess-1", nil) {
			t.Fatal("nil move reported as discarded")
		}
	})

	t.Run("fresh move is not discarded", func(t *testing.T) {
		move := &messagequeue.PendingMove{TaskID: "task-1", QueuedAt: time.Now().UTC()}
		if svc.discardStalePendingMove(ctx, "task-1", "sess-1", move) {
			t.Fatal("fresh move reported as discarded")
		}
	})

	t.Run("stale move is discarded and gets a correlation id", func(t *testing.T) {
		move := &messagequeue.PendingMove{TaskID: "task-1", QueuedAt: staleQueuedAt()}
		if !svc.discardStalePendingMove(ctx, "task-1", "sess-1", move) {
			t.Fatal("stale move was not discarded")
		}
		if move.MoveID == "" {
			t.Fatal("discarded move left without a correlation id")
		}
	})
}

// --- sweep ---

// seedPendingMoveSession creates the task and session rows the sweep's
// existence check reads. Sessions seeded here are "live"; a session ID that is
// never seeded reproduces the orphaned-keyed-session row.
func seedPendingMoveSession(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := repo.GetTask(ctx, taskID); err != nil {
		if err := repo.CreateTask(ctx, &models.Task{
			ID: taskID, Title: taskID, State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("create task %s: %v", taskID, err)
		}
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create session %s: %v", sessionID, err)
	}
}

func armPendingMove(svc *Service, sessionID, taskID, stepID string, queuedAt time.Time) {
	svc.messageQueue.SetPendingMove(context.Background(), sessionID, &messagequeue.PendingMove{
		MoveID:         "move-" + sessionID,
		TaskID:         taskID,
		WorkflowID:     "wf1",
		WorkflowStepID: stepID,
		QueuedAt:       queuedAt,
	})
}

func assertArmed(t *testing.T, svc *Service, sessionID string, want bool) {
	t.Helper()
	_, exists := svc.messageQueue.GetPendingMove(context.Background(), sessionID)
	if exists != want {
		t.Fatalf("session %s armed = %v, want %v", sessionID, exists, want)
	}
}

// TestReapStalePendingMoves covers both sweep reasons in one table, including
// the orphaned-keyed-session case that no replay-time check can ever reach: a
// session that no longer exists will never emit another agent.ready, so its row
// would otherwise stay armed forever.
//
// The two "task-multi" rows mirror the live reproduction, where one task
// carried several armed rows: session_id is UNIQUE in pending_moves, task_id is
// not. Reaping must be per row, never per task.
func TestReapStalePendingMoves(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()

	fresh := time.Now().UTC().Add(-time.Minute)

	seedPendingMoveSession(t, repo, "task-expired", "sess-expired")
	armPendingMove(svc, "sess-expired", "task-expired", "step-blocked", staleQueuedAt())

	seedPendingMoveSession(t, repo, "task-fresh", "sess-fresh")
	armPendingMove(svc, "sess-fresh", "task-fresh", "step-work", fresh)

	// Orphan: armed well within the TTL, but the keyed session is gone.
	armPendingMove(svc, "sess-orphan", "task-orphan", "step-human-qa", fresh)

	// One task, two sessions: one stale, one fresh.
	seedPendingMoveSession(t, repo, "task-multi", "sess-multi-stale")
	seedPendingMoveSession(t, repo, "task-multi", "sess-multi-fresh")
	armPendingMove(svc, "sess-multi-stale", "task-multi", "step-ci-fixup", staleQueuedAt())
	armPendingMove(svc, "sess-multi-fresh", "task-multi", "step-work", fresh)

	svc.reapStalePendingMovesOnce(ctx)

	for sessionID, wantArmed := range map[string]bool{
		"sess-expired":     false, // aged past the TTL
		"sess-orphan":      false, // keyed session no longer exists
		"sess-multi-stale": false, // aged past the TTL
		"sess-fresh":       true,  // healthy
		"sess-multi-fresh": true,  // healthy sibling of a reaped row
	} {
		assertArmed(t, svc, sessionID, wantArmed)
	}

	// The sweep is idempotent: a second tick neither errors nor disturbs the
	// rows it correctly preserved.
	svc.reapStalePendingMovesOnce(ctx)
	assertArmed(t, svc, "sess-fresh", true)
	assertArmed(t, svc, "sess-multi-fresh", true)
}

// TestReapStalePendingMoves_PreservesRowOnSessionLookupError pins the
// fail-closed invariant: a row is reaped for a missing session only when the
// lookup positively says the session is missing. A transient failure must leave
// the row armed for the next tick, never delete it on a guess.
func TestReapStalePendingMoves_PreservesRowOnSessionLookupError(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()

	armPendingMove(svc, "sess-unknown", "task-1", "step-blocked", time.Now().UTC().Add(-time.Minute))

	// Close the database so GetTaskSession fails with a real infrastructure
	// error rather than sql.ErrNoRows. Only ErrNoRows becomes
	// ErrTaskSessionNotFound, so this is exactly the "cannot tell" case.
	if err := repo.DB().Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	svc.reapStalePendingMovesOnce(ctx)

	assertArmed(t, svc, "sess-unknown", true)
}

// TestReapStalePendingMoves_NoQueueIsNoOp guards the nil-service path used by
// orchestrator instances constructed without a message queue.
func TestReapStalePendingMoves_NoQueueIsNoOp(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.messageQueue = nil
	svc.reapStalePendingMovesOnce(context.Background())
}
