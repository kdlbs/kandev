package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
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

func setBranchUpdateWorkflowStep(svc *Service) {
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-1": {ID: "step-1", WorkflowID: "wf-1", Name: "Step"},
	}})
}

// TestUpdateRepositoryBaseBranch_ResetsSessionBases confirms the picker
// path also clears session.base_commit_sha and rewrites session.base_branch
// for affected (task, repo) pairs. Without this the commits panel and
// cumulative diff stay filtered against the captured-at-launch SHA and the
// user sees "commits disappeared" after switching the base.
func TestUpdateRepositoryBaseBranch_ResetsSessionBases(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setBranchUpdateWorkflowStep(svc)
	svc.SetAgentBaseBranchPusher(&fakeBaseBranchPusher{})

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Sessions",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/x"}},
	})
	task := taskResult.Task
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
	setBranchUpdateWorkflowStep(svc)
	pusher := &fakeBaseBranchPusher{}
	svc.SetAgentBaseBranchPusher(pusher)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Promotion chain",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
		},
	})
	task := taskResult.Task
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
	setBranchUpdateWorkflowStep(svc)
	pusher := &fakeBaseBranchPusher{}
	svc.SetAgentBaseBranchPusher(pusher)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "No-op",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
		},
	})
	task := taskResult.Task
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
	setBranchUpdateWorkflowStep(svc)
	pusher := &fakeBaseBranchPusher{}
	svc.SetAgentBaseBranchPusher(pusher)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	taskResult, _ := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "T",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main"}},
	})
	task := taskResult.Task
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
	setBranchUpdateWorkflowStep(svc)
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	taskResult, _ := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Other task",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main"},
		},
	})
	task := taskResult.Task
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

// Regression: collectTaskBaseBranches skipped any task_repositories row whose
// Repository could not be resolved. For a multi-repo task that yields a
// non-empty but INCOMPLETE map — and since agentctl's SetBaseBranches replaces
// the stored map wholesale, pushing it silently drops the base branch of every
// repository that was skipped. The len>0 guard does not catch this, because the
// map is not empty; it is just missing entries. A partial hydration must be
// treated as a failure, not pushed.
func TestUpdateRepositoryBaseBranch_PartialHydrationIsNotPushed(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setBranchUpdateWorkflowStep(svc)
	pusher := &fakeBaseBranchPusher{}
	svc.SetAgentBaseBranchPusher(pusher)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-2", WorkspaceID: "ws-1", Name: "backend", DefaultBranch: "main"})

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Multi repo",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
			{RepositoryID: "repo-2", BaseBranch: "main", CheckoutBranch: "feature/b"},
		},
	})
	task := taskResult.Task
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("ListTaskRepositories: %v, rows=%d", err, len(rows))
	}

	// Delete one repository so its task_repositories row can no longer resolve
	// a Name — the shape a failed GetRepository produces.
	if err := repo.DeleteRepository(ctx, "repo-2"); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}

	var target string
	for _, r := range rows {
		if r.RepositoryID == "repo-1" {
			target = r.ID
		}
	}

	if _, err := svc.UpdateRepositoryBaseBranch(ctx, UpdateRepositoryBaseBranchRequest{
		TaskID:           task.ID,
		TaskRepositoryID: target,
		BaseBranch:       "staging",
	}); err != nil {
		t.Fatalf("UpdateRepositoryBaseBranch: %v", err)
	}

	// The push must be skipped entirely. Pushing {frontend: staging} alone
	// would replace the map and wipe backend's recorded base branch.
	if calls := pusher.snapshot(); len(calls) != 0 {
		t.Fatalf("expected no push for an incomplete hydration, got %d calls: %+v", len(calls), calls)
	}
}

// Regression: collectTaskBaseBranches keyed the map by the bare Repository.Name.
// A task may attach the same repository on several branches, and those siblings
// live in `{RepoName}-{BranchSlug}` worktree directories — which is the name
// their WorkspaceTracker reports. Keying by name alone collapsed every sibling
// onto one entry, so the non-flat trackers found no key and fell back to
// origin/main. Worse, because SetBaseBranches *replaces* the stored map, this
// push overwrote the correctly-keyed map the launch path had already seeded.
//
// The keys must match lifecycle.baseBranchMetadataKey: the lowest-positioned
// branch of a repeated repository keeps the flat legacy path, the rest are
// suffixed with their branch-identity path slug.
func TestUpdateRepositoryBaseBranch_MultiBranchKeysPerWorktree(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setBranchUpdateWorkflowStep(svc)
	pusher := &fakeBaseBranchPusher{}
	svc.SetAgentBaseBranchPusher(pusher)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "frontend", DefaultBranch: "main"})

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Same repo, two branches",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/b"},
		},
	})
	task := taskResult.Task
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("ListTaskRepositories: %v, rows=%d", err, len(rows))
	}
	var sibling string
	for _, r := range rows {
		if r.CheckoutBranch == "feature/b" {
			sibling = r.ID
		}
	}
	if sibling == "" {
		t.Fatal("no task_repositories row for feature/b")
	}

	if _, err := svc.UpdateRepositoryBaseBranch(ctx, UpdateRepositoryBaseBranchRequest{
		TaskID:           task.ID,
		TaskRepositoryID: sibling,
		BaseBranch:       "staging",
	}); err != nil {
		t.Fatalf("UpdateRepositoryBaseBranch: %v", err)
	}

	calls := pusher.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 pusher call, got %d", len(calls))
	}
	got := calls[0].branches

	// feature/a is position 0, so it keeps the flat directory.
	if got["frontend"] != "main" {
		t.Errorf("branches[frontend] = %q, want main (flat sibling keeps its base)", got["frontend"])
	}
	// feature/b is the suffixed sibling and is the row we just updated.
	if got["frontend-feature-b"] != "staging" {
		t.Errorf("branches[frontend-feature-b] = %q, want staging", got["frontend-feature-b"])
	}
	// Multi-row tasks must not publish the empty-key fallback: lookupBaseBranch
	// falls back to it for any unmatched tracker, which would hand one sibling's
	// base branch to the other.
	if _, ok := got[""]; ok {
		t.Errorf("branches[\"\"] present for a multi-row task: %+v", got)
	}
}

// The map keys are read by WorkspaceTracker, whose repositoryName is the
// worktree *directory* basename — a sanitised repo name, not the raw
// Repository.Name. A name with characters the directory sanitiser rewrites
// (e.g. a space) produced a key no tracker could ever match.
func TestCollectTaskBaseBranches_SanitizesRepositoryName(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setBranchUpdateWorkflowStep(svc)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "my repo", DefaultBranch: "main"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-2", WorkspaceID: "ws-1", Name: "backend", DefaultBranch: "main"})

	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Unsanitised name",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "feature/a"},
			{RepositoryID: "repo-2", BaseBranch: "develop", CheckoutBranch: "feature/b"},
		},
	})
	task := taskResult.Task
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	branches, err := svc.collectTaskBaseBranches(ctx, task.ID)
	if err != nil {
		t.Fatalf("collectTaskBaseBranches: %v", err)
	}
	if branches["my-repo"] != "main" {
		t.Errorf("branches[my-repo] = %q, want main; got map %+v", branches["my-repo"], branches)
	}
	if _, ok := branches["my repo"]; ok {
		t.Errorf("raw repository name leaked as a key: %+v", branches)
	}
	if branches["backend"] != "develop" {
		t.Errorf("branches[backend] = %q, want develop", branches["backend"])
	}
}
