package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type failOncePendingMoveDeleteRepository struct {
	messagequeue.Repository
	err error
}

type failPendingMoveReadRepository struct {
	messagequeue.Repository
	err error
}

type replaceBeforePendingMoveDeleteRepository struct {
	messagequeue.Repository
	replacement *messagequeue.PendingMove
}

func (r *failPendingMoveReadRepository) GetPendingMove(
	context.Context,
	string,
) (*messagequeue.PendingMove, error) {
	return nil, r.err
}

func (r *replaceBeforePendingMoveDeleteRepository) DeletePendingMoveIfMatch(
	ctx context.Context,
	expected messagequeue.PendingMoveRecord,
	handoffEntryID string,
) (bool, error) {
	if r.replacement != nil {
		replacement := *r.replacement
		r.replacement = nil
		if err := r.SetPendingMove(ctx, expected.SessionID, &replacement); err != nil {
			return false, err
		}
	}
	return r.Repository.DeletePendingMoveIfMatch(ctx, expected, handoffEntryID)
}

func (r *failOncePendingMoveDeleteRepository) DeletePendingMoveIfMatch(
	ctx context.Context,
	expected messagequeue.PendingMoveRecord,
	handoffEntryID string,
) (bool, error) {
	if r.err != nil {
		err := r.err
		r.err = nil
		return false, err
	}
	return r.Repository.DeletePendingMoveIfMatch(ctx, expected, handoffEntryID)
}

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

func TestPendingMove_ExpiredReplayRetriesAfterPromptRemovalFailure(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	sc.stepGetter.steps[stepInReviewID].Events.OnTurnComplete = nil
	entries, _, err := sc.svc.messageQueue.SnapshotSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("snapshot scenario queue: %v", err)
	}
	queueRepo := &failOncePendingMoveDeleteRepository{
		Repository: messagequeue.NewMemoryRepository(),
		err:        errors.New("queue storage unavailable"),
	}
	sc.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	staleMove := &messagequeue.PendingMove{
		MoveID: "move-retry", TaskID: "task-1", WorkflowID: "wf1",
		WorkflowStepID: stepInProgressID, QueuedAt: staleQueuedAt(),
	}
	for i := range entries {
		if entries[i].QueuedBy == messagequeue.QueuedByMoveTask {
			entries[i].Metadata = map[string]interface{}{messagequeue.MetadataDeferredMoveID: staleMove.MoveID}
		}
	}
	if err := sc.svc.messageQueue.RestoreSession(sc.ctx, sc.reviewSessionID, entries, staleMove); err != nil {
		t.Fatalf("restore scenario queue: %v", err)
	}

	sc.svc.handleAgentReady(sc.ctx, watcherAgentReady(sc.reviewSessionID))

	if move, exists := sc.svc.messageQueue.GetPendingMove(sc.ctx, sc.reviewSessionID); !exists {
		t.Fatal("expired move was lost after replay-time prompt cleanup failure")
	} else if move.MoveID != staleMove.MoveID {
		t.Fatalf("preserved move id = %q, want %q", move.MoveID, staleMove.MoveID)
	}
	if got := sc.svc.messageQueue.GetStatus(sc.ctx, sc.reviewSessionID).Count; got != len(entries) {
		t.Fatalf("queue count after replay-time cleanup failure = %d, want %d", got, len(entries))
	}

	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load session after replay-time cleanup failure: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after replay-time cleanup failure = %q, want %q",
			session.State, models.TaskSessionStateWaitingForInput)
	}
}

func TestPendingMove_ReadFailurePreservesTurnForRetry(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	queueRepo := &failPendingMoveReadRepository{
		Repository: messagequeue.NewMemoryRepository(),
		err:        errors.New("queue storage unavailable"),
	}
	sc.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())

	sc.svc.handleAgentReady(sc.ctx, watcherAgentReady(sc.reviewSessionID))

	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task after pending-move read failure: %v", err)
	}
	if task.WorkflowStepID != stepInReviewID {
		t.Fatalf("pending-move read failure advanced task to %q, want %q",
			task.WorkflowStepID, stepInReviewID)
	}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load session after pending-move read failure: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after pending-move read failure = %q, want %q",
			session.State, models.TaskSessionStateWaitingForInput)
	}
}

func TestPendingMove_ReplaysReplacementAfterClaimRace(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	queueRepo := messagequeue.NewMemoryRepository()
	initialMove := &messagequeue.PendingMove{
		MoveID:         "move-initial",
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
		QueuedAt:       time.Now().UTC().Add(-time.Minute),
	}
	replacement := *initialMove
	replacement.MoveID = "move-replacement"
	if err := queueRepo.SetPendingMove(sc.ctx, sc.reviewSessionID, initialMove); err != nil {
		t.Fatalf("seed initial pending move: %v", err)
	}
	sc.svc.messageQueue = messagequeue.NewService(&replaceBeforePendingMoveDeleteRepository{
		Repository:  queueRepo,
		replacement: &replacement,
	}, messagequeue.DefaultMaxPerSession, testLogger())

	sc.svc.handleAgentReady(sc.ctx, watcherAgentReady(sc.reviewSessionID))

	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task after pending-move claim race: %v", err)
	}
	if task.WorkflowStepID != stepInProgressID {
		t.Fatalf("replacement pending move was not applied: workflow_step_id = %q, want %q",
			task.WorkflowStepID, stepInProgressID)
	}
	if _, exists := sc.svc.messageQueue.GetPendingMove(sc.ctx, sc.reviewSessionID); exists {
		t.Fatal("replacement pending move remained armed after replay")
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
		stale, retryPending := svc.discardStalePendingMove(ctx, "task-1", "sess-1", nil)
		if stale || retryPending {
			t.Fatal("nil move reported as discarded")
		}
	})

	t.Run("fresh move is not discarded", func(t *testing.T) {
		move := &messagequeue.PendingMove{TaskID: "task-1", QueuedAt: time.Now().UTC()}
		stale, retryPending := svc.discardStalePendingMove(ctx, "task-1", "sess-1", move)
		if stale || retryPending {
			t.Fatal("fresh move reported as discarded")
		}
	})

	t.Run("stale move is discarded without mutating the caller", func(t *testing.T) {
		move := &messagequeue.PendingMove{TaskID: "task-1", QueuedAt: staleQueuedAt()}
		svc.messageQueue.SetPendingMove(ctx, "sess-1", move)
		stale, retryPending := svc.discardStalePendingMove(ctx, "task-1", "sess-1", move)
		if !stale || retryPending {
			t.Fatal("stale move was not discarded")
		}
		if move.MoveID != "" {
			t.Fatalf("discard mutated caller move id to %q", move.MoveID)
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

func TestReapStalePendingMoves_RetriesAfterPromptRemovalFailure(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	queueRepo := &failOncePendingMoveDeleteRepository{
		Repository: messagequeue.NewMemoryRepository(),
		err:        errors.New("queue storage unavailable"),
	}
	svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	ctx := context.Background()
	const sessionID = "sess-retry"
	const taskID = "task-retry"
	const moveID = "move-retry"

	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, sessionID, taskID, "target-step hand-off", "", messagequeue.QueuedByMoveTask,
		false, nil, map[string]interface{}{messagequeue.MetadataDeferredMoveID: moveID},
	); err != nil {
		t.Fatalf("queue hand-off prompt: %v", err)
	}
	svc.messageQueue.SetPendingMove(ctx, sessionID, &messagequeue.PendingMove{
		MoveID: moveID, TaskID: taskID, WorkflowID: "wf", WorkflowStepID: "target",
		QueuedAt: staleQueuedAt(),
	})

	svc.reapStalePendingMovesOnce(ctx)

	assertArmed(t, svc, sessionID, true)
	if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 1 {
		t.Fatalf("prompt count after failed cleanup = %d, want 1", got)
	}

	svc.reapStalePendingMovesOnce(ctx)

	assertArmed(t, svc, sessionID, false)
	if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 0 {
		t.Fatalf("prompt count after retry = %d, want 0", got)
	}
}

// Reviewer-requested contract coverage: inject a replacement after the sweep
// has listed stale move A but before its delete. The exact-row claim must lose
// that race and preserve fresh move B.
func TestReapStalePendingMoves_PreservesReplacementBetweenListAndDelete(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	queuedAtB := time.Now().UTC().Add(-time.Minute)
	queueRepo := &replaceBeforePendingMoveDeleteRepository{
		Repository: messagequeue.NewMemoryRepository(),
		replacement: &messagequeue.PendingMove{
			MoveID: "move-b", TaskID: "task-b", WorkflowID: "wf-b",
			WorkflowStepID: "step-b", QueuedAt: queuedAtB,
		},
	}
	svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	ctx := context.Background()
	const sessionID = "sess-replaced"
	svc.messageQueue.SetPendingMove(ctx, sessionID, &messagequeue.PendingMove{
		MoveID: "move-a", TaskID: "task-a", WorkflowID: "wf-a",
		WorkflowStepID: "step-a", QueuedAt: staleQueuedAt(),
	})

	svc.reapStalePendingMovesOnce(ctx)

	got, exists := svc.messageQueue.GetPendingMove(ctx, sessionID)
	if !exists {
		t.Fatal("fresh replacement move B was removed by stale move A's sweep")
	}
	if got.MoveID != "move-b" || got.TaskID != "task-b" || !got.QueuedAt.Equal(queuedAtB) {
		t.Fatalf("replacement move B changed: %+v", got)
	}
}

func armPendingMoveInWorkflow(svc *Service, sessionID, taskID, workflowID string, queuedAt time.Time) {
	svc.messageQueue.SetPendingMove(context.Background(), sessionID, &messagequeue.PendingMove{
		MoveID:         "move-" + sessionID,
		TaskID:         taskID,
		WorkflowID:     workflowID,
		WorkflowStepID: "step-blocked",
		QueuedAt:       queuedAt,
	})
}

// TestReapStalePendingMoves_IsRowLocalAcrossWorkflows pins that the sweep
// decides each row on its own queued_at and its own keyed session, never on
// the row's workflow or on its siblings.
//
// pending_moves is a GLOBAL table with no workspace column and a workflow_id
// that is tempting to filter on. Both directions of that temptation are bugs:
// a sweep scoped to one workflow would leave every other workspace's rows
// armed forever (the orphans observed in production spanned two independent
// workflows), while any cross-row logic would let one workspace's state decide
// another's. Neither shows up in a single-workflow fixture, which is why this
// test seeds two.
func TestReapStalePendingMoves_IsRowLocalAcrossWorkflows(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()

	const workflowA = "wf-workspace-a"
	const workflowB = "wf-workspace-b"
	fresh := time.Now().UTC().Add(-time.Minute)

	// Workflow A: one stale row and one healthy row.
	seedPendingMoveSession(t, repo, "task-a-stale", "sess-a-stale")
	seedPendingMoveSession(t, repo, "task-a-fresh", "sess-a-fresh")
	armPendingMoveInWorkflow(svc, "sess-a-stale", "task-a-stale", workflowA, staleQueuedAt())
	armPendingMoveInWorkflow(svc, "sess-a-fresh", "task-a-fresh", workflowA, fresh)

	// Workflow B: the same shapes, plus an orphan within the TTL. Production
	// had exactly this — an orphan in a workflow other than the one under
	// investigation.
	seedPendingMoveSession(t, repo, "task-b-stale", "sess-b-stale")
	seedPendingMoveSession(t, repo, "task-b-fresh", "sess-b-fresh")
	armPendingMoveInWorkflow(svc, "sess-b-stale", "task-b-stale", workflowB, staleQueuedAt())
	armPendingMoveInWorkflow(svc, "sess-b-fresh", "task-b-fresh", workflowB, fresh)
	armPendingMoveInWorkflow(svc, "sess-b-orphan", "task-b-orphan", workflowB, fresh)

	svc.reapStalePendingMovesOnce(ctx)

	for sessionID, wantArmed := range map[string]bool{
		"sess-a-stale":  false,
		"sess-b-stale":  false,
		"sess-b-orphan": false,
		"sess-a-fresh":  true,
		"sess-b-fresh":  true,
	} {
		assertArmed(t, svc, sessionID, wantArmed)
	}
}
