package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newReachabilityTestRepo(t *testing.T) (*Repository, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reachability.db")
	dbConn, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		_ = sqlxDB.Close()
		t.Fatalf("new repository: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo, path
}

// createSSHExecutor inserts an active SSH executor and returns it, so
// UpsertExecutorReachability's eligibility EXISTS clause is satisfied.
func createSSHExecutor(t *testing.T, repo *Repository) *models.Executor {
	t.Helper()
	executor := &models.Executor{
		ID:     "exec-" + t.Name(),
		Name:   "ssh-host",
		Type:   models.ExecutorTypeSSH,
		Status: models.ExecutorStatusActive,
	}
	if err := repo.CreateExecutor(context.Background(), executor); err != nil {
		t.Fatalf("create executor: %v", err)
	}
	return executor
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.16 — last-write-wins on
// checked_at regardless of commit order, including a whole-second value
// racing a sub-second one.
func TestUpsertExecutorReachabilityNewerCheckedAtWinsRegardlessOfOrder(t *testing.T) {
	repo, _ := newReachabilityTestRepo(t)
	ctx := context.Background()
	executor := createSSHExecutor(t, repo)

	older := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 1, 1, 12, 0, 0, 500_000_000, time.UTC) // same whole second, later fraction

	// Issue the newer write first, the older write second — the newer must survive.
	if err := repo.UpsertExecutorReachability(ctx, models.ExecutorReachabilityObservation{
		ExecutorID: executor.ID, SeenUpdatedAt: executor.UpdatedAt,
		InitialState: models.ExecutorReachabilityStateReachable,
		Reason:       "", Host: "10.0.0.2", CheckedAt: newer, FailureThreshold: 3,
	}); err != nil {
		t.Fatalf("upsert newer: %v", err)
	}
	if err := repo.UpsertExecutorReachability(ctx, models.ExecutorReachabilityObservation{
		ExecutorID: executor.ID, SeenUpdatedAt: executor.UpdatedAt,
		InitialState: models.ExecutorReachabilityStateUnreachable,
		Reason:       models.ExecutorReachabilityReasonNetwork, Message: "connection refused",
		Host: "10.0.0.1", CheckedAt: older, FailureThreshold: 3,
	}); err != nil {
		t.Fatalf("upsert older: %v", err)
	}

	got, err := repo.GetExecutorReachability(ctx, executor.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Host != "10.0.0.2" || got.State != models.ExecutorReachabilityStateReachable {
		t.Fatalf("record = %+v, want the newer (reachable, 10.0.0.2) write to survive", got)
	}
	if got.CheckedAt == nil || !got.CheckedAt.Equal(newer) {
		t.Fatalf("CheckedAt = %v, want %v", got.CheckedAt, newer)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.16 — an equal checked_at leaves
// the row unchanged.
func TestUpsertExecutorReachabilityEqualCheckedAtIsDiscarded(t *testing.T) {
	repo, _ := newReachabilityTestRepo(t)
	ctx := context.Background()
	executor := createSSHExecutor(t, repo)

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	obs := models.ExecutorReachabilityObservation{
		ExecutorID: executor.ID, SeenUpdatedAt: executor.UpdatedAt,
		InitialState: models.ExecutorReachabilityStateReachable,
		Reason:       "", Host: "10.0.0.1", CheckedAt: at, FailureThreshold: 3,
	}
	if err := repo.UpsertExecutorReachability(ctx, obs); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	obs.Host = "10.0.0.9" // a second write at the SAME checked_at, different host
	if err := repo.UpsertExecutorReachability(ctx, obs); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := repo.GetExecutorReachability(ctx, executor.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Host != "10.0.0.1" {
		t.Fatalf("Host = %q, want the first write's host unchanged (equal checked_at must be discarded)", got.Host)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.16 — a record survives a
// repository re-open against the same database file, byte-identical
// including both timestamps.
func TestExecutorReachabilityRoundTripAcrossReopen(t *testing.T) {
	repo, path := newReachabilityTestRepo(t)
	ctx := context.Background()
	executor := createSSHExecutor(t, repo)

	at := time.Date(2026, 3, 4, 5, 6, 7, 123_000_000, time.UTC)
	if err := repo.UpsertExecutorReachability(ctx, models.ExecutorReachabilityObservation{
		ExecutorID: executor.ID, SeenUpdatedAt: executor.UpdatedAt,
		InitialState: models.ExecutorReachabilityStateReachable,
		Reason:       "", Host: "10.0.0.1", CheckedAt: at, FailureThreshold: 3,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	before, err := repo.GetExecutorReachability(ctx, executor.ID)
	if err != nil {
		t.Fatalf("get before reopen: %v", err)
	}

	dbConn, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	reopened, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("reopen repository: %v", err)
	}

	after, err := reopened.GetExecutorReachability(ctx, executor.ID)
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if after.Host != before.Host || after.State != before.State {
		t.Fatalf("record changed across reopen: before=%+v after=%+v", before, after)
	}
	if after.CheckedAt == nil || before.CheckedAt == nil || !after.CheckedAt.Equal(*before.CheckedAt) {
		t.Fatalf("CheckedAt changed across reopen: before=%v after=%v", before.CheckedAt, after.CheckedAt)
	}
	if after.LastSuccessAt == nil || before.LastSuccessAt == nil || !after.LastSuccessAt.Equal(*before.LastSuccessAt) {
		t.Fatalf("LastSuccessAt changed across reopen: before=%v after=%v", before.LastSuccessAt, after.LastSuccessAt)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.21
func TestDeleteExecutorRemovesReachabilityRecord(t *testing.T) {
	repo, _ := newReachabilityTestRepo(t)
	ctx := context.Background()
	executor := createSSHExecutor(t, repo)

	if err := repo.UpsertExecutorReachability(ctx, models.ExecutorReachabilityObservation{
		ExecutorID: executor.ID, SeenUpdatedAt: executor.UpdatedAt,
		InitialState: models.ExecutorReachabilityStateReachable,
		Reason:       "", Host: "10.0.0.1", CheckedAt: time.Now().UTC(), FailureThreshold: 3,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := repo.DeleteExecutor(ctx, executor.ID); err != nil {
		t.Fatalf("delete executor: %v", err)
	}

	if _, err := repo.GetExecutorReachability(ctx, executor.ID); err != models.ErrExecutorReachabilityNotFound {
		t.Fatalf("err = %v, want ErrExecutorReachabilityNotFound", err)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.22
func TestResetExecutorReachabilityClearsTheRecordButAdvancesUpdatedAt(t *testing.T) {
	repo, _ := newReachabilityTestRepo(t)
	ctx := context.Background()
	executor := createSSHExecutor(t, repo)

	at := time.Now().UTC()
	if err := repo.UpsertExecutorReachability(ctx, models.ExecutorReachabilityObservation{
		ExecutorID: executor.ID, SeenUpdatedAt: executor.UpdatedAt,
		InitialState: models.ExecutorReachabilityStateUnreachable, InitialFailures: 1,
		Reason: models.ExecutorReachabilityReasonNetwork, Message: "connection refused",
		Host: "10.0.0.1", CheckedAt: at, FailureThreshold: 1,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	before, err := repo.GetExecutorReachability(ctx, executor.ID)
	if err != nil {
		t.Fatalf("get before reset: %v", err)
	}
	if before.ConsecutiveFailures == 0 || before.CheckedAt == nil {
		t.Fatalf("precondition: expected a populated failure record, got %+v", before)
	}

	if err := repo.ResetExecutorReachability(ctx, executor.ID, "10.0.0.9"); err != nil {
		t.Fatalf("reset: %v", err)
	}

	after, err := repo.GetExecutorReachability(ctx, executor.ID)
	if err != nil {
		t.Fatalf("get after reset: %v", err)
	}
	if after.State != models.ExecutorReachabilityStateUnknown {
		t.Fatalf("State = %q, want unknown", after.State)
	}
	if after.ConsecutiveFailures != 0 || after.Reason != "" || after.Message != "" {
		t.Fatalf("record not cleared: %+v", after)
	}
	if after.Host != "10.0.0.9" {
		t.Fatalf("Host = %q, want the newly saved host", after.Host)
	}
	if after.CheckedAt != nil || after.LastSuccessAt != nil {
		t.Fatalf("timestamps not cleared: checked_at=%v last_success_at=%v", after.CheckedAt, after.LastSuccessAt)
	}
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("UpdatedAt did not advance: before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-001.17, 001.22
func TestResetExecutorReachabilityIsANoOpForANonActiveExecutor(t *testing.T) {
	repo, _ := newReachabilityTestRepo(t)
	ctx := context.Background()
	executor := createSSHExecutor(t, repo)
	at := time.Now().UTC()
	if err := repo.UpsertExecutorReachability(ctx, models.ExecutorReachabilityObservation{
		ExecutorID: executor.ID, SeenUpdatedAt: executor.UpdatedAt,
		InitialState: models.ExecutorReachabilityStateReachable,
		Reason:       "", Host: "10.0.0.1", CheckedAt: at, FailureThreshold: 3,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	executor.Status = models.ExecutorStatusDisabled
	if err := repo.UpdateExecutor(ctx, executor); err != nil {
		t.Fatalf("disable executor: %v", err)
	}

	if err := repo.ResetExecutorReachability(ctx, executor.ID, "10.0.0.9"); err != nil {
		t.Fatalf("reset: %v", err)
	}

	got, err := repo.GetExecutorReachability(ctx, executor.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Host != "10.0.0.1" || got.State != models.ExecutorReachabilityStateReachable {
		t.Fatalf("record = %+v, want it untouched by a reset on a disabled executor", got)
	}
}

func TestListSSHExecutorsForReachabilityOrdersByID(t *testing.T) {
	repo, _ := newReachabilityTestRepo(t)
	ctx := context.Background()

	mustCreate := func(id string, execType models.ExecutorType, status models.ExecutorStatus) {
		t.Helper()
		if err := repo.CreateExecutor(ctx, &models.Executor{ID: id, Name: id, Type: execType, Status: status}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mustCreate("b-ssh", models.ExecutorTypeSSH, models.ExecutorStatusActive)
	mustCreate("a-ssh", models.ExecutorTypeSSH, models.ExecutorStatusActive)
	mustCreate("c-ssh-disabled", models.ExecutorTypeSSH, models.ExecutorStatusDisabled)
	mustCreate("d-docker", models.ExecutorTypeLocalDocker, models.ExecutorStatusActive)

	deleted := &models.Executor{ID: "e-ssh-deleted", Name: "e-ssh-deleted", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive}
	if err := repo.CreateExecutor(ctx, deleted); err != nil {
		t.Fatalf("create deleted: %v", err)
	}
	if err := repo.DeleteExecutor(ctx, deleted.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, err := repo.ListSSHExecutorsForReachability(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 || got[0].ID != "a-ssh" || got[1].ID != "b-ssh" {
		ids := make([]string, len(got))
		for i, e := range got {
			ids[i] = e.ID
		}
		t.Fatalf("ids = %v, want [a-ssh b-ssh]", ids)
	}
}
