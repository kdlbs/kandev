package kubernetes

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/task/models"
)

func TestListSessionsUsesRecordedNamespaceAfterExecutorNamespaceChange(t *testing.T) {
	createdAt := time.Date(2026, time.September, 2, 9, 0, 0, 0, time.UTC)
	run := kubernetesRunningRow(
		"instance-1", "session-1", "task-1", "pod-1", "pod-uid-1", createdAt,
	)
	run.Metadata[metadataNamespace] = "old-agents"
	session := kubernetesTaskSession("session-1", "task-1", "executor-1", "profile-1")
	pod := kubernetesOwnedPod("pod-1", "pod-uid-1", "instance-1", session)
	pod.Namespace = "old-agents"
	config := validHandlerExecutorConfig()
	config["namespace"] = "new-agents"
	repo := &fakeResourceRepository{
		executor: &models.Executor{
			ID: "executor-1", Type: models.ExecutorTypeKubernetes, Config: config,
		},
		runs:     []*models.ExecutorRunning{run},
		sessions: map[string]*models.TaskSession{"session-1": session},
	}
	clientset := kubernetesfake.NewSimpleClientset(pod)
	handler := NewHandler(repo, &fakeAccessChecker{}, func(got agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
		require.Equal(t, "new-agents", got.Namespace, "current config must build the client")
		return &agentkubernetes.Client{Clientset: clientset}, nil
	})

	rows, err := handler.listSessions(context.Background(), "executor-1", SessionFilter{})

	require.NoError(t, err)
	require.Equal(t, []SessionRow{{
		SessionID: "session-1", TaskID: "task-1", PodName: "pod-1",
		PodPhase: "Running", ContainerState: "running", Restarts: 2,
		WorkspaceKind: "empty_dir", CreatedAt: createdAt.Format(time.RFC3339),
	}}, rows)
	require.Len(t, clientset.Actions(), 1)
	getAction, ok := clientset.Actions()[0].(k8stesting.GetAction)
	require.True(t, ok)
	require.Equal(t, "old-agents", getAction.GetNamespace())
	require.Equal(t, "pod-1", getAction.GetName())
}
