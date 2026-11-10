package github

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTaskPRWorkflowAttentionRoundTripAndHeadScopedRecovery(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	attention := &WorkflowAttention{
		State:      WorkflowAttentionApprovalRequired,
		HeadSHA:    "old-head",
		ObservedAt: now,
		Runs:       []WorkflowAttentionRun{{RunID: 7, RunAttempt: 1, WorkflowID: 9, Name: "Run tests", URL: "https://github.com/acme/widget/actions/runs/7", Reason: workflowAttentionApprovalRequiredReason}},
	}
	tp := &TaskPR{
		TaskID:            "workflow-attention-task",
		WorkspaceID:       "workflow-attention-workspace",
		Owner:             "acme",
		Repo:              "widget",
		PRNumber:          143,
		PRURL:             "https://github.com/acme/widget/pull/143",
		PRTitle:           "workflow attention",
		HeadBranch:        "feature",
		BaseBranch:        "main",
		HeadSHA:           "old-head",
		State:             "open",
		WorkflowAttention: attention,
		CreatedAt:         now,
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}
	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if got.WorkflowAttention == nil || got.WorkflowAttention.State != WorkflowAttentionApprovalRequired {
		t.Fatalf("round-trip workflow attention = %#v, want approval_required", got.WorkflowAttention)
	}

	unknown := &PRStatus{
		PR:                         &PR{HeadSHA: "old-head"},
		WorkflowAttention:          workflowAttentionUnknown("old-head"),
		WorkflowAttentionPopulated: true,
	}
	got.WorkflowAttention = resolveTaskPRWorkflowAttention(got, unknown, "old-head")
	if err := store.UpdateTaskPR(ctx, got); err != nil {
		t.Fatalf("UpdateTaskPR (unavailable same head): %v", err)
	}
	got, err = store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID after unavailable read: %v", err)
	}
	if got.WorkflowAttention == nil || got.WorkflowAttention.State != WorkflowAttentionApprovalRequired || !got.WorkflowAttention.Stale {
		t.Fatalf("same-head unavailable attention = %#v, want stale approval", got.WorkflowAttention)
	}

	newHeadStatus := &PRStatus{
		PR:                         &PR{HeadSHA: "new-head"},
		WorkflowAttention:          workflowAttentionUnknown("new-head"),
		WorkflowAttentionPopulated: true,
	}
	got.HeadSHA = "new-head"
	got.WorkflowAttention = resolveTaskPRWorkflowAttention(got, newHeadStatus, "new-head")
	if err := store.UpdateTaskPR(ctx, got); err != nil {
		t.Fatalf("UpdateTaskPR (new head): %v", err)
	}
	got, err = store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID after head change: %v", err)
	}
	if got.WorkflowAttention == nil || got.WorkflowAttention.State != WorkflowAttentionUnknown || got.WorkflowAttention.HeadSHA != "new-head" || HasActiveWorkflowAttention(got) {
		t.Fatalf("new-head attention = %#v, want unknown without active old claim", got.WorkflowAttention)
	}
}

func TestTaskPRWorkflowAttentionHeadGuardedWrite(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tp := &TaskPR{
		TaskID: "workflow-guard-task", WorkspaceID: "workflow-guard-workspace",
		Owner: "acme", Repo: "widget", PRNumber: 7, HeadSHA: "head-a",
		State: "open", CreatedAt: now,
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}
	approval := &WorkflowAttention{
		State: WorkflowAttentionApprovalRequired, HeadSHA: "head-a", ObservedAt: now,
		Runs: []WorkflowAttentionRun{{RunID: 1, Name: "CI", Reason: workflowAttentionApprovalRequiredReason}},
	}

	if row, changed, err := store.UpdateTaskPRWorkflowAttention(
		ctx, tp.TaskID, tp.RepositoryID, tp.PRNumber, "stale-head", approval,
	); err != nil || row != nil || changed {
		t.Fatalf("stale-head write = (%v, %v, %v), want a no-op", row, changed, err)
	}

	row, changed, err := store.UpdateTaskPRWorkflowAttention(
		ctx, tp.TaskID, tp.RepositoryID, tp.PRNumber, "head-a", approval,
	)
	if err != nil || !changed || row == nil || row.WorkflowAttention == nil ||
		row.WorkflowAttention.State != WorkflowAttentionApprovalRequired {
		t.Fatalf("first head-guarded write = (%#v, %v, %v)", row, changed, err)
	}

	if row, changed, err := store.UpdateTaskPRWorkflowAttention(
		ctx, tp.TaskID, tp.RepositoryID, tp.PRNumber, "head-a", approval,
	); err != nil || row != nil || changed {
		t.Fatalf("semantically equal write = (%v, %v, %v), want a no-op", row, changed, err)
	}

	cleared := workflowAttentionNone("head-a")
	row, changed, err = store.UpdateTaskPRWorkflowAttention(
		ctx, tp.TaskID, tp.RepositoryID, tp.PRNumber, "head-a", cleared,
	)
	if err != nil || !changed || row == nil || row.WorkflowAttention == nil ||
		row.WorkflowAttention.State != WorkflowAttentionNone {
		t.Fatalf("clearing write = (%#v, %v, %v), want none", row, changed, err)
	}

	stored, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if stored.WorkflowAttention == nil || stored.WorkflowAttention.State != WorkflowAttentionNone {
		t.Fatalf("stored attention = %#v, want none", stored.WorkflowAttention)
	}
}

func TestTaskPRWorkflowAttentionColumnIsMigratedAndProjected(t *testing.T) {
	store := newTestStore(t)
	columns, err := store.tableColumns("github_task_prs")
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}
	if _, ok := columns["workflow_attention"]; !ok {
		t.Fatal("github_task_prs is missing workflow_attention")
	}
	if err := store.initSchema(false); err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	if !strings.Contains(taskPRColumns, "workflow_attention") || !strings.Contains(taskPRColumnsQualified, "gtp.workflow_attention") {
		t.Fatalf("workflow_attention is missing from task PR projections")
	}
}

func TestTaskPRWorkflowAttentionRejectsEmptyExpectedHead(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	tp := &TaskPR{
		TaskID: "workflow-empty-head-task", WorkspaceID: "workflow-empty-head-workspace",
		Owner: "acme", Repo: "widget", PRNumber: 8, State: "open", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("CreateTaskPR: %v", err)
	}
	attention := &WorkflowAttention{
		State: WorkflowAttentionUnknown, HeadSHA: "", ObservedAt: time.Now().UTC(),
		Runs: []WorkflowAttentionRun{},
	}
	row, changed, err := store.UpdateTaskPRWorkflowAttention(
		ctx, tp.TaskID, tp.RepositoryID, tp.PRNumber, "", attention,
	)
	if err != nil || row != nil || changed {
		t.Fatalf("empty-head write = (%v, %v, %v), want no-op", row, changed, err)
	}
	stored, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("GetTaskPRByID: %v", err)
	}
	if stored.WorkflowAttention != nil {
		t.Fatalf("empty-head row attention = %#v, want nil", stored.WorkflowAttention)
	}
}
