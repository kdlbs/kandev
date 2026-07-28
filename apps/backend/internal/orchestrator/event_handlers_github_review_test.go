package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// countingReviewTaskCreator records how many times CreateReviewTask was called.
type countingReviewTaskCreator struct {
	calls  int
	err    error
	errs   []error
	taskID string
}

func (c *countingReviewTaskCreator) CreateReviewTask(_ context.Context, _ *ReviewTaskRequest) (*models.Task, error) {
	c.calls++
	if len(c.errs) > 0 {
		err := c.errs[0]
		c.errs = c.errs[1:]
		return nil, err
	}
	if c.err != nil {
		return nil, c.err
	}
	id := c.taskID
	if id == "" {
		id = "task-created"
	}
	return &models.Task{ID: id}, nil
}

// newReviewEvent builds a NewReviewPREvent for createReviewTask tests.
func newReviewEvent() *github.NewReviewPREvent {
	return &github.NewReviewPREvent{
		ReviewWatchID:  "w1",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		PR: &github.PR{
			Number: 42, Title: "Some PR", HTMLURL: "https://gh/acme/widget/pull/42",
			RepoOwner: "acme", RepoName: "widget",
		},
	}
}

// setupReviewTaskTest builds a Service with a seeded workflow step (needed
// because createReviewTask runs shouldAutoStartStep after task creation).
func setupReviewTaskTest(t *testing.T) (*Service, *mockStepGetter) {
	t.Helper()
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{}, // no auto-start action
	}
	return createTestService(repo, stepGetter, newMockTaskRepo()), stepGetter
}

var assertErrTaskCreate = &taskCreateErr{"simulated task creation failure"}

type taskCreateErr struct{ msg string }

func (e *taskCreateErr) Error() string { return e.msg }

// TestCreateReviewTask_SkipsWhenAlreadyReserved is the regression test for the
// duplicate review-task bug: if another handler has already reserved the
// dedup slot for this PR, createReviewTask must NOT call CreateReviewTask.
func TestCreateReviewTask_SkipsWhenAlreadyReserved(t *testing.T) {
	svc, _ := setupReviewTaskTest(t)
	ghSvc := &mockGitHubService{reserveReturn: false} // reservation lost to concurrent handler
	svc.SetGitHubService(ghSvc)
	creator := &countingReviewTaskCreator{}
	svc.SetReviewTaskCreator(creator)

	svc.createReviewTask(context.Background(), newReviewEvent())

	if ghSvc.reserveCalls != 1 {
		t.Errorf("expected 1 Reserve call, got %d", ghSvc.reserveCalls)
	}
	if creator.calls != 0 {
		t.Errorf("expected CreateReviewTask NOT to be called when reservation lost, got %d calls", creator.calls)
	}
	if ghSvc.releaseCalls != 0 {
		t.Errorf("expected no Release calls when reservation was never held, got %d", ghSvc.releaseCalls)
	}
}

// TestCreateReviewTask_ReservesThenAssignsTaskID verifies the happy path:
// Reserve -> CreateReviewTask -> AssignReviewPRTaskID.
func TestCreateReviewTask_ReservesThenAssignsTaskID(t *testing.T) {
	svc, _ := setupReviewTaskTest(t)
	ghSvc := &mockGitHubService{reserveReturn: true}
	svc.SetGitHubService(ghSvc)
	creator := &countingReviewTaskCreator{taskID: "task-999"}
	svc.SetReviewTaskCreator(creator)

	svc.createReviewTask(context.Background(), newReviewEvent())

	if ghSvc.reserveCalls != 1 {
		t.Errorf("expected 1 Reserve call, got %d", ghSvc.reserveCalls)
	}
	if creator.calls != 1 {
		t.Errorf("expected 1 CreateReviewTask call, got %d", creator.calls)
	}
	if ghSvc.assignCalls != 1 {
		t.Errorf("expected 1 AssignReviewPRTaskID call, got %d", ghSvc.assignCalls)
	}
	if ghSvc.assignedTaskID != "task-999" {
		t.Errorf("AssignReviewPRTaskID got taskID=%q, want %q", ghSvc.assignedTaskID, "task-999")
	}
	if ghSvc.releaseCalls != 0 {
		t.Errorf("expected no Release calls on happy path, got %d", ghSvc.releaseCalls)
	}
}

// TestCreateReviewTask_ReleasesOnTaskCreationFailure verifies that a failed
// CreateReviewTask triggers a Release so the slot can be retried on the next
// poll instead of being permanently blocked by an orphan reservation.
func TestCreateReviewTask_ReleasesOnTaskCreationFailure(t *testing.T) {
	svc, _ := setupReviewTaskTest(t)
	ghSvc := &mockGitHubService{reserveReturn: true}
	svc.SetGitHubService(ghSvc)
	creator := &countingReviewTaskCreator{err: assertErrTaskCreate}
	svc.SetReviewTaskCreator(creator)

	svc.createReviewTask(context.Background(), newReviewEvent())

	if ghSvc.reserveCalls != 1 {
		t.Errorf("expected 1 Reserve call, got %d", ghSvc.reserveCalls)
	}
	if creator.calls != 1 {
		t.Errorf("expected 1 CreateReviewTask call, got %d", creator.calls)
	}
	if ghSvc.releaseCalls != 1 {
		t.Errorf("expected 1 Release call after task-create failure, got %d", ghSvc.releaseCalls)
	}
	if ghSvc.assignCalls != 0 {
		t.Errorf("expected no Assign call after failure, got %d", ghSvc.assignCalls)
	}
}

func TestGitHubWatcherTaskCreateError_ClassifiesWIPAsDeferred(t *testing.T) {
	err := wfmodels.NewWIPLimitError("step1", 2, 2)
	if !isWatcherCapacityRejection(err) {
		t.Fatal("expected WIP-limit rejection to be classified as deferred capacity")
	}
}

func TestCreateReviewTask_WIPRejectionIsRetriedOnLaterPoll(t *testing.T) {
	svc, _ := setupReviewTaskTest(t)
	ghSvc := &mockGitHubService{reserveReturn: true}
	svc.SetGitHubService(ghSvc)
	creator := &countingReviewTaskCreator{
		errs:   []error{wfmodels.NewWIPLimitError("step1", 2, 2)},
		taskID: "task-after-capacity",
	}
	svc.SetReviewTaskCreator(creator)

	svc.createReviewTask(context.Background(), newReviewEvent())
	svc.createReviewTask(context.Background(), newReviewEvent())

	if creator.calls != 2 {
		t.Fatalf("expected one rejected attempt and one retry, got %d calls", creator.calls)
	}
	if ghSvc.releaseCalls != 1 {
		t.Fatalf("expected only the rejected attempt to release its reservation, got %d", ghSvc.releaseCalls)
	}
	if ghSvc.assignCalls != 1 || ghSvc.assignedTaskID != "task-after-capacity" {
		t.Fatalf("expected retry to assign task ID, calls=%d task=%q", ghSvc.assignCalls, ghSvc.assignedTaskID)
	}
}

func TestReviewWatchLifecycle_BootReadyStaysOnReviewUntilTurnComplete(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "review-task", "review-session", "review")

	steps := newMockStepGetter()
	steps.steps["review"] = &wfmodels.WorkflowStep{
		ID: "review", WorkflowID: "wf1", Name: "Review", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter:        []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}},
		},
	}
	steps.steps["done"] = &wfmodels.WorkflowStep{ID: "done", WorkflowID: "wf1", Name: "Done", Position: 1}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), agentMgr)

	if !svc.shouldAutoStartStep(ctx, "review") {
		t.Fatal("review step should auto-start an admitted watcher task")
	}
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{TaskID: "review-task", SessionID: "review-session"})
	bootTask, err := repo.GetTask(ctx, "review-task")
	if err != nil {
		t.Fatalf("load task after boot-ready: %v", err)
	}
	if bootTask.WorkflowStepID != "review" {
		t.Fatalf("boot-ready must not advance review task, got step %q", bootTask.WorkflowStepID)
	}

	if err := repo.UpdateTaskSessionState(ctx, "review-session", models.TaskSessionStateRunning, ""); err != nil {
		t.Fatalf("mark review turn running: %v", err)
	}
	task, err := repo.GetTask(ctx, "review-task")
	if err != nil {
		t.Fatalf("reload task before turn completion: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "review-session")
	if err != nil {
		t.Fatalf("reload session before turn completion: %v", err)
	}
	if !svc.processOnTurnComplete(ctx, task, session) {
		t.Fatal("first genuine turn completion should advance the review task")
	}
	completedTask, err := repo.GetTask(ctx, "review-task")
	if err != nil {
		t.Fatalf("load task after turn completion: %v", err)
	}
	if completedTask.WorkflowStepID != "done" {
		t.Fatalf("expected turn completion to move task to done, got %q", completedTask.WorkflowStepID)
	}
}
