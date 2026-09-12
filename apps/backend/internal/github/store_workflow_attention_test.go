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
