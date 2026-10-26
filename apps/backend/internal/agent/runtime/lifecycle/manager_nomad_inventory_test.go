package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/task/models"
)

type nomadInventoryStoreFake struct {
	row          *models.ExecutorRunning
	checkpoints  int
	lastExpected uint64
}

func (f *nomadInventoryStoreFake) CheckpointNomadExecutorInventory(_ context.Context, running *models.ExecutorRunning) error {
	f.checkpoints++
	f.lastExpected = running.ExpectedNomadExecutorRevision
	state, err := nomad.EnvironmentStateFromMetadata(running.Metadata)
	if err != nil {
		return err
	}
	if f.row == nil {
		state.Revision = 1
	} else {
		state.Revision = f.lastExpected + 1
	}
	if err := nomad.PutEnvironmentState(running.Metadata, state); err != nil {
		return err
	}
	running.ExpectedNomadExecutorRevision = state.Revision
	f.row = running
	return nil
}

func (f *nomadInventoryStoreFake) GetExecutorRunningBySessionID(_ context.Context, sessionID string) (*models.ExecutorRunning, error) {
	if f.row == nil || f.row.SessionID != sessionID {
		return nil, models.ErrExecutorRunningNotFound
	}
	return f.row, nil
}

func (*nomadInventoryStoreFake) UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error {
	return nil
}

func (*nomadInventoryStoreFake) DeleteExecutorRunningBySessionID(context.Context, string) error {
	return nil
}

func (*nomadInventoryStoreFake) RepairExecutorRunningDead(context.Context, string) error {
	return nil
}

func TestWireNomadInventoryPersistenceCheckpointsTypedStateAndRequiresReceiptToRelease(t *testing.T) {
	store := &nomadInventoryStoreFake{}
	manager := &Manager{runningWriter: store}
	request := nomadInventoryRequest("execution-1")
	request.Metadata["nomad_token"] = "$kandev-secret:secret-ref-nomad"
	request.Metadata["nomad_token_value"] = "resolved-token-must-not-persist"
	request.Metadata["ssh_private_key"] = "private-key-must-not-persist"
	request.Metadata["agentctl_bootstrap_nonce"] = "nonce-must-not-persist"
	manager.wireNomadInventoryPersistence(request, string(models.ExecutorTypeNomad))
	if request.CheckpointRuntimeInventory == nil || request.ReleaseRuntimeInventory == nil {
		t.Fatal("fresh Nomad request did not receive checkpoint and guarded-release callbacks")
	}
	intent := nomadInventoryState("execution-1", "intent")
	metadata := map[string]interface{}{}
	if err := nomad.PutEnvironmentState(metadata, intent); err != nil {
		t.Fatal(err)
	}
	if err := request.CheckpointRuntimeInventory(context.Background(), metadata); err != nil {
		t.Fatalf("initial checkpoint: %v", err)
	}
	registered := nomadInventoryState("execution-1", "registered")
	request.Metadata[MetadataKeyExecutorType] = string(models.ExecutorTypeNomad)
	metadata = map[string]interface{}{}
	if err := nomad.PutEnvironmentState(metadata, registered); err != nil {
		t.Fatal(err)
	}
	if err := request.CheckpointRuntimeInventory(context.Background(), metadata); err != nil {
		t.Fatalf("registered checkpoint: %v", err)
	}
	if store.checkpoints != 2 || store.lastExpected != 1 {
		t.Fatalf("checkpoint count/revision = %d/%d, want 2/1", store.checkpoints, store.lastExpected)
	}
	state, err := nomad.EnvironmentStateFromMetadata(store.row.Metadata)
	if err != nil || state.Phase != "registered" || state.Revision != 2 {
		t.Fatalf("stored state = %+v, %v", state, err)
	}
	if store.row.Metadata[MetadataKeyExecutorType] != string(models.ExecutorTypeNomad) {
		t.Fatalf("checkpoint lost unrelated metadata: %#v", store.row.Metadata)
	}
	if store.row.Metadata["nomad_token"] != "$kandev-secret:secret-ref-nomad" {
		t.Fatalf("checkpoint lost the vault reference: %#v", store.row.Metadata)
	}
	for _, key := range []string{"nomad_token_value", "ssh_private_key", "agentctl_bootstrap_nonce"} {
		if _, found := store.row.Metadata[key]; found {
			t.Fatalf("checkpoint persisted transient secret field %q", key)
		}
	}
	if err := request.ReleaseRuntimeInventory(context.Background()); err == nil {
		t.Fatal("release accepted inventory without positive absence receipt")
	}
	state.Phase = "absent"
	state.DrainReceiptDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	state.DrainReceiptGeneration = state.Generation
	if err := nomad.PutEnvironmentState(store.row.Metadata, state); err != nil {
		t.Fatal(err)
	}
	if err := request.ReleaseRuntimeInventory(context.Background()); err != nil {
		t.Fatalf("release with positive receipt: %v", err)
	}
	if store.row == nil {
		t.Fatal("release deleted the retained absence tombstone")
	}
}

func TestWireNomadInventoryPersistenceDoesNotAttachToOtherRuntime(t *testing.T) {
	manager := &Manager{runningWriter: &nomadInventoryStoreFake{}}
	request := nomadInventoryRequest("execution-1")
	manager.wireNomadInventoryPersistence(request, string(models.ExecutorTypeSSH))
	if request.CheckpointRuntimeInventory != nil || request.ReleaseRuntimeInventory != nil {
		t.Fatal("Nomad inventory callbacks were attached to SSH runtime")
	}
}

func TestWireNomadInventoryPersistenceRejectsMissingTypedState(t *testing.T) {
	manager := &Manager{runningWriter: &nomadInventoryStoreFake{}}
	request := nomadInventoryRequest("execution-1")
	manager.wireNomadInventoryPersistence(request, string(models.ExecutorTypeNomad))
	err := request.CheckpointRuntimeInventory(context.Background(), map[string]interface{}{})
	if err == nil || errors.Is(err, models.ErrExecutionRotated) {
		t.Fatalf("checkpoint error = %v, want missing typed-state error", err)
	}
}

func TestNomadRuntimeSecretPersistsOnlyReferenceAndRejectsCredentialRotation(t *testing.T) {
	store := &nomadInventoryStoreFake{}
	secrets := newInMemorySecretStore()
	manager := &Manager{runningWriter: store, secretStore: secrets}
	request := nomadInventoryRequest("execution-secret")
	manager.wireNomadInventoryPersistence(request, string(models.ExecutorTypeNomad))
	intent := nomadInventoryState("execution-secret", "intent")
	if err := nomad.PutEnvironmentState(request.Metadata, intent); err != nil {
		t.Fatal(err)
	}
	if err := request.CheckpointRuntimeInventory(context.Background(), request.Metadata); err != nil {
		t.Fatal(err)
	}

	const nonce = "one-time-bootstrap-nonce"
	if err := request.PersistNomadRuntimeSecret(context.Background(), MetadataKeyBootstrapNonceSecret, nonce); err != nil {
		t.Fatalf("persist original nonce: %v", err)
	}
	if err := request.CheckpointRuntimeInventory(context.Background(), request.Metadata); err != nil {
		t.Fatalf("checkpoint nonce reference: %v", err)
	}
	ref := getMetadataString(store.row.Metadata, MetadataKeyBootstrapNonceSecret)
	if ref == "" {
		t.Fatalf("bootstrap secret reference missing from durable inventory: %#v", store.row.Metadata)
	}
	for _, key := range []string{"agentctl_bootstrap_nonce", "nonce", "value"} {
		if _, exists := store.row.Metadata[key]; exists {
			t.Fatalf("raw credential field %q persisted", key)
		}
	}
	got, err := secrets.Reveal(context.Background(), ref)
	if err != nil || got != nonce {
		t.Fatalf("stored nonce reveal = matching:%v, error:%v", got == nonce, err)
	}
	if err := request.PersistNomadRuntimeSecret(context.Background(), MetadataKeyBootstrapNonceSecret, nonce); err != nil {
		t.Fatalf("same-operation retry did not reuse original nonce: %v", err)
	}
	if err := request.PersistNomadRuntimeSecret(context.Background(), MetadataKeyBootstrapNonceSecret, "different-nonce"); err == nil {
		t.Fatal("same operation accepted a rotated bootstrap nonce")
	}
}

func TestRetainedNomadExecutionIdentityRequiresExactCurrentOwnerAndRecoverablePhase(t *testing.T) {
	state := nomadInventoryState("execution-1", "registered")
	state.OperationID = "execution-1"
	metadata := map[string]interface{}{}
	if err := nomad.PutEnvironmentState(metadata, state); err != nil {
		t.Fatal(err)
	}
	info := &WorkspaceInfo{
		ExecutorType: string(models.ExecutorTypeNomad), AgentExecutionID: "execution-1",
		TaskEnvironmentID: "environment-1", ValidatedTaskEnvironmentGeneration: 1, Metadata: metadata,
	}
	if id, retained, err := retainedNomadExecutionIdentity(info); err != nil || !retained || id != "execution-1" {
		t.Fatalf("retained identity = %q, %v, %v", id, retained, err)
	}
	intentState := state
	intentState.Phase = "intent"
	intentMetadata := map[string]interface{}{}
	if err := nomad.PutEnvironmentState(intentMetadata, intentState); err != nil {
		t.Fatal(err)
	}
	intentInfo := *info
	intentInfo.Metadata = intentMetadata
	if id, retained, err := retainedNomadExecutionIdentity(&intentInfo); err != nil || !retained || id != "execution-1" {
		t.Fatalf("ambiguous intent identity = %q, %v, %v; committed stable-job reconciliation must retain the original operation", id, retained, err)
	}
	readyState := state
	readyState.Phase = "ready"
	readyMetadata := map[string]interface{}{}
	if err := nomad.PutEnvironmentState(readyMetadata, readyState); err != nil {
		t.Fatal(err)
	}
	readyInfo := *info
	readyInfo.Metadata = readyMetadata
	if id, retained, err := retainedNomadExecutionIdentity(&readyInfo); err != nil || !retained || id != "execution-1" {
		t.Fatalf("ready pre-ACP identity = %q, %v, %v; must attach the existing agentctl instance without creating a replacement", id, retained, err)
	}

	wrongOwner := *info
	wrongOwner.TaskEnvironmentID = "other-environment"
	if _, _, err := retainedNomadExecutionIdentity(&wrongOwner); err == nil {
		t.Fatal("retained operation accepted another task environment")
	}
	wrongGeneration := *info
	wrongGeneration.ValidatedTaskEnvironmentGeneration = 2
	if _, _, err := retainedNomadExecutionIdentity(&wrongGeneration); err == nil {
		t.Fatal("retained operation accepted another ownership generation")
	}
	terminal := nomadInventoryState("execution-1", "cleanup_pending")
	terminal.OperationID = "execution-1"
	terminalMetadata := map[string]interface{}{}
	if err := nomad.PutEnvironmentState(terminalMetadata, terminal); err != nil {
		t.Fatal(err)
	}
	wrongPhase := *info
	wrongPhase.Metadata = terminalMetadata
	if _, _, err := retainedNomadExecutionIdentity(&wrongPhase); err == nil {
		t.Fatal("retained operation accepted a cleanup-pending allocation")
	}
}

func nomadInventoryRequest(executionID string) *ExecutorCreateRequest {
	return &ExecutorCreateRequest{
		InstanceID: executionID, TaskID: "task-1", SessionID: "session-1",
		TaskEnvironmentID: "environment-1", AgentProfileID: "profile-1",
		Metadata: map[string]interface{}{"executor_id": "executor-1", MetadataKeyExecutorType: string(models.ExecutorTypeNomad)},
	}
}

func nomadInventoryState(executionID, phase string) nomad.EnvironmentState {
	return nomad.EnvironmentState{
		Version: 1, Revision: 0, EnvironmentID: "environment-1", Generation: 1,
		OperationID: "operation-1", InstallationID: "installation-1", ExecutionID: executionID,
		InputDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", JobID: "job-1",
		Namespace: "default", Region: "global", Datacenter: "dc1", NodeID: "node-1", Platform: "darwin/arm64",
		SSHHost: "worker.internal", SSHUser: "runner", SSHPort: 22,
		SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		WorkspaceRoot:      "/srv/kandev/workspaces", WorkspacePath: "/srv/kandev/workspaces/environments/environment-1/g1",
		AllocationRoot: "/srv/kandev/allocations",
		CPUMHz:         1000, MemoryMB: 2048, Phase: phase,
	}
}
