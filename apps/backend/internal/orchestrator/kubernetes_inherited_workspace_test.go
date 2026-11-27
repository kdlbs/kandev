package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4
func TestKubernetesInheritedWorkspaceAdmission(t *testing.T) {
	for _, mode := range []string{"inherit_parent", "shared_group"} {
		for _, source := range []string{"parent_session", "workspace_group"} {
			t.Run(mode+"/"+source, func(t *testing.T) {
				svc, child := inheritedWorkspaceService(t, mode, source, models.ExecutorTypeKubernetes)
				ctx := context.Background()
				_, created, err := svc.prepareSessionForStart(ctx, child.ToAPI(), "agent-profile", "", "executor", "executor-profile", "")
				require.ErrorContains(t, err, "workspace reuse is unsupported")
				require.False(t, created)
				sessions, err := svc.repo.ListTaskSessions(ctx, child.ID)
				require.NoError(t, err)
				require.Empty(t, sessions)
				env, err := svc.repo.GetTaskEnvironment(ctx, "parent-env")
				require.NoError(t, err)
				require.Equal(t, "parent", env.TaskID)
				require.Equal(t, models.TaskEnvironmentStatusReady, env.Status)
			})
		}
	}
}

func TestKubernetesInheritedWorkspaceAdmissionSupportedControl(t *testing.T) {
	svc, child := inheritedWorkspaceService(t, "inherit_parent", "parent_session", models.ExecutorTypeLocalDocker)
	sessionID, created, err := svc.prepareSessionForStart(context.Background(), child.ToAPI(), "agent-profile", "", "executor", "executor-profile", "")
	require.NoError(t, err)
	require.True(t, created)
	session, err := svc.repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "parent-env", session.TaskEnvironmentID)
}

func TestKubernetesInheritedWorkspaceAdmissionEffectiveProfile(t *testing.T) {
	svc, child := inheritedWorkspaceService(t, "inherit_parent", "parent_session", models.ExecutorTypeKubernetes)
	_, created, err := svc.prepareSessionForStart(context.Background(), child.ToAPI(), "agent-profile", "", "", "", "")
	require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsupported)
	require.False(t, created)
	require.Equal(t, "executor-profile", child.Metadata["executor_profile_id"])
	require.NotContains(t, child.Metadata, "executor_id", "preflight must not mutate task metadata")
}

func TestKubernetesInheritedWorkspaceAdmissionStart(t *testing.T) {
	svc, child := inheritedWorkspaceService(t, "inherit_parent", "parent_session", models.ExecutorTypeKubernetes)
	_, err := svc.LaunchSession(context.Background(), &LaunchSessionRequest{TaskID: child.ID, Intent: IntentStart, AgentProfileID: "agent-profile"})
	require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsupported)
	sessions, err := svc.repo.ListTaskSessions(context.Background(), child.ID)
	require.NoError(t, err)
	require.Empty(t, sessions)
}

func TestKubernetesInheritedWorkspaceAdmissionArchivedGroupOwner(t *testing.T) {
	svc, child := inheritedWorkspaceService(t, "shared_group", "workspace_group", models.ExecutorTypeKubernetes)
	require.NoError(t, svc.repo.(*sqliterepo.Repository).ArchiveTask(context.Background(), "parent"))
	_, _, err := svc.prepareSessionForStart(context.Background(), child.ToAPI(), "agent-profile", "", "executor", "executor-profile", "")
	require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsafe)
}

func TestKubernetesInheritedWorkspaceAdmissionLateParent(t *testing.T) {
	for _, executorType := range []models.ExecutorType{models.ExecutorTypeKubernetes, models.ExecutorTypeLocalDocker} {
		t.Run(string(executorType), func(t *testing.T) {
			svc, child := inheritedWorkspaceService(t, "inherit_parent", "workspace_group", executorType)
			svc.SetWorkspaceMaterializer(&lateInheritedWorkspaceMaterializer{
				stubMaterializer: stubMaterializer{envByTask: map[string]string{child.ID: "parent-env"}},
			})
			ctx := context.Background()
			before, err := svc.repo.GetTaskEnvironment(ctx, "parent-env")
			require.NoError(t, err)
			sessionID, created, err := svc.prepareSessionForStart(ctx, child.ToAPI(), "agent-profile", "", "executor", "executor-profile", "")
			if executorType == models.ExecutorTypeKubernetes {
				require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsupported)
				require.False(t, created)
				require.Empty(t, sessionID)
				sessions, listErr := svc.repo.ListTaskSessions(ctx, child.ID)
				require.NoError(t, listErr)
				require.Empty(t, sessions)
			} else {
				require.NoError(t, err)
				require.True(t, created)
				session, readErr := svc.repo.GetTaskSession(ctx, sessionID)
				require.NoError(t, readErr)
				require.Equal(t, "parent-env", session.TaskEnvironmentID)
			}
			after, err := svc.repo.GetTaskEnvironment(ctx, "parent-env")
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

type lateInheritedWorkspaceMaterializer struct {
	stubMaterializer
	reads int
}

func (m *lateInheritedWorkspaceMaterializer) GetSharedGroupEnvironment(ctx context.Context, taskID string) string {
	m.reads++
	if m.reads == 1 {
		return ""
	}
	return m.stubMaterializer.GetSharedGroupEnvironment(ctx, taskID)
}

func inheritedWorkspaceService(t *testing.T, mode, source string, executorType models.ExecutorType) (*Service, *models.Task) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	tasks := newMockTaskRepo()
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), tasks, &mockAgentManager{})
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow", WorkspaceID: "workspace", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	parent := &models.Task{ID: "parent", WorkspaceID: "workspace", WorkflowID: "workflow", Title: "Parent", CreatedAt: now, UpdatedAt: now}
	child := &models.Task{ID: "child", ParentID: parent.ID, WorkspaceID: "workspace", WorkflowID: "workflow", Title: "Child",
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": mode}, "agent_profile_id": "agent-profile", "executor_profile_id": "executor-profile"}, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateTask(ctx, parent))
	require.NoError(t, repo.CreateTask(ctx, child))
	tasks.tasks[child.ID] = child.ToAPI()
	require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{ID: "executor", Name: "Test", Type: executorType}))
	require.NoError(t, repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{ID: "executor-profile", ExecutorID: "executor", Name: "Test"}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "parent-env", TaskID: parent.ID,
		ExecutorType: string(executorType), ExecutorID: "executor", ExecutorProfileID: "executor-profile", Status: models.TaskEnvironmentStatusReady}))
	if source == "parent_session" {
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent-session", TaskID: parent.ID,
			AgentProfileID: "agent-profile", ExecutorID: "executor", ExecutorProfileID: "executor-profile", IsPrimary: true,
			TaskEnvironmentID: "parent-env", State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}))
	}
	svc.SetWorkspaceMaterializer(&stubMaterializer{envByTask: map[string]string{child.ID: "parent-env"}})
	return svc, child
}
