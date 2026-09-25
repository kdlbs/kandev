package lifecycle

import (
	"context"
	"github.com/kandev/kandev/internal/secrets"
	"testing"
	"time"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestKubernetesTaskPodBackendRestartKeepsSessionInstances(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	f.launch(t, 2)
	f.restartBackend(t)
	req := taskPodRequest(1)
	req.InstanceID = "resumed-execution"
	req.PreviousExecutionID = a.InstanceID
	req.Metadata = cloneKubernetesMetadata(a.Metadata)
	resumed, err := f.runtime.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, a.Metadata[MetadataKeyKubernetesPodUID], resumed.Metadata[MetadataKeyKubernetesPodUID])
	require.Equal(t, "instance-1", resumed.Metadata[MetadataKeyKubernetesAgentctlInstanceID])
	_, active, _ := f.control.snapshot()
	require.Len(t, active, 2, "resume must not leave a second agent process for the same session")
	require.Len(t, f.resources.createdPods, 1)
}

func TestKubernetesTaskPodRefreshKeepsSiblingSessionIdentity(t *testing.T) {
	req := taskPodRequest(2)
	req.Metadata[metadataKubernetesTaskOwned] = true
	instance := &ExecutorInstance{InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID, Metadata: req.Metadata}
	identity := kubeexecutor.ResourceIdentity{TaskID: req.TaskID, SessionID: "session-1", EnvironmentID: req.TaskEnvironmentID}
	refreshed := kubernetesActiveRefreshRequest(req, instance, identity)
	require.Equal(t, "session-2", refreshed.SessionID)
}

func TestKubernetesTaskPodContainerRestartSharesRotatedControlToken(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	b := f.launch(t, 2)
	f.control.reboot()
	f.resources.mu.Lock()
	f.resources.pod.Status.ContainerStatuses[0].RestartCount = 1
	f.resources.mu.Unlock()
	ctx := context.Background()
	for _, instance := range []*ExecutorInstance{a, b} {
		refreshed, err := f.runtime.RefreshRemoteInstance(ctx, kubernetesRefreshInstance(instance, taskPodRequest(1).Metadata))
		require.NoError(t, err)
		require.NotNil(t, refreshed)
		require.Equal(t, instance.SessionID, refreshed.Instance.SessionID)
		require.NoError(t, refreshed.Commit(nil))
	}
	_, active, handshakes := f.control.snapshot()
	require.Len(t, active, 2)
	require.Equal(t, 1, handshakes)
}

func TestKubernetesTaskPodRejectsPreparingEnvironment(t *testing.T) {
	f := newTaskPodFixture(t)
	_, err := f.runtime.CreateInstance(context.Background(), taskPodRequest(2))
	require.ErrorIs(t, err, models.ErrWorkspacePreparing)
	require.Empty(t, f.resources.createdPods)
	_, err = f.repo.GetKubernetesEnvironment(context.Background(), "environment-1")
	require.ErrorIs(t, err, models.ErrKubernetesEnvironmentNotFound)
}

func TestKubernetesTaskPodConcurrentAttachAndFailure(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	results := make(chan error, 2)
	for _, n := range []int{2, 3} {
		go func(number int) {
			_, err := f.runtime.CreateInstance(context.Background(), taskPodRequest(number))
			results <- err
		}(n)
	}
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	require.Len(t, f.resources.createdPods, 1)
	require.Len(t, f.resources.createdPVCs, 1)
	f.control.mu.Lock()
	f.control.failID = "instance-4"
	f.control.mu.Unlock()
	_, err := f.runtime.CreateInstance(context.Background(), taskPodRequest(4))
	require.Error(t, err)
	_, active, _ := f.control.snapshot()
	require.Len(t, active, 3)
	require.Contains(t, active, a.InstanceID)
	require.Empty(t, f.resources.deletedPods)
}

func TestKubernetesTaskPodResumeValidationAllowsSibling(t *testing.T) {
	f := newTaskPodFixture(t)
	f.launch(t, 1)
	b := f.launch(t, 2)
	require.NoError(t, ValidateKubernetesResumeMetadata(b.Metadata, b.TaskID, b.SessionID, map[string]string{"auth_mode": "in_cluster", "namespace": "kandev-agents"}))
}

func TestKubernetesTaskPodMissingPodReplacementSharedAcrossResumes(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	b := f.launch(t, 2)
	f.restartBackend(t)
	f.control.reboot()
	f.resources.mu.Lock()
	f.resources.pod = nil
	f.resources.nextPodUID = "replacement-uid"
	f.resources.mu.Unlock()
	for _, instance := range []*ExecutorInstance{a, b} {
		req := taskPodRequest(2)
		req.SessionID = instance.SessionID
		req.InstanceID = instance.InstanceID + "-resumed"
		req.PreviousExecutionID = instance.InstanceID
		req.Metadata = cloneKubernetesMetadata(instance.Metadata)
		resumed, err := f.runtime.CreateInstance(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, "replacement-uid", resumed.Metadata[MetadataKeyKubernetesPodUID])
	}
	require.Len(t, f.resources.createdPods, 2)
	require.Len(t, f.resources.createdPVCs, 1)
	_, active, handshakes := f.control.snapshot()
	require.Len(t, active, 2)
	require.Equal(t, 1, handshakes)
	require.NoError(t, f.runtime.StopInstance(context.Background(), a, true))
	_, active, _ = f.control.snapshot()
	require.NotContains(t, active, "instance-1")
	require.Contains(t, active, "instance-2")
}

func TestKubernetesTaskPodStaleSessionCannotOverwriteRotatedToken(t *testing.T) {
	for _, layer := range []string{"lifecycle", "manager"} {
		t.Run(layer, func(t *testing.T) {
			f := newTaskPodFixture(t)
			instance := f.launch(t, 1)
			ctx := context.Background()
			id := getMetadataString(instance.Metadata, MetadataKeyAuthTokenSecret)
			rotated := "newer-sibling-token"
			require.NoError(t, f.secretStore.Update(ctx, id, &secrets.UpdateSecretRequest{Value: &rotated}))
			if layer == "lifecycle" {
				record, err := f.repo.GetKubernetesEnvironment(ctx, "environment-1")
				require.NoError(t, err)
				require.NoError(t, f.runtime.persistKubernetesEnvironmentSecrets(ctx, record, instance))
			} else {
				execution := &AgentExecution{ID: instance.InstanceID, metadata: cloneKubernetesMetadata(instance.Metadata)}
				_, err := f.manager.persistRequiredKubernetesRuntimeSecrets(ctx, instance, execution)
				require.NoError(t, err)
			}
			got, err := f.secretStore.Reveal(ctx, id)
			require.NoError(t, err)
			require.Equal(t, rotated, got)
		})
	}
}

func TestKubernetesTaskPodRetriesRotatedTokenPersistence(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	f.launch(t, 2)
	id := getMetadataString(a.Metadata, MetadataKeyAuthTokenSecret)
	store := &failingUpdateSecretStore{inMemorySecretStore: f.secretStore.(*inMemorySecretStore), failUpdateFor: id}
	f.runtime.secretStore = store
	f.control.reboot()
	f.resources.mu.Lock()
	f.resources.pod.Status.ContainerStatuses[0].RestartCount = 1
	f.resources.mu.Unlock()
	ctx := context.Background()
	_, err := f.runtime.RefreshRemoteInstance(ctx, kubernetesRefreshInstance(a, taskPodRequest(1).Metadata))
	require.ErrorContains(t, err, "injected secret update failure")
	store.failUpdateFor = ""
	f.restartBackend(t)
	req := taskPodRequest(1)
	req.PreviousExecutionID = a.InstanceID
	req.Metadata = cloneKubernetesMetadata(a.Metadata)
	_, err = f.runtime.CreateInstance(ctx, req)
	require.NoError(t, err)
	_, _, handshakes := f.control.snapshot()
	require.Equal(t, 1, handshakes, "storage retry must preserve the single-use handshake result")
	f.restartBackend(t)
	f.launch(t, 3)
	require.Len(t, f.resources.createdPods, 1)
}

// Reviewer-requested contract coverage: attaching a different session may overlap
// deletion of the stopped session's remote agent without deleting shared storage.
func TestKubernetesTaskPodConcurrentStopAndAttach(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	f.launch(t, 2)
	entered, release := make(chan struct{}), make(chan struct{})
	f.control.beforeDelete = func() { close(entered); <-release }
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- f.runtime.StopInstance(ctx, a, true) }()
	select {
	case <-entered:
	case <-ctx.Done():
		close(release)
		t.Fatal("stop did not reach remote deletion")
	}
	_, attachErr := f.runtime.CreateInstance(ctx, taskPodRequest(3))
	close(release)
	require.NoError(t, attachErr)
	require.NoError(t, <-stopped)
	_, active, _ := f.control.snapshot()
	require.NotContains(t, active, "instance-1")
	require.Contains(t, active, "instance-2")
	require.Contains(t, active, "instance-3")
	require.Len(t, f.resources.createdPods, 1)
	require.Empty(t, f.resources.deletedPods)
	require.Empty(t, f.resources.deletedPVCs)
}

func TestKubernetesTaskPodStopOutlivesCanceledCaller(t *testing.T) {
	f := newTaskPodFixture(t)
	a := f.launch(t, 1)
	f.launch(t, 2)
	f.resources.rejectCanceledGetContexts = true
	f.secretStore.(*inMemorySecretStore).rejectCanceledCalls = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, f.runtime.StopInstance(ctx, a, true))
	_, active, _ := f.control.snapshot()
	require.NotContains(t, active, "instance-1")
	require.Contains(t, active, "instance-2")
	require.Empty(t, f.resources.deletedPods)
}
