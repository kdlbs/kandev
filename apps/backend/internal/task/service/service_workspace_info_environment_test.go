package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestApplyTaskEnvironmentToWorkspaceInfoProjectsDockerControlHandle(t *testing.T) {
	info := &lifecycle.WorkspaceInfo{}
	applyTaskEnvironmentToWorkspaceInfo(info, &models.TaskEnvironment{
		ID:                                "environment-1",
		ContainerID:                       "container-1",
		ContainerControlAuthTokenSecretID: "container-control-secret",
	})

	if got := info.Metadata[lifecycle.MetadataKeyContainerControlAuthSecret]; got != "container-control-secret" {
		t.Fatalf("container control handle = %q, want environment control handle", got)
	}
}

func TestApplyTaskEnvironmentToWorkspaceInfoProjectsEnvironmentAttachmentHandles(t *testing.T) {
	tests := []struct {
		name string
		env  *models.TaskEnvironment
		key  string
		want string
	}{
		{
			name: "Docker bootstrap nonce",
			env: &models.TaskEnvironment{
				ContainerID:                     "container-1",
				ContainerBootstrapNonceSecretID: "bootstrap-secret",
			},
			key:  lifecycle.MetadataKeyBootstrapNonceSecret,
			want: "bootstrap-secret",
		},
		{
			name: "SSH canonical task directory",
			env: &models.TaskEnvironment{
				ExecutorType:  string(models.ExecutorTypeSSH),
				WorkspacePath: "/remote/tasks/task-1",
			},
			key:  lifecycle.MetadataKeySSHRemoteTaskDir,
			want: "/remote/tasks/task-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &lifecycle.WorkspaceInfo{}
			applyTaskEnvironmentToWorkspaceInfo(info, tt.env)

			if got := info.Metadata[tt.key]; got != tt.want {
				t.Fatalf("metadata[%q] = %q, want environment attachment handle", tt.key, got)
			}
		})
	}
}

func TestApplyTaskEnvironmentToWorkspaceInfoReplacesRelativeSessionPath(t *testing.T) {
	canonicalWorkspacePath := t.TempDir()
	info := &lifecycle.WorkspaceInfo{WorkspacePath: "."}
	applyTaskEnvironmentToWorkspaceInfo(info, &models.TaskEnvironment{
		ID:            "environment-1",
		WorkspacePath: canonicalWorkspacePath,
	})
	if info.WorkspacePath != canonicalWorkspacePath {
		t.Fatalf("WorkspacePath = %q, want canonical environment path %q", info.WorkspacePath, canonicalWorkspacePath)
	}

	worktreePath := t.TempDir()
	info.WorkspacePath = worktreePath
	applyTaskEnvironmentToWorkspaceInfo(info, &models.TaskEnvironment{
		ID:            "environment-1",
		WorkspacePath: canonicalWorkspacePath,
	})
	if info.WorkspacePath != worktreePath {
		t.Fatalf("WorkspacePath = %q, want selected absolute worktree path %q", info.WorkspacePath, worktreePath)
	}
}

func TestApplyTaskEnvironmentToWorkspaceInfoUsesEffectiveParentWorkspaceRoot(t *testing.T) {
	info := &lifecycle.WorkspaceInfo{WorkspacePath: "/tasks/task-1/repository"}
	applyTaskEnvironmentToWorkspaceInfo(info, &models.TaskEnvironment{
		WorkspacePath:   "/tasks/task-1",
		WorkspaceLayout: "",
		TaskDirName:     "task-1",
		Repos: []*models.TaskEnvironmentRepo{{
			WorktreePath: "/tasks/task-1/repository",
		}},
	})

	if info.WorkspaceLayout != WorkspaceLayoutTaskRoot {
		t.Fatalf("effective workspace layout = %q, want task_root", info.WorkspaceLayout)
	}
	if info.WorkspacePath != "/tasks/task-1" {
		t.Fatalf("workspace path = %q, want parent task workspace", info.WorkspacePath)
	}
}

type retainedLocalProjectionFixture struct {
	service        *Service
	repo           *sqliterepo.Repository
	child          *models.Task
	session        *models.TaskSession
	environment    *models.TaskEnvironment
	source         *models.Repository
	parentAttached bool
	parentArchived bool
}

func newRetainedLocalProjectionFixture(t *testing.T, executorType string) *retainedLocalProjectionFixture {
	t.Helper()
	service, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	child := &models.Task{
		ID: "r3-projection-child", ParentID: "task-123", WorkspaceID: "ws-1",
		WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Retained local child", Priority: "medium",
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}
	source := &models.Repository{
		ID: "r3-projection-source", WorkspaceID: "ws-1", Name: "Historical local",
		SourceType: "local", LocalPath: t.TempDir(), DefaultBranch: "main",
	}
	session := &models.TaskSession{
		ID: "r3-projection-session", TaskID: child.ID, RepositoryID: source.ID,
		TaskEnvironmentID: "r3-projection-env", QueueIncarnationID: "r3-projection-incarnation",
		State: models.TaskSessionStateWaitingForInput, AgentProfileID: "profile-1",
		AgentProfileSnapshot: map[string]interface{}{"agent_name": "auggie"},
		Metadata:             map[string]interface{}{"acp_session_id": "r3-projection-native"},
		StartedAt:            time.Now().UTC(),
	}
	env := &models.TaskEnvironment{
		ID: session.TaskEnvironmentID, TaskID: "task-123", ExecutorType: executorType,
		WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "r3-projection-inventory", TaskEnvironmentID: session.TaskEnvironmentID,
			RepositoryID: source.ID, Status: "deleted", CreatedAt: time.Now().UTC(),
		}},
	}
	require.NoError(t, repo.CreateTask(ctx, child))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "r3-projection-foreign", WorkspaceID: "ws-1", WorkflowID: "wf-123",
		WorkflowStepID: "step-123", Title: "Other owner", Priority: "medium",
	}))
	require.NoError(t, repo.CreateRepository(ctx, source))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "r3-projection-link", TaskID: child.ID, RepositoryID: source.ID, BaseBranch: "main",
	}))
	return &retainedLocalProjectionFixture{service, repo, child, session, env, source, false, false}
}

func (f *retainedLocalProjectionFixture) persist(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if f.parentAttached {
		require.NoError(t, f.repo.CreateTaskRepository(ctx, &models.TaskRepository{
			ID: "r3-projection-parent-link", TaskID: "task-123", RepositoryID: f.source.ID, BaseBranch: "main",
		}))
	}
	require.NoError(t, f.repo.CreateTaskEnvironment(ctx, f.environment))
	require.NoError(t, f.repo.CreateTaskSession(ctx, f.session))
	if f.parentArchived {
		_, err := f.repo.DB().ExecContext(ctx, "UPDATE tasks SET archived_at = ? WHERE id = ?", time.Now().UTC(), "task-123")
		require.NoError(t, err)
	}
	env, err := f.repo.GetTaskEnvironment(ctx, f.environment.ID)
	require.NoError(t, err)
	require.NotEmpty(t, env.Repos, "fixture must contain persisted historical inventory")
	require.Equal(t, f.environment.Repos[0].Status, env.Repos[0].Status)
}

func (f *retainedLocalProjectionFixture) requireHistoryUnchanged(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	session, err := f.repo.GetTaskSession(ctx, f.session.ID)
	require.NoError(t, err)
	require.Equal(t, f.source.ID, session.RepositoryID)
	require.Equal(t, f.environment.ID, session.TaskEnvironmentID)
	require.Equal(t, "r3-projection-native", session.Metadata["acp_session_id"])
	attached, err := f.repo.ListTaskRepositories(ctx, f.child.ID)
	require.NoError(t, err)
	require.Len(t, attached, 1)
	require.Equal(t, f.source.ID, attached[0].RepositoryID)
	env, err := f.repo.GetTaskEnvironment(ctx, f.environment.ID)
	require.NoError(t, err)
	require.Equal(t, f.environment.TaskID, env.TaskID)
	require.Equal(t, f.environment.Repos[0].Status, env.Repos[0].Status)
	require.Equal(t, f.environment.Repos[0].WorktreePath, env.Repos[0].WorktreePath)
	require.Equal(t, f.environment.Repos[0].WorktreeSourceClonePath, env.Repos[0].WorktreeSourceClonePath)
	require.Equal(t, f.environment.Repos[0].WorktreeSourceCommonDir, env.Repos[0].WorktreeSourceCommonDir)
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.2
func TestRetainedLocalColdWorkspaceProjectionUsesInheritedContext(t *testing.T) {
	for _, executorType := range []string{string(models.ExecutorTypeLocal), "local_pc"} {
		t.Run(executorType, func(t *testing.T) {
			f := newRetainedLocalProjectionFixture(t, executorType)
			f.persist(t)
			info, err := f.service.GetWorkspaceInfoForSession(context.Background(), f.child.ID, f.session.ID)
			require.NoError(t, err)
			require.Equal(t, f.environment.WorkspacePath, info.WorkspacePath)
			require.Empty(t, info.WorkspaceRepositories, "deleted historical source must not be replayed as physical workspace inventory")
			require.Equal(t, f.environment.TaskID, info.EnvironmentOwnerTaskID)
			require.Equal(t, f.environment.ID, info.ValidatedTaskEnvironmentID)
			require.Equal(t, "r3-projection-native", info.ACPSessionID)
			f.requireHistoryUnchanged(t)
		})
	}
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.2
func TestRetainedLocalColdProjectionPreservesOtherContexts(t *testing.T) {
	cases := []struct {
		name   string
		change func(*retainedLocalProjectionFixture)
	}{
		{"foreign_parent", func(f *retainedLocalProjectionFixture) { f.environment.TaskID = "r3-projection-foreign" }},
		{"archived_parent", func(f *retainedLocalProjectionFixture) { f.parentArchived = true }},
		{"worktree_executor", func(f *retainedLocalProjectionFixture) {
			f.environment.ExecutorType = string(models.ExecutorTypeWorktree)
		}},
		{"multiple_inventory_rows", func(f *retainedLocalProjectionFixture) {
			second := *f.environment.Repos[0]
			second.ID = "r3-projection-second-inventory"
			second.BranchSlug = "second-slot"
			second.WorkspaceRelativePath = "second"
			f.environment.Repos = append(f.environment.Repos, &second)
		}},
		{"parent_attached_source", func(f *retainedLocalProjectionFixture) { f.parentAttached = true }},
		{"active_inventory", func(f *retainedLocalProjectionFixture) { f.environment.Repos[0].Status = "ready" }},
		{"physical_worktree_id", func(f *retainedLocalProjectionFixture) { f.environment.Repos[0].WorktreeID = "materialized" }},
		{"physical_worktree_path", func(f *retainedLocalProjectionFixture) { f.environment.Repos[0].WorktreePath = f.source.LocalPath }},
		{"physical_clone", func(f *retainedLocalProjectionFixture) {
			f.environment.Repos[0].WorktreeSourceClonePath = f.source.LocalPath
		}},
		{"physical_common_dir", func(f *retainedLocalProjectionFixture) {
			f.environment.Repos[0].WorktreeSourceCommonDir = f.source.LocalPath
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRetainedLocalProjectionFixture(t, string(models.ExecutorTypeLocal))
			tc.change(f)
			f.persist(t)
			info, err := f.service.GetWorkspaceInfoForSession(context.Background(), f.child.ID, f.session.ID)
			if err == nil {
				require.NotEmpty(t, info.WorkspaceRepositories, "nonmatching context must retain normal inventory projection")
			}
			f.requireHistoryUnchanged(t)
		})
	}
}
