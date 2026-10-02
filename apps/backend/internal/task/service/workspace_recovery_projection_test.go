package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceRecoveryProjectsRelocationErrorFromEveryEntryPoint(t *testing.T) {
	for _, test := range []struct {
		name     string
		initial  *models.LastAgentError
		expected string
	}{
		{name: "legacy generic error"},
		{
			name: "modern generic error",
			initial: &models.LastAgentError{
				Message: "workspace could not be restored", OccurredAt: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), StampValue: "generic-error-stamp",
			},
			expected: "generic-error-stamp",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, eventBus, repo := createTestService(t)
			ctx := context.Background()
			fixture := newWorkspaceRecoveryProjectionFixture(t)
			taskID, sessionID, environmentID := "task-workspace-recovery-projection", "session-workspace-recovery-projection", "environment-workspace-recovery-projection"
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-recovery-projection", Name: "Recovery projection"}))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: "workspace-recovery-projection", Title: "Recovery projection"}))
			require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
				ID: "repository-workspace-recovery-projection", WorkspaceID: "workspace-recovery-projection",
				Name: "widget", LocalPath: fixture.destinationClone, DefaultBranch: "main",
				Provider: "github", ProviderHost: "github.com", ProviderOwner: "acme", ProviderName: "widget",
			}))
			require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
				ID: "task-repository-workspace-recovery-projection", TaskID: taskID,
				RepositoryID: "repository-workspace-recovery-projection", BaseBranch: "main", CheckoutBranch: fixture.branch,
			}))
			require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
				ID: environmentID, TaskID: taskID, OwnershipGeneration: 7,
				ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
				WorkspacePath: fixture.worktreePath, TaskDirName: fixture.taskDirName,
				Repos: []*models.TaskEnvironmentRepo{{
					ID: "environment-repository-workspace-recovery-projection", TaskEnvironmentID: environmentID,
					RepositoryID: "repository-workspace-recovery-projection", BranchSlug: "feature-recovery",
					WorktreeID: fixture.worktreeID, WorktreePath: fixture.worktreePath, WorktreeBranch: fixture.branch,
					WorktreeSourceClonePath: fixture.sourceClone, WorktreeSourceCommonDir: filepath.Join(fixture.sourceClone, ".git"),
					Status: "active", Position: 0,
				}},
			}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
				State: models.TaskSessionStateCancelled, ErrorMessage: "legacy workspace restore failure",
				ExecutorID: models.ExecutorIDWorktree, RepositoryID: "repository-workspace-recovery-projection",
				WorkspacePath: fixture.worktreePath,
				Metadata: map[string]interface{}{
					"provider": map[string]interface{}{"resume_token": "preserve-provider"},
					"retained": "preserve-metadata",
				},
			}))
			if test.initial != nil {
				require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, *test.initial))
			}
			svc.SetRepoCloneLocation(workspaceRecoveryProjectionCloneLocation{root: fixture.managedRoot, source: fixture.sourceClone, destination: fixture.destinationClone})
			info, err := svc.GetWorkspaceInfoForSession(ctx, taskID, sessionID)
			require.NoError(t, err)
			require.NotNil(t, info.RecoveryErrorObservation)
			require.True(t, info.RecoveryErrorObservation.SelectionSnapshot.Valid())
			require.Len(t, info.RecoveryErrorObservation.SelectionSnapshot.Slots, 1)
			observation := *info.RecoveryErrorObservation
			require.Equal(t, test.expected, observation.ExpectedErrorStamp)

			store := &workspaceRecoveryProjectionWorktreeStore{worktree: &worktree.Worktree{
				ID: fixture.worktreeID, TaskID: taskID, TaskEnvironmentID: environmentID,
				TaskDirName: fixture.taskDirName, RepositoryID: "repository-workspace-recovery-projection",
				BranchSlug: "feature-recovery", Path: fixture.worktreePath, RepositoryPath: fixture.destinationClone,
				Branch: fixture.branch, BaseBranch: "main", Status: worktree.StatusActive,
			}, selectionSnapshot: observation.SelectionSnapshot}
			worktreeManager, err := worktree.NewManager(worktree.Config{
				Enabled: true, TasksBasePath: fixture.tasksBasePath, BranchPrefix: "feature/",
			}, store, logger.Default())
			require.NoError(t, err)
			lifecycleManager := lifecycle.NewManager(nil, eventBus, nil, nil, nil, nil,
				lifecycle.ExecutorFallbackWarn, t.TempDir(), logger.Default())
			lifecycleManager.SetWorkspaceInfoProvider(svc)
			lifecycleManager.SetWorkspaceRecoveryErrorReporter(svc)
			lifecycleManager.SetWorktreeManager(worktreeManager)

			_, err = lifecycleManager.EnsureWorkspaceExecutionForSession(ctx, taskID, sessionID)
			var projectionErr *lifecycle.WorkspaceRecoveryProjectionError
			require.ErrorAs(t, err, &projectionErr, "lifecycle reconstruction must project the verified refusal")
			require.NotEmpty(t, projectionErr.Stamp)
			firstStamp := projectionErr.Stamp

			manualRequest := worktree.RecoveryAdmissionRequest{
				TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
				OwnerTaskID: taskID, OwnershipGeneration: 7, ExecutorType: string(models.ExecutorTypeWorktree),
				SelectionSnapshot: observation.SelectionSnapshot,
				Slots: []worktree.RecoverySlot{{
					WorktreeID: fixture.worktreeID, RepositoryID: "repository-workspace-recovery-projection",
					BranchSlug: "feature-recovery", RepositoryPath: fixture.destinationClone,
					CloneRelocation: info.WorkspaceRepositories[0].CloneRelocation,
				}},
			}
			_, err = worktreeManager.AdmitRecovery(ctx, manualRequest)
			var relocationRequired *worktree.ManagedCloneRelocationRequiredError
			require.ErrorAs(t, err, &relocationRequired, "manual preflight must use the real worktree manager")
			manualStamp, err := svc.ReportManagedCloneRelocationRequired(ctx, observation)
			require.NoError(t, err)
			require.Equal(t, firstStamp, manualStamp)

			_, err = lifecycleManager.EnsureWorkspaceExecutionForSession(ctx, taskID, sessionID)
			require.ErrorAs(t, err, &projectionErr, "explicit restore must report the same current projection")
			require.Equal(t, firstStamp, projectionErr.Stamp)

			stored, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateCancelled, stored.State)
			require.Equal(t, "legacy workspace restore failure", stored.ErrorMessage)
			require.Equal(t, "preserve-metadata", stored.Metadata["retained"])
			require.Equal(t, map[string]interface{}{"resume_token": "preserve-provider"}, stored.Metadata["provider"])
			lastErr, ok := models.LoadLastAgentError(stored.Metadata)
			require.True(t, ok)
			require.Equal(t, models.LaunchErrorCategoryManagedCloneRelocationRequired, lastErr.Code)
			require.Equal(t, models.RecoveryActionRelocateAndResume, lastErr.RecoveryActions[0])
			require.Equal(t, firstStamp, lastErr.Stamp())

			published := eventBus.GetPublishedEvents()
			var errorEvents int
			for _, event := range published {
				if event.Type == events.TaskSessionErrorChanged {
					errorEvents++
				}
			}
			require.Equal(t, 1, errorEvents, "repeated detections must publish one durable error change")
		})
	}
}

type workspaceRecoveryProjectionFixture struct {
	managedRoot      string
	sourceClone      string
	destinationClone string
	tasksBasePath    string
	taskDirName      string
	worktreePath     string
	worktreeID       string
	branch           string
}

func newWorkspaceRecoveryProjectionFixture(t *testing.T) workspaceRecoveryProjectionFixture {
	t.Helper()
	base := t.TempDir()
	fixture := workspaceRecoveryProjectionFixture{
		managedRoot:   filepath.Join(base, "repos"),
		tasksBasePath: filepath.Join(base, "tasks"),
		taskDirName:   "workspace-recovery-projection-root", worktreeID: "worktree-workspace-recovery-projection",
		branch: "feature/recovery",
	}
	fixture.sourceClone = filepath.Join(fixture.managedRoot, "_providers", "github", "github.com", "acme", "widget")
	fixture.destinationClone = filepath.Join(fixture.managedRoot, "workspaces", "workspace-recovery-projection", "github", "acme", "widget")
	fixture.worktreePath = filepath.Join(fixture.tasksBasePath, fixture.taskDirName, "widget")
	seed := filepath.Join(base, "seed")
	require.NoError(t, os.MkdirAll(seed, 0o755))
	runWorkspaceRecoveryProjectionGit(t, seed, "init", "-b", "main")
	runWorkspaceRecoveryProjectionGit(t, seed, "config", "user.email", "test@example.com")
	runWorkspaceRecoveryProjectionGit(t, seed, "config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(seed, "README.md"), []byte("original\n"), 0o644))
	runWorkspaceRecoveryProjectionGit(t, seed, "add", "README.md")
	runWorkspaceRecoveryProjectionGit(t, seed, "commit", "-m", "initial")
	for _, clone := range []string{fixture.sourceClone, fixture.destinationClone} {
		require.NoError(t, os.MkdirAll(filepath.Dir(clone), 0o755))
		runWorkspaceRecoveryProjectionGit(t, seed, "clone", "--no-hardlinks", seed, clone)
		runWorkspaceRecoveryProjectionGit(t, clone, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
	}
	runWorkspaceRecoveryProjectionGit(t, fixture.sourceClone, "branch", fixture.branch)
	require.NoError(t, os.MkdirAll(filepath.Dir(fixture.worktreePath), 0o755))
	runWorkspaceRecoveryProjectionGit(t, fixture.sourceClone, "worktree", "add", fixture.worktreePath, fixture.branch)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.worktreePath, "README.md"), []byte("preserve local edits\n"), 0o644))
	taskRoot := filepath.Join(fixture.tasksBasePath, fixture.taskDirName)
	require.NoError(t, workspaces.WriteOwnershipMarker(taskRoot, workspaces.OwnershipMarker{
		TaskID: "task-workspace-recovery-projection", TaskDirName: fixture.taskDirName,
		LayoutVersion: workspaces.LayoutVersionSemantic,
	}))
	return fixture
}

func runWorkspaceRecoveryProjectionGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), output)
}

type workspaceRecoveryProjectionCloneLocation struct {
	root, source, destination string
}

func (c workspaceRecoveryProjectionCloneLocation) ExpandedBasePath() (string, error) {
	return c.root, nil
}

func (c workspaceRecoveryProjectionCloneLocation) ManagedCloneRelocationPaths(
	*models.Repository,
) (string, string, string, string, bool, error) {
	return c.root, c.source, "", c.destination, true, nil
}

type workspaceRecoveryProjectionWorktreeStore struct {
	worktree.Store
	worktree          *worktree.Worktree
	selectionSnapshot models.WorkspaceRecoverySelectionSnapshot
}

func (s *workspaceRecoveryProjectionWorktreeStore) GetWorktreeByID(_ context.Context, id string) (*worktree.Worktree, error) {
	if s.worktree == nil || s.worktree.ID != id {
		return nil, fmt.Errorf("selected worktree not found")
	}
	copy := *s.worktree
	return &copy, nil
}

func (s *workspaceRecoveryProjectionWorktreeStore) ReadRecoverySelectionSnapshot(
	_ context.Context,
	_ models.WorkspaceRecoverySelectionSnapshot,
) (models.WorkspaceRecoverySelectionSnapshot, error) {
	return s.selectionSnapshot.Canonical(), nil
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-002.4, AC-TASKS-MANAGED-CLONE-RELOCATION-003.1
func TestWorkspaceRecoveryProjectionRejectsInventoryChangedAfterInspection(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	const (
		workspaceID   = "workspace-recovery-stale-inventory"
		taskID        = "task-recovery-stale-inventory"
		sessionID     = "session-recovery-stale-inventory"
		environmentID = "environment-recovery-stale-inventory"
		repositoryID  = "repository-recovery-stale-inventory"
		slotID        = "environment-repository-recovery-stale-inventory"
	)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Recovery inventory"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Recovery inventory"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: repositoryID, WorkspaceID: workspaceID, Name: "widget", SourceType: "github",
		LocalPath: "/managed/widget", Provider: "github", ProviderHost: "github.com",
		ProviderOwner: "acme", ProviderName: "widget",
	}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, OwnershipGeneration: 9,
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/tasks/recovery-stale-inventory",
	}))
	slot := &models.TaskEnvironmentRepo{
		ID: slotID, TaskEnvironmentID: environmentID, RepositoryID: repositoryID,
		BranchSlug: "main", WorktreeID: "worktree-before", WorktreePath: "/tasks/before",
		WorktreeBranch: "feature/before", WorktreeSourceClonePath: "/managed/old-widget",
		WorktreeSourceCommonDir: "/managed/old-widget/.git", Status: "active",
	}
	require.NoError(t, repo.CreateTaskEnvironmentRepo(ctx, slot))
	genericError := models.LastAgentError{
		Message: "workspace restore failed", OccurredAt: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
		StampValue: "generic-workspace-error",
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
		State: models.TaskSessionStateCancelled, ErrorMessage: "legacy workspace error",
	}))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, genericError))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	environment, err := repo.GetTaskEnvironment(ctx, environmentID)
	require.NoError(t, err)
	selectionSnapshot, err := svc.workspaceRecoverySelectionSnapshot(ctx, session, environment)
	require.NoError(t, err)
	observation := models.WorkspaceRecoveryErrorObservation{
		TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
		EnvironmentOwnerTaskID: taskID, OwnershipGeneration: 9,
		SelectionSnapshot: selectionSnapshot,
		SessionState:      models.TaskSessionStateCancelled, ExpectedErrorStamp: genericError.Stamp(),
	}

	slot.WorktreeID = "worktree-after"
	slot.WorktreePath = "/tasks/after"
	slot.WorktreeBranch = "feature/after"
	require.NoError(t, repo.UpdateTaskEnvironmentRepo(ctx, slot))
	stamp, err := svc.ReportManagedCloneRelocationRequired(ctx, observation)
	require.NoError(t, err)
	require.Empty(t, stamp)

	stored, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, stored.State)
	require.Equal(t, "legacy workspace error", stored.ErrorMessage)
	lastError, ok := models.LoadLastAgentError(stored.Metadata)
	require.True(t, ok)
	require.Equal(t, genericError.Stamp(), lastError.Stamp())
	for _, event := range eventBus.GetPublishedEvents() {
		require.NotEqual(t, events.TaskSessionErrorChanged, event.Type, "stale inventory must not publish an actionable error")
	}
}
