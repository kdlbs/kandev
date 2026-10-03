package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/db"
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
		ProcessIdentity:    processidentity.Identity{PID: 4312, GroupID: 4312, SessionID: 4312, BirthToken: "linux:boot-id:8765"},
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
	if got.ProcessIdentity != record.ProcessIdentity {
		t.Errorf("ProcessIdentity = %#v, want %#v", got.ProcessIdentity, record.ProcessIdentity)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("CreatedAt/UpdatedAt not set: %#v", got)
	}
}

func TestControlServerRecordProcessIdentitySurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control-server.db")
	firstDB, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open first database: %v", err)
	}
	firstConn := sqlx.NewDb(firstDB, "sqlite3")
	firstRepo, err := NewWithDB(firstConn, firstConn, nil)
	if err != nil {
		_ = firstConn.Close()
		t.Fatalf("initialize first repository: %v", err)
	}
	identity := processidentity.Identity{PID: 4312, GroupID: 4312, SessionID: 4312, BirthToken: "linux:boot-id:8765"}
	if err := firstRepo.UpsertControlServerRecord(context.Background(), &models.ControlServerRecord{
		Endpoint: "127.0.0.1:41123", ServerIdentity: "server-identity", CredentialSecretID: "secret",
		DiagnosticLogPath: "/home/kandev/logs/agentctl.log", ProcessIdentity: identity,
	}); err != nil {
		_ = firstConn.Close()
		t.Fatalf("write control server record: %v", err)
	}
	if err := firstConn.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}

	secondDB, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	secondConn := sqlx.NewDb(secondDB, "sqlite3")
	t.Cleanup(func() { _ = secondConn.Close() })
	secondRepo, err := NewWithDB(secondConn, secondConn, nil)
	if err != nil {
		t.Fatalf("initialize reopened repository: %v", err)
	}
	got, err := secondRepo.GetControlServerRecord(context.Background())
	if err != nil {
		t.Fatalf("read control server record after reopen: %v", err)
	}
	if got.ProcessIdentity != identity {
		t.Fatalf("ProcessIdentity after reopen = %#v, want %#v", got.ProcessIdentity, identity)
	}
}

func TestControlServerRecordProcessIdentityMigrationResumesPartialUpgrade(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	legacy := &models.ControlServerRecord{
		Endpoint: "127.0.0.1:41001", ServerIdentity: "legacy", CredentialSecretID: "secret",
		DiagnosticLogPath: "/tmp/agentctl.log",
	}
	if err := repo.UpsertControlServerRecord(ctx, legacy); err != nil {
		t.Fatalf("write legacy record: %v", err)
	}
	for _, column := range []string{"process_birth_token", "process_session_id", "process_group_id", "process_id"} {
		if _, err := repo.db.ExecContext(ctx, "ALTER TABLE control_server_records DROP COLUMN "+column); err != nil {
			t.Fatalf("drop %s to model prior schema: %v", column, err)
		}
	}
	if _, err := repo.db.ExecContext(ctx, `ALTER TABLE control_server_records ADD COLUMN process_id INTEGER NOT NULL DEFAULT 0`); err != nil {
		t.Fatalf("simulate interrupted process identity migration: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `ALTER TABLE control_server_records ADD COLUMN process_group_id INTEGER NOT NULL DEFAULT 0`); err != nil {
		t.Fatalf("simulate partially applied process identity migration: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("upgrade prior control server schema: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("replay control server schema upgrade: %v", err)
	}
	got, err := repo.GetControlServerRecord(ctx)
	if err != nil {
		t.Fatalf("read migrated record: %v", err)
	}
	if got.Endpoint != legacy.Endpoint || got.ProcessIdentity != (processidentity.Identity{}) {
		t.Fatalf("migrated record = %#v, want preserved endpoint and unknown identity", got)
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
