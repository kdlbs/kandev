package plugins

import (
	"errors"
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

func TestApprovalLedgerGrantRejectsZeroRevision(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)

	if _, err := ledger.grant("inst-1", "ws-1", 0, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", time.Now().UTC()); err == nil {
		t.Fatal("grant accepted revision zero")
	}
	if _, ok, err := ledger.get("inst-1", "ws-1"); err != nil || ok {
		t.Fatalf("grant persisted revision zero approval: ok=%v err=%v", ok, err)
	}
}

func TestApprovalLedgerGrantRejectsNonInitialRevision(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)

	if _, err := ledger.grant("inst-1", "ws-1", 2, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", time.Now().UTC()); err == nil {
		t.Fatal("grant accepted non-initial revision")
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
	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1"); err != nil {
		t.Fatalf("initial grant: %v", err)
	}
	if _, err := svc.approvalGrant("inst-1", "ws-1", 2, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-2"); err != nil {
		t.Fatalf("second grant: %v", err)
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

func TestAuthorizePluginCapabilityDeniesMalformedRequest(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	svc.SetPluginsDir(dir)

	cases := []struct {
		name           string
		installationID string
		workspaceID    string
		requestDigest  string
		methodDigest   string
	}{
		{"missing installation", "", "ws-1", "req", "method"},
		{"missing workspace", "inst-1", "", "req", "method"},
		{"missing request digest", "inst-1", "ws-1", "", "method"},
		{"missing method digest", "inst-1", "ws-1", "req", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := svc.authorizePluginCapability(tc.installationID, tc.workspaceID, "api_read:tasks", 1, tc.requestDigest, tc.methodDigest)
			if decision.Allowed {
				t.Fatalf("decision unexpectedly allowed: %#v", decision)
			}
			if decision.Reason != ApprovalDenyMalformedRequest {
				t.Fatalf("reason = %q, want malformed_request", decision.Reason)
			}
		})
	}
}

func TestAuthorizePluginCapabilityDeniesUnsupportedCapabilityID(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	svc.SetPluginsDir(dir)
	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}

	for _, capabilityID := range []string{"", "api_read:*", "api_read:tas?s", " api_read:tasks"} {
		decision := svc.authorizePluginCapability("inst-1", "ws-1", capabilityID, 1, "req", "method")
		if decision.Allowed {
			t.Fatalf("capability %q unexpectedly allowed", capabilityID)
		}
		if decision.Reason != ApprovalDenyUnsupportedCapability {
			t.Fatalf("capability %q reason = %q, want unsupported_capability", capabilityID, decision.Reason)
		}
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

func TestApprovalGrantRetryRejectsChangedAuditPayload(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)
	at := time.Now().UTC()
	if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", at); err != nil {
		t.Fatalf("first grant: %v", err)
	}

	tests := []struct {
		name   string
		actor  string
		reason string
	}{
		{name: "actor", actor: "other-human", reason: "grant"},
		{name: "reason", actor: "human", reason: "different reason"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, tc.actor, tc.reason, "audit-1", at.Add(time.Second)); !errors.Is(err, ErrApprovalIdempotencyConflict) {
				t.Fatalf("changed grant payload error = %v, want idempotency conflict", err)
			}
		})
	}
}

func TestApprovalRevokeRetryRejectsChangedAuditPayload(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)
	at := time.Now().UTC()
	if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "grant-1", at); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := ledger.revokeIfRevision("inst-1", "ws-1", 1, "human", "revoke", "revoke-1", at.Add(time.Second), false); err != nil {
		t.Fatalf("first revoke: %v", err)
	}

	tests := []struct {
		name             string
		expectedRevision uint64
		actor            string
		reason           string
	}{
		{name: "revision", expectedRevision: 99, actor: "human", reason: "revoke"},
		{name: "actor", expectedRevision: 1, actor: "other-human", reason: "revoke"},
		{name: "reason", expectedRevision: 1, actor: "human", reason: "different reason"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ledger.revokeIfRevision("inst-1", "ws-1", tc.expectedRevision, tc.actor, tc.reason, "revoke-1", at.Add(2*time.Second), false); !errors.Is(err, ErrApprovalIdempotencyConflict) {
				t.Fatalf("changed revoke payload error = %v, want idempotency conflict", err)
			}
		})
	}
}

func TestApprovalGrantRetryReplaysAfterLaterRevision(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)
	at := time.Now().UTC()
	first, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", at)
	if err != nil {
		t.Fatalf("first grant: %v", err)
	}
	if _, err := ledger.grant("inst-1", "ws-1", 2, "digest-b", []string{"api_read:tasks"}, "human", "upgrade", "audit-2", at.Add(time.Second)); err != nil {
		t.Fatalf("second grant: %v", err)
	}

	replayed, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", at.Add(2*time.Second))
	if err != nil {
		t.Fatalf("retry original grant after later revision: %v", err)
	}
	if replayed.Revision != first.Revision || replayed.ManifestDigest != first.ManifestDigest || !replayed.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("retry did not replay original result: first=%#v replayed=%#v", first, replayed)
	}
	file, err := ledger.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(file.Events) != 2 {
		t.Fatalf("event count = %d, want original two events only", len(file.Events))
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

func TestApprovalTombstoneRetryDoesNotAdvanceRevisionOrDuplicateEvents(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)
	at := time.Now().UTC()
	if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", at); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := ledger.tombstoneInstallation("inst-1", at.Add(time.Second)); err != nil {
		t.Fatalf("first tombstone: %v", err)
	}
	first, ok, err := ledger.get("inst-1", "ws-1")
	if err != nil || !ok {
		t.Fatalf("get after first tombstone: ok=%v err=%v", ok, err)
	}
	if err := ledger.tombstoneInstallation("inst-1", at.Add(2*time.Second)); err != nil {
		t.Fatalf("second tombstone: %v", err)
	}
	second, ok, err := ledger.get("inst-1", "ws-1")
	if err != nil || !ok {
		t.Fatalf("get after second tombstone: ok=%v err=%v", ok, err)
	}
	if second.Revision != first.Revision || !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("tombstone retry advanced row: first=%#v second=%#v", first, second)
	}
	file, err := ledger.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(file.Events) != 2 {
		t.Fatalf("event count = %d, want grant plus one tombstone revoke", len(file.Events))
	}
}

func TestApprovalLedgerRejectsGrantForTombstonedInstallation(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)
	if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1", time.Now().UTC()); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := ledger.tombstoneInstallation("inst-1", time.Now().UTC()); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	if _, err := ledger.grant("inst-1", "ws-1", 3, "digest-b", []string{"api_read:tasks"}, "human", "regrant", "audit-2", time.Now().UTC()); err == nil {
		t.Fatal("grant accepted tombstoned installation")
	}
}

func TestApprovalLedgerRejectsMissingAuditID(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)
	if _, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "", time.Now().UTC()); err == nil {
		t.Fatal("grant accepted empty audit id")
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
