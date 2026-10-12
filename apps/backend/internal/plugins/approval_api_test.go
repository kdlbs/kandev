package plugins

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestApprovalAPIExportsCurrentRowsAndDecision(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	if err := svc.SetPluginsDir(dir); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, "digest-a", []string{"host.v2.read:tasks"}, "human", "grant", "audit-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}

	rows, err := svc.ListCapabilityApprovals("inst-1")
	if err != nil {
		t.Fatalf("ListCapabilityApprovals: %v", err)
	}
	if len(rows) != 1 || rows[0].WorkspaceID != "ws-1" {
		t.Fatalf("rows = %#v", rows)
	}

	row, ok, err := svc.GetCapabilityApproval("inst-1", "ws-1")
	if err != nil || !ok {
		t.Fatalf("GetCapabilityApproval: ok=%v err=%v", ok, err)
	}
	if row.Revision != 1 {
		t.Fatalf("row = %#v", row)
	}

	decision := svc.AuthorizeCapability("inst-1", "ws-1", "host.v2.read:tasks", 1, "req", "method")
	if !decision.Allowed {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestApprovalAPIRevokeRetryReplaysOriginalResult(t *testing.T) {
	svc := &Service{}
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.GrantCapabilityApproval("inst-1", "ws-1", 1, "digest-a", []string{"host.v2.read:tasks"}, "human", "grant", "grant-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	first, err := svc.RevokeCapabilityApproval("inst-1", "ws-1", 1, "human", "revoke", "revoke-1")
	if err != nil {
		t.Fatalf("first revoke: %v", err)
	}
	replayed, err := svc.RevokeCapabilityApproval("inst-1", "ws-1", 1, "human", "revoke", "revoke-1")
	if err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	if replayed.Revision != first.Revision || replayed.UpdatedAt != first.UpdatedAt {
		t.Fatalf("retry changed original result: first=%#v replayed=%#v", first, replayed)
	}
	if _, err := svc.RevokeCapabilityApproval("inst-1", "ws-1", 2, "human", "revoke", "revoke-1"); !errors.Is(err, ErrApprovalIdempotencyConflict) {
		t.Fatalf("changed expected revision error = %v, want idempotency conflict", err)
	}
}

func TestGrantCapabilityApprovalRequiresInstalledManifestBinding(t *testing.T) {
	svc := &Service{registry: NewRegistry()}
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	installed := &store.Record{
		Manifest:       manifest.Manifest{ID: "plugin-a", Capabilities: manifest.Capabilities{APIRead: []string{"tasks"}}},
		InstallationID: "inst-1",
	}
	svc.registry.Add(installed)

	_, err := svc.GrantCapabilityApproval("inst-1", "ws-1", 1, ManifestCapabilityDigest(installed.Manifest), []string{"host.v2.read:messages"}, "human", "grant", "audit-1")
	if err == nil {
		t.Fatal("GrantCapabilityApproval accepted a capability outside the installed manifest")
	}
}

func TestApprovalPolicyChangesInvalidateManagedConversationsOnce(t *testing.T) {
	svc := &Service{registry: NewRegistry()}
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	installed := &store.Record{
		Manifest: manifest.Manifest{ID: "plugin-a", Capabilities: manifest.Capabilities{
			APIRead: []string{"managed_agent_conversations"}, APIWrite: []string{"managed_agent_tools"},
		}},
		InstallationID: "inst-1",
	}
	svc.registry.Add(installed)
	managed := &managedConversationServiceFake{conversations: make(map[string]pluginsdk.ManagedAgentConversationDescriptor)}
	svc.SetManagedAgentConversations(managed)
	digest := ManifestCapabilityDigest(installed.Manifest)

	first, err := svc.GrantCapabilityApproval("inst-1", "ws-1", 1, digest, []string{"host.v2.read:managed_agent_conversations"}, "human", "grant", "grant-1")
	if err != nil || first.Revision != 1 {
		t.Fatalf("first grant = %+v, err=%v", first, err)
	}
	if _, err := svc.GrantCapabilityApproval("inst-1", "ws-1", 1, digest, []string{"host.v2.read:managed_agent_conversations"}, "human", "grant", "grant-1"); err != nil {
		t.Fatalf("grant replay: %v", err)
	}
	managed.mu.Lock()
	if len(managed.invalidations) != 1 {
		t.Fatalf("invalidations after idempotent grant replay = %v, want one policy change", managed.invalidations)
	}
	managed.mu.Unlock()

	if _, err := svc.GrantCapabilityApproval("inst-1", "ws-1", 2, digest, []string{"host.v2.write:managed_agent_tools"}, "human", "replace", "grant-2"); err != nil {
		t.Fatalf("replacement grant: %v", err)
	}
	if _, err := svc.RevokeCapabilityApproval("inst-1", "ws-1", 2, "human", "revoke", "revoke-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if len(managed.invalidations) != 3 {
		t.Fatalf("invalidations = %v, want grant, replacement, and revoke", managed.invalidations)
	}
	for _, workspace := range managed.invalidations {
		if workspace != "inst-1/ws-1" {
			t.Fatalf("invalidation target = %q, want installation workspace", workspace)
		}
	}
}

func TestManifestCapabilityDigestPreservesLegacyAndBindsManagedTools(t *testing.T) {
	legacy := manifest.Manifest{Capabilities: manifest.Capabilities{
		APIRead: []string{"tasks"}, APIWrite: []string{"messages"},
	}}
	capabilityIDs, err := ManifestCapabilityIDs(legacy)
	if err != nil {
		t.Fatalf("ManifestCapabilityIDs: %v", err)
	}
	if got, want := ManifestCapabilityDigest(legacy), CanonicalApprovalDigest(capabilityIDs...); got != want {
		t.Fatalf("legacy digest = %q, want stable digest %q", got, want)
	}

	managed := manifest.Manifest{Capabilities: manifest.Capabilities{APIWrite: []string{"managed_agent_tools"}}, AgentTools: []manifest.AgentTool{{
		Name: "read_task", Description: "Read one task.", Surfaces: []string{manifest.AgentToolSurfaceManaged},
		InputSchema: map[string]any{"type": "object"},
	}}}
	first := ManifestCapabilityDigest(managed)
	managed.AgentTools[0].Description = "Read task details."
	if next := ManifestCapabilityDigest(managed); next == first {
		t.Fatal("managed tool declaration change did not change the approval digest")
	}
}

func TestManifestUpgradeInvalidatesManagedConversationPolicy(t *testing.T) {
	svc := &Service{registry: NewRegistry()}
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	installed := &store.Record{
		Manifest: manifest.Manifest{
			ID:           "plugin-a",
			Capabilities: manifest.Capabilities{APIWrite: []string{"managed_agent_tools"}},
			AgentTools: []manifest.AgentTool{{
				Name: "read_task", Description: "Read one task.",
				Surfaces: []string{manifest.AgentToolSurfaceManaged}, InputSchema: map[string]any{"type": "object"},
			}},
		},
		InstallationID: "inst-1",
	}
	svc.registry.Add(installed)
	managed := &managedConversationServiceFake{conversations: make(map[string]pluginsdk.ManagedAgentConversationDescriptor)}
	svc.SetManagedAgentConversations(managed)
	if _, err := svc.GrantCapabilityApproval("inst-1", "ws-1", 1, ManifestCapabilityDigest(installed.Manifest),
		[]string{"host.v2.write:managed_agent_tools"}, "human", "grant", "grant-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	installed.Manifest.AgentTools[0].Description = "Read the current task summary."
	if err := svc.reviewInstalledApprovals(installed); err != nil {
		t.Fatalf("reviewInstalledApprovals: %v", err)
	}
	approval, found, err := svc.GetCapabilityApproval("inst-1", "ws-1")
	if err != nil || !found || approval.Revision != 2 || approval.ManifestDigest != ManifestCapabilityDigest(installed.Manifest) {
		t.Fatalf("reviewed approval = %+v found=%t err=%v", approval, found, err)
	}
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if len(managed.invalidations) != 2 || managed.invalidations[1] != "inst-1/ws-1" {
		t.Fatalf("managed invalidations = %v, want workspace policy invalidation after manifest change", managed.invalidations)
	}
}
