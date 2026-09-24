package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

type sharedKubernetesControlStub struct {
	createRequest *agentctl.CreateInstanceRequest
}

func (c *sharedKubernetesControlStub) CreateInstance(
	_ context.Context,
	request *agentctl.CreateInstanceRequest,
) (*agentctl.CreateInstanceResponse, error) {
	c.createRequest = request
	return &agentctl.CreateInstanceResponse{ID: request.ID, Port: 41001}, nil
}

func (*sharedKubernetesControlStub) GetInstance(
	context.Context,
	string,
) (*agentctl.InstanceInfo, error) {
	return nil, agentctl.ErrInstanceNotFound
}

func TestGetOrCreateSharedKubernetesInstanceRestoresDurableJournalPath(t *testing.T) {
	control := &sharedKubernetesControlStub{}
	req := &ExecutorCreateRequest{
		InstanceID:                "new-execution",
		SessionID:                 "session-1",
		TaskID:                    "task-1",
		TaskEnvironmentID:         "environment-1",
		WorkspacePath:             kubernetesWorkspacePath,
		DurableJournalOwnerID:     "session-1",
		DeliveryIncarnationID:     "incarnation-1",
		DeliveryHarnessGeneration: 2,
		DeliveryStreamID:          "incarnation-1:g2",
		Metadata: map[string]interface{}{
			MetadataKeyKubernetesRuntimeWorkspaceMode: string(kubeexecutor.WorkspaceModeManagedPVC),
		},
	}

	response, created, err := getOrCreateSharedKubernetesInstance(
		context.Background(), control, req, "remote-instance",
	)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, &agentctl.CreateInstanceResponse{ID: "remote-instance", Port: 41001}, response)
	require.NotNil(t, control.createRequest)
	require.Equal(t, "incarnation-1:g2", control.createRequest.DeliveryStreamID)
	require.Equal(t,
		"/workspace/.kandev/agentctl-journals/session-1/delivery.bbolt",
		control.createRequest.DurableJournalPath,
	)
}
