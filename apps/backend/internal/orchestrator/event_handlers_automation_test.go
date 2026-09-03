package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
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

func TestResolveAutomationRepository_EmptyListUsesNoRepository(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-1", []string{"repo-only"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	a := &automation.Automation{WorkspaceID: "ws-1"}
	evt := &automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeScheduled}

	resolved := svc.resolveAutomationRepository(context.Background(), a, evt)

	if len(resolved) != 0 {
		t.Fatalf("expected no repository, got %+v", resolved)
	}
}

// @covers AC-OFFICE-AUTOMATION-TARGETS-001.10
func TestResolveAutomationRepository_UsesSavedBaseBranches(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-1", []string{"repo-a"})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	a := &automation.Automation{
		WorkspaceID: "ws-1",
		Repositories: []automation.AutomationRepository{
			{RepositoryID: "repo-a", BaseBranch: "release/2"},
		},
	}

	resolved := svc.resolveAutomationRepository(context.Background(), a, &automation.AutomationTriggeredEvent{
		TriggerType: automation.TriggerTypeScheduled,
	})

	require.Len(t, resolved, 1)
	require.Equal(t, "release/2", resolved[0].BaseBranch)
	require.Equal(t, "release/2", resolved[0].CheckoutBranch)
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

func TestFindAutomationContinuationRejectsChangedLaunchIdentity(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationWorkspaceRepos(t, repo, "ws-1", []string{"repo-a"})
	now := time.Now().UTC()
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             "continuation-task",
		WorkspaceID:    "ws-1",
		WorkflowID:     "workflow-a",
		WorkflowStepID: "step-a",
		Origin:         models.TaskOriginAutomationRun,
		Metadata: map[string]interface{}{
			models.MetaKeyAgentProfileID:    "agent-a",
			models.MetaKeyExecutorProfileID: "executor-a",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "continuation-repo", TaskID: "continuation-task", RepositoryID: "repo-a", Position: 0,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "continuation-session", TaskID: "continuation-task",
		State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now,
	}))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	a := &automation.Automation{
		ContinuationTaskID: "continuation-task", WorkspaceID: "ws-1",
		WorkflowID: "workflow-b", WorkflowStepID: "step-a",
		AgentProfileID: "agent-a", ExecutorProfileID: "executor-a",
		RepositoryIDs: []string{"repo-a"},
	}

	task, session, reason := svc.findAutomationContinuation(ctx, a, &automation.AutomationTriggeredEvent{
		TriggerType: automation.TriggerTypeScheduled,
	})
	require.Nil(t, task)
	require.Nil(t, session)
	require.Equal(t, "previous continuation task uses a different workflow", reason)
}

func TestFindAutomationContinuationKeepsImplicitStepAndRotatesRepositoryMode(t *testing.T) {
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             "implicit-step-task",
		WorkspaceID:    "ws-1",
		WorkflowID:     "workflow-a",
		WorkflowStepID: "resolved-start-step",
		Origin:         models.TaskOriginAutomationRun,
		Metadata: map[string]interface{}{
			models.MetaKeyAgentProfileID:           "agent-a",
			models.MetaKeyExecutorProfileID:        "executor-a",
			models.MetaKeyAutomationTaskMode:       string(automation.TaskModeAutomationRun),
			models.MetaKeyAutomationRepositoryMode: string(automation.RepositoryModeNone),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "implicit-step-session", TaskID: "implicit-step-task",
		State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now,
	}))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	a := &automation.Automation{
		ContinuationTaskID: "implicit-step-task", WorkspaceID: "ws-1",
		WorkflowID: "workflow-a", AgentProfileID: "agent-a", ExecutorProfileID: "executor-a",
		TaskMode: automation.TaskModeAutomationRun, RepositoryMode: automation.RepositoryModeNone,
	}

	task, session, reason := svc.findAutomationContinuation(ctx, a, &automation.AutomationTriggeredEvent{
		TriggerType: automation.TriggerTypeScheduled,
	})
	require.Equal(t, "implicit-step-task", task.ID)
	require.Equal(t, "implicit-step-session", session.ID)
	require.Equal(t, "continued the previous task and session", reason)

	a.RepositoryMode = automation.RepositoryModeWorkspaceDefault
	task, session, reason = svc.findAutomationContinuation(ctx, a, &automation.AutomationTriggeredEvent{
		TriggerType: automation.TriggerTypeScheduled,
	})
	require.Nil(t, task)
	require.Nil(t, session)
	require.Equal(t, "previous continuation task uses a different repository mode", reason)
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

	// Retention: what the sweep is offered, what it asked for, and a failure
	// to prove finalization does not depend on the answer.
	prunable     []string
	prunableKeep int
	prunableErr  error
	inUse        map[string]bool
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

func (s *stubAutomationService) PrunableRunTaskIDs(_ context.Context, _ string, keep int) ([]string, error) {
	s.prunableKeep = keep
	if s.prunableErr != nil {
		return nil, s.prunableErr
	}
	return s.prunable, nil
}

func (s *stubAutomationService) RunWorkspaceInUse(_ context.Context, taskID string) (bool, error) {
	return s.inUse[taskID], nil
}

// rejectingAutomationService models a run that is stopped after admission,
// but before the service-owned dispatcher can acquire its lock. The
// continuation task must not receive the firing's target in that case.
type rejectingAutomationService struct {
	*stubAutomationService
	dispatchErr error
}

func (s *rejectingAutomationService) DispatchRun(
	context.Context,
	string,
	automation.ThreadAction,
	string,
	func() (automation.RunDispatch, error),
) error {
	return s.dispatchErr
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

func TestCreateAutomationTask_BindsMergedPRTarget(t *testing.T) {
	repo := setupTestRepo(t)
	creator := &stubReviewTaskCreator{task: &models.Task{ID: "t-created"}}
	autoSvc := &stubAutomationService{automation: &automation.Automation{
		ID: "a-merged", WorkspaceID: "ws-merged", Name: "archive merged", Prompt: "archive", Enabled: true,
	}}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)
	svc.reviewTaskCreator = creator
	seedAutomationWorkspaceRepo(t, repo, "ws-merged")

	svc.createAutomationTask(context.Background(), &automation.AutomationTriggeredEvent{
		AutomationID: "a-merged", TriggerID: "trg-merged", TriggerType: automation.TriggerTypeGitHubPRMerged,
		TriggerData: json.RawMessage(`{"task_id":"target-task-1"}`),
	})

	require.NotNil(t, creator.got)
	require.Equal(t, "target-task-1", creator.got.Metadata[models.MetaKeyAutomationTargetTaskID])
}

// TestRefreshAutomationContinuationMetadataOnResume is the regression
// guard for the reuse_thread + github_pr_merged defect: a resumed task keeps
// whatever automation_target_task_id its FIRST firing stamped, so every merge
// after the first is refused by validateAutomationArchiveTarget forever. The
// per-firing metadata computed for the SECOND firing must be merged onto the
// resumed task after dispatch admission (and persisted), not discarded at the
// reuse early return. It also proves the write goes through the per-key
// SetTaskMetadataKey primitive rather than a full-row UpdateTask (the persisted
// deferred-launch value survives even though the firing's own metadata carries
// a conflicting value for that key — the skip branch is exercised, not
// vacuously true).
func TestRefreshAutomationContinuationMetadataOnResume(t *testing.T) {
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-1", Name: "Test", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          "continuation-task",
		WorkspaceID: "ws-1",
		Origin:      models.TaskOriginAutomationRun,
		Metadata: map[string]interface{}{
			models.MetaKeyAutomationTaskMode:       string(automation.TaskModeAutomationRun),
			models.MetaKeyAutomationRepositoryMode: string(automation.RepositoryModeNone),
			models.MetaKeyAutomationTargetTaskID:   "target-task-1",
			models.MetaKeyDeferredLaunch:           "must-survive",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "continuation-session", TaskID: "continuation-task",
		State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now,
	}))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	a := &automation.Automation{
		ID: "a-merged", WorkspaceID: "ws-1", ContinuationTaskID: "continuation-task",
		ContinuationPolicy: automation.ContinuationPolicyReuseThread,
		TaskMode:           automation.TaskModeAutomationRun,
		RepositoryMode:     automation.RepositoryModeNone,
	}
	evt := &automation.AutomationTriggeredEvent{
		TriggerType: automation.TriggerTypeGitHubPRMerged,
		TriggerData: json.RawMessage(`{"task_id":"target-task-2"}`),
	}
	metadata := map[string]interface{}{
		"trigger_id":                         "trg-2",
		"trigger_type":                       string(automation.TriggerTypeGitHubPRMerged),
		models.MetaKeyAutomationTargetTaskID: "target-task-2",
		// Deliberately present (with a different value than the task already
		// carries) so the skip branch in refreshAutomationContinuationMetadata
		// actually executes. Without this key in the input map, the assertion
		// below would pass even with no skip at all — a merge trivially
		// preserves a key the source map never mentions.
		models.MetaKeyDeferredLaunch: "attacker-supplied-value",
	}

	task, session, action, reason, err := svc.prepareAutomationTask(ctx, a, evt, "title", "prompt", metadata)
	require.NoError(t, err)
	require.Equal(t, automation.ThreadActionResumed, action, "reason=%q", reason)
	require.NotNil(t, session)
	require.Equal(t, "target-task-1", task.Metadata[models.MetaKeyAutomationTargetTaskID],
		"preparing a resumed task must not mutate the target before dispatch admission")
	require.NoError(t, svc.refreshAutomationContinuationMetadata(ctx, task, metadata))
	require.Equal(t, "target-task-2", task.Metadata[models.MetaKeyAutomationTargetTaskID],
		"the admitted firing must carry the SECOND firing's target, not the first")

	persisted, err := repo.GetTask(ctx, "continuation-task")
	require.NoError(t, err)
	require.Equal(t, "target-task-2", persisted.Metadata[models.MetaKeyAutomationTargetTaskID],
		"the refreshed binding must be persisted, or the guard reads the stale value back out on the next lookup")
	require.Equal(t, "must-survive", persisted.Metadata[models.MetaKeyDeferredLaunch],
		"deferred launch ownership is server-managed and must not be clobbered by a metadata refresh, even when the firing's own metadata map carries a value for it")
	require.Equal(t, "must-survive", task.Metadata[models.MetaKeyDeferredLaunch],
		"the in-memory task must also keep the original value, not the firing's attempted overwrite")
}

func TestPrepareAutomationTask_DefersRefreshUntilDispatchAdmission(t *testing.T) {
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-stop", Name: "Test", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          "continuation-stop-task",
		WorkspaceID: "ws-stop",
		Origin:      models.TaskOriginAutomationRun,
		Metadata: map[string]interface{}{
			models.MetaKeyAutomationTaskMode:       string(automation.TaskModeAutomationRun),
			models.MetaKeyAutomationRepositoryMode: string(automation.RepositoryModeNone),
			models.MetaKeyAutomationTargetTaskID:   "target-before-stop",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "continuation-stop-session", TaskID: "continuation-stop-task",
		State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now,
	}))

	autoSvc := &rejectingAutomationService{
		stubAutomationService: &stubAutomationService{automation: &automation.Automation{
			ID: "a-stop", WorkspaceID: "ws-stop", ContinuationTaskID: "continuation-stop-task",
			ContinuationPolicy: automation.ContinuationPolicyReuseThread,
			TaskMode:           automation.TaskModeAutomationRun,
			RepositoryMode:     automation.RepositoryModeNone,
		}},
		dispatchErr: errors.New("run stopped before dispatch"),
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)
	evt := &automation.AutomationTriggeredEvent{
		RunID:       "run-stop",
		TriggerType: automation.TriggerTypeGitHubPRMerged,
		TriggerData: json.RawMessage(`{"task_id":"target-after-stop"}`),
	}
	metadata := map[string]interface{}{
		"trigger_id":                         "trigger-stop",
		"trigger_type":                       string(automation.TriggerTypeGitHubPRMerged),
		models.MetaKeyAutomationTargetTaskID: "target-after-stop",
	}

	task, session, action, reason, err := svc.prepareAutomationTask(ctx, autoSvc.automation, evt, "title", "prompt", metadata)
	require.NoError(t, err)
	require.Equal(t, automation.ThreadActionResumed, action, "reason=%q", reason)
	require.NotNil(t, session)
	require.Equal(t, "target-before-stop", task.Metadata[models.MetaKeyAutomationTargetTaskID],
		"preparing a resumed task must not authorize a run before dispatch admission")

	svc.dispatchAutomationContinuation(ctx, autoSvc.automation, task, session, "prompt", metadata, evt.RunID, action, reason)
	persisted, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "target-before-stop", persisted.Metadata[models.MetaKeyAutomationTargetTaskID],
		"a run rejected before dispatch must not leave its target on the continuation task")
}

func TestRefreshAutomationContinuationMetadataPublishesTaskUpdated(t *testing.T) {
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws-publish", Name: "Test", CreatedAt: now, UpdatedAt: now,
	}))
	task := &models.Task{
		ID: "continuation-publish-task", WorkspaceID: "ws-publish",
		Origin: models.TaskOriginAutomationRun,
		Metadata: map[string]interface{}{
			models.MetaKeyAutomationTargetTaskID: "target-before-publish",
		},
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	publisher := &recordingTaskUpdatedPublisher{}
	svc.SetTaskEventPublisher(publisher)
	require.NoError(t, svc.refreshAutomationContinuationMetadata(ctx, task, map[string]interface{}{
		models.MetaKeyAutomationTargetTaskID: "target-after-publish",
	}))

	require.Equal(t, []string{task.ID}, publisher.updatedTaskIDs,
		"metadata changes must publish the refreshed task to live subscribers")
}

func TestOrderedAutomationContinuationMetadataKeysPutsTargetFirst(t *testing.T) {
	keys := orderedAutomationContinuationMetadataKeys(map[string]interface{}{
		"zeta":                               true,
		models.MetaKeyDeferredLaunch:         "must-survive",
		models.MetaKeyAutomationTargetTaskID: "target",
		"alpha":                              true,
	})

	require.Equal(t, []string{models.MetaKeyAutomationTargetTaskID, "alpha", "zeta"}, keys)
}

func TestRecordFailedRun_MergedPRDoesNotConsumeDedupKey(t *testing.T) {
	autoSvc := &stubAutomationService{}
	svc := &Service{automationService: autoSvc}
	svc.recordFailedRun(context.Background(), &automation.AutomationTriggeredEvent{
		AutomationID: "a-merged", TriggerID: "trg-merged", TriggerType: automation.TriggerTypeGitHubPRMerged,
		DedupKey: "pr_merged:task-1:acme/api#7", TriggerData: json.RawMessage(`{}`),
	}, "no repository available")

	require.Len(t, autoSvc.runs, 1)
	require.Empty(t, autoSvc.runs[0].DedupKey)
}

func TestRecordFailedRun_OtherTriggerKeepsDedupKey(t *testing.T) {
	autoSvc := &stubAutomationService{}
	svc := &Service{automationService: autoSvc}
	svc.recordFailedRun(context.Background(), &automation.AutomationTriggeredEvent{
		AutomationID: "a-scheduled", TriggerID: "trg-scheduled", TriggerType: automation.TriggerTypeScheduled,
		DedupKey: "scheduled:trg:1", TriggerData: json.RawMessage(`{}`),
	}, "no repository available")

	require.Len(t, autoSvc.runs, 1)
	require.Equal(t, "scheduled:trg:1", autoSvc.runs[0].DedupKey)
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

func TestPrepareAutomationTask_AllowsRepositoryFreeHiddenRun(t *testing.T) {
	repo := setupTestRepo(t)
	creator := &stubReviewTaskCreator{task: &models.Task{ID: "t-scratch"}}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.reviewTaskCreator = creator
	a := &automation.Automation{
		ID: "a-scratch", WorkspaceID: "ws-1", Name: "scratch", Enabled: true,
		TaskMode:       automation.TaskModeAutomationRun,
		RepositoryMode: automation.RepositoryModeNone,
	}

	task, session, action, _, err := svc.prepareAutomationTask(
		context.Background(), a,
		&automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeScheduled},
		"Scratch", "report", map[string]interface{}{},
	)

	require.NoError(t, err)
	require.NotNil(t, task)
	require.Nil(t, session)
	require.Equal(t, automation.ThreadActionCreated, action)
	require.Empty(t, creator.got.Repositories)
	require.Equal(t, models.TaskOriginAutomationRun, creator.got.Origin)
}

func TestPrepareAutomationTaskCreatesVisibleNormalTaskWithoutRepository(t *testing.T) {
	repo := setupTestRepo(t)
	creator := &stubReviewTaskCreator{task: &models.Task{ID: "t-visible"}}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.reviewTaskCreator = creator
	a := &automation.Automation{
		ID: "a-visible", WorkspaceID: "ws-1", Name: "visible", Enabled: true,
		TaskMode:       automation.TaskModeNormalTask,
		RepositoryMode: automation.RepositoryModeNone,
		WorkflowID:     "workflow-1",
		WorkflowStepID: "step-1",
	}

	task, _, _, _, err := svc.prepareAutomationTask(
		context.Background(), a,
		&automation.AutomationTriggeredEvent{TriggerType: automation.TriggerTypeScheduled},
		"Visible", "report", map[string]interface{}{},
	)

	require.NoError(t, err)
	require.NotNil(t, task)
	require.Empty(t, creator.got.Repositories)
	require.Equal(t, "automation_task", creator.got.Origin)
	require.Equal(t, "workflow-1", creator.got.WorkflowID)
	require.Equal(t, "step-1", creator.got.WorkflowStepID)
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
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-keep", TaskID: "t-keep", ExecutorType: "worktree",
		WorkspacePath: "/tmp", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "s-keep")
	require.NoError(t, err)
	session.TaskEnvironmentID = "env-keep"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		TaskEnvironmentID: "env-keep",
		RepositoryID:      "repo-1",
		WorktreeID:        "wt-keep",
		WorktreePath:      "/tmp/kandev/t-keep",
	}))

	mgr := &mockAgentManager{}
	autoSvc := &stubAutomationService{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), mgr)
	svc.SetAutomationService(autoSvc)

	session, err = repo.GetTaskSession(ctx, "s-keep")
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

// --- Run-worktree retention ---

// fakeWorktreeReaper stands in for worktree.Manager. Retention's job is
// deciding *which* workspaces go and leaving everything else alone, so git is
// not involved — but the directory and the worktree row are, because the
// contract under test is about what is actually on disk afterwards.
//
// Two production behaviours are modelled exactly, and getting either of them
// convenient instead of accurate would hand the sweep a pass it does not earn:
//
//   - the row is marked deleted (in memory *and* in the DB the candidate query
//     reads) as removeWorktree's ReleaseWorktreeReference does, and
//   - with swallowRemoval set, a failed directory removal returns *nil*, which
//     is what removeWorktree does today: it logs the failure at Warn, marks the
//     row deleted and falls through. A fake that returned an error there would
//     be testing a manager that does not exist.
type fakeWorktreeReaper struct {
	mu             sync.Mutex
	db             *sqlx.DB
	byTask         map[string][]*worktree.Worktree
	removed        []string
	branchRemovals []bool
	listErr        error
	removeErr      error
	swallowRemoval bool
}

func newFakeWorktreeReaper() *fakeWorktreeReaper {
	return &fakeWorktreeReaper{byTask: map[string][]*worktree.Worktree{}}
}

func (f *fakeWorktreeReaper) seed(taskID, path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byTask[taskID] = append(f.byTask[taskID], &worktree.Worktree{
		ID: "wt-" + taskID, TaskID: taskID, Path: path, Status: worktree.StatusActive,
	})
}

func (f *fakeWorktreeReaper) setSwallowRemoval(swallow bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.swallowRemoval = swallow
}

func (f *fakeWorktreeReaper) GetAllByTaskID(_ context.Context, taskID string) ([]*worktree.Worktree, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.byTask[taskID], nil
}

func (f *fakeWorktreeReaper) RemoveByID(_ context.Context, worktreeID string, removeBranch bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, worktreeID)
	f.branchRemovals = append(f.branchRemovals, removeBranch)
	for _, wts := range f.byTask {
		for _, wt := range wts {
			if wt.ID != worktreeID {
				continue
			}
			// Marked deleted whether or not the directory went away — that
			// unconditional release is the trap this fake exists to reproduce.
			wt.Status = worktree.StatusDeleted
			f.releaseRow(worktreeID)
			if !f.swallowRemoval && wt.Path != "" {
				_ = os.RemoveAll(wt.Path)
			}
		}
	}
	return nil
}

// releaseRow mirrors ReleaseWorktreeReference in the database the retention
// query reads, so a reclaimed run really does drop out of the candidate set
// instead of the test asserting that it would.
func (f *fakeWorktreeReaper) releaseRow(worktreeID string) {
	if f.db == nil {
		return
	}
	_, _ = f.db.Exec(
		`UPDATE task_environment_repos SET status = ?, deleted_at = ? WHERE worktree_id = ?`,
		worktree.StatusDeleted, time.Now().UTC(), worktreeID,
	)
}

func (f *fakeWorktreeReaper) survivingWorktrees() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var alive []string
	for _, wts := range f.byTask {
		for _, wt := range wts {
			if wt.Status != worktree.StatusDeleted {
				alive = append(alive, wt.ID)
			}
		}
	}
	return alive
}

// automationRetentionFixture wires the orchestrator against a *real* automation
// store sharing the task repository's database. The claim under test is that
// reclaiming a workspace leaves the run row and its task behind, and a stub
// automation service could only ever restate that claim rather than test it.
type automationRetentionFixture struct {
	svc           *Service
	repo          *sqliterepo.Repository
	autoStore     *automation.Store
	autoSvc       *automation.Service
	db            *sqlx.DB
	reaper        *fakeWorktreeReaper
	workspaceRoot string
}

func setupAutomationRetentionFixture(t *testing.T) *automationRetentionFixture {
	t.Helper()
	root := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(root, "retention.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	autoStore, err := automation.NewStore(sqlxDB, sqlxDB)
	require.NoError(t, err)

	reaper := newFakeWorktreeReaper()
	reaper.db = sqlxDB
	autoSvc := automation.NewService(autoStore, nil, testLogger())
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)
	svc.worktreeReaper = reaper

	return &automationRetentionFixture{
		svc: svc, repo: repo, autoStore: autoStore, autoSvc: autoSvc,
		db: sqlxDB, reaper: reaper, workspaceRoot: filepath.Join(root, "workspaces"),
	}
}

// workspacePath is where a run's checkout lives on disk. Retention's claim is
// about bytes, so the fixture puts real directories there and the assertions
// read them back.
func (f *automationRetentionFixture) workspacePath(taskID string) string {
	return filepath.Join(f.workspaceRoot, taskID)
}

// seedSessionWorktree writes the rows that make a task look like it holds a
// checkout: a session, a worktree row hanging off it, and the directory itself.
// The candidate query walks task → session → worktree, so a run without these
// is not a retention candidate at all.
func (f *automationRetentionFixture) seedSessionWorktree(t *testing.T, taskID string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(f.workspacePath(taskID), 0o750))
	now := time.Now().UTC()
	sessionID := taskID + "-session"
	_, err := f.db.Exec(
		`INSERT INTO task_sessions (id, task_id, state, is_primary, started_at, updated_at)
		 VALUES (?, ?, 'WAITING_FOR_INPUT', 1, ?, ?)`,
		sessionID, taskID, now, now)
	require.NoError(t, err)
	_, err = f.db.Exec(
		`INSERT INTO task_environments (id, task_id, executor_type, status, workspace_path, created_at, updated_at)
		 VALUES (?, ?, 'worktree', 'ready', ?, ?, ?)`,
		"env-"+taskID, taskID, f.workspacePath(taskID), now, now)
	require.NoError(t, err)
	_, err = f.db.Exec(
		`UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`,
		"env-"+taskID, sessionID)
	require.NoError(t, err)
	_, err = f.db.Exec(
		`INSERT INTO task_environment_repos
			(id, task_environment_id, worktree_id, repository_id, worktree_path, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'repo-1', ?, 'active', ?, ?)`,
		taskID+"-ter", "env-"+taskID, "wt-"+taskID, f.workspacePath(taskID), now, now)
	require.NoError(t, err)
}

// startSession puts a second session on a task in a live state — what replying
// to an aged-out run does.
func (f *automationRetentionFixture) startSession(t *testing.T, taskID, state string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := f.db.Exec(
		`INSERT INTO task_sessions (id, task_id, state, is_primary, started_at, updated_at)
		 VALUES (?, ?, ?, 0, ?, ?)`,
		taskID+"-reply", taskID, state, now, now)
	require.NoError(t, err)
}

// seedRunSeries writes one automation plus n runs, oldest first, each with its
// own task and workspace. Every run but the last is already terminal; the last
// is the one still at task_created that the finalization under test closes out.
func (f *automationRetentionFixture) seedRunSeries(t *testing.T, n int) (*automation.Automation, []string) {
	t.Helper()
	ctx := context.Background()
	a := &automation.Automation{WorkspaceID: "ws-retention", Name: "every five minutes", Enabled: true}
	require.NoError(t, f.autoStore.CreateAutomation(ctx, a))

	base := time.Now().UTC().Truncate(time.Second).Add(-time.Duration(n) * time.Hour)
	taskIDs := make([]string, 0, n)
	for i := range n {
		taskID := fmt.Sprintf("t-run-%02d", i)
		seedAutomationTask(t, f.repo, taskID, models.TaskOriginAutomationRun, false)
		f.seedSessionWorktree(t, taskID)
		f.reaper.seed(taskID, f.workspacePath(taskID))

		status := automation.RunStatusSucceeded
		if i == n-1 {
			status = automation.RunStatusTaskCreated
		}
		run := &automation.AutomationRun{
			AutomationID: a.ID, TriggerType: automation.TriggerTypeScheduled,
			TaskID: taskID, Status: status, TriggerData: json.RawMessage(`{}`),
		}
		require.NoError(t, f.autoStore.CreateRun(ctx, run))
		_, err := f.db.Exec(`UPDATE automation_runs SET created_at = ? WHERE id = ?`,
			base.Add(time.Duration(i)*time.Minute), run.ID)
		require.NoError(t, err)

		taskIDs = append(taskIDs, taskID)
	}
	return a, taskIDs
}

// The reported defect: every firing becomes a persistent task that deliberately
// retains its worktree, with no limit and no cleanup, so a frequent schedule
// accumulates full checkouts until the disk fills. Finalizing a run must
// reclaim the workspaces that just aged out of the retention window — and only
// those, so the recent runs a user goes back to are still openable.
func TestFinalizeAutomationRun_ReclaimsOnlyTheWorkspacesPastTheRetentionWindow(t *testing.T) {
	f := setupAutomationRetentionFixture(t)
	_, tasks := f.seedRunSeries(t, automation.DefaultRunWorktreeRetention+3)

	f.svc.finalizeAutomationRun(context.Background(), tasks[len(tasks)-1], true, "")

	require.ElementsMatch(t, []string{"wt-" + tasks[0], "wt-" + tasks[1], "wt-" + tasks[2]}, f.reaper.removed,
		"exactly the three runs beyond the newest %d must lose their workspace",
		automation.DefaultRunWorktreeRetention)

	var expectedSurvivors []string
	for _, taskID := range tasks[3:] {
		expectedSurvivors = append(expectedSurvivors, "wt-"+taskID)
	}
	require.ElementsMatch(t, expectedSurvivors, f.reaper.survivingWorktrees(),
		"every run inside the retention window keeps its workspace")

	for _, removedBranch := range f.reaper.branchRemovals {
		require.False(t, removedBranch,
			"only the checkout is reclaimed; the branch holds the commits the run produced")
	}
}

// Retention reclaims disk, not history. A run that lost its workspace must
// still be listable, still carry its task id, and still have a task row behind
// it — the transcript is the whole record of what a hidden automation run did.
func TestFinalizeAutomationRun_KeepsRunRowsAndTasksAfterReclaimingWorkspaces(t *testing.T) {
	ctx := context.Background()
	f := setupAutomationRetentionFixture(t)
	a, tasks := f.seedRunSeries(t, automation.DefaultRunWorktreeRetention+3)

	f.svc.finalizeAutomationRun(ctx, tasks[len(tasks)-1], true, "")
	require.Len(t, f.reaper.removed, 3, "precondition: the sweep actually reclaimed something")

	runs, err := f.autoStore.ListRuns(ctx, a.ID, 100)
	require.NoError(t, err)
	require.Len(t, runs, len(tasks), "no run row may be deleted by a workspace reclaim")

	byTaskID := map[string]*automation.AutomationRun{}
	for _, run := range runs {
		byTaskID[run.TaskID] = run
	}
	for _, taskID := range tasks[:3] {
		require.Contains(t, byTaskID, taskID, "a reclaimed run must still be reachable by its task id")
		require.Equal(t, automation.RunStatusSucceeded, byTaskID[taskID].Status,
			"reclaiming a workspace must not rewrite the run's recorded outcome")

		task, taskErr := f.repo.GetTask(ctx, taskID)
		require.NoError(t, taskErr)
		require.NotNil(t, task, "the task carrying the transcript must outlive its workspace")
	}
}

// A run stays inside the aged-out window for as long as its row lives, so
// every later firing looks at it again. Handing an already-reclaimed worktree
// back to git on every subsequent run would turn a bounded sweep into a
// permanent stream of failures.
func TestFinalizeAutomationRun_DoesNotReclaimTheSameWorkspaceTwice(t *testing.T) {
	ctx := context.Background()
	f := setupAutomationRetentionFixture(t)
	_, tasks := f.seedRunSeries(t, automation.DefaultRunWorktreeRetention+3)

	f.svc.finalizeAutomationRun(ctx, tasks[len(tasks)-1], true, "")
	require.Len(t, f.reaper.removed, 3)

	f.svc.finalizeAutomationRun(ctx, tasks[len(tasks)-1], true, "")
	require.Len(t, f.reaper.removed, 3, "an already-reclaimed workspace must not be removed again")
}

// The firing already happened and its outcome is already known. A reclaim that
// fails costs disk; a reclaim that propagates its failure would leave the run
// row disagreeing with what the agent actually did.
func TestFinalizeAutomationRun_SurvivesAWorkspaceReclaimFailure(t *testing.T) {
	ctx := context.Background()
	f := setupAutomationRetentionFixture(t)
	f.reaper.removeErr = errors.New("git worktree remove: permission denied")
	a, tasks := f.seedRunSeries(t, automation.DefaultRunWorktreeRetention+3)

	f.svc.finalizeAutomationRun(ctx, tasks[len(tasks)-1], true, "")

	runs, err := f.autoStore.ListRuns(ctx, a.ID, 100)
	require.NoError(t, err)
	var finalized *automation.AutomationRun
	for _, run := range runs {
		if run.TaskID == tasks[len(tasks)-1] {
			finalized = run
		}
	}
	require.NotNil(t, finalized)
	require.Equal(t, automation.RunStatusSucceeded, finalized.Status,
		"a failed workspace reclaim must not stop the run from reaching its terminal status")
	require.Empty(t, f.reaper.removed)
}

// Same contract one layer up: if retention cannot even work out what has aged
// out, the run still finalizes and frees its concurrency slot.
func TestMarkAutomationRunTerminal_SurvivesARetentionLookupFailure(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationTask(t, repo, "t-auto", models.TaskOriginAutomationRun, false)

	autoSvc := &stubAutomationService{prunableErr: errors.New("database is locked")}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)
	svc.worktreeReaper = newFakeWorktreeReaper()

	svc.finalizeAutomationRun(context.Background(), "t-auto", true, "")

	require.Equal(t, []string{"t-auto"}, autoSvc.succeeded,
		"a retention lookup failure must not stop the run from reaching its terminal status")
	require.Equal(t, automation.DefaultRunWorktreeRetention, autoSvc.prunableKeep,
		"the sweep must ask for the shipped retention default, not an inlined number")
}

// An install with no worktree manager wired (and every test that builds the
// service without one) keeps every workspace rather than panicking.
func TestMarkAutomationRunTerminal_SkipsRetentionWithoutAWorktreeManager(t *testing.T) {
	repo := setupTestRepo(t)
	seedAutomationTask(t, repo, "t-auto", models.TaskOriginAutomationRun, false)

	autoSvc := &stubAutomationService{prunable: []string{"t-old"}}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetAutomationService(autoSvc)
	svc.SetWorktreeManager(nil)

	svc.finalizeAutomationRun(context.Background(), "t-auto", true, "")

	require.Equal(t, []string{"t-auto"}, autoSvc.succeeded)
}

// racingRetention lets a test act inside the gap the defect lives in: after
// retention has named a run, before the sweep deletes its checkout. A fixture
// that only sets state up front cannot reach that gap at all — the selection
// query would simply never offer the run — so nothing static can stand in for
// this.
type racingRetention struct {
	*automation.Service
	onSelected func([]string)
}

func (r *racingRetention) PrunableRunTaskIDs(ctx context.Context, taskID string, keep int) ([]string, error) {
	selected, err := r.Service.PrunableRunTaskIDs(ctx, taskID, keep)
	if r.onSelected != nil {
		r.onSelected(selected)
	}
	return selected, err
}

// raceAfterSelection re-wires the service so hook runs between the sweep's
// selection and its first removal.
func (f *automationRetentionFixture) raceAfterSelection(hook func([]string)) {
	f.svc.SetAutomationService(&racingRetention{Service: f.autoSvc, onSelected: hook})
}

// observeLogs swaps in a logger whose entries the test can read back, for the
// claims retention only ever makes out loud.
func (f *automationRetentionFixture) observeLogs(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	f.svc.logger = log
	return logs
}

// The time-of-check/time-of-use defect. Retention decides what has aged out
// once, then removes checkouts one after another; a user replying to an
// aged-out run anywhere in that gap gets an agent working in a directory the
// sweep is already committed to deleting.
//
// The worktree manager does not save us here: removeWorktree counts active
// references with the worktree's *own* session excluded, and for an automation
// run that own session is exactly the one that just came alive. So the sweep
// has to ask again itself, immediately before each removal.
func TestFinalizeAutomationRun_KeepsTheWorkspaceOfARunThatGoesLiveAfterSelection(t *testing.T) {
	f := setupAutomationRetentionFixture(t)
	_, tasks := f.seedRunSeries(t, automation.DefaultRunWorktreeRetention+3)
	revived := tasks[0]

	var selection []string
	f.raceAfterSelection(func(selected []string) {
		selection = append([]string(nil), selected...)
		f.startSession(t, revived, "RUNNING")
	})

	f.svc.finalizeAutomationRun(context.Background(), tasks[len(tasks)-1], true, "")

	require.Contains(t, selection, revived,
		"precondition: the run was named as prunable before it went live")
	require.NotContains(t, f.reaper.removed, "wt-"+revived,
		"a run that went live between selection and removal must keep its checkout")
	require.DirExists(t, f.workspacePath(revived),
		"the live agent's working tree must still be there")
	require.ElementsMatch(t, []string{"wt-" + tasks[1], "wt-" + tasks[2]}, f.reaper.removed,
		"the runs that stayed idle are still reclaimed")
}

// A removal that frees nothing must not be reported as one. The worktree
// manager logs a failed directory removal at Warn, marks the row deleted and
// returns nil, so a sweep that trusts the nil error announces reclaimed disk
// that is still fully occupied — and the run then drops out of the candidate
// set for good. Stat-ing the path is the only honest confirmation available.
func TestFinalizeAutomationRun_DoesNotReportAReclaimWhoseDirectorySurvived(t *testing.T) {
	f := setupAutomationRetentionFixture(t)
	logs := f.observeLogs(t)
	f.reaper.setSwallowRemoval(true)
	_, tasks := f.seedRunSeries(t, automation.DefaultRunWorktreeRetention+3)

	f.svc.finalizeAutomationRun(context.Background(), tasks[len(tasks)-1], true, "")

	require.Len(t, f.reaper.removed, 3, "precondition: removal was attempted for all three")
	for _, taskID := range tasks[:3] {
		require.DirExists(t, f.workspacePath(taskID),
			"precondition: the manager swallowed the failure and left the directory behind")
	}
	require.Empty(t, logs.FilterMessage("reclaimed the workspace of an aged-out automation run").All(),
		"a removal that freed no disk must not be logged as a reclaim")
	require.Len(t, logs.FilterLevelExact(zapcore.ErrorLevel).All(), 3,
		"each surviving workspace must be surfaced as a failure, with its path")
}

// …and it must be retryable rather than terminal. By the time the failure is
// visible the manager has already marked the worktree row deleted, so the run
// no longer has a live checkout as far as the candidate query is concerned and
// no later sweep would ever look at it again. The retry queue is its only way
// back.
func TestFinalizeAutomationRun_RetriesAWorkspaceThatSurvivedItsRemoval(t *testing.T) {
	ctx := context.Background()
	f := setupAutomationRetentionFixture(t)
	f.reaper.setSwallowRemoval(true)
	_, tasks := f.seedRunSeries(t, automation.DefaultRunWorktreeRetention+3)

	f.svc.finalizeAutomationRun(ctx, tasks[len(tasks)-1], true, "")
	require.Len(t, f.reaper.removed, 3, "precondition: the first sweep tried and freed nothing")

	agedOut, err := f.autoSvc.PrunableRunTaskIDs(ctx, tasks[len(tasks)-1], automation.DefaultRunWorktreeRetention)
	require.NoError(t, err)
	require.Empty(t, agedOut,
		"precondition: the rows now say deleted, so retention alone can no longer see these runs")

	f.reaper.setSwallowRemoval(false)
	f.svc.finalizeAutomationRun(ctx, tasks[len(tasks)-1], true, "")

	require.Len(t, f.reaper.removed, 6, "each survivor must be handed back to the manager")
	for _, taskID := range tasks[:3] {
		require.NoDirExists(t, f.workspacePath(taskID), "the retry must actually free the disk")
	}
}
