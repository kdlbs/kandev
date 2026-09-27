package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// The task-row lock orders a concurrent move before the settlement's step
// decision on PostgreSQL. Run with KANDEV_TEST_POSTGRES_DSN and -race.
func TestPostgresCompletionIntentSettlementSeesConcurrentMove(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 3)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Task", WorkflowStepID: "step1"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session", StartedAt: now}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, _, err := repo.CreateOrGetCompletionIntent(ctx, &models.CompletionIntent{
		ID: "intent", TaskID: "task", SessionID: "session", TurnID: "turn", WorkflowStepID: "step1",
		State: models.CompletionIntentStatePending, RequestedAt: now, EligibleAt: now,
	}); err != nil {
		t.Fatalf("CreateOrGetCompletionIntent: %v", err)
	}
	if _, err := repo.ClaimCompletionIntentForSettlement(ctx, "intent", now, now.Add(time.Minute)); err != nil {
		t.Fatalf("ClaimCompletionIntentForSettlement: %v", err)
	}

	moveTx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx move: %v", err)
	}
	defer func() { _ = moveTx.Rollback() }()
	if _, err := moveTx.ExecContext(ctx, db.Rebind(`UPDATE tasks SET workflow_step_id = ? WHERE id = ?`), "step2", "task"); err != nil {
		t.Fatalf("move task: %v", err)
	}

	type outcome struct {
		state models.CompletionIntentState
		ok    bool
		err   error
	}
	result := make(chan outcome, 1)
	go func() {
		state, ok, settleErr := repo.CompleteTurnAndTransitionCompletionIntent(ctx, "turn", "intent", models.CompletionIntentStateSettling, now, &models.SessionControlEvent{
			ActorTaskID: "task", ActorSessionID: "session", TargetTaskID: "task",
			TargetSessionID: "session", TargetTurnID: "turn", AuthorityBasis: "same_task_peer", EvidenceCode: "eligible_completion_intent",
		})
		result <- outcome{state: state, ok: ok, err: settleErr}
	}()
	if err := moveTx.Commit(); err != nil {
		t.Fatalf("commit task move: %v", err)
	}
	got := <-result
	if got.err != nil || !got.ok || got.state != models.CompletionIntentStateSuperseded {
		t.Fatalf("settlement = (%q, %v, %v), want superseded true nil", got.state, got.ok, got.err)
	}
	intent, err := repo.GetCompletionIntent(ctx, "intent")
	if err != nil || intent.State != models.CompletionIntentStateSuperseded {
		t.Fatalf("stored intent = (%+v, %v), want superseded", intent, err)
	}
}
