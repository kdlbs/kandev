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

func TestManagedConversationLifetime(t *testing.T) {
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
	capabilities := []string{
		"host.v2.read:managed_agent_conversations", "host.v2.write:managed_agent_conversations",
	}
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-one", 1, digest, capabilities, "human", "grant", "approval-one"); err != nil {
		t.Fatalf("grant managed conversation capabilities: %v", err)
	}
	managed := &managedConversationServiceFake{conversations: make(map[string]pluginsdk.ManagedAgentConversationDescriptor)}
	svc.SetManagedAgentConversations(managed)
	host := svc.hostForPlugin(record.ID).(pluginsdk.ExactHost)
	manager := host.ManagedAgentConversations()
	input := pluginsdk.ManagedAgentConversationSpec{
		RequestID: "request-one", IdempotencyKey: "ensure-one", WorkspaceID: "workspace-one",
		InstanceKey: "lead", ApprovalRevision: 1, ManifestDigest: digest,
		AgentProfileID: "profile-one", ExecutorID: "executor-one", ExecutorProfileID: "executor-profile-one",
		BasePrompt: "Coordinate assigned work", InstructionVersion: "prompt-v1",
	}
	created, first, err := manager.Ensure(context.Background(), input)
	if err != nil || created.Status != pluginsdk.CommandApplied || created.Receipt == nil || first.Revision != 1 {
		t.Fatalf("Ensure managed conversation = %+v %+v, err=%v", created, first, err)
	}
	replayed, same, err := manager.Ensure(context.Background(), input)
	if err != nil || replayed.Status != created.Status || same.TaskID != first.TaskID {
		t.Fatalf("Ensure retry = %+v %+v, err=%v", replayed, same, err)
	}
	conflictInput := input
	conflictInput.BasePrompt = "different payload under the same key"
	conflict, _, err := manager.Ensure(context.Background(), conflictInput)
	if err != nil || conflict.Status != pluginsdk.CommandConflict {
		t.Fatalf("same-key payload conflict = %+v, err=%v", conflict, err)
	}
	observed, err := manager.Get(context.Background(), pluginsdk.ManagedAgentConversationQuery{
		WorkspaceID: "workspace-one", InstanceKey: "lead", ApprovalRevision: 1, ManifestDigest: digest,
	})
	if err != nil || observed.TaskID != first.TaskID {
		t.Fatalf("Get managed conversation = %+v, err=%v", observed, err)
	}
	paused, updated, err := manager.SetPaused(context.Background(), pluginsdk.ManagedAgentConversationPause{
		RequestID: "request-pause", IdempotencyKey: "pause-one", WorkspaceID: "workspace-one", InstanceKey: "lead",
		ExpectedRevision: first.Revision, ApprovalRevision: 1, ManifestDigest: digest, Paused: true,
	})
	if err != nil || paused.Status != pluginsdk.CommandApplied || !updated.DesiredPaused || updated.Revision != 2 {
		t.Fatalf("SetPaused managed conversation = %+v %+v, err=%v", paused, updated, err)
	}
	deleteInput := pluginsdk.ManagedAgentConversationSpec{
		RequestID: "request-delete-ensure", IdempotencyKey: "ensure-delete", WorkspaceID: "workspace-one",
		InstanceKey: "delete-me", ApprovalRevision: 1, ManifestDigest: digest,
	}
	if result, _, err := manager.Ensure(context.Background(), deleteInput); err != nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("Ensure delete-test conversation = %+v, err=%v", result, err)
	}
	managed.failDeleteAfterCommit = true
	deleteRequest := pluginsdk.ManagedAgentConversationDelete{
		RequestID: "request-delete", IdempotencyKey: "delete-one", WorkspaceID: "workspace-one",
		InstanceKey: "delete-me", ExpectedRevision: 1, ApprovalRevision: 1, ManifestDigest: digest,
	}
	firstDelete, err := manager.Delete(context.Background(), deleteRequest)
	if err != nil || firstDelete.Status != pluginsdk.CommandUnavailable {
		t.Fatalf("Delete with lost response = %+v, err=%v; want pending unavailable", firstDelete, err)
	}
	replayedDelete, err := manager.Delete(context.Background(), deleteRequest)
	if err != nil || replayedDelete.Status != pluginsdk.CommandNotFound {
		t.Fatalf("Delete retry after committed mutation = %+v, err=%v; want NOT_FOUND without durable deletion evidence", replayedDelete, err)
	}
	if _, err := svc.approvalRevoke(record.InstallationID, "workspace-one", "human", "revoke", "approval-revoke"); err != nil {
		t.Fatalf("revoke managed conversation access: %v", err)
	}
	denied, _, err := manager.Ensure(context.Background(), pluginsdk.ManagedAgentConversationSpec{
		RequestID: "request-denied", IdempotencyKey: "ensure-denied", WorkspaceID: "workspace-one", InstanceKey: "other",
		ExpectedRevision: 0, ApprovalRevision: 1, ManifestDigest: digest,
	})
	if err != nil || denied.Status != pluginsdk.CommandDenied {
		t.Fatalf("revoked Ensure = %+v, err=%v, want DENIED", denied, err)
	}
	if got := managed.count(); got != 1 {
		t.Fatalf("conversation count after revoked request = %d, want 1", got)
	}
}

type managedConversationServiceFake struct {
	mu                    sync.Mutex
	conversations         map[string]pluginsdk.ManagedAgentConversationDescriptor
	failDeleteAfterCommit bool
	invalidations         []string
}

func (f *managedConversationServiceFake) EnsureManaged(_ context.Context, _ string, installationID string, spec pluginsdk.ManagedAgentConversationSpec, _, _ string) (pluginsdk.ManagedAgentConversationDescriptor, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := managedConversationHostKey(installationID, spec.WorkspaceID, spec.InstanceKey)
	current, exists := f.conversations[key]
	if !exists {
		if spec.ExpectedRevision != 0 {
			return pluginsdk.ManagedAgentConversationDescriptor{}, "", status.Error(codes.Aborted, "stale revision")
		}
		current = pluginsdk.ManagedAgentConversationDescriptor{
			InstallationID: installationID, TaskID: "task-" + spec.InstanceKey, SessionID: "session-" + spec.InstanceKey,
			WorkspaceID: spec.WorkspaceID, InstanceKey: spec.InstanceKey, Revision: 1,
			RetentionMode: "retain_on_uninstall",
		}
		f.applySpec(&current, spec)
		f.conversations[key] = current
		return current, "created", nil
	}
	if current.Revision != spec.ExpectedRevision {
		return pluginsdk.ManagedAgentConversationDescriptor{}, "", status.Error(codes.Aborted, "stale revision")
	}
	if current.AgentProfileID != spec.AgentProfileID || current.ExecutorID != spec.ExecutorID ||
		current.ExecutorProfileID != spec.ExecutorProfileID || current.BasePrompt != spec.BasePrompt ||
		current.InstructionVersion != spec.InstructionVersion {
		current.Revision++
		f.applySpec(&current, spec)
		f.conversations[key] = current
	}
	return current, "exists", nil
}

func (f *managedConversationServiceFake) applySpec(value *pluginsdk.ManagedAgentConversationDescriptor, spec pluginsdk.ManagedAgentConversationSpec) {
	value.AgentProfileID, value.ExecutorID, value.ExecutorProfileID = spec.AgentProfileID, spec.ExecutorID, spec.ExecutorProfileID
	value.BasePrompt, value.InstructionVersion = spec.BasePrompt, spec.InstructionVersion
}

func (f *managedConversationServiceFake) GetManaged(_ context.Context, installationID, workspaceID, instanceKey string) (pluginsdk.ManagedAgentConversationDescriptor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.conversations[managedConversationHostKey(installationID, workspaceID, instanceKey)]
	if !ok || value.Detached {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.NotFound, "managed conversation not found")
	}
	return value, nil
}

func (f *managedConversationServiceFake) ListManaged(_ context.Context, installationID, workspaceID string) ([]pluginsdk.ManagedAgentConversationDescriptor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []pluginsdk.ManagedAgentConversationDescriptor
	for _, value := range f.conversations {
		if value.InstallationID == installationID && value.WorkspaceID == workspaceID && !value.Detached {
			result = append(result, value)
		}
	}
	return result, nil
}

func (f *managedConversationServiceFake) SetManagedPaused(_ context.Context, installationID, workspaceID, instanceKey string, expectedRevision uint64, paused bool, _, _ string) (pluginsdk.ManagedAgentConversationDescriptor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := managedConversationHostKey(installationID, workspaceID, instanceKey)
	value, ok := f.conversations[key]
	if !ok || value.Detached {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.NotFound, "managed conversation not found")
	}
	if value.Revision != expectedRevision {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.Aborted, "stale revision")
	}
	if value.DesiredPaused != paused {
		value.DesiredPaused = paused
		value.Revision++
		f.conversations[key] = value
	}
	return value, nil
}

func (f *managedConversationServiceFake) DeleteManaged(_ context.Context, installationID, workspaceID, instanceKey string, expectedRevision uint64, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := managedConversationHostKey(installationID, workspaceID, instanceKey)
	value, ok := f.conversations[key]
	if !ok {
		return status.Error(codes.NotFound, "managed conversation not found")
	}
	if value.Revision != expectedRevision {
		return status.Error(codes.Aborted, "stale revision")
	}
	delete(f.conversations, key)
	if f.failDeleteAfterCommit {
		f.failDeleteAfterCommit = false
		return status.Error(codes.Unavailable, "simulated lost delete response")
	}
	return nil
}

func (f *managedConversationServiceFake) PauseManagedForInstallation(_ context.Context, installationID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, value := range f.conversations {
		if value.InstallationID == installationID && !value.Detached {
			value.DesiredPaused = true
			f.conversations[key] = value
		}
	}
	return nil
}

func (f *managedConversationServiceFake) InvalidateManagedForInstallationWorkspace(_ context.Context, installationID, workspaceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidations = append(f.invalidations, installationID+"/"+workspaceID)
	return nil
}

func (f *managedConversationServiceFake) DetachManagedForInstallation(_ context.Context, installationID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, value := range f.conversations {
		if value.InstallationID == installationID && !value.Detached {
			value.Detached = true
			value.DesiredPaused = true
			value.Revision++
			f.conversations[key] = value
		}
	}
	return nil
}

func (f *managedConversationServiceFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.conversations)
}

func managedConversationHostKey(installationID, workspaceID, instanceKey string) string {
	return installationID + "/" + workspaceID + "/" + instanceKey
}

func TestManagedConversationInactiveInstallationCannotUseOldHost(t *testing.T) {
	registry := NewRegistry()
	registry.Add(&pluginstore.Record{Manifest: manifest.Manifest{ID: "coordinator"}, InstallationID: "installation-stopped", Status: StatusDisabled})
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	defer func() { _ = svc.Close() }()
	managed := &managedConversationServiceFake{conversations: make(map[string]pluginsdk.ManagedAgentConversationDescriptor)}
	svc.SetManagedAgentConversations(managed)
	manager := svc.hostForPlugin("coordinator").(pluginsdk.ExactHost).ManagedAgentConversations()
	result, _, err := manager.Ensure(context.Background(), pluginsdk.ManagedAgentConversationSpec{
		RequestID: "request-one", IdempotencyKey: "ensure-one", WorkspaceID: "workspace-one", InstanceKey: "lead",
		ApprovalRevision: 1, ManifestDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil || result.Status != pluginsdk.CommandDenied || managed.count() != 0 {
		t.Fatalf("disabled installation result=%+v err=%v conversations=%d, want DENIED without mutation", result, err, managed.count())
	}
}

func TestManagedConversationDisableKeepsInstallationOwnedHistory(t *testing.T) {
	svc, _, _ := newTestService(t)
	record := installTestPlugin(t, svc, "kandev-plugin-coordinator")
	managed := seededManagedConversation(record.InstallationID)
	svc.SetManagedAgentConversations(managed)
	if err := svc.Disable(record.ID); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	value := managed.conversation(record.InstallationID, "workspace-one", "lead")
	if value == nil || value.Detached || !value.DesiredPaused {
		t.Fatalf("conversation after disable = %+v, want retained, attached, and paused", value)
	}
}

func TestManagedConversationUninstallDetachesAndReinstallCannotAdopt(t *testing.T) {
	svc, _, _ := newTestService(t)
	first := installTestPlugin(t, svc, "kandev-plugin-coordinator")
	managed := seededManagedConversation(first.InstallationID)
	svc.SetManagedAgentConversations(managed)
	if err := svc.Uninstall(context.Background(), first.ID); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	retained := managed.conversation(first.InstallationID, "workspace-one", "lead")
	if retained == nil || !retained.Detached || !retained.DesiredPaused || retained.Revision != 4 {
		t.Fatalf("conversation after uninstall = %+v, want retained, detached, paused, and revisioned", retained)
	}
	if _, err := managed.GetManaged(context.Background(), first.InstallationID, "workspace-one", "lead"); status.Code(err) != codes.NotFound {
		t.Fatalf("uninstalled plugin read = %v, want NotFound", err)
	}

	second := installTestPlugin(t, svc, first.ID)
	if second.InstallationID == first.InstallationID {
		t.Fatalf("reinstall reused installation identity %q", second.InstallationID)
	}
	if _, err := managed.GetManaged(context.Background(), second.InstallationID, "workspace-one", "lead"); status.Code(err) != codes.NotFound {
		t.Fatalf("reinstalled plugin read of old conversation = %v, want NotFound", err)
	}
}

func seededManagedConversation(installationID string) *managedConversationServiceFake {
	return &managedConversationServiceFake{conversations: map[string]pluginsdk.ManagedAgentConversationDescriptor{
		managedConversationHostKey(installationID, "workspace-one", "lead"): {
			InstallationID: installationID, TaskID: "task-lead", SessionID: "session-lead",
			WorkspaceID: "workspace-one", InstanceKey: "lead", Revision: 3,
			RetentionMode: "retain_on_uninstall",
		},
	}}
}

func (f *managedConversationServiceFake) conversation(installationID, workspaceID, instanceKey string) *pluginsdk.ManagedAgentConversationDescriptor {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.conversations[managedConversationHostKey(installationID, workspaceID, instanceKey)]
	if !ok {
		return nil
	}
	return &value
}
