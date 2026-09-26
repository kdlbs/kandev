package sqlite

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

// sqlExecutor is satisfied by both *sqlx.DB and *sqlx.Tx, letting a read
// helper run against the repository's own reader/writer or a
// caller-supplied transaction without duplicating the query.
type sqlExecutor interface {
	QueryRowxContext(ctx context.Context, query string, args ...interface{}) *sqlx.Row
	Rebind(query string) string
}

// agentEnqueueLockPrefix namespaces this package's advisory-lock keyspace.
// Postgres advisory locks share one flat int64 keyspace per database, so
// this only guards against an accidental collision with another
// package's own FNV-1a-derived key (see internal/secrets.WorkspaceLockKey
// for the precedent this follows).
const agentEnqueueLockPrefix = "runs-enqueue:"

// agentEnqueueLockKey returns the deterministic Postgres advisory-lock
// key serializing every enqueue for one agent profile
// (AC-OFFICE-LAUNCH-SAFETY-003.8): the depth check, the self-trigger
// window count, the idempotency check, and the insert must be serialized
// against every other concurrent enqueue for the same agent profile, on
// every supported database engine.
func agentEnqueueLockKey(agentProfileID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(agentEnqueueLockPrefix + agentProfileID))
	return int64(h.Sum64())
}

// BeginEnqueueTx starts the transaction QueueRun performs its
// idempotency check, coalescing check, causation resolution (including
// the depth and self-trigger gates), and insert inside
// (AC-OFFICE-LAUNCH-SAFETY-003.8). On PostgreSQL it also takes an
// advisory lock scoped to agentProfileID for the duration of the
// transaction, so two concurrent enqueues for the same agent profile
// cannot both observe the same self-trigger count before either
// commits. SQLite's single writer already serializes concurrent
// transactions, matching ClaimNextEligibleRun's precedent in claim.go,
// so no additional lock is taken there.
func (r *Repository) BeginEnqueueTx(ctx context.Context, agentProfileID string) (*sqlx.Tx, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, r.db.Rebind("SELECT pg_advisory_xact_lock(?)"), agentEnqueueLockKey(agentProfileID)); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("acquire enqueue lock: %w", err)
		}
	}
	return tx, nil
}
