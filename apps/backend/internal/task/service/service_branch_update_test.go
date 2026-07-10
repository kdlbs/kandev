package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func eventBusHasType(bus *MockEventBus, eventType string) bool {
	for _, e := range bus.GetPublishedEvents() {
		if e.Type == eventType {
			return true
		}
	}
	return false
}

// fakeBaseBranchPusher records calls so tests can assert the service
// invoked the live agentctl push with the right per-repo map.
type fakeBaseBranchPusher struct {
	mu    sync.Mutex
	calls []fakeBaseBranchPusherCall
}

type fakeBaseBranchPusherCall struct {
	taskID   string
	branches map[string]string
}

func (f *fakeBaseBranchPusher) PushBaseBranchesForTask(_ context.Context, taskID string, branches map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeBaseBranchPusherCall{taskID: taskID, branches: branches})
}

func (f *fakeBaseBranchPusher) snapshot() []fakeBaseBranchPusherCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeBaseBranchPusherCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// TestUpdateRepositoryBaseBranch_ResetsSessionBases confirms the picker
// path also clears session.base_commit_sha and rewrites session.base_branch
// for affected (task, repo) pairs. Without this the commits panel and
// cumulative diff stay filtered against the captured-at-launch SHA and the
// user sees "commits disappeared" after switching the base.
func TestUpdateRepositoryBaseBranch_ResetsSessionBases(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	svc.SetAgentBaseBranchPusher(&fakeBaseBranchPusher{})

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Sessions",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/x"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	rows, _ := repo.ListTaskRepositories(ctx, task.ID)
	taskRepoID := rows[0].ID

	sess := &models.TaskSession{
		ID: "sess-1", TaskID: task.ID, RepositoryID: "repo-1",
		BaseBranch: "main", BaseCommitSHA: "captured-old-sha",
	}
	if err := repo.CreateTaskSession(ctx, sess); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	if _, err := svc.UpdateRepositoryBaseBranch(ctx, UpdateRepositoryBaseBranchRequest{
		TaskID: task.ID, TaskRepositoryID: taskRepoID, BaseBranch: "staging",
	}); err != nil {
		t.Fatalf("UpdateRepositoryBaseBranch: %v", err)
	}

	reread, err := repo.GetTaskSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if reread.BaseBranch != "staging" {
		t.Errorf("session BaseBranch = %q, want staging", reread.BaseBranch)
	}
	if reread.BaseCommitSHA != "" {
		t.Errorf("session BaseCommitSHA = %q, want empty (cleared)", reread.BaseCommitSHA)
	}
}

// TestUpdateRepositoryBaseBranch_PersistsAndPushes covers the happy path:
// DB write, task.updated event, and a live agentctl push containing the
// expected per-repo map keyed by Repository.Name.
func TestUpdateRepositoryBaseBranch_PersistsAndPushes(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	pusher := &fakeBaseBranchPusher{}
	svc.SetAgentBaseBranchPusher(pusher)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Promotion chain",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListTaskRepositories: %v, rows=%d", err, len(rows))
	}
	taskRepoID := rows[0].ID

	bus.ClearEvents()

	updated, err := svc.UpdateRepositoryBaseBranch(ctx, UpdateRepositoryBaseBranchRequest{
		TaskID:           task.ID,
		TaskRepositoryID: taskRepoID,
		BaseBranch:       "staging",
	})
	if err != nil {
		t.Fatalf("UpdateRepositoryBaseBranch: %v", err)
	}
	if updated.BaseBranch != "staging" {
		t.Errorf("returned BaseBranch = %q, want staging", updated.BaseBranch)
	}

	reread, err := repo.GetTaskRepository(ctx, taskRepoID)
	if err != nil {
		t.Fatalf("GetTaskRepository after update: %v", err)
	}
	if reread.BaseBranch != "staging" {
		t.Errorf("DB BaseBranch = %q, want staging", reread.BaseBranch)
	}

	if !eventBusHasType(bus, events.TaskUpdated) {
		t.Error("expected task.updated event")
	}

	calls := pusher.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 pusher call, got %d", len(calls))
	}
	if calls[0].taskID != task.ID {
		t.Errorf("pusher call taskID = %q, want %q", calls[0].taskID, task.ID)
	}
	// Single-repo legacy fallback: the map must carry the empty-key entry
	// AND the named entry so both root and per-repo trackers find a value.
	if got := calls[0].branches["frontend"]; got != "staging" {
		t.Errorf("pusher branches[frontend] = %q, want staging", got)
	}
	if got := calls[0].branches[""]; got != "staging" {
		t.Errorf("pusher branches[\"\"] = %q, want staging (single-repo fallback)", got)
	}
}

// TestUpdateRepositoryBaseBranch_NoChangeSkipsWork is a sanity check: when
// the new value equals the stored value, the service short-circuits before
// the DB write so callers don't trigger spurious task.updated events or
// agentctl refreshes.
func TestUpdateRepositoryBaseBranch_NoChangeSkipsWork(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	pusher := &fakeBaseBranchPusher{}
	svc.SetAgentBaseBranchPusher(pusher)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "No-op",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	rows, _ := repo.ListTaskRepositories(ctx, task.ID)
	bus.ClearEvents()

	_, err = svc.UpdateRepositoryBaseBranch(ctx, UpdateRepositoryBaseBranchRequest{
		TaskID:           task.ID,
		TaskRepositoryID: rows[0].ID,
		BaseBranch:       "main", // identical to stored value
	})
	if err != nil {
		t.Fatalf("UpdateRepositoryBaseBranch: %v", err)
	}
	if eventBusHasType(bus, events.TaskUpdated) {
		t.Error("identical update should not emit task.updated")
	}
	if len(pusher.snapshot()) != 0 {
		t.Error("identical update should not invoke pusher")
	}
}

// TestUpdateRepositoryBaseBranch_RejectsUnsafeRefs ensures unsafe ref
// names (leading "-", shell metacharacters, …) are rejected at the
// service boundary before reaching the DB or the live agentctl push. The
// picker payload is user-controlled and ultimately interpolated into a
// `git` argument list inside agentctl — letting through "-upload-pack="
// would risk command-flag injection.
func TestUpdateRepositoryBaseBranch_RejectsUnsafeRefs(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	pusher := &fakeBaseBranchPusher{}
	svc.SetAgentBaseBranchPusher(pusher)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, _ := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "T",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main"}},
	})
	rows, _ := repo.ListTaskRepositories(ctx, task.ID)

	for _, bad := range []string{"-upload-pack=evil", "main;rm -rf", "branch with space", "/leading-slash"} {
		_, err := svc.UpdateRepositoryBaseBranch(ctx, UpdateRepositoryBaseBranchRequest{
			TaskID: task.ID, TaskRepositoryID: rows[0].ID, BaseBranch: bad,
		})
		if err == nil {
			t.Errorf("UpdateRepositoryBaseBranch(%q): expected error, got nil", bad)
		}
	}
	if len(pusher.snapshot()) != 0 {
		t.Errorf("unsafe inputs should not trigger pusher; got %d calls", len(pusher.snapshot()))
	}
}

// TestUpdateRepositoryBaseBranch_NotFound covers the two missing-row cases:
// unknown task_repository_id, and a row that exists but belongs to a
// different task than the caller claimed. Both fold into the typed
// ErrTaskRepositoryNotFound so handlers can return 404 cleanly.
func TestUpdateRepositoryBaseBranch_NotFound(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	task, _ := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Other task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	rows, _ := repo.ListTaskRepositories(ctx, task.ID)

	t.Run("unknown row id", func(t *testing.T) {
		_, err := svc.UpdateRepositoryBaseBranch(ctx, UpdateRepositoryBaseBranchRequest{
			TaskID:           task.ID,
			TaskRepositoryID: "does-not-exist",
			BaseBranch:       "staging",
		})
		if !errors.Is(err, ErrTaskRepositoryNotFound) {
			t.Errorf("got %v, want ErrTaskRepositoryNotFound", err)
		}
	})

	t.Run("wrong task id", func(t *testing.T) {
		_, err := svc.UpdateRepositoryBaseBranch(ctx, UpdateRepositoryBaseBranchRequest{
			TaskID:           "some-other-task",
			TaskRepositoryID: rows[0].ID,
			BaseBranch:       "staging",
		})
		if !errors.Is(err, ErrTaskRepositoryNotFound) {
			t.Errorf("got %v, want ErrTaskRepositoryNotFound", err)
		}
	})
}
