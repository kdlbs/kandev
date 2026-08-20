package service

import (
	"context"
	"errors"
	"testing"
)

func TestListRelatedForRequest_AllowsCoordinatorCompactWorkspaceTreeRead(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("caller-parent", "", "workspace-a")
	tasks.addTask("caller", "caller-parent", "workspace-a")
	tasks.addTask("unrelated-parent", "", "workspace-a")
	tasks.addTask("unrelated", "unrelated-parent", "workspace-a")
	tasks.setDescription("unrelated", "private task description")

	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())
	related, err := svc.ListRelatedForRequest(context.Background(), RelatedReadRequest{
		CallerTaskID: "caller",
		TargetTaskID: "unrelated",
		Scope:        RelatedReadScopeWorkspaceTaskTree,
	})
	if err != nil {
		t.Fatalf("coordinator compact workspace-tree read: %v", err)
	}
	if related.Task.ID != "unrelated" {
		t.Fatalf("task ID = %q, want unrelated", related.Task.ID)
	}
	if related.Task.Description != "" {
		t.Fatalf("compact description = %q, want omitted", related.Task.Description)
	}
}

func TestListRelatedForRequest_ProtectsUnrelatedVerboseAndUnavailableTargets(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "workspace-a")
	tasks.addTask("caller", "parent", "workspace-a")
	tasks.addTask("unrelated", "", "workspace-a")
	tasks.addTask("foreign", "", "workspace-b")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	tests := []struct {
		name   string
		req    RelatedReadRequest
		reason RelatedReadDenialReason
	}{
		{
			name:   "ordinary unrelated task needs scope",
			req:    RelatedReadRequest{CallerTaskID: "caller", TargetTaskID: "unrelated"},
			reason: RelatedReadDenialScopeRequired,
		},
		{
			name: "coordinator verbose remains document scoped",
			req: RelatedReadRequest{
				CallerTaskID: "caller", TargetTaskID: "unrelated",
				Scope: RelatedReadScopeWorkspaceTaskTree, Verbose: true,
			},
			reason: RelatedReadDenialVerboseDocumentScopeNeeded,
		},
		{
			name: "foreign target is unavailable",
			req: RelatedReadRequest{
				CallerTaskID: "caller", TargetTaskID: "foreign", Scope: RelatedReadScopeWorkspaceTaskTree,
			},
			reason: RelatedReadDenialTargetUnavailable,
		},
		{
			name: "unknown target is unavailable",
			req: RelatedReadRequest{
				CallerTaskID: "caller", TargetTaskID: "missing", Scope: RelatedReadScopeWorkspaceTaskTree,
			},
			reason: RelatedReadDenialTargetUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ListRelatedForRequest(context.Background(), tt.req)
			if !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("error = %v, want access denial", err)
			}
			if got, ok := RelatedReadDenialReasonFor(err); !ok || got != tt.reason {
				t.Fatalf("reason = %q, found=%v, want %q", got, ok, tt.reason)
			}
		})
	}
}

func TestListRelatedForRequest_FiltersSensitiveFieldsPerRelatedNode(t *testing.T) {
	svc, repo := newDocumentHandoffService(t, nil)
	ctx := context.Background()
	seedHandoffDocument(t, repo, "parent", "spec", "parent document")
	seedHandoffDocument(t, repo, "stranger", "secret", "stranger document")
	for _, update := range []struct{ id, description string }{
		{"parent", "parent description"},
		{"stranger", "stranger description"},
	} {
		task, err := repo.GetTask(ctx, update.id)
		if err != nil {
			t.Fatalf("get %s: %v", update.id, err)
		}
		task.Description = update.description
		if err := repo.UpdateTask(ctx, task); err != nil {
			t.Fatalf("update %s: %v", update.id, err)
		}
	}

	compact, err := svc.ListRelatedForRequest(ctx, RelatedReadRequest{
		CallerTaskID: "child-a", TargetTaskID: "stranger", Scope: RelatedReadScopeWorkspaceTaskTree,
	})
	if err != nil {
		t.Fatalf("compact coordinator read: %v", err)
	}
	if compact.Task.Description != "" || len(compact.Task.DocumentKeys) != 0 {
		t.Fatalf("unrelated compact sensitive fields = %+v", compact.Task)
	}

	verbose, err := svc.ListRelatedForRequest(ctx, RelatedReadRequest{
		CallerTaskID: "child-a", TargetTaskID: "parent", Verbose: true,
	})
	if err != nil {
		t.Fatalf("relation verbose read: %v", err)
	}
	if verbose.Task.Description != "parent description" {
		t.Fatalf("description = %q", verbose.Task.Description)
	}
	if len(verbose.Task.DocumentKeys) != 1 || verbose.Task.DocumentKeys[0] != "spec" {
		t.Fatalf("document keys = %v", verbose.Task.DocumentKeys)
	}
}
