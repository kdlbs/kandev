package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestGetControlServerRecordNotFound pins startup step 4's "no record"
// branch: before any server has ever been started or adopted on this
// installation, the read returns the distinguishable not-found sentinel
// rather than an empty record, so the caller can tell "spawn fresh" apart
// from "record read failed".
func TestGetControlServerRecordNotFound(t *testing.T) {
	repo := newRepoForSessionTests(t)

	_, err := repo.GetControlServerRecord(context.Background())
	if !errors.Is(err, models.ErrControlServerRecordNotFound) {
		t.Fatalf("err = %v, want %v", err, models.ErrControlServerRecordNotFound)
	}
}

// TestUpsertControlServerRecordThenGetRoundTrips pins that every field
// written by UpsertControlServerRecord (endpoint, identity, credential
// reference, capability set, diagnostic log location) survives a round trip
// unchanged.
func TestUpsertControlServerRecordThenGetRoundTrips(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	record := &models.ControlServerRecord{
		Endpoint:           "127.0.0.1:41123",
		ServerIdentity:     "server-identity-1",
		CredentialSecretID: "secret-ref-1",
		Capabilities:       []string{"resume", "diagnostics"},
		DiagnosticLogPath:  "/home/kandev/logs/agentctl-diagnostic.log",
	}
	if err := repo.UpsertControlServerRecord(ctx, record); err != nil {
		t.Fatalf("UpsertControlServerRecord: %v", err)
	}

	got, err := repo.GetControlServerRecord(ctx)
	if err != nil {
		t.Fatalf("GetControlServerRecord: %v", err)
	}
	if got.Endpoint != record.Endpoint {
		t.Errorf("Endpoint = %q, want %q", got.Endpoint, record.Endpoint)
	}
	if got.ServerIdentity != record.ServerIdentity {
		t.Errorf("ServerIdentity = %q, want %q", got.ServerIdentity, record.ServerIdentity)
	}
	if got.CredentialSecretID != record.CredentialSecretID {
		t.Errorf("CredentialSecretID = %q, want %q", got.CredentialSecretID, record.CredentialSecretID)
	}
	if len(got.Capabilities) != 2 || got.Capabilities[0] != "resume" || got.Capabilities[1] != "diagnostics" {
		t.Errorf("Capabilities = %#v, want [resume diagnostics]", got.Capabilities)
	}
	if got.DiagnosticLogPath != record.DiagnosticLogPath {
		t.Errorf("DiagnosticLogPath = %q, want %q", got.DiagnosticLogPath, record.DiagnosticLogPath)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("CreatedAt/UpdatedAt not set: %#v", got)
	}
}

// TestUpsertControlServerRecordIsSingleton pins the installation-scoped
// invariant: a second write replaces the one existing row (own server started
// after a refused or failed adoption rewrites the record to name the new
// server) rather than creating a second row, and CreatedAt is preserved
// across the rewrite while UpdatedAt advances.
func TestUpsertControlServerRecordIsSingleton(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	first := &models.ControlServerRecord{
		Endpoint:           "127.0.0.1:41123",
		ServerIdentity:     "server-identity-1",
		CredentialSecretID: "secret-ref-1",
		DiagnosticLogPath:  "/home/kandev/logs/agentctl-diagnostic.log",
	}
	if err := repo.UpsertControlServerRecord(ctx, first); err != nil {
		t.Fatalf("UpsertControlServerRecord(first): %v", err)
	}
	firstCreatedAt := first.CreatedAt

	time.Sleep(2 * time.Millisecond)

	second := &models.ControlServerRecord{
		Endpoint:           "127.0.0.1:52222",
		ServerIdentity:     "server-identity-2",
		CredentialSecretID: "secret-ref-2",
		DiagnosticLogPath:  "/home/kandev/logs/agentctl-diagnostic.log",
	}
	if err := repo.UpsertControlServerRecord(ctx, second); err != nil {
		t.Fatalf("UpsertControlServerRecord(second): %v", err)
	}

	got, err := repo.GetControlServerRecord(ctx)
	if err != nil {
		t.Fatalf("GetControlServerRecord: %v", err)
	}
	if got.Endpoint != second.Endpoint || got.ServerIdentity != second.ServerIdentity {
		t.Fatalf("got = %#v, want the rewritten (second) record", got)
	}
	if !got.CreatedAt.Equal(firstCreatedAt) {
		t.Errorf("CreatedAt = %v, want preserved original %v", got.CreatedAt, firstCreatedAt)
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("UpdatedAt = %v, want after CreatedAt %v", got.UpdatedAt, got.CreatedAt)
	}

	var count int
	if err := repo.ro.QueryRowContext(ctx, `SELECT COUNT(*) FROM control_server_records`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}
}
