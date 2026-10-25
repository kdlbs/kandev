package plugins

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestListManagedConversationDestinationsAvailabilityAndScope(t *testing.T) {
	svc, _, _ := newTestService(t)
	record := &pluginstore.Record{
		Manifest: manifest.Manifest{ID: "kandev-plugin-coordinator", DisplayName: "Coordinator", Capabilities: manifest.Capabilities{
			APIRead: []string{"managed_agent_conversations"}, APIWrite: []string{"managed_agent_conversations"},
		}},
		InstallationID: "coordinator-installation", Status: StatusActive,
	}
	svc.registry.Add(record)
	managed := &managedConversationServiceFake{conversations: map[string]pluginsdk.ManagedAgentConversationDescriptor{
		managedConversationHostKey(record.InstallationID, "workspace-targets", "daily-brief"): {
			InstallationID: record.InstallationID, WorkspaceID: "workspace-targets", InstanceKey: "daily-brief",
			Revision: 4, DesiredPaused: true, TaskID: "hidden-task-id",
		},
		managedConversationHostKey(record.InstallationID, "other-workspace", "private"): {
			InstallationID: record.InstallationID, WorkspaceID: "other-workspace", InstanceKey: "private", Revision: 1,
		},
	}}
	svc.SetManagedAgentConversations(managed)

	items, err := svc.ListManagedConversationDestinations(context.Background(), "workspace-targets")
	if err != nil || len(items) != 1 {
		t.Fatalf("destinations = %+v, err=%v; want one workspace-local destination", items, err)
	}
	if items[0].PluginID != record.ID || items[0].PluginName != record.DisplayName || items[0].Revision != 4 || !items[0].Paused {
		t.Fatalf("destination projection = %+v", items[0])
	}
	if items[0].UnavailableReason == "" {
		t.Fatal("destination without approval must remain visible as unavailable")
	}

	digest := ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-targets", 1, digest,
		[]string{"host.v2.write:managed_agent_conversations"}, "human", "approve", "destination-grant"); err != nil {
		t.Fatalf("grant destination capability: %v", err)
	}
	items, err = svc.ListManagedConversationDestinations(context.Background(), "workspace-targets")
	if err != nil || len(items) != 1 || items[0].UnavailableReason != "" {
		t.Fatalf("approved destinations = %+v, err=%v", items, err)
	}
	encoded, err := json.Marshal(items[0])
	if err != nil {
		t.Fatalf("marshal destination: %v", err)
	}
	if string(encoded) == "" || containsAny(string(encoded), "hidden-task-id", record.InstallationID, "session_id") {
		t.Fatalf("destination API projection leaked private identity: %s", encoded)
	}

	if _, err := svc.RevokeCapabilityApproval(record.InstallationID, "workspace-targets", 1, "human", "revoke", "destination-revoke"); err != nil {
		t.Fatalf("revoke destination capability: %v", err)
	}
	items, err = svc.ListManagedConversationDestinations(context.Background(), "workspace-targets")
	if err != nil || len(items) != 1 || items[0].UnavailableReason == "" {
		t.Fatalf("revoked destination = %+v, err=%v", items, err)
	}
}

func TestResolveManagedConversationDestinationJoinsScheduleApprovalTransaction(t *testing.T) {
	svc, _, _ := newTestService(t)
	record := &pluginstore.Record{
		Manifest: manifest.Manifest{ID: "coordinator", Capabilities: manifest.Capabilities{
			APIRead: []string{"managed_agent_conversations"}, APIWrite: []string{"managed_agent_conversations"},
		}},
		InstallationID: "coordinator-installation", Status: StatusActive,
	}
	svc.registry.Add(record)
	workspaceID := "workspace-schedule-approval"
	digest := ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.approvalGrant(record.InstallationID, workspaceID, 1, digest,
		[]string{"host.v2.write:managed_agent_conversations"}, "human", "grant", "schedule-destination-grant"); err != nil {
		t.Fatalf("grant destination capability: %v", err)
	}
	svc.SetManagedAgentConversations(&managedConversationServiceFake{conversations: map[string]pluginsdk.ManagedAgentConversationDescriptor{
		managedConversationHostKey(record.InstallationID, workspaceID, "release-lead"): {
			InstallationID: record.InstallationID, WorkspaceID: workspaceID, InstanceKey: "release-lead",
			Revision: 3, TaskID: "managed-task",
		},
	}})

	// The schedule Host command holds this lock across authorization and its
	// durable side effect. Destination validation must reuse that transaction
	// instead of trying to acquire the non-reentrant lock a second time.
	svc.approvalEffectMu.Lock()
	type result struct {
		installationID string
		taskID         string
		err            error
	}
	done := make(chan result, 1)
	go func() {
		installationID, taskID, _, err := svc.ResolveManagedConversationDestination(
			withManagedScheduleApprovalLock(context.Background(), "schedule-owner-installation"),
			workspaceID, record.ID, "release-lead", 3,
		)
		done <- result{installationID: installationID, taskID: taskID, err: err}
	}()
	select {
	case got := <-done:
		svc.approvalEffectMu.Unlock()
		if got.err != nil || got.installationID != record.InstallationID || got.taskID != "managed-task" {
			t.Fatalf("resolved destination = %q/%q, err=%v", got.installationID, got.taskID, got.err)
		}
	case <-time.After(time.Second):
		svc.approvalEffectMu.Unlock()
		t.Fatal("destination validation deadlocked while joining the schedule approval transaction")
	}
}

func containsAny(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if fragment != "" && strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
