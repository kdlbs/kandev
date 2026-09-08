package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestBuildReviewTaskRequest_TruncatesTitle is the regression test for the
// dropped-review-task bug: a PR whose "PR #<n>: <title>" string exceeds the
// 60-rune task title limit must be shortened (prefix kept, ≤60 runes, trailing
// ellipsis) so CreateReviewTask accepts it instead of rejecting the whole task.
func TestBuildReviewTaskRequest_TruncatesTitle(t *testing.T) {
	tests := []struct {
		name      string
		prTitle   string
		wantExact string // when non-empty, the full expected title
		wantTrunc bool
	}{
		{
			name:      "short ascii unchanged",
			prTitle:   "Fix login bug",
			wantExact: "PR #42: Fix login bug",
		},
		{
			name:      "long ascii truncated",
			prTitle:   strings.Repeat("a", 100),
			wantTrunc: true,
		},
		{
			name:      "long multibyte not split",
			prTitle:   strings.Repeat("é", 100),
			wantTrunc: true,
		},
		{
			// Prefix "PR #42: " is 8 runes, so a 52-rune PR title yields exactly
			// the 60-rune limit and must pass through untouched (boundary case).
			name:      "exactly at limit unchanged",
			prTitle:   strings.Repeat("a", taskservice.TaskTitleMaxLength-len("PR #42: ")),
			wantExact: "PR #42: " + strings.Repeat("a", taskservice.TaskTitleMaxLength-len("PR #42: ")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := newReviewEvent()
			evt.PR.Title = tt.prTitle
			req := buildReviewTaskRequest(evt, nil, "acme/widget")

			if got := utf8.RuneCountInString(req.Title); got > taskservice.TaskTitleMaxLength {
				t.Fatalf("title = %d runes, want ≤ %d", got, taskservice.TaskTitleMaxLength)
			}
			if !utf8.ValidString(req.Title) {
				t.Fatalf("title %q is not valid UTF-8; byte-based truncation split a rune", req.Title)
			}
			if err := taskservice.ValidateTaskTitle(req.Title); err != nil {
				t.Fatalf("ValidateTaskTitle: %v", err)
			}
			if !strings.HasPrefix(req.Title, "PR #42:") {
				t.Errorf("title %q lost the PR prefix", req.Title)
			}
			if tt.wantExact != "" && req.Title != tt.wantExact {
				t.Errorf("title = %q, want %q", req.Title, tt.wantExact)
			}
			if tt.wantTrunc && !strings.HasSuffix(req.Title, "…") {
				t.Errorf("truncated title %q should end with ellipsis", req.Title)
			}
		})
	}
}

// TestBuildIssueTaskTitle_TruncatesTitle mirrors the review-title regression for
// the GitHub issue watcher's "Issue #<n>: <title>" builder.
func TestBuildIssueTaskTitle_TruncatesTitle(t *testing.T) {
	tests := []struct {
		name       string
		issueTitle string
		wantExact  string
		wantTrunc  bool
	}{
		{
			name:       "short ascii unchanged",
			issueTitle: "Broken export",
			wantExact:  "Issue #7: Broken export",
		},
		{
			name:       "long ascii truncated",
			issueTitle: strings.Repeat("b", 100),
			wantTrunc:  true,
		},
		{
			name:       "long multibyte not split",
			issueTitle: strings.Repeat("ü", 100),
			wantTrunc:  true,
		},
		{
			// Prefix "Issue #7: " is 10 runes, so a 50-rune issue title yields
			// exactly the 60-rune limit and must pass through untouched.
			name:       "exactly at limit unchanged",
			issueTitle: strings.Repeat("b", taskservice.TaskTitleMaxLength-len("Issue #7: ")),
			wantExact:  "Issue #7: " + strings.Repeat("b", taskservice.TaskTitleMaxLength-len("Issue #7: ")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildIssueTaskTitle(7, tt.issueTitle)

			if n := utf8.RuneCountInString(got); n > taskservice.TaskTitleMaxLength {
				t.Fatalf("title = %d runes, want ≤ %d", n, taskservice.TaskTitleMaxLength)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("title %q is not valid UTF-8; byte-based truncation split a rune", got)
			}
			if err := taskservice.ValidateTaskTitle(got); err != nil {
				t.Fatalf("ValidateTaskTitle: %v", err)
			}
			if !strings.HasPrefix(got, "Issue #7:") {
				t.Errorf("title %q lost the Issue prefix", got)
			}
			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("title = %q, want %q", got, tt.wantExact)
			}
			if tt.wantTrunc && !strings.HasSuffix(got, "…") {
				t.Errorf("truncated title %q should end with ellipsis", got)
			}
		})
	}
}

// countingReviewTaskCreator records how many times CreateReviewTask was called.
type countingReviewTaskCreator struct {
	calls    int
	err      error
	errs     []error
	taskID   string
	metadata map[string]interface{} // extra metadata added to returned task
}

func (c *countingReviewTaskCreator) CreateReviewTask(_ context.Context, _ *ReviewTaskRequest) (*taskmodels.Task, error) {
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
	return &taskmodels.Task{ID: id, Metadata: c.metadata}, nil
}

// newReviewEvent builds a NewReviewPREvent for createReviewTask tests.
func newReviewEvent() *github.NewReviewPREvent {
	return &github.NewReviewPREvent{
		ReviewWatchID:  "w1",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		AgentProfileID: testAgentProfileID,
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

// --- Auto-start idempotency regression tests ---

// seedReviewTask inserts a workspace, workflow, and task row into the real SQLite
// repo so that StartTask (which reads the task) can proceed. The task carries
// MetaKeyAutoStartClaimed so both auto-start paths compete for the same token.
// testAgentProfileID is a stable profile ID used in auto-start regression tests.
// It must be non-empty so executor.PrepareSession doesn't return ErrNoAgentProfileID.
const testAgentProfileID = "test-profile-1"

func seedReviewTask(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *taskmodels.Workspace) error
	CreateWorkflow(context.Context, *taskmodels.Workflow) error
	CreateTask(context.Context, *taskmodels.Task) error
}, taskID, stepID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now})
	task := &taskmodels.Task{
		ID:             taskID,
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepID,
		Title:          "Review PR #42",
		Description:    "Review this PR",
		State:          v1.TaskStateInProgress,
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyAutoStartGuard:   true,
			taskmodels.MetaKeyAutoStartClaimed: true,
			taskmodels.MetaKeyAgentProfileID:   testAgentProfileID,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("seedReviewTask: %v", err)
	}
}

// TestAutoStart_BothPathsFireExactlyOnce is the regression test for the
// duplicate-agent bug: a review task placed into a feeder and immediately
// promoted (so both the watcher's synchronous Path B and the promotion's
// event-driven Path A see an admitted, auto-start-eligible task) must launch
// exactly one agent session.
func TestAutoStart_BothPathsFireExactlyOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-review-42"
	const stepID = "step-review"

	sg := newMockStepGetter()
	sg.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf1", Name: "Review", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	seedReviewTask(t, repo, taskID, stepID)

	// mockTaskRepo must also contain the task so startTask's s.scheduler.GetTask lookup succeeds.
	// Include agent_profile_id so executor.PrepareSession doesn't return ErrNoAgentProfileID.
	// Include auto_start_guard so autoStartTaskForStep knows to compete for the claim token.
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID:    taskID,
		State: v1.TaskStateInProgress,
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyAutoStartGuard:   true,
			taskmodels.MetaKeyAutoStartClaimed: true,
			taskmodels.MetaKeyAgentProfileID:   testAgentProfileID,
		},
	}

	var launchCount atomic.Int32
	launched := make(chan struct{}, 2) // buffered so neither sender blocks
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCount.Add(1)
			launched <- struct{}{}
			return &executor.LaunchAgentResponse{}, nil
		},
	}

	svc := createTestServiceWithScheduler(repo, sg, taskRepo, agentMgr)

	// Load the DB task (which has the MetaKeyAutoStartClaimed token in metadata).
	dbTask, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	// Path B: watcher's synchronous auto-start on an admitted task with the token.
	svc.autoStartReviewTask(ctx, newReviewEvent(), dbTask)

	// Path A: promotion event-driven auto-start on the same task.
	// autoStartTaskForStep spawns a goroutine — we synchronise via the launched channel.
	svc.autoStartTaskForStep(ctx, taskID, stepID, "task.queue_promoted", 0)

	// Wait for exactly one launch. Allow 2 seconds for the async goroutine.
	deadline := time.After(2 * time.Second)
	select {
	case <-launched:
	case <-deadline:
		t.Fatal("timed out waiting for at least one launch")
	}

	// Give any erroneous second launch a short window to arrive.
	select {
	case <-launched:
	case <-time.After(200 * time.Millisecond):
	}

	if got := launchCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 LaunchAgent call, got %d", got)
	}

	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected exactly 1 session in DB, got %d", len(sessions))
	}
}

// TestAutoStart_ClaimIsAtomic verifies that two competing calls to claimAutoStart
// for the same task produce exactly one winner.
func TestAutoStart_ClaimIsAtomic(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-claim-atomic"
	seedReviewTask(t, repo, taskID, "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	first := svc.claimAutoStart(ctx, taskID, "test")
	second := svc.claimAutoStart(ctx, taskID, "test")

	if !first {
		t.Error("first claimAutoStart should have succeeded")
	}
	if second {
		t.Error("second claimAutoStart should have failed (token already removed)")
	}
}

// TestAutoStart_ClaimIsRaceSafe drives the two production auto-start paths'
// claim through a barrier so both goroutines contend for the single token at
// once, then joins both results and asserts exactly one winner. The sequential
// TestAutoStart_ClaimIsAtomic only proves the token disappears after the first
// call; it would still pass under a non-atomic read-then-delete that lets two
// concurrent callers both win. This exercises the real read-modify-write under
// contention against the SQLite repo's conditional UPDATE.
func TestAutoStart_ClaimIsRaceSafe(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-claim-race"
	seedReviewTask(t, repo, taskID, "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	// Release both claimers at the same instant so their RemoveTaskMetadataKey
	// calls overlap. Results are joined through a buffered channel.
	start := make(chan struct{})
	results := make(chan bool, 2)
	for range 2 {
		go func() {
			<-start
			results <- svc.claimAutoStart(ctx, taskID, "test")
		}()
	}
	close(start)

	winners := 0
	for range 2 {
		if <-results {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly one concurrent claim winner, got %d", winners)
	}
}

// TestAutoStart_FailedLaunchRestoresClaim verifies that when an auto-start
// launch fails, restoreAutoStartClaim puts the token back so a later trigger
// can retry.
func TestAutoStart_FailedLaunchRestoresClaim(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-restore-claim"
	seedReviewTask(t, repo, taskID, "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	// Claim succeeds — simulate a failed launch.
	if !svc.claimAutoStart(ctx, taskID, "test") {
		t.Fatal("initial claim should succeed")
	}

	// Restore the claim.
	svc.restoreAutoStartClaim(ctx, taskID, "test")

	// A subsequent claim should succeed again.
	if !svc.claimAutoStart(ctx, taskID, "test") {
		t.Error("claim after restore should succeed")
	}
}

// TestAutoStart_NoTokenDoesNotBlock verifies that autoStartTaskForStep still
// launches a task that has no MetaKeyAutoStartClaimed token (ordinary auto-start
// tasks unrelated to the review watcher).
func TestAutoStart_NoTokenDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-no-token"
	const stepID = "step-review"

	sg := newMockStepGetter()
	sg.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf1", Name: "Review", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	// Seed a task WITHOUT the auto-start-claimed token.
	now := time.Now().UTC()
	ctx2 := context.Background()
	_ = repo.CreateWorkspace(ctx2, &taskmodels.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx2, &taskmodels.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateTask(ctx2, &taskmodels.Task{
		ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: stepID,
		Title: "T", Description: "D", State: v1.TaskStateInProgress,
		Metadata:  map[string]interface{}{taskmodels.MetaKeyAgentProfileID: testAgentProfileID},
		CreatedAt: now, UpdatedAt: now,
	})

	// Also seed the task in mockTaskRepo so startTask's scheduler.GetTask lookup succeeds.
	// Include agent_profile_id so executor.PrepareSession doesn't return ErrNoAgentProfileID.
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID:    taskID,
		State: v1.TaskStateInProgress,
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyAgentProfileID: testAgentProfileID,
		},
	}

	launched := make(chan struct{}, 1)
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launched <- struct{}{}
			return &executor.LaunchAgentResponse{}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, sg, taskRepo, agentMgr)

	svc.autoStartTaskForStep(ctx, taskID, stepID, "task.moved", 0)

	select {
	case <-launched:
	case <-time.After(2 * time.Second):
		t.Fatal("autoStartTaskForStep should launch a task with no token")
	}
}

// TestAutoStart_FailedLaunchWithoutGuardDoesNotStampClaimed is the regression
// test for the bogus-claim-restore bug: restoreAutoStartClaim used to run
// unconditionally on any launch failure, so a task that never carried
// MetaKeyAutoStartGuard (and therefore never claimed MetaKeyAutoStartClaimed
// in the first place) still gained the token after a failed launch. A failed
// launch on a guardless task must leave auto_start_claimed absent, and must
// stamp auto_start_failed instead so the failure surfaces on the card.
func TestAutoStart_FailedLaunchWithoutGuardDoesNotStampClaimed(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-fail-no-guard"
	const stepID = "step-review"

	sg := newMockStepGetter()
	sg.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf1", Name: "Review", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	now := time.Now().UTC()
	_ = repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateTask(ctx, &taskmodels.Task{
		ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: stepID,
		Title: "T", Description: "D", State: v1.TaskStateInProgress,
		Metadata:  map[string]interface{}{taskmodels.MetaKeyAgentProfileID: testAgentProfileID},
		CreatedAt: now, UpdatedAt: now,
	})

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID:    taskID,
		State: v1.TaskStateInProgress,
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyAgentProfileID: testAgentProfileID,
		},
	}

	attempted := make(chan struct{}, 1)
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			attempted <- struct{}{}
			return nil, errors.New("boom")
		},
	}
	svc := createTestServiceWithScheduler(repo, sg, taskRepo, agentMgr)

	svc.autoStartTaskForStep(ctx, taskID, stepID, "task.moved", 0)

	select {
	case <-attempted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected the launch attempt to fire")
	}

	deadline := time.After(2 * time.Second)
	for {
		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if task.Metadata[taskmodels.MetaKeyAutoStartFailed] != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for auto_start_failed marker to be set")
		case <-time.After(10 * time.Millisecond):
		}
	}

	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Metadata[taskmodels.MetaKeyAutoStartClaimed] != nil {
		t.Error("a failed launch on a task without the guard must not stamp auto_start_claimed")
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

	if err := repo.UpdateTaskSessionState(ctx, "review-session", taskmodels.TaskSessionStateRunning, ""); err != nil {
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
