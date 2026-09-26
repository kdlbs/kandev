package service

// Ledger attribution coverage for the service-layer callers that wrap
// steptelemetry.Attribution onto ctx: MoveTaskWithOptions (manual_move,
// outermost-caller-wins), BulkMoveSelectedTasks (bulk_move), UpdateTask
// (task_update), and pullNextTaskOnVacate (wip_pull).

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestMoveTaskWithOptionsRecordsManualMoveByDefault(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-manual-move", "step-a")

	if _, err := svc.MoveTaskWithOptions(ctx, "task-manual-move", "wf-stamp", "step-b", 0, MoveTaskOptions{}); err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}

	trigger, actorKind, _, _ := lastLedgerAttribution(t, repo, "task-manual-move")
	if trigger != string(steptelemetry.TriggerManualMove) {
		t.Fatalf("trigger = %q, want %q", trigger, steptelemetry.TriggerManualMove)
	}
	if actorKind != string(steptelemetry.ActorSystem) {
		t.Fatalf("actor_kind = %q, want %q (no identity on a bare context)", actorKind, steptelemetry.ActorSystem)
	}
}

// TestMoveTaskWithOptionsRecordsHumanActorFromIdentity proves spec.md:504-507's
// actual claim ("actor kind human, and actor_id equal to the acting user's
// ID") — the sibling test above only exercises the no-identity default. Also
// covers spec.md:590-592 (the synthetic auth-disabled identity moving a
// card): authn.WithIdentity with Synthetic:true resolves to the same
// ActorHuman/UserID pair HumanOrSystemActor already gives a real
// authenticated identity, so one test proves both.
func TestMoveTaskWithOptionsRecordsHumanActorFromIdentity(t *testing.T) {
	svc, _, repo := createTestService(t)
	setupStepStampTask(t, repo, "task-manual-move-human", "step-a")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "user-1", Role: authn.RoleAdmin, Synthetic: true,
	})

	if _, err := svc.MoveTaskWithOptions(ctx, "task-manual-move-human", "wf-stamp", "step-b", 0, MoveTaskOptions{}); err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}

	trigger, actorKind, actorID, _ := lastLedgerAttribution(t, repo, "task-manual-move-human")
	if trigger != string(steptelemetry.TriggerManualMove) {
		t.Fatalf("trigger = %q, want %q", trigger, steptelemetry.TriggerManualMove)
	}
	if actorKind != string(steptelemetry.ActorHuman) {
		t.Fatalf("actor_kind = %q, want %q", actorKind, steptelemetry.ActorHuman)
	}
	if actorID == nil || *actorID != "user-1" {
		t.Fatalf("actor_id = %v, want user-1", actorID)
	}
}

func TestMoveTaskWithOptionsPreservesOuterMCPMoveTrigger(t *testing.T) {
	svc, _, repo := createTestService(t)
	setupStepStampTask(t, repo, "task-outer-trigger", "step-a")

	ctx := steptelemetry.WithAttribution(context.Background(), steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerMCPMove, ActorKind: steptelemetry.ActorAgent, ActorID: "sess-1",
	})
	if _, err := svc.MoveTaskWithOptions(ctx, "task-outer-trigger", "wf-stamp", "step-b", 0, MoveTaskOptions{}); err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}

	trigger, _, _, _ := lastLedgerAttribution(t, repo, "task-outer-trigger")
	if trigger != string(steptelemetry.TriggerMCPMove) {
		t.Fatalf("trigger = %q, want %q (outer caller must win over manual_move default)", trigger, steptelemetry.TriggerMCPMove)
	}
}

func TestBulkMoveSelectedTasksRecordsBulkMove(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-bulk-1", "step-a")

	if _, err := svc.BulkMoveSelectedTasks(ctx, []string{"task-bulk-1"}, "wf-stamp", "step-b"); err != nil {
		t.Fatalf("BulkMoveSelectedTasks: %v", err)
	}

	trigger, _, _, sessionID := lastLedgerAttribution(t, repo, "task-bulk-1")
	if trigger != string(steptelemetry.TriggerBulkMove) {
		t.Fatalf("trigger = %q, want %q", trigger, steptelemetry.TriggerBulkMove)
	}
	if sessionID != nil {
		t.Fatalf("session_id = %v, want NULL (spec.md:508-510)", sessionID)
	}
}

func TestServiceUpdateTaskWithNewStepRecordsTaskUpdate(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-update-step", "step-a")

	newStep := "step-b"
	if _, err := svc.UpdateTask(ctx, "task-update-step", &UpdateTaskRequest{WorkflowStepID: &newStep}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	trigger, _, _, _ := lastLedgerAttribution(t, repo, "task-update-step")
	if trigger != string(steptelemetry.TriggerTaskUpdate) {
		t.Fatalf("trigger = %q, want %q", trigger, steptelemetry.TriggerTaskUpdate)
	}
}

func TestServiceUpdateTaskWithoutStepChangeRecordsNoLedgerRow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupStepStampTask(t, repo, "task-update-nostep", "step-a")

	before := countLedgerRows(t, repo, "task-update-nostep")
	newDesc := "updated description"
	if _, err := svc.UpdateTask(ctx, "task-update-nostep", &UpdateTaskRequest{Description: &newDesc}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	after := countLedgerRows(t, repo, "task-update-nostep")
	if after != before {
		t.Fatalf("rows after non-step update = %d, want unchanged %d", after, before)
	}
}

// staticLedgerStepGetter is a minimal WorkflowStepGetter that returns fixed
// steps, including PullFromStepID, so a feeder-step promotion (which
// actually changes workflow_step_id, unlike a same-step admission) can be
// exercised without the full workflow package.
type staticLedgerStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
}

func (g *staticLedgerStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	step, ok := g.steps[stepID]
	if !ok {
		return nil, fmt.Errorf("workflow step not found: %s", stepID)
	}
	return step, nil
}

func (*staticLedgerStepGetter) GetNextStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func TestPullNextTaskOnVacatePromotesFeederTaskWithWIPPull(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	svc.SetWorkflowStepGetter(&staticLedgerStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-vacate":    {ID: "step-vacate", WorkflowID: "wf-stamp", PullFromStepID: "step-feeder"},
		"step-elsewhere": {ID: "step-elsewhere", WorkflowID: "wf-stamp"},
		"step-feeder":    {ID: "step-feeder", WorkflowID: "wf-stamp"},
	}})

	setupStepStampTask(t, repo, "task-occupant-wip", "step-vacate")
	feederTask := &models.Task{
		ID: "task-feeder-wip", WorkspaceID: "ws-stamp", WorkflowID: "wf-stamp",
		WorkflowStepID: "step-feeder", QueuedForStepID: "step-vacate", WIPAdmitted: false,
		Title: "Queued in feeder", Priority: "medium", State: v1.TaskStateTODO,
	}
	if err := repo.CreateTask(ctx, feederTask); err != nil {
		t.Fatalf("CreateTask feeder: %v", err)
	}
	// spec.md:511-513's scenario is specifically "a task with TWO live
	// sessions" — the point being that the writer must not opportunistically
	// grab one of them as the initiator. Seed both so the assertion below
	// can't pass merely because there was nothing to grab. WAITING_FOR_INPUT
	// (not RUNNING/STARTING) so feederCandidateBlocked's unrelated "don't
	// promote a task with an in-flight session" rule doesn't itself block
	// the promotion this test is trying to observe.
	for _, id := range []string{"session-feeder-wip-1", "session-feeder-wip-2"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: id, TaskID: "task-feeder-wip", State: models.TaskSessionStateWaitingForInput,
		}); err != nil {
			t.Fatalf("CreateTaskSession %s: %v", id, err)
		}
	}

	if _, err := svc.MoveTaskWithOptions(ctx, "task-occupant-wip", "wf-stamp", "step-elsewhere", 0, MoveTaskOptions{}); err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}

	reread, err := repo.GetTask(ctx, "task-feeder-wip")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reread.WorkflowStepID != "step-vacate" {
		t.Fatalf("feeder task WorkflowStepID = %q, want promoted to step-vacate", reread.WorkflowStepID)
	}

	trigger, actorKind, _, sessionID := lastLedgerAttribution(t, repo, "task-feeder-wip")
	if trigger != string(steptelemetry.TriggerWIPPull) {
		t.Fatalf("trigger = %q, want %q", trigger, steptelemetry.TriggerWIPPull)
	}
	if actorKind != string(steptelemetry.ActorSystem) {
		t.Fatalf("actor_kind = %q, want %q", actorKind, steptelemetry.ActorSystem)
	}
	if sessionID != nil {
		t.Fatalf("session_id = %v, want NULL (task has two live sessions, neither is the initiator)", sessionID)
	}
}

func TestService_MoveTaskFeederPullRecordsManualLedgerBeforeWIPPull(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-c": {ID: "step-c", WorkflowID: "wf-source", Name: "C", Position: 2},
		"step-a": {ID: "step-a", WorkflowID: "wf-source", Name: "A", Position: 0},
		"step-b": {
			ID: "step-b", WorkflowID: "wf-source", Name: "B", Position: 1,
			WIPLimit: 1, PullFromStepID: "step-a",
		},
	}})
	createMoveTask(t, ctx, repo, "task-ledger-causal-move", "wf-source", "step-c", nil)

	if _, err := svc.MoveTask(ctx, "task-ledger-causal-move", "wf-source", "step-a", 0); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	triggers := ledgerTriggersForTask(t, repo, "task-ledger-causal-move")
	want := []string{
		string(steptelemetry.TriggerTaskCreated),
		string(steptelemetry.TriggerManualMove),
		string(steptelemetry.TriggerWIPPull),
	}
	if len(triggers) != len(want) {
		t.Fatalf("ledger triggers = %v, want %v", triggers, want)
	}
	for index := range want {
		if triggers[index] != want[index] {
			t.Fatalf("ledger triggers = %v, want %v", triggers, want)
		}
	}
}

func ledgerTriggersForTask(t *testing.T, repo *sqliterepo.Repository, taskID string) []string {
	t.Helper()
	rows, err := repo.DB().QueryContext(context.Background(), `
		SELECT trigger FROM task_step_transitions WHERE task_id = ? ORDER BY id ASC
	`, taskID)
	if err != nil {
		t.Fatalf("query ledger triggers for %s: %v", taskID, err)
	}
	defer func() { _ = rows.Close() }()

	var triggers []string
	for rows.Next() {
		var trigger string
		if err := rows.Scan(&trigger); err != nil {
			t.Fatalf("scan ledger trigger: %v", err)
		}
		triggers = append(triggers, trigger)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate ledger triggers: %v", err)
	}
	return triggers
}

func lastLedgerAttribution(t *testing.T, repo *sqliterepo.Repository, taskID string) (trigger, actorKind string, actorID, sessionID *string) {
	t.Helper()
	row := repo.DB().QueryRowContext(context.Background(), `
		SELECT trigger, actor_kind, actor_id, session_id FROM task_step_transitions
		WHERE task_id = ? ORDER BY occurred_at DESC, id DESC LIMIT 1
	`, taskID)
	if err := row.Scan(&trigger, &actorKind, &actorID, &sessionID); err != nil {
		t.Fatalf("scan last ledger row for %s: %v", taskID, err)
	}
	return trigger, actorKind, actorID, sessionID
}

func countLedgerRows(t *testing.T, repo *sqliterepo.Repository, taskID string) int {
	t.Helper()
	var count int
	if err := repo.DB().QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?
	`, taskID).Scan(&count); err != nil {
		t.Fatalf("count ledger rows for %s: %v", taskID, err)
	}
	return count
}
