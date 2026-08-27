package service

import (
	"context"
	"errors"
	"testing"

	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Per-user scoping for the task lifecycle mutations that used to start at
// s.tasks.GetTask with no authorization step: state changes, workflow moves and
// base-branch edits. Their WS actions name the task `id` rather than `task_id`,
// so the gateway backstop parsed no refs and let them through — these guards are
// what actually refuses the call.
//
// Every case runs the identical request as the OWNER afterwards and asserts it
// succeeds. Without that witness a denial test still passes when the fixture is
// broken (wrong ID, unseeded row) rather than when the guard works.

// seedAuthzWorkflowFixture builds one workspace owned by user-b holding a
// workflow with two steps and a task parked on the first, plus a repository the
// task checks out. Returns the task-repository row ID for the base-branch case.
func seedAuthzWorkflowFixture(t *testing.T, svc *Service, repo *sqliterepo.Repository) string {
	t.Helper()
	ctx := context.Background()
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-b-1": {ID: "step-b-1", WorkflowID: "wf-b", Name: "First", Position: 0},
		"step-b-2": {ID: "step-b-2", WorkflowID: "wf-b", Name: "Second", Position: 1},
	}})
	must(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-b", Name: "B's", OwnerID: "user-b"}))
	must(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-b", WorkspaceID: "ws-b", Name: "B flow"}))
	must(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-b", WorkspaceID: "ws-b", Name: "backend", DefaultBranch: "main",
	}))
	must(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-b", WorkspaceID: "ws-b", WorkflowID: "wf-b", WorkflowStepID: "step-b-1",
		Title: "Victim", State: v1.TaskStateTODO, Priority: priorityMedium,
	}))
	must(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-b", TaskID: "task-b", RepositoryID: "repo-b",
		BaseBranch: "main", CheckoutBranch: "feature/x",
	}))
	return "task-repo-b"
}

func TestUpdateTaskStateDeniesForeignTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedAuthzWorkflowFixture(t, svc, repo)

	task, err := svc.UpdateTaskState(ctxAs("user-a"), "task-b", v1.TaskStateInProgress)
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign state change: err = %v, want ErrTaskNotFound", err)
	}
	if task != nil {
		t.Fatalf("denial leaked the task: %+v", task)
	}

	stored, err := repo.GetTask(context.Background(), "task-b")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.State != v1.TaskStateTODO {
		t.Fatalf("a denied state change reached the repository: state = %s", stored.State)
	}

	// The owner must still get through, otherwise the denial above proves nothing.
	owned, err := svc.UpdateTaskState(ctxAs("user-b"), "task-b", v1.TaskStateInProgress)
	if err != nil {
		t.Fatalf("owner state change: %v", err)
	}
	if owned.State != v1.TaskStateInProgress {
		t.Fatalf("owner state change did not apply: state = %s", owned.State)
	}
}

func TestMoveTaskDeniesForeignTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedAuthzWorkflowFixture(t, svc, repo)

	result, err := svc.MoveTask(ctxAs("user-a"), "task-b", "wf-b", "step-b-2", 3)
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign move: err = %v, want ErrTaskNotFound", err)
	}
	if result != nil {
		t.Fatalf("denial leaked the move result: %+v", result)
	}

	stored, err := repo.GetTask(context.Background(), "task-b")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.WorkflowStepID != "step-b-1" || stored.Position != 0 {
		t.Fatalf("a denied move reached the repository: step = %s position = %d",
			stored.WorkflowStepID, stored.Position)
	}

	owned, err := svc.MoveTask(ctxAs("user-b"), "task-b", "wf-b", "step-b-2", 3)
	if err != nil {
		t.Fatalf("owner move: %v", err)
	}
	if owned.Task.WorkflowStepID != "step-b-2" {
		t.Fatalf("owner move did not apply: step = %s", owned.Task.WorkflowStepID)
	}
}

func TestUpdateRepositoryBaseBranchDeniesForeignTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	taskRepoID := seedAuthzWorkflowFixture(t, svc, repo)
	svc.SetAgentBaseBranchPusher(&fakeBaseBranchPusher{})

	req := UpdateRepositoryBaseBranchRequest{
		TaskID: "task-b", TaskRepositoryID: taskRepoID, BaseBranch: "staging",
	}

	updated, err := svc.UpdateRepositoryBaseBranch(ctxAs("user-a"), req)
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign base-branch update: err = %v, want ErrTaskNotFound", err)
	}
	if updated != nil {
		t.Fatalf("denial leaked the task repository: %+v", updated)
	}

	rows, err := repo.ListTaskRepositories(context.Background(), "task-b")
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if rows[0].BaseBranch != "main" {
		t.Fatalf("a denied base-branch update reached the repository: %s", rows[0].BaseBranch)
	}

	owned, err := svc.UpdateRepositoryBaseBranch(ctxAs("user-b"), req)
	if err != nil {
		t.Fatalf("owner base-branch update: %v", err)
	}
	if owned.BaseBranch != "staging" {
		t.Fatalf("owner base-branch update did not apply: %s", owned.BaseBranch)
	}
}

// TestScopedMoveStillPromotesFeederQueuedTask covers the follow-on work flagged
// in review: the move guards run on a ctx that internal continuations inherit,
// and feeder queue promotion is the one that can reach a guarded method. Freeing
// a slot in a WIP-limited step as the owner must still pull the task queued in
// the feeder step behind it.
//
// What this pins, precisely: promotion still happens end to end under a real
// identity. It does NOT exercise the guard, because the SQLite repository
// implements workflowQueuedTaskPromoter, so promotion takes the atomic branch and
// never calls MoveTaskWithOptions at all — the guarded fallback in
// promoteFeederQueuedTask is unreachable in production and only runs for repos
// without that method. Verified by mutation: breaking the feeder pull fails this
// test, while forcing a denying identity into the promotion path does not.
//
// The invariant that keeps the fallback safe is documented at that call site: a
// step's PullFromStepID resolves within the same workflow, a workflow belongs to
// one workspace, and a workspace has one owner.
func TestScopedMoveStillPromotesFeederQueuedTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-b-feed": {ID: "step-b-feed", WorkflowID: "wf-b", Name: "Backlog", Position: 0},
		"step-b-wip": {
			ID: "step-b-wip", WorkflowID: "wf-b", Name: "Doing", Position: 1,
			WIPLimit: 1, PullFromStepID: "step-b-feed",
		},
		"step-b-done": {ID: "step-b-done", WorkflowID: "wf-b", Name: "Done", Position: 2},
	}})
	must(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-b", Name: "B's", OwnerID: "user-b"}))
	must(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-b", WorkspaceID: "ws-b", Name: "B flow"}))

	// occupant fills the WIP-limited step; candidate waits in the feeder step.
	must(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-occupant", WorkspaceID: "ws-b", WorkflowID: "wf-b", WorkflowStepID: "step-b-wip",
		Title: "Occupant", State: v1.TaskStateTODO, Priority: priorityMedium, WIPAdmitted: true,
	}))
	must(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-queued", WorkspaceID: "ws-b", WorkflowID: "wf-b", WorkflowStepID: "step-b-feed",
		Title: "Queued", State: v1.TaskStateTODO, Priority: priorityMedium,
		QueuedForStepID: "step-b-wip",
	}))

	// The owner moves the occupant out, freeing the slot — under their identity,
	// so every guard on the promotion path is live.
	if _, err := svc.MoveTask(ctxAs("user-b"), "task-occupant", "wf-b", "step-b-done", 0); err != nil {
		t.Fatalf("owner move out of the WIP step: %v", err)
	}

	promoted, err := repo.GetTask(ctx, "task-queued")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if promoted.WorkflowStepID != "step-b-wip" || !promoted.WIPAdmitted {
		t.Fatalf("a scoped move did not pull the feeder-queued task: step=%s admitted=%v",
			promoted.WorkflowStepID, promoted.WIPAdmitted)
	}
}

// TestTaskWorkflowGuardsPrecedeRepositoryUse pins guard placement. The service
// carries only the two repositories the guard itself reads; a denial that ran
// after the first unrelated repo call would nil-panic instead of returning.
func TestTaskWorkflowGuardsPrecedeRepositoryUse(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedAuthzWorkflowFixture(t, svc, repo)

	guarded := &Service{
		tasks:      svc.tasks,
		workspaces: svc.workspaces,
		logger:     svc.logger,
	}

	cases := map[string]func() error{
		"UpdateTaskState": func() error {
			_, err := guarded.UpdateTaskState(ctxAs("user-a"), "task-b", v1.TaskStateInProgress)
			return err
		},
		"MoveTaskWithOptions": func() error {
			_, err := guarded.MoveTaskWithOptions(
				ctxAs("user-a"), "task-b", "wf-b", "step-b-2", 0, MoveTaskOptions{})
			return err
		},
		"UpdateRepositoryBaseBranch": func() error {
			_, err := guarded.UpdateRepositoryBaseBranch(ctxAs("user-a"), UpdateRepositoryBaseBranchRequest{
				TaskID: "task-b", TaskRepositoryID: "task-repo-b", BaseBranch: "staging",
			})
			return err
		},
	}

	for name, invoke := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("guard runs too late — panicked on a nil dependency: %v", r)
				}
			}()
			if err := invoke(); !errors.Is(err, repoerrors.ErrTaskNotFound) {
				t.Fatalf("err = %v, want ErrTaskNotFound", err)
			}
		})
	}
}
