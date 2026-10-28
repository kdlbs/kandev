package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

type pluginExecutorClaimStoreFake struct {
	record     *models.ExecutorRunning
	claim      *models.TaskEnvironmentRecoveryClaim
	checkpoint models.PluginExecutorClaimCheckpoint
	written    *models.ExecutorRunning
	writes     int
}

func (f *pluginExecutorClaimStoreFake) GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error) {
	if f.record == nil {
		return nil, models.ErrExecutorRunningNotFound
	}
	copy := *f.record
	return &copy, nil
}

func (f *pluginExecutorClaimStoreFake) GetTaskEnvironmentRecoveryClaim(context.Context, string) (*models.TaskEnvironmentRecoveryClaim, error) {
	if f.claim == nil {
		return nil, errors.New("no claim")
	}
	copy := *f.claim
	return &copy, nil
}

func (f *pluginExecutorClaimStoreFake) CheckpointPluginExecutorClaimInventory(_ context.Context, record *models.ExecutorRunning, cp models.PluginExecutorClaimCheckpoint) (bool, error) {
	f.writes++
	f.written, f.checkpoint = record, cp
	return true, nil
}

func claimCheckpointFixture(t *testing.T, negotiated []string) (*Manager, *pluginExecutorClaimStoreFake, *pluginsdk.CheckpointExecutorResourceRequest) {
	t.Helper()
	provider := testPluginExecutorLaunchProvider()
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"two"}`, Platform: "linux-amd64", StateVersion: 1}
	record := pluginExecutorRecoveryRecord(t, "ready", &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1})
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	inventory.NegotiatedCapabilities = negotiated
	record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	store := &pluginExecutorClaimStoreFake{record: record, claim: &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: "environment-plugin-recovery", OwnerTaskID: "task-plugin-recovery", OwnershipGeneration: 7,
		SessionID: "session-plugin-recovery", OperationID: "operation-plugin-recovery:cleanup", ExecutorType: string(models.ExecutorTypePluginRemote),
	}}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{Provider: provider, ProfileID: "profile-plugin-recovery"}}
	manager := &Manager{logger: newTestLogger(), pluginExecutorProfileLoader: loader, pluginExecutorClaimStore: store}
	request := &pluginsdk.CheckpointExecutorResourceRequest{
		Context: &pluginsdk.ExecutorProviderRequestContext{
			PluginId: "example", InstallationId: "install-1", ProviderKey: "remote", ContractVersion: 1,
			TaskId: "task-plugin-recovery", SessionId: "session-plugin-recovery", EnvironmentId: "environment-plugin-recovery",
			ExecutionId: "execution-plugin-recovery", OperationId: "operation-plugin-recovery", EnvironmentGeneration: 7, DispatchGeneration: 3,
		},
		Resource: resource, Phase: "cleanup_progress", Sequence: 1,
	}
	return manager, store, request
}

var declaresDestroyCheckpoints = []string{"destroy_checkpoints@1"}

func TestClaimCheckpointIsAcceptedUnderTheActiveCleanupClaim(t *testing.T) {
	manager, store, request := claimCheckpointFixture(t, declaresDestroyCheckpoints)
	if err := manager.CheckpointExecutorResource(context.Background(), request); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if store.writes != 1 || store.checkpoint.Kind != models.PluginClaimKindCleanup || store.checkpoint.Sequence != 1 || store.checkpoint.Digest == "" {
		t.Fatalf("write = %d %+v", store.writes, store.checkpoint)
	}
	envelope, ok := store.written.Metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory)
	if !ok || envelope.Phase != "cleanup_progress" || envelope.Resource.GetStateJson() != `{"resource":"two"}` {
		t.Fatalf("written envelope = %#v", store.written.Metadata)
	}
}

func TestClaimCheckpointDigestSeparatesPayloads(t *testing.T) {
	manager, store, request := claimCheckpointFixture(t, declaresDestroyCheckpoints)
	_ = manager.CheckpointExecutorResource(context.Background(), request)
	first := store.checkpoint.Digest
	_ = manager.CheckpointExecutorResource(context.Background(), request)
	if store.checkpoint.Digest != first {
		t.Fatal("the same payload must give the same digest")
	}
	request.Resource.StateJson = `{"resource":"three"}`
	_ = manager.CheckpointExecutorResource(context.Background(), request)
	if store.checkpoint.Digest == first {
		t.Fatal("a different payload must give a different digest")
	}
}

func TestClaimCheckpointRefusalsChangeNothing(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*pluginExecutorClaimStoreFake, *pluginsdk.CheckpointExecutorResourceRequest)
		negot  []string
		want   error
	}{
		{"missing sequence", func(_ *pluginExecutorClaimStoreFake, r *pluginsdk.CheckpointExecutorResourceRequest) { r.Sequence = 0 }, declaresDestroyCheckpoints, models.ErrExecutorInvalidConfig},
		{"recovery phase under a cleanup claim", func(_ *pluginExecutorClaimStoreFake, r *pluginsdk.CheckpointExecutorResourceRequest) {
			r.Phase = "recovery_progress"
		}, declaresDestroyCheckpoints, models.ErrExecutorInvalidConfig},
		{"released claim", func(s *pluginExecutorClaimStoreFake, _ *pluginsdk.CheckpointExecutorResourceRequest) { s.claim = nil }, declaresDestroyCheckpoints, models.ErrExecutorClaimNotFound},
		{"successor claim of another operation", func(s *pluginExecutorClaimStoreFake, _ *pluginsdk.CheckpointExecutorResourceRequest) {
			s.claim.OperationID = "operation-successor:cleanup"
		}, declaresDestroyCheckpoints, models.ErrExecutorClaimNotFound},
		{"replaced generation", func(_ *pluginExecutorClaimStoreFake, r *pluginsdk.CheckpointExecutorResourceRequest) {
			r.Context.EnvironmentGeneration = 8
		}, declaresDestroyCheckpoints, models.ErrExecutorClaimNotFound},
		{"claim of another generation", func(s *pluginExecutorClaimStoreFake, _ *pluginsdk.CheckpointExecutorResourceRequest) {
			s.claim.OwnershipGeneration = 8
		}, declaresDestroyCheckpoints, models.ErrExecutorClaimNotFound},
		{"rotated execution", func(_ *pluginExecutorClaimStoreFake, r *pluginsdk.CheckpointExecutorResourceRequest) {
			r.Context.ExecutionId = "execution-other"
		}, declaresDestroyCheckpoints, models.ErrExecutorClaimNotFound},
		{"capability not negotiated", func(*pluginExecutorClaimStoreFake, *pluginsdk.CheckpointExecutorResourceRequest) {}, nil, models.ErrExecutorConflict},
		{"state outside the schema", func(_ *pluginExecutorClaimStoreFake, r *pluginsdk.CheckpointExecutorResourceRequest) {
			r.Resource.StateJson = `{"unexpected":"field"}`
		}, declaresDestroyCheckpoints, nil},
		{"envelope over the cap", func(_ *pluginExecutorClaimStoreFake, r *pluginsdk.CheckpointExecutorResourceRequest) {
			r.Resource.StateJson = `{"resource":"` + strings.Repeat("x", 13000) + `"}`
		}, declaresDestroyCheckpoints, models.ErrExecutorStateTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager, store, request := claimCheckpointFixture(t, tc.negot)
			tc.mutate(store, request)
			err := manager.CheckpointExecutorResource(context.Background(), request)
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if store.writes != 0 {
				t.Fatal("a refused claim checkpoint wrote")
			}
		})
	}
}

func TestClaimCheckpointFailsClosedWithoutAStore(t *testing.T) {
	manager, _, request := claimCheckpointFixture(t, declaresDestroyCheckpoints)
	manager.pluginExecutorClaimStore = nil
	if err := manager.CheckpointExecutorResource(context.Background(), request); err == nil {
		t.Fatal("a claim checkpoint without a store was accepted")
	}
}

func TestClaimCheckpointKeepsTheRecordedWorkspaceDescriptor(t *testing.T) {
	manager, store, request := claimCheckpointFixture(t, []string{"destroy_checkpoints@1", "workspace_descriptor@1"})
	request.Resource.WorkspacePath, request.Resource.WorkspaceIdentity = "/srv/work", "worker-1"
	inventory, _ := decodePluginExecutorInventory(store.record.Metadata)
	inventory.WorkspaceDescriptor = nil
	store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	if err := manager.CheckpointExecutorResource(context.Background(), request); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	envelope := store.written.Metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory)
	if envelope.WorkspaceDescriptor == nil || envelope.WorkspaceDescriptor.Path != "/srv/work" {
		t.Fatalf("descriptor = %+v", envelope.WorkspaceDescriptor)
	}
	request.Resource.WorkspacePath = "/srv/other"
	inventory.WorkspaceDescriptor = envelope.WorkspaceDescriptor
	store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	if err := manager.CheckpointExecutorResource(context.Background(), request); !errors.Is(err, models.ErrExecutorConflict) {
		t.Fatalf("a changed descriptor: err = %v, want conflict", err)
	}
}
