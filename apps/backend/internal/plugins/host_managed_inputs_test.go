package plugins

import (
	"context"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// @covers AC-PLUGINS-MANAGED-COORDINATION-003.1
// @covers AC-PLUGINS-MANAGED-COORDINATION-003.2
func TestManagedInputReceipts(t *testing.T) {
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open command database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	commandStore, err := state.NewCommandStore(db.NewPool(connection, connection))
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	record := &pluginstore.Record{
		Manifest: manifest.Manifest{ID: "coordinator", Capabilities: manifest.Capabilities{
			APIRead: []string{"managed_agent_conversations"}, APIWrite: []string{"managed_agent_conversations"},
		}},
		InstallationID: "installation-one", Status: StatusActive,
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = svc.Close() })
	digest := ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-one", 1, digest, []string{
		"host.v2.read:managed_agent_conversations", "host.v2.write:managed_agent_conversations",
	}, "human", "grant", "approval-one"); err != nil {
		t.Fatalf("grant managed conversation capabilities: %v", err)
	}
	managed := newManagedInputServiceFake()
	svc.SetManagedAgentConversations(managed)
	host := svc.hostForPlugin(record.ID).(pluginsdk.ExactHost)
	manager := host.ManagedAgentConversations()
	ctx := context.Background()

	ensured, conversation, err := manager.Ensure(ctx, pluginsdk.ManagedAgentConversationSpec{
		RequestID: "ensure-request", IdempotencyKey: "ensure-key", WorkspaceID: "workspace-one",
		InstanceKey: "lead", ApprovalRevision: 1, ManifestDigest: digest,
	})
	if err != nil || ensured.Status != pluginsdk.CommandApplied || conversation.Revision != 1 {
		t.Fatalf("Ensure conversation = %+v %+v, err=%v", ensured, conversation, err)
	}

	input := pluginsdk.ManagedAgentInputEnqueue{
		RequestID: "enqueue-request", IdempotencyKey: "enqueue-key", WorkspaceID: "workspace-one",
		InstanceKey: "lead", ExpectedConversationRevision: conversation.Revision,
		ApprovalRevision: 1, ManifestDigest: digest, OccurrenceKey: "human-turn-one",
		Origin: pluginsdk.ManagedAgentInputHuman, Payload: "Please inspect the latest run.",
	}
	accepted, first, err := manager.EnqueueInput(ctx, input)
	if err != nil || accepted.Status != pluginsdk.CommandApplied || first.State != pluginsdk.ManagedAgentInputAccepted {
		t.Fatalf("EnqueueInput = %+v %+v, err=%v", accepted, first, err)
	}
	if first.HostInputID == "" || first.Sequence == 0 || first.Payload != input.Payload {
		t.Fatalf("input receipt is incomplete: %+v", first)
	}

	got, err := manager.GetInput(ctx, pluginsdk.ManagedAgentInputQuery{
		WorkspaceID: input.WorkspaceID, InstanceKey: input.InstanceKey, HostInputID: first.HostInputID,
		ApprovalRevision: 1, ManifestDigest: digest,
	})
	if err != nil || got.HostInputID != first.HostInputID {
		t.Fatalf("GetInput = %+v, err=%v", got, err)
	}
	page, err := manager.ListInputs(ctx, pluginsdk.ManagedAgentInputListQuery{
		WorkspaceID: input.WorkspaceID, InstanceKey: input.InstanceKey, Limit: 20,
		ApprovalRevision: 1, ManifestDigest: digest,
	})
	if err != nil || len(page.Inputs) != 1 || page.Inputs[0].HostInputID != first.HostInputID {
		t.Fatalf("ListInputs = %+v, err=%v", page, err)
	}

	retried, replay, err := manager.EnqueueInput(ctx, input)
	if err != nil || retried.Status != accepted.Status || replay.HostInputID != first.HostInputID || managed.inputCount() != 1 {
		t.Fatalf("idempotent enqueue = %+v %+v, count=%d, err=%v", retried, replay, managed.inputCount(), err)
	}

	conflicting := input
	conflicting.Payload = "different payload"
	conflict, _, err := manager.EnqueueInput(ctx, conflicting)
	if err != nil || conflict.Status != pluginsdk.CommandConflict {
		t.Fatalf("same-key payload conflict = %+v, err=%v", conflict, err)
	}

	cancelled, receipt, err := manager.CancelInput(ctx, pluginsdk.ManagedAgentInputCancel{
		RequestID: "cancel-request", IdempotencyKey: "cancel-key", WorkspaceID: input.WorkspaceID,
		InstanceKey: input.InstanceKey, HostInputID: first.HostInputID,
		ExpectedConversationRevision: conversation.Revision, ApprovalRevision: 1, ManifestDigest: digest,
	})
	if err != nil || cancelled.Status != pluginsdk.CommandApplied || receipt.State != pluginsdk.ManagedAgentInputCancelled {
		t.Fatalf("CancelInput = %+v %+v, err=%v", cancelled, receipt, err)
	}

	if _, err := svc.approvalRevoke(record.InstallationID, "workspace-one", "human", "revoke", "approval-revoke"); err != nil {
		t.Fatalf("revoke managed conversation access: %v", err)
	}
	_, err = manager.GetInput(ctx, pluginsdk.ManagedAgentInputQuery{
		WorkspaceID: input.WorkspaceID, InstanceKey: input.InstanceKey, HostInputID: first.HostInputID,
		ApprovalRevision: 1, ManifestDigest: digest,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetInput after approval revocation error = %v, want permission denied", err)
	}
}

type managedInputServiceFake struct {
	*managedConversationServiceFake
	inputMu sync.Mutex
	inputs  map[string]pluginsdk.ManagedAgentInputReceipt
}

func newManagedInputServiceFake() *managedInputServiceFake {
	return &managedInputServiceFake{
		managedConversationServiceFake: &managedConversationServiceFake{conversations: make(map[string]pluginsdk.ManagedAgentConversationDescriptor)},
		inputs:                         make(map[string]pluginsdk.ManagedAgentInputReceipt),
	}
}

func (f *managedInputServiceFake) EnqueueManagedInput(_ context.Context, installationID, hostInputID string, input pluginsdk.ManagedAgentInputEnqueue, _, _ string) (pluginsdk.ManagedAgentInputReceipt, bool, error) {
	f.inputMu.Lock()
	defer f.inputMu.Unlock()
	key := managedInputFakeKey(installationID, input.WorkspaceID, input.InstanceKey, hostInputID)
	if current, ok := f.inputs[key]; ok {
		if current.Payload != input.Payload || current.OccurrenceKey != input.OccurrenceKey {
			return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.AlreadyExists, "occurrence payload differs")
		}
		return current, true, nil
	}
	current := pluginsdk.ManagedAgentInputReceipt{
		HostInputID: hostInputID, OccurrenceKey: input.OccurrenceKey, Sequence: uint64(len(f.inputs) + 1),
		Origin: input.Origin, Payload: input.Payload, CoalesceKey: input.CoalesceKey,
		ConversationRevision: input.ExpectedConversationRevision, State: pluginsdk.ManagedAgentInputAccepted,
	}
	f.inputs[key] = current
	return current, false, nil
}

func (f *managedInputServiceFake) GetManagedInput(_ context.Context, installationID string, query pluginsdk.ManagedAgentInputQuery) (pluginsdk.ManagedAgentInputReceipt, error) {
	f.inputMu.Lock()
	defer f.inputMu.Unlock()
	current, ok := f.inputs[managedInputFakeKey(installationID, query.WorkspaceID, query.InstanceKey, query.HostInputID)]
	if !ok {
		return pluginsdk.ManagedAgentInputReceipt{}, status.Error(codes.NotFound, "input not found")
	}
	return current, nil
}

func (f *managedInputServiceFake) ListManagedInputs(_ context.Context, installationID string, query pluginsdk.ManagedAgentInputListQuery) (pluginsdk.ManagedAgentInputPage, error) {
	f.inputMu.Lock()
	defer f.inputMu.Unlock()
	page := pluginsdk.ManagedAgentInputPage{Inputs: make([]pluginsdk.ManagedAgentInputReceipt, 0)}
	for key, current := range f.inputs {
		prefix := installationID + "/" + query.WorkspaceID + "/" + query.InstanceKey + "/"
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix && current.Sequence > query.SequenceCursor {
			page.Inputs = append(page.Inputs, current)
		}
	}
	return page, nil
}

func (f *managedInputServiceFake) CancelManagedInput(_ context.Context, installationID string, input pluginsdk.ManagedAgentInputCancel, _, _ string) (pluginsdk.ManagedAgentInputReceipt, bool, error) {
	f.inputMu.Lock()
	defer f.inputMu.Unlock()
	key := managedInputFakeKey(installationID, input.WorkspaceID, input.InstanceKey, input.HostInputID)
	current, ok := f.inputs[key]
	if !ok {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.NotFound, "input not found")
	}
	if current.State == pluginsdk.ManagedAgentInputAccepted {
		current.State = pluginsdk.ManagedAgentInputCancelled
		f.inputs[key] = current
		return current, true, nil
	}
	return current, false, status.Error(codes.FailedPrecondition, "input is no longer accepted")
}

func (f *managedInputServiceFake) DispatchManagedInput(context.Context, string, pluginsdk.ManagedAgentConversationDispatch, string, string) (pluginsdk.ManagedAgentDispatchStatus, pluginsdk.ManagedAgentConversationDescriptor, error) {
	return pluginsdk.ManagedAgentDispatchBusy, pluginsdk.ManagedAgentConversationDescriptor{}, nil
}

func (f *managedInputServiceFake) inputCount() int {
	f.inputMu.Lock()
	defer f.inputMu.Unlock()
	return len(f.inputs)
}

func managedInputFakeKey(installationID, workspaceID, instanceKey, inputID string) string {
	return installationID + "/" + workspaceID + "/" + instanceKey + "/" + inputID
}
