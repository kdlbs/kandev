package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedAutomationWorkspaceRepos creates a workspace with the given repository
// IDs (each with a distinct default branch derived from its ID) for
// exercising resolveAutomationRepository / resolveExplicitRepositories.
func seedAutomationWorkspaceRepos(t *testing.T, repo *sqliterepo.Repository, workspaceID string, repoIDs []string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for _, id := range repoIDs {
		r := &models.Repository{
			ID:            id,
			WorkspaceID:   workspaceID,
			Name:          id,
			SourceType:    "local",
			LocalPath:     "/tmp/" + id,
			DefaultBranch: "main-" + id,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := repo.CreateRepository(ctx, r); err != nil {
			t.Fatalf("create repository %s: %v", id, err)
		}
	}
}

func TestResolveAutomationRepository_MultipleExplicitRepositories(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-1", []string{"repo-a", "repo-b"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a := &automation.Automation{WorkspaceID: "ws-1", RepositoryIDs: []string{"repo-a", "repo-b"}}
	evt := &automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeScheduled}

	resolved := svc.resolveAutomationRepository(context.Background(), a, evt)

	if len(resolved) != 2 {
		t.Fatalf("expected 2 resolved repositories, got %d: %+v", len(resolved), resolved)
	}
	if resolved[0].RepositoryID != "repo-a" || resolved[0].BaseBranch != "main-repo-a" || resolved[0].CheckoutBranch != "main-repo-a" {
		t.Errorf("unexpected first repository: %+v", resolved[0])
	}
	if resolved[1].RepositoryID != "repo-b" || resolved[1].BaseBranch != "main-repo-b" || resolved[1].CheckoutBranch != "main-repo-b" {
		t.Errorf("unexpected second repository: %+v", resolved[1])
	}
}

func TestResolveAutomationRepository_SkipsUnloadableID(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-1", []string{"repo-a"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a := &automation.Automation{WorkspaceID: "ws-1", RepositoryIDs: []string{"repo-a", "repo-missing"}}
	evt := &automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeScheduled}

	resolved := svc.resolveAutomationRepository(context.Background(), a, evt)

	if len(resolved) != 1 || resolved[0].RepositoryID != "repo-a" {
		t.Fatalf("expected only repo-a to resolve, got %+v", resolved)
	}
}

func TestResolveAutomationRepository_EmptyListFallsBackToWorkspace(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-1", []string{"repo-only"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a := &automation.Automation{WorkspaceID: "ws-1"}
	evt := &automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeScheduled}

	resolved := svc.resolveAutomationRepository(context.Background(), a, evt)

	if len(resolved) != 1 || resolved[0].RepositoryID != "repo-only" {
		t.Fatalf("expected fallback to the workspace's only repository, got %+v", resolved)
	}
}

func TestResolveAutomationTaskTitleTruncatesRenderedTitle(t *testing.T) {
	svc := &Service{}
	longTitle := strings.Repeat("x", taskservice.TaskTitleMaxLength+20)
	a := &automation.Automation{TaskTitleTemplate: "{{pr.title}}"}
	evt := &automation.AutomationTriggeredEvent{
		TriggerType: automation.TriggerTypeGitHubPR,
		TriggerData: json.RawMessage(`{"title":"` + longTitle + `"}`),
	}

	got := svc.resolveAutomationTaskTitle(a, evt)
	if got != strings.Repeat("x", taskservice.TaskTitleMaxLength-1)+"…" {
		t.Fatalf("resolved title = %q, want rendered title truncated with ellipsis", got)
	}
}

// TestResolveAutomationRepository_GitHubPRIgnoresConfiguredRepositoryIDs is
// the regression guard for a CodeRabbit review finding on PR #2077: the
// frontend disables (but does not clear) the repository picker for
// github_pr triggers, so a previously-configured multi-repo selection can
// stay in the saved payload. Prove the backend never reads it — the PR's
// own repository (from trigger data) always wins, regardless of what
// RepositoryIDs holds.
func TestResolveAutomationRepository_GitHubPRIgnoresConfiguredRepositoryIDs(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-1", []string{"repo-a", "repo-b", "repo-pr"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.repositoryResolver = stubReviewResolver{repoID: "repo-pr", baseBranch: "main-repo-pr"}

	a := &automation.Automation{WorkspaceID: "ws-1", RepositoryIDs: []string{"repo-a", "repo-b"}}
	evt := &automation.AutomationTriggeredEvent{
		TriggerType: automation.TriggerTypeGitHubPR,
		TriggerData: json.RawMessage(`{"repo":"owner/name","head_branch":"feature/x","base_branch":"main-repo-pr"}`),
	}

	resolved := svc.resolveAutomationRepository(context.Background(), a, evt)

	if len(resolved) != 1 || resolved[0].RepositoryID != "repo-pr" {
		t.Fatalf("expected only the PR's own repository (repo-pr), got %+v", resolved)
	}
	if resolved[0].CheckoutBranch != "feature/x" {
		t.Errorf("expected checkout branch from the PR's head branch, got %q", resolved[0].CheckoutBranch)
	}
}

// stubReviewTaskCreator records the request the automation handler builds and
// returns a task the caller can then look up in the repo.
type stubReviewTaskCreator struct {
	got  *ReviewTaskRequest
	task *models.Task
	err  error
}

func (s *stubReviewTaskCreator) CreateReviewTask(_ context.Context, req *ReviewTaskRequest) (*models.Task, error) {
	s.got = req
	if s.err != nil {
		return nil, s.err
	}
	return s.task, nil
}

// stubAutomationService serves one automation and records the runs written.
type stubAutomationService struct {
	automation   *automation.Automation
	runs         []*automation.AutomationRun
	succeeded    []string
	failed       map[string]string
	recordRunErr error
}

func (s *stubAutomationService) GetAutomation(context.Context, string) (*automation.Automation, error) {
	return s.automation, nil
}

func (s *stubAutomationService) RecordRun(_ context.Context, run *automation.AutomationRun) error {
	if s.recordRunErr != nil {
		return s.recordRunErr
	}
	s.runs = append(s.runs, run)
	return nil
}

func (s *stubAutomationService) MarkRunFailedByTaskID(_ context.Context, taskID, errMsg string) error {
	if s.failed == nil {
		s.failed = map[string]string{}
	}
	s.failed[taskID] = errMsg
	return nil
}

func (s *stubAutomationService) MarkRunSucceededByTaskID(_ context.Context, taskID string) error {
	s.succeeded = append(s.succeeded, taskID)
	return nil
}

// seedAutomationTask writes a task exactly as the automation path now creates
// one: ordinary and persistent, tagged only by its origin.
func seedAutomationTask(t *testing.T, repo *sqliterepo.Repository, taskID, origin string, ephemeral bool) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-" + taskID, Name: "Automation", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          taskID,
		WorkspaceID: "ws-" + taskID,
		Title:       "Automation run",
		State:       v1.TaskStateInProgress,
		Origin:      origin,
		IsEphemeral: ephemeral,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
}

// seedAutomationWorkspaceRepo gives the automation path a repository to
// resolve; without one a firing is recorded as a failed run and never reaches
// task creation.
func seedAutomationWorkspaceRepo(t *testing.T, repo *sqliterepo.Repository, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: workspaceID, Name: "Automation", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-" + workspaceID, WorkspaceID: workspaceID, Name: "app",
		SourceType: "local", LocalPath: t.TempDir(), DefaultBranch: "main",
	}))
}

// A firing produces one kind of run: an ordinary task tagged by origin. It is
// hidden by that origin, so it must NOT be marked ephemeral — ephemerality is
// what used to reap its worktree and strand its run at task_created.
func TestCreateAutomationTask_TagsOriginAndLeavesTaskNonEphemeral(t *testing.T) {
	repo := setupTestRepo(t)
	creator := &stubReviewTaskCreator{task: &models.Task{ID: "t-created"}}
	autoSvc := &stubAutomationService{automation: &automation.Automation{
		ID: "a-1", WorkspaceID: "ws-1", Name: "nightly sweep", Prompt: "sweep",
		WorkflowID: "wf-1", WorkflowStepID: "step-1", Enabled: true,
	}}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)
	svc.reviewTaskCreator = creator
	seedAutomationWorkspaceRepo(t, repo, "ws-1")

	svc.createAutomationTask(context.Background(), &automation.AutomationTriggeredEvent{
		AutomationID: "a-1", TriggerID: "trg-1", TriggerType: automation.TriggerTypeScheduled,
	})

	require.NotNil(t, creator.got, "expected the automation to create its task; runs=%+v", autoSvc.runs)
	require.Equal(t, models.TaskOriginAutomationRun, creator.got.Origin)
	require.False(t, creator.got.IsEphemeral,
		"automation tasks are hidden by origin; marking them ephemeral reaps the worktree and strands the run")
	require.Equal(t, "wf-1", creator.got.WorkflowID, "configured workflow fields pass through unchanged")
	require.Equal(t, "step-1", creator.got.WorkflowStepID)
	require.NotContains(t, creator.got.Metadata, "execution_mode",
		"the execution mode is withdrawn and must not be stamped on the task")
}

// Workflow and step are optional for every automation: no run is placed on a
// board, so no automation needs a starting column.
func TestCreateAutomationTask_WorksWithoutWorkflowOrStep(t *testing.T) {
	repo := setupTestRepo(t)
	creator := &stubReviewTaskCreator{task: &models.Task{ID: "t-created"}}
	autoSvc := &stubAutomationService{automation: &automation.Automation{
		ID: "a-2", WorkspaceID: "ws-1", Name: "no workflow", Prompt: "report", Enabled: true,
	}}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)
	svc.reviewTaskCreator = creator
	seedAutomationWorkspaceRepo(t, repo, "ws-1")

	svc.createAutomationTask(context.Background(), &automation.AutomationTriggeredEvent{
		AutomationID: "a-2", TriggerID: "trg-2", TriggerType: automation.TriggerTypeScheduled,
	})

	require.NotNil(t, creator.got, "a workflow-less automation must still create its task")
	require.Empty(t, creator.got.WorkflowID)
	require.Empty(t, creator.got.WorkflowStepID)
	require.Equal(t, models.TaskOriginAutomationRun, creator.got.Origin)
}

// The deadlock the execution-mode split caused: a non-ephemeral automation
// task never reached a terminal run status, so its automation_run row sat at
// task_created forever and held a max_concurrent_runs slot until a human
// archived it. Finalization keys on origin alone.
func TestFinalizeAutomationRun_NonEphemeralTaskReachesTerminalStatus(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationTask(t, repo, "t-auto", models.TaskOriginAutomationRun, false)

	autoSvc := &stubAutomationService{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)

	svc.finalizeAutomationRun(context.Background(), "t-auto", true, "")
	require.Equal(t, []string{"t-auto"}, autoSvc.succeeded,
		"a non-ephemeral automation task must still reach a terminal run status")

	svc.finalizeAutomationRun(context.Background(), "t-auto", false, "agent failed")
	require.Equal(t, "agent failed", autoSvc.failed["t-auto"])
}

// Only automation-origin tasks are finalized; an ordinary task must not have
// an automation run flipped underneath it.
func TestFinalizeAutomationRun_IgnoresNonAutomationTasks(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationTask(t, repo, "t-manual", models.TaskOriginManual, false)

	autoSvc := &stubAutomationService{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)

	svc.finalizeAutomationRun(context.Background(), "t-manual", true, "")
	require.Empty(t, autoSvc.succeeded)
	require.Empty(t, autoSvc.failed)
}

// A run's files are the point of running it, and an agent that ends by asking
// a question needs its workspace to still exist. The turn-complete path
// finalizes the run and stops the agent, but leaves the session's worktree
// association — and therefore the worktree — in place.
func TestHandleAutomationTurnComplete_FinalizesWithoutReapingTheWorktree(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedAutomationRunSession(t, repo, "t-keep", "s-keep", "exec-keep")
	require.NoError(t, repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		SessionID:    "s-keep",
		WorktreeID:   "wt-keep",
		RepositoryID: "repo-1",
		WorktreePath: "/tmp/kandev/t-keep",
	}))

	mgr := &mockAgentManager{}
	autoSvc := &stubAutomationService{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), mgr)
	svc.SetAutomationService(autoSvc)

	session, err := repo.GetTaskSession(ctx, "s-keep")
	require.NoError(t, err)
	handled := svc.handleAutomationTurnComplete(ctx, "t-keep", "s-keep", session, "end_turn", false, "")
	require.True(t, handled)
	require.Equal(t, []string{"t-keep"}, autoSvc.succeeded)

	worktrees, err := repo.ListTaskSessionWorktrees(ctx, "s-keep")
	require.NoError(t, err)
	require.Len(t, worktrees, 1,
		"the automation run's worktree must survive its turn — the files it wrote are the point of the run")
	require.Equal(t, "wt-keep", worktrees[0].WorktreeID)
}

// A finished run has to stay answerable. COMPLETED is a terminal session state
// that explicitly refuses resume ("create a new session instead"), so parking a
// successful run there would turn every report into a receipt — the user opens
// it, types a follow-up, and is told the session has ended.
func TestHandleAutomationTurnComplete_LeavesASuccessfulRunAnswerable(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedAutomationRunSession(t, repo, "t-reply", "s-reply", "exec-reply")

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.SetAutomationService(&stubAutomationService{})

	session, err := repo.GetTaskSession(ctx, "s-reply")
	require.NoError(t, err)
	require.True(t, svc.handleAutomationTurnComplete(ctx, "t-reply", "s-reply", session, "end_turn", false, ""))

	after, err := repo.GetTaskSession(ctx, "s-reply")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, after.State,
		"a successful run parks where an ordinary finished turn parks, so the user can reply to it")
	require.False(t, isTerminalSessionState(after.State))
}

// A failed or cancelled run is a different matter: those states are terminal by
// design and carry the failure the user needs to see.
func TestHandleAutomationTurnComplete_KeepsFailureStatesTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedAutomationRunSession(t, repo, "t-fail", "s-fail", "exec-fail")

	autoSvc := &stubAutomationService{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.SetAutomationService(autoSvc)

	session, err := repo.GetTaskSession(ctx, "s-fail")
	require.NoError(t, err)
	require.True(t, svc.handleAutomationTurnComplete(ctx, "t-fail", "s-fail", session, "error", true, "boom"))

	after, err := repo.GetTaskSession(ctx, "s-fail")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, after.State)
	require.Equal(t, "boom", autoSvc.failed["t-fail"])
}

// The run row is written before the launch. If the launch then fails and
// nothing marks the run terminal, no completion event is ever coming — the run
// sits at task_created and holds the concurrency slot, so one bad launch stops
// the automation permanently.
func TestAutoStartAutomationTask_FailedLaunchReleasesTheConcurrencySlot(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationTask(t, repo, "t-nostart", models.TaskOriginAutomationRun, false)

	autoSvc := &stubAutomationService{}
	// Fail the launch the way a real one fails: inside StartTask, after the
	// run row has already been written.
	taskRepo := newMockTaskRepo()
	taskRepo.getTaskErr = errors.New("executor unavailable")
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	svc.SetAutomationService(autoSvc)

	svc.autoStartAutomationTask(
		context.Background(),
		&automation.Automation{ID: "a-nostart", WorkspaceID: "ws-t-nostart"},
		&models.Task{ID: "t-nostart", Description: "sweep"},
		"",
	)

	require.Contains(t, autoSvc.failed, "t-nostart",
		"a launch that never happened must still mark the run terminal, or the cap jams forever")
	require.NotEmpty(t, autoSvc.failed["t-nostart"], "the launch error is what explains the failed run")
	require.Empty(t, autoSvc.succeeded)
}

// The run row is the only record a firing happened: it carries the concurrency
// accounting, and it is the sole way the work is reachable, since the task is
// hidden from every board and list. Launching an agent without it would leave
// an automation running that nothing can see and nothing can finalize.
func TestCreateAutomationTask_DoesNotLaunchWhenTheRunCannotBeRecorded(t *testing.T) {
	repo := setupTestRepo(t)
	creator := &stubReviewTaskCreator{task: &models.Task{ID: "t-unrecorded"}}
	autoSvc := &stubAutomationService{
		automation: &automation.Automation{
			ID: "a-1", WorkspaceID: "ws-1", Name: "sweep", Prompt: "sweep", Enabled: true,
		},
		recordRunErr: errors.New("disk full"),
	}

	taskRepo := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	svc.SetAutomationService(autoSvc)
	svc.reviewTaskCreator = creator
	seedAutomationWorkspaceRepo(t, repo, "ws-1")

	svc.createAutomationTask(context.Background(), &automation.AutomationTriggeredEvent{
		AutomationID: "a-1", TriggerID: "trg-1", TriggerType: automation.TriggerTypeScheduled,
	})

	require.NotNil(t, creator.got, "the task is created before the run row, as it is today")
	require.Empty(t, taskRepo.tasks, "no agent may be launched for a run nothing can see")
	require.Empty(t, autoSvc.succeeded)
}

// A firing produces both a task and its run row, or neither. The task is hidden
// from every board and list by its origin, so one left behind with no run
// pointing at it is invisible, unfinalizable, and holds a concurrency slot
// nobody can see or clear.
func TestCreateAutomationTask_DeletesTheTaskWhenTheRunCannotBeRecorded(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepo(t, repo, "ws-1")
	// A real row, so "was it deleted" is a question the repository can answer.
	seedAutomationTask(t, repo, "t-orphan", models.TaskOriginAutomationRun, false)

	creator := &stubReviewTaskCreator{task: &models.Task{ID: "t-orphan"}}
	autoSvc := &stubAutomationService{
		automation: &automation.Automation{
			ID: "a-1", WorkspaceID: "ws-1", Name: "sweep", Prompt: "sweep", Enabled: true,
		},
		recordRunErr: errors.New("disk full"),
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	svc.SetAutomationService(autoSvc)
	svc.reviewTaskCreator = creator

	svc.createAutomationTask(ctx, &automation.AutomationTriggeredEvent{
		AutomationID: "a-1", TriggerID: "trg-1", TriggerType: automation.TriggerTypeScheduled,
	})

	require.NotNil(t, creator.got, "the task is created before the run row, as it is today")
	surviving, err := repo.GetTask(ctx, "t-orphan")
	if err == nil {
		require.Nil(t, surviving,
			"a task whose run row was never written must not outlive the firing")
	}
}
