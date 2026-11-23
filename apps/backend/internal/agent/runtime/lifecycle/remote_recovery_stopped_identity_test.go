package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestValidateRemoteSessionAssociationAcceptsStoppedRetainedRemote(t *testing.T) {
	for _, runtime := range []agentruntime.Runtime{agentruntime.RuntimeRemoteDocker, agentruntime.RuntimePluginRemote} {
		t.Run(string(runtime), func(t *testing.T) {
			instance := &ExecutorInstance{
				InstanceID: "execution-stopped", SessionID: "session-stopped", RuntimeName: runtime,
				ProviderSessionID: "native-saved",
				Metadata:          map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "agentctl-stopped"},
			}
			association := stoppedRemoteAssociation()
			require.NoError(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"))

			association.NativeSessionID = "native-saved"
			require.NoError(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"))

			association.NativeSessionID = "foreign-native"
			require.Error(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"))
		})
	}
}

func TestValidateRemoteSessionAssociationKeepsRunningProofStrict(t *testing.T) {
	instance := &ExecutorInstance{
		InstanceID: "execution-stopped", SessionID: "session-stopped", RuntimeName: agentruntime.RuntimeRemoteDocker,
		Metadata: map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "agentctl-stopped"},
	}
	association := stoppedRemoteAssociation()
	association.AgentStatus = agentProcessStatusRunning
	require.Error(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"),
		"a running process without its initialized native session is not adoptable")
	association.NativeSessionID = "native-running"
	require.NoError(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"))
}

func TestFailedRemoteRecoveryWithoutDeliveryRequiresStoppedAssociation(t *testing.T) {
	for _, runtime := range []agentruntime.Runtime{agentruntime.RuntimeRemoteDocker, agentruntime.RuntimePluginRemote} {
		t.Run(string(runtime), func(t *testing.T) {
			snapshot := &RemoteRecoverySnapshot{
				Runtime: string(runtime), SessionState: models.TaskSessionStateFailed,
			}
			require.NoError(t, validateRemoteRecoverySnapshotSession(snapshot))
			require.True(t, remoteRecoveryNeedsStoppedAgentProof(snapshot))

			instance := &ExecutorInstance{agentSessionAssociation: &agentctl.AgentSessionAssociation{
				AgentStatus: agentProcessStatusRunning,
			}}
			require.Error(t, validateFailedRemoteRecoveryAgentProof(snapshot, instance),
				"a live process cannot prove a failed session without delivery evidence")
			instance.agentSessionAssociation.AgentStatus = agentProcessStatusStopped
			require.NoError(t, validateFailedRemoteRecoveryAgentProof(snapshot, instance))

			snapshot.RecoverySourcePresent = true
			require.False(t, remoteRecoveryNeedsStoppedAgentProof(snapshot),
				"the durable delivery path keeps its existing recovery requirements")
		})
	}
}

func TestFailedRecoveryWithoutDeliveryRejectsUnrelatedRuntime(t *testing.T) {
	snapshot := &RemoteRecoverySnapshot{
		Runtime: string(agentruntime.RuntimeSSH), SessionState: models.TaskSessionStateFailed,
	}
	require.Error(t, validateRemoteRecoverySnapshotSession(snapshot))
}

func TestValidateRemoteSessionAssociationRejectsStoppedIdentityConflicts(t *testing.T) {
	instance := &ExecutorInstance{
		InstanceID: "execution-stopped", SessionID: "session-stopped", RuntimeName: agentruntime.RuntimeRemoteDocker,
		Metadata: map[string]interface{}{MetadataKeyRemoteDockerAgentctlInstanceID: "agentctl-stopped"},
	}
	for _, mutate := range []struct {
		name string
		edit func(*agentctl.AgentSessionAssociation)
	}{
		{name: "instance", edit: func(a *agentctl.AgentSessionAssociation) { a.InstanceID = "foreign" }},
		{name: "session", edit: func(a *agentctl.AgentSessionAssociation) { a.SessionID = "foreign" }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			association := stoppedRemoteAssociation()
			mutate.edit(association)
			require.Error(t, validateRemoteSessionAssociation(instance, association, "agentctl-stopped"))
		})
	}
	instance.RuntimeName = agentruntime.RuntimeSSH
	require.Error(t, validateRemoteSessionAssociation(instance, stoppedRemoteAssociation(), "agentctl-stopped"),
		"only retained Remote Docker and plugin runtimes may settle stopped evidence")
}

func stoppedRemoteAssociation() *agentctl.AgentSessionAssociation {
	return &agentctl.AgentSessionAssociation{
		InstanceID: "agentctl-stopped", SessionID: "session-stopped",
		IncarnationID: "incarnation-stopped", HarnessGeneration: 2,
		AgentStatus: agentProcessStatusStopped,
	}
}
