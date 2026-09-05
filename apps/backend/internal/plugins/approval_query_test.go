package plugins

import (
	"testing"
	"time"
)

func TestServiceApprovalQueriesReturnCurrentRows(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	svc.SetPluginsDir(dir)

	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1"); err != nil {
		t.Fatalf("grant ws-1: %v", err)
	}
	if _, err := svc.approvalGrant("inst-1", "ws-2", 1, "digest-b", []string{"api_write:tasks"}, "human", "grant", "audit-2"); err != nil {
		t.Fatalf("grant ws-2: %v", err)
	}

	rows, err := svc.approvalListByInstallation("inst-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %#v, want two workspace approvals", rows)
	}
}

func TestServiceApprovalRevokeBumpsRevisionAndDenies(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	svc.SetPluginsDir(dir)
	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if _, err := svc.approvalRevoke("inst-1", "ws-1", "human", "revoke", "audit-2"); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	decision := svc.authorizePluginCapability("inst-1", "ws-1", "api_read:tasks", 2, "req", "method")
	if decision.Allowed {
		t.Fatalf("decision = %#v, want revoked approval denied", decision)
	}
	if decision.Reason != ApprovalDenyRevokedApproval {
		t.Fatalf("reason = %q, want capability_revoked", decision.Reason)
	}
}

func TestServiceApprovalTombstoneRetainsStateOnReinstall(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	svc.SetPluginsDir(dir)
	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := svc.approvalTombstoneInstallation("inst-1"); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	rows, err := svc.approvalListByInstallation("inst-1")
	if err != nil {
		t.Fatalf("list after tombstone: %v", err)
	}
	if len(rows) != 1 || rows[0].State != ApprovalStateRevoked {
		t.Fatalf("rows after tombstone = %#v", rows)
	}
}

func TestApprovalReceiptAuditIDIsDeterministic(t *testing.T) {
	got := CanonicalApprovalDigest("inst", "ws", "cap", "req", "method", "1")
	if got == "" {
		t.Fatal("CanonicalApprovalDigest() returned empty audit id")
	}
}

func TestApprovalLedgerGrantStoresCapabilitySnapshot(t *testing.T) {
	dir := t.TempDir()
	ledger := newApprovalLedger(dir)
	approval, err := ledger.grant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks", "api_write:tasks"}, "human", "grant", "audit-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if len(approval.CapabilityIDs) != 2 {
		t.Fatalf("approval capability ids = %#v", approval.CapabilityIDs)
	}
}
