package kubernetes

import (
	"context"
	"testing"
	"time"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

type taskPodStatusRepository struct {
	*fakeResourceRepository
	environment *models.KubernetesEnvironment
}

func (r *taskPodStatusRepository) GetKubernetesEnvironment(context.Context, string) (*models.KubernetesEnvironment, error) {
	return r.environment, nil
}

func TestSharedTaskPodStatusUsesCanonicalInventory(t *testing.T) {
	run := kubernetesRunningRow("environment-1", "session-2", "task-1", "pod-1", "old-uid", time.Now())
	run.Metadata[agentkubernetes.MetadataKeyTaskOwned] = true
	run.Metadata[agentkubernetes.MetadataKeyOwnershipVersion] = agentkubernetes.TaskOwnershipVersion
	run.Metadata[metadataResourceSession] = "session-1"
	metadata := make(map[string]interface{}, len(run.Metadata))
	for k, v := range run.Metadata {
		metadata[k] = v
	}
	metadata[metadataPodUID] = "new-uid"
	session := kubernetesTaskSession("session-2", "task-1", "executor-1", "profile-1")
	repo := &taskPodStatusRepository{fakeResourceRepository: &fakeResourceRepository{
		executor: &models.Executor{ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig()},
		runs:     []*models.ExecutorRunning{run}, sessions: map[string]*models.TaskSession{session.ID: session},
	}, environment: &models.KubernetesEnvironment{EnvironmentID: "environment-1", TaskID: "task-1", Metadata: metadata}}
	pod := kubernetesOwnedPodForIdentity("pod-1", "new-uid", agentkubernetes.ResourceIdentity{ExecutorID: "executor-1", ProfileID: "profile-1", InstanceID: "environment-1", TaskID: "task-1", SessionID: "session-1", EnvironmentID: "environment-1", TaskOwned: true})
	clientset := kubernetesfake.NewSimpleClientset(pod)
	handler := NewHandler(repo, &fakeAccessChecker{}, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	rows, err := handler.listSessions(context.Background(), "executor-1", SessionFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].FailureReason)
	require.Equal(t, "session-2", rows[0].SessionID)
	require.Equal(t, "Running", rows[0].PodPhase)
	repo.environment.TaskID = "foreign-task"
	rows, err = handler.listSessions(context.Background(), "executor-1", SessionFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, rows[0].FailureReason)
}

func (r *taskPodStatusRepository) ListKubernetesEnvironments(context.Context) ([]*models.KubernetesEnvironment, error) {
	return []*models.KubernetesEnvironment{r.environment}, nil
}

func TestSharedTaskPodStatusRetainedWithoutSessions(t *testing.T) {
	run := kubernetesRunningRow("environment-1", "session-1", "task-1", "pod-1", "pod-uid", time.Now())
	run.Metadata[agentkubernetes.MetadataKeyTaskOwned] = true
	run.Metadata[agentkubernetes.MetadataKeyOwnershipVersion] = agentkubernetes.TaskOwnershipVersion
	repo := &taskPodStatusRepository{fakeResourceRepository: &fakeResourceRepository{
		executor: &models.Executor{ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: validHandlerExecutorConfig()},
	}, environment: &models.KubernetesEnvironment{EnvironmentID: "environment-1", TaskID: "task-1", Metadata: run.Metadata}}
	identity, _ := recordedResourceIdentity(run.Metadata)
	pod := kubernetesOwnedPodForIdentity("pod-1", "pod-uid", identity)
	clientset := kubernetesfake.NewSimpleClientset(pod)
	access := &fakeAccessChecker{}
	handler := NewHandler(repo, access, func(agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})
	rows, err := handler.listSessions(context.Background(), "executor-1", SessionFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1, "retained task pod must remain visible after deleting every session")
	require.Empty(t, rows[0].SessionID)
	require.Equal(t, "task-1", rows[0].TaskID)
	require.Equal(t, "retained", rows[0].RetentionState)
	require.Equal(t, "Running", rows[0].PodPhase)
	require.NotEmpty(t, access.calls)
}
