package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-EXECUTORS-KUBERNETES-TASK-POD-001.6
func TestKubernetesInheritedWorkspaceLegacyResume(t *testing.T) {
	for _, mode := range []string{"inherit_parent", "shared_group", ""} {
		t.Run(mode, func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			repo.tasks["parent"] = &models.Task{ID: "parent"}
			env := &models.TaskEnvironment{ID: "parent-env", TaskID: "parent", ExecutorType: "k8s", Status: models.TaskEnvironmentStatusReady}
			repo.taskEnvironments[env.ID] = env
			task := &v1.Task{ID: "child", Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": mode}}}
			session := &models.TaskSession{ID: "child-session", TaskID: task.ID, TaskEnvironmentID: env.ID}
			_, _, _, err := exec.prepareResumeRepositorySettings(context.Background(), task, session, &LaunchAgentRequest{TaskID: task.ID, ExecutorType: "k8s"}, ResumeOptions{})
			require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsupported)
			require.Equal(t, env.ID, session.TaskEnvironmentID)
			require.Empty(t, repo.writeCallLog)
		})
	}
}

func TestKubernetesInheritedWorkspaceLegacyLaunch(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["parent"] = &models.Task{ID: "parent"}
	env := &models.TaskEnvironment{ID: "parent-env", TaskID: "parent", ExecutorType: "k8s", Status: models.TaskEnvironmentStatusReady}
	repo.taskEnvironments[env.ID] = env
	repo.executors["kube"] = &models.Executor{ID: "kube", Type: models.ExecutorTypeKubernetes, Status: models.ExecutorStatusActive}
	session := &models.TaskSession{ID: "legacy-child", TaskID: "child", AgentProfileID: "profile-123", TaskEnvironmentID: env.ID, State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now()}
	repo.sessions[session.ID] = session
	launched := false
	exec := newTestExecutor(t, &mockAgentManager{launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
		launched = true
		return nil, nil
	}}, repo)
	_, err := exec.LaunchPreparedSession(context.Background(), &v1.Task{ID: session.TaskID, WorkspaceID: "workspace"}, session.ID,
		LaunchOptions{AgentProfileID: session.AgentProfileID, ExecutorID: "kube"})
	require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsupported)
	require.False(t, launched)
	require.Empty(t, repo.createTaskEnvironmentCalls)
	require.Equal(t, "parent", env.TaskID)
	require.Equal(t, env.ID, session.TaskEnvironmentID)
}

func TestKubernetesInheritedWorkspaceSameTaskAllowed(t *testing.T) {
	task := &v1.Task{ID: "owner", Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "shared_group"}}}
	env := &models.TaskEnvironment{ID: "owned-env", TaskID: task.ID, ExecutorType: "k8s"}
	require.NoError(t, rejectInheritedEnvironmentExecutorMismatch(task, env, "k8s"))
}

func TestKubernetesInheritedWorkspaceIncompleteOwnership(t *testing.T) {
	task := &v1.Task{ID: "child", Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}}
	for _, tc := range []struct {
		name         string
		owner        string
		executorType string
		want         error
	}{
		{name: "missing owner", executorType: "k8s", want: models.ErrWorkspaceReuseUnsafe},
		{name: "creating environment without type", owner: "parent", want: models.ErrWorkspaceReuseUnsupported},
		{name: "executor mismatch", owner: "parent", executorType: "local_pc", want: models.ErrWorkspaceReuseUnsafe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := rejectInheritedEnvironmentExecutorMismatch(task, &models.TaskEnvironment{TaskID: tc.owner, ExecutorType: tc.executorType}, "k8s")
			require.ErrorIs(t, err, tc.want)
		})
	}
}
