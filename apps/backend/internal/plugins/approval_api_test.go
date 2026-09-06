package plugins

import (
	"errors"
	"testing"
)

func TestApprovalAPIExportsCurrentRowsAndDecision(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{}
	svc.SetPluginsDir(dir)
	if _, err := svc.approvalGrant("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "audit-1"); err != nil {
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

	decision := svc.AuthorizeCapability("inst-1", "ws-1", "api_read:tasks", 1, "req", "method")
	if !decision.Allowed {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestApprovalAPIRevokeRetryReplaysOriginalResult(t *testing.T) {
	svc := &Service{}
	svc.SetPluginsDir(t.TempDir())
	if _, err := svc.GrantCapabilityApproval("inst-1", "ws-1", 1, "digest-a", []string{"api_read:tasks"}, "human", "grant", "grant-1"); err != nil {
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
