package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLUGINS-WORKFLOW-HISTORY-001.1 AC-PLUGINS-WORKFLOW-HISTORY-001.3
func TestTransitionReadServicePagesRecordedMoves(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "history-ws", Name: "History"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "history-task", WorkspaceID: "history-ws", WorkflowID: "history-wf", WorkflowStepID: "draft", Title: "History", Priority: "medium"}); err != nil {
		t.Fatal(err)
	}
	items, next, err := svc.ListTaskStepTransitions(ctx, "history-task", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || next != "" {
		t.Fatalf("items = %+v, next = %q, want one genesis row", items, next)
	}
}

// @covers AC-PLUGINS-WORKFLOW-HISTORY-001.1 AC-PLUGINS-WORKFLOW-HISTORY-001.3
func TestTransitionReadServiceRejectsForeignAndMalformedCursors(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "cursor-ws", Name: "Cursor"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"cursor-a", "cursor-b"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "cursor-ws", WorkflowID: "wf", WorkflowStepID: "draft", Title: id, Priority: "medium"}); err != nil {
			t.Fatal(err)
		}
	}
	foreign := encodeTransitionCursor(transitionCursor{Scope: "task:cursor-a", ID: 1})
	for _, cursor := range []string{foreign, "not-base64!"} {
		items, next, err := svc.ListTaskStepTransitions(ctx, "cursor-b", 10, cursor)
		if err != ErrInvalidTransitionCursor || len(items) != 0 || next != "" {
			t.Fatalf("cursor %q: items=%+v next=%q err=%v", cursor, items, next, err)
		}
	}
}

// @covers AC-PLUGINS-WORKFLOW-HISTORY-002.1
func TestWorkflowTransitionGroupServiceReadsOwnedWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "route-ws", Name: "Routes"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "route-wf", WorkspaceID: "route-ws", Name: "Routes"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "route-task", WorkspaceID: "route-ws", WorkflowID: "route-wf", WorkflowStepID: "draft", Title: "Task", Priority: "medium"}); err != nil {
		t.Fatal(err)
	}
	groups, next, err := svc.ListWorkflowTransitionGroups(ctx, "route-wf", 10, "")
	if err != nil || len(groups) != 1 || groups[0].Kind != "entry" || next != "" {
		t.Fatalf("groups=%+v next=%q err=%v", groups, next, err)
	}
}
