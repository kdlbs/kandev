package plugins

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

func TestApprovalLedgerGrantRevokeAndTombstone(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)

	grantAt := time.Date(2026, time.August, 31, 12, 1, 0, 0, time.UTC)
	approval, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", grantAt)
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if approval.State != ApprovalStateActive || approval.Revision != 1 {
		t.Fatalf("grant approval = %#v", approval)
	}

	fetched, ok, err := ledger.get("inst-1", "ws-1")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if fetched.ManifestDigest != "digest-a" || len(fetched.CapabilityIDs) != 1 {
		t.Fatalf("fetched = %#v", fetched)
	}

	revoked, err := ledger.revoke("inst-1", "ws-1", "human", "revoke", "audit-2", grantAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked.State != ApprovalStateRevoked || revoked.Revision != 2 {
		t.Fatalf("revoked = %#v", revoked)
	}

	if err := ledger.tombstoneInstallation("inst-1", grantAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	after, ok, err := ledger.get("inst-1", "ws-1")
	if err != nil || !ok {
		t.Fatalf("get after tombstone: ok=%v err=%v", ok, err)
	}
	if after.Revision != 3 || after.State != ApprovalStateRevoked {
		t.Fatalf("after tombstone = %#v", after)
	}
}

func TestAuthorizePluginCapabilityStableDenyReasons(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	svc.SetPluginsDir(dir)

	decision := svc.authorizePluginCapability("inst-1", "ws-1", "api_read:tasks", 1, "req", "method")
	if decision.Allowed {
		t.Fatal("decision unexpectedly allowed without approval")
	}
	if decision.Reason != ApprovalDenyMissingApproval {
		t.Fatalf("reason = %q, want missing approval", decision.Reason)
	}
}

func TestApprovalLedgerGrantRejectsRevisionRegression(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)

	if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", time.Now().UTC()); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-b", []string{"api_read:tasks"}, "human", "grant", "audit-2", time.Now().UTC()); err == nil {
		t.Fatal("grant accepted a non-incrementing revision")
	}
}

func TestApprovalLedgerTombstonePersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)

	if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", time.Now().UTC()); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := ledger.tombstoneInstallation("inst-1", time.Now().UTC()); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	reloaded := newApprovalLedger(dir)
	approval, ok, err := reloaded.get("inst-1", "ws-1")
	if err != nil || !ok {
		t.Fatalf("reloaded get: ok=%v err=%v", ok, err)
	}
	if approval.State != ApprovalStateRevoked || approval.Revision != 2 {
		t.Fatalf("approval after reload = %#v", approval)
	}
}

func TestAuthorizePluginCapabilityAllowsExactCurrentRevision(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	svc.SetPluginsDir(dir)
	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}

	decision := svc.authorizePluginCapability("inst-1", "ws-1", "api_read:tasks", 1, "req", "method")
	if !decision.Allowed {
		t.Fatalf("decision = %#v, want allowed for exact current revision", decision)
	}
	if decision.Receipt.Result != "allowed" {
		t.Fatalf("receipt result = %q, want allowed", decision.Receipt.Result)
	}
}

func TestAuthorizePluginCapabilityDeniesStaleRevision(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	svc.SetPluginsDir(dir)
	if _, err := svc.approvalGrant("inst-1", "ws-1", 2, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}

	decision := svc.authorizePluginCapability("inst-1", "ws-1", "api_read:tasks", 1, "req", "method")
	if decision.Allowed {
		t.Fatalf("decision = %#v, want stale revision denied", decision)
	}
	if decision.Reason != ApprovalDenyStaleRevision {
		t.Fatalf("reason = %q, want stale_capability_revision", decision.Reason)
	}
}

func TestAuthorizePluginCapabilityDeniesHumanReservedCapability(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	svc.SetPluginsDir(dir)
	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, "digest-a", []string{"merge"}, "human", "grant", "audit-1"); err == nil {
		t.Fatal("approvalGrant persisted a Human-reserved capability")
	}

	decision := svc.authorizePluginCapability("inst-1", "ws-1", "merge", 1, "req", "method")
	if decision.Allowed {
		t.Fatalf("decision = %#v, want human-reserved deny", decision)
	}
	if decision.Reason != ApprovalDenyHumanReserved {
		t.Fatalf("reason = %q, want human_reserved", decision.Reason)
	}
}

func TestApprovalGrantRetryIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)
	at := time.Now().UTC()
	first, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", at)
	if err != nil {
		t.Fatalf("first grant: %v", err)
	}
	second, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", at.Add(time.Second))
	if err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if second.Revision != first.Revision || !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("retry changed approval: first=%#v second=%#v", first, second)
	}
	file, err := ledger.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(file.Events) != 1 {
		t.Fatalf("event count = %d, want one idempotent event", len(file.Events))
	}
}

func TestApprovalTombstoneAppendsWorkspaceEventAndMarksRow(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)
	at := time.Now().UTC()
	if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", at); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := ledger.tombstoneInstallation("inst-1", at.Add(time.Second)); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	row, ok, err := ledger.get("inst-1", "ws-1")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if row.TombstonedAt == nil {
		t.Fatal("tombstone did not mark current approval")
	}
	file, err := ledger.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(file.Events) != 2 || file.Events[1].Type != CapabilityApprovalEventRevoke || file.Events[1].AuditID == "" {
		t.Fatalf("tombstone events = %#v", file.Events)
	}
}

func TestAuthorizePluginCapabilityRequiresCurrentInstalledManifest(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{registry: NewRegistry()}
	svc.SetPluginsDir(dir)
	svc.registry.Add(&store.Record{
		Manifest:       manifest.Manifest{ID: "plugin-a", Capabilities: manifest.Capabilities{APIRead: []string{"tasks"}}},
		InstallationID: "inst-1",
	})
	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, ManifestCapabilityDigest(svc.registry.List()[0].Manifest), []string{"api_read:tasks", "api_write:tasks"}, "human", "grant", "audit-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	decision := svc.authorizePluginCapability("inst-1", "ws-1", "api_write:tasks", 1, "req", "method")
	if decision.Allowed || decision.Reason != ApprovalDenyUndeclaredCapability {
		t.Fatalf("decision = %#v, want manifest intersection denial", decision)
	}
}
