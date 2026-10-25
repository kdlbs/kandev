package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestKubernetesResumePreservesProfileForExistingWorkspaceStart(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	session := cloneMockTaskSession(repo.sessions["sess-1"])
	session.ExecutorProfileID = "untrusted-profile"
	repo.sessions[session.ID] = cloneMockTaskSession(session)
	repo.executors["recorded-kubernetes-executor"] = &models.Executor{ID: "recorded-kubernetes-executor", Type: models.ExecutorTypeKubernetes, Config: map[string]string{"auth_mode": "kubeconfig", "kubeconfig_path": "/tmp/test-config", "namespace": "kandev-agents"}}
	repo.executorsRunning[session.ID] = &models.ExecutorRunning{
		SessionID: session.ID, TaskID: session.TaskID, ExecutorID: "recorded-kubernetes-executor",
		Runtime: agentruntime.RuntimeKubernetes, Metadata: recordedKubernetesResumeMetadata(),
	}
	repo.executorProfiles["recorded-profile"] = &models.ExecutorProfile{
		ID: "recorded-profile", ExecutorID: "recorded-kubernetes-executor",
		EnvVars: []models.ProfileEnvVar{{Key: "PYTHONPATH", Value: "/runtime"}, {Key: "DATABASE_URL", SecretID: "database"}},
	}
	repo.executorProfiles["untrusted-profile"] = &models.ExecutorProfile{
		ID: "untrusted-profile", EnvVars: []models.ProfileEnvVar{{Key: "PYTHONPATH", Value: "/wrong"}, {Key: "INJECTED", Value: "bad"}},
	}
	profileManager := &lifecycle.Manager{}
	profileManager.SetExecutorProfileReader(&resumeEnvironmentProfileReader{repo})
	profileManager.SetSecretStore(&mockSecretStore{secrets: map[string]string{"database": "synthetic-database"}})
	var delivered map[string]string
	manager := &mockAgentManager{
		executorProfileEnvFunc: profileManager.ExecutorProfileEnvForSession,
		setExecutionEnvFunc: func(_ context.Context, _ string, env map[string]string) error {
			delivered = env
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)
	task := &v1.Task{ID: session.TaskID, WorkspaceID: "workspace-1"}
	if _, _, _, _, _, err := exec.buildResumeRequest(ctx, task, session, true); err != nil {
		t.Fatal(err)
	}
	if err := exec.persistSessionFullRowIfCurrentState(ctx, session, session.State); err != nil {
		t.Fatal(err)
	}
	reloaded, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	request := &LaunchAgentRequest{ExecutorType: string(models.ExecutorTypeKubernetes)}
	if err := exec.configureExistingWorkspace(ctx, task, reloaded, "execution", "", request); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"PYTHONPATH": "/runtime", "DATABASE_URL": "synthetic-database"}
	if delivered["PYTHONPATH"] != want["PYTHONPATH"] || delivered["DATABASE_URL"] != want["DATABASE_URL"] || delivered["INJECTED"] != "" {
		t.Fatalf("next subprocess environment = %#v, want %#v", delivered, want)
	}
}

type resumeEnvironmentProfileReader struct{ *mockRepository }

func (*resumeEnvironmentProfileReader) HasActiveTaskResourceCleanupJob(context.Context, string) (bool, error) {
	return false, nil
}
