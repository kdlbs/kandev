package lifecycle

import (
	"context"
	"errors"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"testing"

	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type taskPodFailedSecrets struct{ secrets.SecretStore }

func (s taskPodFailedSecrets) Get(context.Context, string) (*secrets.Secret, error) {
	return nil, errors.New("injected secret storage failure")
}

func TestKubernetesTaskPodSecretFailureRollsBackAttachment(t *testing.T) {
	f := newTaskPodFixture(t)
	f.launch(t, 1)
	f.runtime.secretStore = taskPodFailedSecrets{f.secretStore}
	_, err := f.runtime.CreateInstance(context.Background(), taskPodRequest(2))
	require.ErrorContains(t, err, "injected secret storage failure")
	_, active, _ := f.control.snapshot()
	require.Len(t, active, 1, "failed attachment must not leave a remote agent")
	require.Contains(t, active, "instance-1")
	require.Empty(t, f.resources.deletedPods)
	require.Empty(t, f.resources.deletedPVCs)
	f.runtime.mu.Lock()
	_, exists := f.runtime.sessions["instance-2"]
	f.runtime.mu.Unlock()
	require.False(t, exists, "failed attachment must close its forward")
}

func TestKubernetesTaskPodPinsCleanupScript(t *testing.T) {
	f := newTaskPodFixture(t)
	req := taskPodRequest(1)
	req.Metadata[MetadataKeyCleanupScript] = "echo clean-task"
	_, err := f.runtime.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	record, err := f.repo.GetKubernetesEnvironment(context.Background(), req.TaskEnvironmentID)
	require.NoError(t, err)
	require.Equal(t, "echo clean-task", getMetadataString(record.Metadata, MetadataKeyCleanupScript))
}

func TestKubernetesTaskPodRejectsDifferentProfile(t *testing.T) {
	f := newTaskPodFixture(t)
	f.launch(t, 1)
	req := taskPodRequest(2)
	req.Metadata[MetadataKeyExecutorProfileID] = "different-profile"
	_, err := f.runtime.CreateInstance(context.Background(), req)
	require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsafe)
	require.Len(t, f.resources.createdPods, 1)
}

type taskPodLegacyStore struct {
	KubernetesEnvironmentStore
	rows []*models.ExecutorRunning
}

func (s taskPodLegacyStore) ListExecutorsRunningByTaskID(context.Context, string) ([]*models.ExecutorRunning, error) {
	return s.rows, nil
}

func TestKubernetesTaskPodLegacyInventoryMustAgree(t *testing.T) {
	for _, key := range []string{MetadataKeyKubernetesPodUID, MetadataKeyKubernetesNamespace, MetadataKeyKubernetesResourceExecutorID, MetadataKeyKubernetesProfileSnapshot} {
		t.Run(key, func(t *testing.T) {
			f := newTaskPodFixture(t)
			first := f.launch(t, 1)
			second := cloneKubernetesMetadata(first.Metadata)
			second[key] = "conflicting"
			f.runtime.environmentStore = taskPodLegacyStore{KubernetesEnvironmentStore: f.repo, rows: []*models.ExecutorRunning{
				{Runtime: "k8s", Metadata: first.Metadata}, {Runtime: "k8s", Metadata: second},
			}}
			record := &models.KubernetesEnvironment{}
			err := f.runtime.adoptKubernetesEnvironment(context.Background(), taskPodRequest(2), record)
			require.ErrorIs(t, err, models.ErrWorkspaceReuseUnsafe)
		})
	}
}

func TestKubernetesTaskPodLegacySessionCannotDeleteAdoptedPod(t *testing.T) {
	f := newTaskPodFixture(t)
	first := f.launch(t, 1)
	f.launch(t, 2)
	delete(first.Metadata, metadataKubernetesTaskOwned)
	first.StopReason = StopReasonTaskDeleted
	require.NoError(t, f.runtime.StopInstance(context.Background(), first, true))
	require.Empty(t, f.resources.deletedPods)
	require.Empty(t, f.resources.deletedPVCs)
	_, active, _ := f.control.snapshot()
	require.Contains(t, active, "instance-2")
}

type taskPodFailedReleaseStore struct{ KubernetesEnvironmentStore }

func (s taskPodFailedReleaseStore) SaveKubernetesEnvironment(ctx context.Context, record *models.KubernetesEnvironment, release bool) error {
	if release {
		return errors.New("injected release failure")
	}
	return s.KubernetesEnvironmentStore.SaveKubernetesEnvironment(ctx, record, release)
}

func TestKubernetesTaskPodReleaseFailureRollsBackAttachment(t *testing.T) {
	f := newTaskPodFixture(t)
	f.launch(t, 1)
	f.runtime.environmentStore = taskPodFailedReleaseStore{f.repo}
	_, err := f.runtime.CreateInstance(context.Background(), taskPodRequest(2))
	require.ErrorContains(t, err, "injected release failure")
	_, active, _ := f.control.snapshot()
	require.Len(t, active, 1)
	require.Contains(t, active, "instance-1")
}

func TestKubernetesTaskPodConnectionFailureRollsBackOnlyNewInstance(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(map[bool]string{false: "new attachment", true: "existing instance"}[resume], func(t *testing.T) {
			f := newTaskPodFixture(t)
			first := f.launch(t, 1)
			attaching := newFakeKubernetesExecutor(t, f.resources, f.execs, map[uint16]uint16{uint16(kubeexecutor.DefaultAgentctlPort): f.port})
			attaching.environmentStore = f.repo
			attaching.secretStore = f.secretStore
			t.Cleanup(func() { require.NoError(t, attaching.Close()) })
			req := taskPodRequest(2)
			if resume {
				req.SessionID = first.SessionID
				req.PreviousExecutionID = first.InstanceID
				req.Metadata = cloneKubernetesMetadata(first.Metadata)
			}
			_, err := attaching.CreateInstance(context.Background(), req)
			require.Error(t, err)
			_, active, _ := f.control.snapshot()
			require.Len(t, active, 1)
			require.Contains(t, active, "instance-1")
		})
	}
}

func TestKubernetesTaskPodLegacyResumeDoesNotRequireConsolidation(t *testing.T) {
	f := newTaskPodFixture(t)
	ctx := context.Background()
	req := taskPodRequest(1)
	first, err := f.runtime.createSessionInstance(ctx, req)
	require.NoError(t, err)
	other := cloneKubernetesMetadata(first.Metadata)
	other[MetadataKeyKubernetesPodUID] = "different-legacy-pod"
	f.runtime.environmentStore = taskPodLegacyStore{KubernetesEnvironmentStore: f.repo, rows: []*models.ExecutorRunning{{Runtime: "k8s", Metadata: first.Metadata}, {Runtime: "k8s", Metadata: other}}}
	resumed := taskPodRequest(1)
	resumed.InstanceID = "resumed-legacy"
	resumed.PreviousExecutionID = first.InstanceID
	resumed.AuthToken = first.AuthToken
	resumed.BootstrapNonce = first.BootstrapNonce
	for key, value := range first.Metadata {
		resumed.Metadata[key] = value
	}
	instance, err := f.runtime.CreateInstance(ctx, resumed)
	require.NoError(t, err, "an existing legacy session can resume its exact pod without consolidating other pods")
	require.Equal(t, first.Metadata[MetadataKeyKubernetesPodUID], instance.Metadata[MetadataKeyKubernetesPodUID])
	require.Len(t, f.resources.createdPods, 1)
}
