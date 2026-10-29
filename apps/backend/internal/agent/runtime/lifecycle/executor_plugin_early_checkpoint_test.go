package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/plugins/workspacedescriptor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

type earlyCheckpointHarness struct {
	manager     *Manager
	context     *pluginsdk.ExecutorProviderRequestContext
	checkpoints []pluginExecutorInventory
	request     *ExecutorCreateRequest
}

func newEarlyCheckpointHarness(t *testing.T) *earlyCheckpointHarness {
	t.Helper()
	provider := capabilityLaunchProvider()
	launch := pluginExecutorLaunchRequest(provider)
	launch.PluginExecutor.Profile.OwnershipGeneration = 4
	reader := &fakeExecutorProfileReader{
		session: &models.TaskSession{ID: launch.SessionID, TaskID: launch.TaskID, TaskEnvironmentID: launch.TaskEnvironmentID},
		env:     &models.TaskEnvironment{ID: launch.TaskEnvironmentID, TaskID: launch.TaskID, OwnershipGeneration: 4},
	}
	h := &earlyCheckpointHarness{manager: &Manager{logger: newTestLogger(), executorProfileReader: reader}, request: launch}
	if err := h.manager.registerPluginExecutorCallbacks(launch); err != nil {
		t.Fatalf("register callbacks: %v", err)
	}
	t.Cleanup(func() { h.manager.unregisterPluginExecutorCallbacks(launch.InstanceID) })
	launch.CheckpointRuntimeInventory = func(_ context.Context, value map[string]interface{}) error {
		h.checkpoints = append(h.checkpoints, value[MetadataKeyPluginExecutor].(pluginExecutorInventory))
		launch.Metadata = value
		return nil
	}
	h.context = &pluginsdk.ExecutorProviderRequestContext{
		PluginId: provider.PluginID, InstallationId: provider.InstallationID, ProviderKey: provider.Key,
		ContractVersion: int32(provider.ContractVersion), TaskId: launch.TaskID, SessionId: launch.SessionID,
		EnvironmentId: launch.TaskEnvironmentID, ExecutionId: launch.InstanceID, OperationId: launch.InstanceID,
		EnvironmentGeneration: 4, DispatchGeneration: 11, InputDigest: "digest",
	}
	return h
}

func (h *earlyCheckpointHarness) checkpoint(phase, path, identity string) error {
	return h.manager.CheckpointExecutorResource(context.Background(), &pluginsdk.CheckpointExecutorResourceRequest{
		Context: h.context, Phase: phase,
		Resource: &pluginsdk.ExecutorResourceDescriptor{
			ResourceHandle: "resource-1", StateJson: `{"resource_id":"fixture-1"}`, Platform: "linux-amd64", StateVersion: 1,
			WorkspacePath: path, WorkspaceIdentity: identity,
		},
	})
}

func TestEarlyLaunchCheckpointsAreAcceptedBeforeTheWorkspaceDescriptorIsKnown(t *testing.T) {
	for _, phase := range []string{pluginExecutorPhaseProvisioned, pluginExecutorPhaseBootstrapping, pluginExecutorPhaseArtifactStaging} {
		t.Run(phase, func(t *testing.T) {
			h := newEarlyCheckpointHarness(t)
			if err := h.checkpoint(phase, "", ""); err != nil {
				t.Fatalf("checkpoint without a descriptor was refused: %v", err)
			}
			got := h.checkpoints[len(h.checkpoints)-1]
			if got.Phase != phase || got.WorkspaceDescriptor != nil {
				t.Fatalf("recorded phase=%q descriptor=%#v, want %q with no descriptor", got.Phase, got.WorkspaceDescriptor, phase)
			}
		})
	}
}

func TestEarlyLaunchCheckpointStillRejectsAPartialDescriptor(t *testing.T) {
	h := newEarlyCheckpointHarness(t)
	for name, args := range map[string][2]string{"path only": {testBackendOnlyWorkspace, ""}, "identity only": {"", "worker-7"}} {
		if err := h.checkpoint(pluginExecutorPhaseBootstrapping, args[0], args[1]); err == nil {
			t.Fatalf("%s: a half-supplied descriptor was accepted", name)
		}
	}
}

func TestLaterCheckpointCannotEraseTheRecordedDescriptor(t *testing.T) {
	h := newEarlyCheckpointHarness(t)
	if err := h.checkpoint(pluginExecutorPhaseProvisioned, testBackendOnlyWorkspace, "worker-7"); err != nil {
		t.Fatalf("checkpoint with a descriptor: %v", err)
	}
	err := h.checkpoint(pluginExecutorPhaseBootstrapping, "", "")
	if err == nil || !strings.Contains(err.Error(), "workspace descriptor") {
		t.Fatalf("an empty descriptor replaced the recorded one: %v", err)
	}
	if got := h.checkpoints[len(h.checkpoints)-1].WorkspaceDescriptor; got == nil || *got != (workspacedescriptor.Descriptor{Path: testBackendOnlyWorkspace, Identity: "worker-7"}) {
		t.Fatalf("recorded descriptor changed: %#v", got)
	}
}
