package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakePRResolver is a stand-in for the github service in PR-number search tests.
type fakePRResolver struct {
	byPR   map[int][]string
	err    error
	called bool
}

func (f *fakePRResolver) FindTaskIDsByPRNumber(_ context.Context, _ string, prNumber int) ([]string, error) {
	f.called = true
	if f.err != nil {
		return nil, f.err
	}
	return f.byPR[prNumber], nil
}

func seedPRSearchTasks(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	tasks := []*models.Task{
		{ID: "task-pr", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Unrelated work", State: v1.TaskStateCreated, Priority: "medium"},
		{ID: "task-title", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Bug in PR 1243 handler", State: v1.TaskStateCreated, Priority: "medium"},
	}
	for _, tk := range tasks {
		if err := repo.CreateTask(ctx, tk); err != nil {
			t.Fatalf("create task %s: %v", tk.ID, err)
		}
	}
}

func taskIDSet(tasks []*models.Task) map[string]bool {
	set := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		set[t.ID] = true
	}
	return set
}

func TestListTasksByWorkspace_PRNumberSurfacesTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedPRSearchTasks(t, repo)
	svc.SetPRTaskResolver(&fakePRResolver{byPR: map[int][]string{1243: {"task-pr"}}})
	ctx := context.Background()

	// "#1243" — task-pr has no "1243" in its title, only the PR association.
	tasks, total, err := svc.ListTasksByWorkspace(ctx, "ws-1", "", "", "#1243", 1, 5, true, false, false, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	got := taskIDSet(tasks)
	if !got["task-pr"] {
		t.Errorf("expected task-pr surfaced by PR number, got %v", got)
	}
	if total < 1 {
		t.Errorf("expected total >= 1, got %d", total)
	}
}

func TestListTasksByWorkspace_PRNumberDedupes(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedPRSearchTasks(t, repo)
	// Resolver returns task-title, which ALSO matches the LIKE search for "1243".
	svc.SetPRTaskResolver(&fakePRResolver{byPR: map[int][]string{1243: {"task-title"}}})
	ctx := context.Background()

	tasks, _, err := svc.ListTasksByWorkspace(ctx, "ws-1", "", "", "1243", 1, 5, true, false, false, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	count := 0
	for _, tk := range tasks {
		if tk.ID == "task-title" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected task-title exactly once, got %d", count)
	}
}

func TestListTasksByWorkspace_NonNumericQuerySkipsResolver(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedPRSearchTasks(t, repo)
	resolver := &fakePRResolver{byPR: map[int][]string{1243: {"task-pr"}}}
	svc.SetPRTaskResolver(resolver)
	ctx := context.Background()

	tasks, _, err := svc.ListTasksByWorkspace(ctx, "ws-1", "", "", "unrelated", 1, 5, true, false, false, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if resolver.called {
		t.Error("resolver should not be consulted for a non-numeric query")
	}
	// task-pr matches the title LIKE search, task-title does not.
	if got := taskIDSet(tasks); !got["task-pr"] {
		t.Errorf("expected title match task-pr, got %v", got)
	}
}

func TestListTasksByWorkspace_NilResolverNoPanic(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedPRSearchTasks(t, repo)
	ctx := context.Background()

	// No resolver wired — PR-number search must be a safe no-op.
	tasks, _, err := svc.ListTasksByWorkspace(ctx, "ws-1", "", "", "#1243", 1, 5, true, false, false, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// task-pr is only reachable via the resolver, so it must be absent.
	if got := taskIDSet(tasks); got["task-pr"] {
		t.Errorf("did not expect task-pr without a resolver, got %v", got)
	}
}

func TestListTasksByWorkspace_PRMatchRespectsArchivedFilter(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedPRSearchTasks(t, repo)
	if err := repo.ArchiveTask(context.Background(), "task-pr"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	svc.SetPRTaskResolver(&fakePRResolver{byPR: map[int][]string{1243: {"task-pr"}}})
	ctx := context.Background()

	// includeArchived=false → archived PR task excluded.
	excluded, _, err := svc.ListTasksByWorkspace(ctx, "ws-1", "", "", "#1243", 1, 5, false, false, false, false)
	if err != nil {
		t.Fatalf("search excluded: %v", err)
	}
	if taskIDSet(excluded)["task-pr"] {
		t.Error("archived task-pr should be excluded when includeArchived=false")
	}

	// includeArchived=true → archived PR task included.
	included, _, err := svc.ListTasksByWorkspace(ctx, "ws-1", "", "", "#1243", 1, 5, true, false, false, false)
	if err != nil {
		t.Fatalf("search included: %v", err)
	}
	if !taskIDSet(included)["task-pr"] {
		t.Error("archived task-pr should be included when includeArchived=true")
	}
}

var _ PRTaskResolver = (*fakePRResolver)(nil)
