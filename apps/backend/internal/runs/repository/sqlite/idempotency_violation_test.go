package sqlite

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestIsWakeWaveUniqueViolation_SQLite(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	db.MustExec(`CREATE TABLE runs (
		id TEXT PRIMARY KEY,
		wake_wave_key TEXT NOT NULL DEFAULT '',
		agent_profile_id TEXT NOT NULL,
		idempotency_key TEXT
	)`)
	db.MustExec(`CREATE UNIQUE INDEX idx_run_wake_wave ON runs(wake_wave_key, agent_profile_id) WHERE wake_wave_key <> ''`)
	db.MustExec(`CREATE UNIQUE INDEX idx_run_idempotency ON runs(idempotency_key) WHERE idempotency_key IS NOT NULL`)
	db.MustExec(`INSERT INTO runs (id, wake_wave_key, agent_profile_id) VALUES ('a', 'wave-1', 'agent-1')`)

	_, err = db.Exec(`INSERT INTO runs (id, wake_wave_key, agent_profile_id) VALUES ('b', 'wave-1', 'agent-1')`)
	if err == nil {
		t.Fatal("expected a unique-index violation, got none")
	}
	if !IsWakeWaveUniqueViolation(err) {
		t.Errorf("IsWakeWaveUniqueViolation(%v) = false, want true", err)
	}
	if IsIdempotencyKeyUniqueViolation(err) {
		t.Error("a wake-wave violation must not be classified as an idempotency-key violation")
	}

	db.MustExec(`INSERT INTO runs (id, idempotency_key, agent_profile_id) VALUES ('c', 'idem-1', 'agent-1')`)
	_, err = db.Exec(`INSERT INTO runs (id, idempotency_key, agent_profile_id) VALUES ('d', 'idem-1', 'agent-1')`)
	if err == nil {
		t.Fatal("expected an idempotency-key violation, got none")
	}
	if IsWakeWaveUniqueViolation(err) {
		t.Error("an idempotency-key violation must not be classified as a wake-wave violation")
	}
	if !IsIdempotencyKeyUniqueViolation(err) {
		t.Errorf("IsIdempotencyKeyUniqueViolation(%v) = false, want true", err)
	}
}

func TestIsWakeWaveUniqueViolation_Nil(t *testing.T) {
	if IsWakeWaveUniqueViolation(nil) {
		t.Error("IsWakeWaveUniqueViolation(nil) = true, want false")
	}
}

func TestIsWakeWaveUniqueViolation_UnrelatedError(t *testing.T) {
	if IsWakeWaveUniqueViolation(errors.New("some other failure")) {
		t.Error("IsWakeWaveUniqueViolation(unrelated error) = true, want false")
	}
}

func TestIsWakeWaveUniqueViolation_PostgresConstraintName(t *testing.T) {
	matching := &pgconn.PgError{Code: "23505", ConstraintName: "idx_run_wake_wave"}
	if !IsWakeWaveUniqueViolation(matching) {
		t.Error("expected a matching pgconn.PgError to classify as a wake-wave violation")
	}

	other := &pgconn.PgError{Code: "23505", ConstraintName: "idx_run_idempotency"}
	if IsWakeWaveUniqueViolation(other) {
		t.Error("a differently-named unique violation must not classify as a wake-wave violation")
	}
}
