package provideraccess

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestStoreAuditReceiptPersistsOnlyReviewedFields(t *testing.T) {
	store := newGrantTestStore(t)
	event := AuditEvent{
		ID: "audit-1", GrantID: "grant-1", LeaseID: "lease-1",
		PluginInstallationID: "installation-1", WorkspaceID: "workspace-1",
		ManagedTaskID: "managed-task-1", SessionID: "session-1",
		TargetDigest: "target-digest-1", GrantGeneration: 1,
		ApprovalRevision: 3, ConnectionGeneration: "connection-1",
		Provider: "github", Purpose: "actions_write",
		Outcome: AuditLeaseIssued, At: time.Now().UTC(),
	}
	if err := store.RecordAudit(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetAudit(context.Background(), event.ID)
	if err != nil || got == nil || got.Outcome != event.Outcome || got.SessionID != event.SessionID {
		t.Fatalf("audit = %+v, err = %v", got, err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"token", "authorization", "provider_response", "idempotency_key"} {
		if _, exists := fields[forbidden]; exists {
			t.Fatalf("audit includes unreviewed field %s", forbidden)
		}
	}
}

func TestStoreAuditRejectsUnknownOutcomeAndMissingIdentity(t *testing.T) {
	store := newGrantTestStore(t)
	event := AuditEvent{ID: "audit-1", Outcome: AuditOutcome("provider response: secret")}
	if err := store.RecordAudit(context.Background(), event); err == nil {
		t.Fatal("unreviewed provider response was accepted as an outcome")
	}
	event.Outcome = AuditLeaseIssued
	if err := store.RecordAudit(context.Background(), event); err == nil {
		t.Fatal("missing audit identity was accepted")
	}
}
