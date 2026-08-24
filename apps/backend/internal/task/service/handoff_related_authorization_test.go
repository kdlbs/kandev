package service

import (
	"context"
	"errors"
	"testing"

	orchmodels "github.com/kandev/kandev/internal/office/models"
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

func TestListRelatedForRequest_FiltersCrossWorkspaceRelationsAndCachesAuthorization(t *testing.T) {
	ctx := context.Background()
	tasks := newFakeTaskRepo()
	tasks.addTask("caller", "", "workspace-a")
	tasks.addTask("foreign-parent", "", "workspace-b")
	tasks.addTask("target", "foreign-parent", "workspace-a")
	tasks.addTask("same-child", "target", "workspace-a")
	tasks.addTask("foreign-child", "target", "workspace-b")
	tasks.addTask("same-sibling", "foreign-parent", "workspace-a")
	tasks.addTask("foreign-sibling", "foreign-parent", "workspace-b")
	tasks.addTask("same-blocker", "", "workspace-a")
	tasks.addTask("foreign-blocker", "", "workspace-b")
	tasks.addTask("same-blocked", "", "workspace-a")
	tasks.addTask("foreign-blocked", "", "workspace-b")

	blockers := &fakeBlockerRepo{}
	for _, blocker := range []string{"same-blocker", "foreign-blocker"} {
		if err := blockers.CreateTaskBlocker(ctx, &orchmodels.TaskBlocker{TaskID: "target", BlockerTaskID: blocker}); err != nil {
			t.Fatalf("create target blocker %q: %v", blocker, err)
		}
	}
	for _, blocked := range []string{"same-blocked", "foreign-blocked"} {
		if err := blockers.CreateTaskBlocker(ctx, &orchmodels.TaskBlocker{TaskID: blocked, BlockerTaskID: "target"}); err != nil {
			t.Fatalf("create blocked task %q: %v", blocked, err)
		}
	}

	svc := newPhase4Service(t, tasks, blockers, newCascadeWSGroupRepo())
	related, err := svc.ListRelatedForRequest(ctx, RelatedReadRequest{
		CallerTaskID: "caller", TargetTaskID: "target", Scope: RelatedReadScopeWorkspaceTaskTree,
	})
	if err != nil {
		t.Fatalf("list compact related tasks: %v", err)
	}

	if related.Parent != nil || related.Task.ParentID != "" {
		t.Fatalf("foreign parent leaked: parent=%+v task.parent_id=%q", related.Parent, related.Task.ParentID)
	}
	assertRelatedTaskIDs(t, related.Children, "same-child")
	assertRelatedTaskIDs(t, related.Siblings, "same-sibling")
	assertRelatedTaskIDs(t, related.Blockers, "same-blocker")
	assertRelatedTaskIDs(t, related.BlockedBy, "same-blocked")
	for _, task := range relatedTaskPointers(related) {
		if task != nil && task.WorkspaceID != "workspace-a" {
			t.Fatalf("cross-workspace task projected: %+v", task)
		}
	}

	for _, id := range []string{
		"caller", "foreign-parent", "target", "same-child", "foreign-child", "same-sibling", "foreign-sibling",
		"same-blocker", "foreign-blocker", "same-blocked", "foreign-blocked",
	} {
		if calls := tasks.getTaskCallCount(id); calls > 1 {
			t.Errorf("GetTask(%q) called %d times, want at most once", id, calls)
		}
	}
}

func assertRelatedTaskIDs(t *testing.T, tasks []*RelatedTask, want ...string) {
	t.Helper()
	if len(tasks) != len(want) {
		t.Fatalf("related task count = %d, want %d: %+v", len(tasks), len(want), tasks)
	}
	for i, task := range tasks {
		if task.ID != want[i] {
			t.Fatalf("related task[%d] = %q, want %q", i, task.ID, want[i])
		}
	}
}
