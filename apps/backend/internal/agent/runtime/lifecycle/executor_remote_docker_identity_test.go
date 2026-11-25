package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerAdoptedAgentctlIdentityIsDurableAndSessionScoped(t *testing.T) {
	require.True(t, ShouldPersistMetadataKey(MetadataKeyRemoteDockerAgentctlInstanceID))
	require.True(t, IsSessionScopedMetadataKey(MetadataKeyRemoteDockerAgentctlInstanceID),
		"a sibling session must not inherit another session's agentctl identity")

	filtered := FilterPersistentMetadata(map[string]interface{}{
		MetadataKeyRemoteDockerAgentctlInstanceID: "adopted-instance",
		"transient_request_value":                 "discard",
	})
	require.Equal(t, "adopted-instance", filtered[MetadataKeyRemoteDockerAgentctlInstanceID])
	require.NotContains(t, filtered, "transient_request_value")

	request := &ExecutorCreateRequest{
		InstanceID: "backend-execution",
		Metadata:   map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "adopted-instance"},
	}
	require.Equal(t, "adopted-instance", remoteDockerAgentctlInstanceID(request))
	require.Equal(t, "backend-execution", remoteDockerAgentctlInstanceID(&ExecutorCreateRequest{InstanceID: "backend-execution"}))
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerAdoptedIdentitySurvivesPersistenceAndRecoveryRequest(t *testing.T) {
	instance := &ExecutorInstance{
		InstanceID: "backend-new", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeRemoteDocker, ReusedWorkspace: true,
		Metadata: map[string]interface{}{
			MetadataKeyRemoteDockerAgentctlInstanceID: "agentctl-old",
			MetadataKeyContainerID:                    "container-1",
		},
	}
	execution := instance.ToAgentExecution(&ExecutorCreateRequest{
		InstanceID: "backend-new", TaskID: "task-1", SessionID: "session-1",
		ExecutorType: "remote_docker",
	})
	require.Equal(t, "agentctl-old", execution.MetadataSnapshot()[MetadataKeyRemoteDockerAgentctlInstanceID])

	persisted := FilterPersistentMetadata(execution.MetadataSnapshot())
	require.Equal(t, "agentctl-old", persisted[MetadataKeyRemoteDockerAgentctlInstanceID])
	req, containerID, ok := remoteDockerRecoveryRequest(&models.ExecutorRunning{
		AgentExecutionID: execution.ID, SessionID: execution.SessionID, TaskID: execution.TaskID,
		Runtime: agentruntime.RuntimeRemoteDocker, ContainerID: "container-1", Metadata: persisted,
	})
	require.True(t, ok)
	require.Equal(t, "container-1", containerID)
	require.Equal(t, "backend-new", req.InstanceID)
	require.Equal(t, "agentctl-old", remoteDockerAgentctlInstanceID(req))
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestDockerAgentctlIdentityResolverUsesAdoptedMetadata(t *testing.T) {
	for _, runtime := range []agentruntime.Runtime{
		agentruntime.RuntimeDocker,
		agentruntime.RuntimeRemoteDocker,
	} {
		t.Run(string(runtime), func(t *testing.T) {
			got, err := recordedAgentctlInstanceID(runtime, "backend-new", map[string]interface{}{
				MetadataKeyRemoteDockerAgentctlInstanceID: "agentctl-old",
			})
			require.NoError(t, err)
			require.Equal(t, "agentctl-old", got)

			got, err = recordedAgentctlInstanceID(runtime, "backend-new", nil)
			require.NoError(t, err)
			require.Equal(t, "backend-new", got, "legacy records without adopted metadata keep the backend identity")
		})
	}
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestValidateExistingAgentIdentityAcceptsAdoptedDockerAgentctlID(t *testing.T) {
	execution := &AgentExecution{
		ID: "backend-new", SessionID: "session-1", RuntimeName: agentruntime.RuntimeRemoteDocker,
		ACPSessionID: "native-session",
		metadata: map[string]interface{}{
			MetadataKeyRemoteDockerAgentctlInstanceID: "agentctl-old",
		},
	}
	association := &agentctl.AgentSessionAssociation{
		AgentStatus:       agentProcessStatusRunning,
		InstanceID:        "agentctl-old",
		SessionID:         "session-1",
		NativeSessionID:   "native-session",
		IncarnationID:     "incarnation-1",
		HarnessGeneration: 1,
	}
	require.NoError(t, validateExistingAgentIdentity(execution, association))
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestFreshDockerLaunchDropsInheritedAdoptedIdentity(t *testing.T) {
	stale := &ExecutorCreateRequest{
		InstanceID: "fresh", TaskID: "task-1", SessionID: "session-1", ExecutorType: "remote_docker",
		Metadata: map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "stale-adopted"},
	}
	fresh := (&ExecutorInstance{
		InstanceID: "fresh", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeRemoteDocker, ReusedWorkspace: false,
		Metadata: map[string]interface{}{MetadataKeyContainerID: "container-1"},
	}).ToAgentExecution(stale)
	require.NotContains(t, fresh.MetadataSnapshot(), MetadataKeyRemoteDockerAgentctlInstanceID,
		"a fresh allocation must not inherit the previous execution's adopted identity")

	reused := (&ExecutorInstance{
		InstanceID: "fresh", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeRemoteDocker, ReusedWorkspace: true,
		Metadata: map[string]interface{}{
			MetadataKeyContainerID:                    "container-1",
			MetadataKeyRemoteDockerAgentctlInstanceID: "current-adopted",
		},
	}).ToAgentExecution(stale)
	require.Equal(t, "current-adopted", reused.MetadataSnapshot()[MetadataKeyRemoteDockerAgentctlInstanceID])
}
