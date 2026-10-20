package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
)

func insertStepTransitionRecordFixture(t *testing.T, repo *Repository, taskID, fromStep, toStep, trigger, actorKind string, actorID any, occurredAt time.Time) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_step_transitions
		(task_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, actor_id, contract_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		taskID, "workflow-1", fromStep, "workflow-1", toStep, trigger, actorKind, actorID, 1, occurredAt); err != nil {
		t.Fatalf("insert ledger row: %v", err)
	}
}

func insertStepTransitionNameFixture(t *testing.T, repo *Repository, stepID, name string) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, ?)`),
		stepID, "workflow-1", name, 0); err != nil {
		t.Fatalf("insert workflow step: %v", err)
	}
}

// @covers AC-TASKS-STEP-TRANSITION-READ-001.1
func TestTaskStepTransitionRecordsAreOldestFirstWithAttribution(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1", Role: authn.RoleMember})
	createStepTransitionsTestTaskWithCtx(t, repo, ctx, "records-task", "workflow-1", "draft")
	insertStepTransitionNameFixture(t, repo, "draft", "Draft")
	insertStepTransitionNameFixture(t, repo, "review", "Review")
	// Equal timestamps: the ledger id, not occurred_at, orders the rows.
	at := time.Now().UTC().Add(time.Minute)
	insertStepTransitionRecordFixture(t, repo, "records-task", "draft", "review", "mcp_move", "agent", "session-1", at)
	insertStepTransitionRecordFixture(t, repo, "records-task", "review", "removed-step", "engine_transition", "system", nil, at)

	rows, err := repo.ListTaskStepTransitionRecords(context.Background(), "records-task", 0, 10)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(rows) != 3 || rows[0].ID >= rows[1].ID || rows[1].ID >= rows[2].ID {
		t.Fatalf("rows = %+v, want three rows in ascending id order", rows)
	}
	genesis := rows[0]
	if genesis.Trigger != "task_created" || genesis.ActorKind != "human" || genesis.ActorID == nil || *genesis.ActorID != "user-1" {
		t.Fatalf("genesis attribution = %+v", genesis)
	}
	if genesis.FromWorkflowStepID != nil || genesis.FromWorkflowStepName != nil || genesis.ToWorkflowStepName == nil || *genesis.ToWorkflowStepName != "Draft" {
		t.Fatalf("genesis endpoints = %+v", genesis)
	}
	agentMove := rows[1]
	if agentMove.Trigger != "mcp_move" || agentMove.ActorKind != "agent" || agentMove.ActorID == nil || *agentMove.ActorID != "session-1" {
		t.Fatalf("agent move attribution = %+v", agentMove)
	}
	if agentMove.FromWorkflowStepName == nil || *agentMove.FromWorkflowStepName != "Draft" || agentMove.ToWorkflowStepName == nil || *agentMove.ToWorkflowStepName != "Review" {
		t.Fatalf("agent move names = %+v", agentMove)
	}
	removed := rows[2]
	if removed.ActorID != nil || removed.ToWorkflowStepID == nil || *removed.ToWorkflowStepID != "removed-step" || removed.ToWorkflowStepName != nil {
		t.Fatalf("removed step row = %+v, want recorded id with null name and null actor", removed)
	}

	page, err := repo.ListTaskStepTransitionRecords(context.Background(), "records-task", rows[0].ID, 1)
	if err != nil || len(page) != 1 || page[0].ID != rows[1].ID {
		t.Fatalf("page after genesis = %+v, err %v", page, err)
	}
}

// @covers AC-TASKS-STEP-TRANSITION-READ-001.3
func TestTaskStepTransitionRecordsEmptyForTaskWithoutMoves(t *testing.T) {
	repo := newStepTransitionsTestRepo(t)
	createStepTransitionsTestTask(t, repo, "no-workflow", "", "")
	rows, err := repo.ListTaskStepTransitionRecords(context.Background(), "no-workflow", 0, 10)
	if err != nil || rows == nil || len(rows) != 0 {
		t.Fatalf("rows = %#v, err %v, want empty non-nil slice", rows, err)
	}
}
