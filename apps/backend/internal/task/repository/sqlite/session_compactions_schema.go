package sqlite

// sessionCompactionsSchema owns the durable per-attempt compaction receipts.
// A receipt outlives transport receipts and timeline history; only deletion of
// the owning task session removes it. The unique key deliberately excludes
// process, stream, and execution identities so reconnect duplicates cannot
// increment twice.
const sessionCompactionsSchema = `
		CREATE TABLE IF NOT EXISTS task_session_compactions (
			session_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			native_session_id TEXT NOT NULL,
			compaction_attempt_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL DEFAULT '',
			harness_generation BIGINT NOT NULL,
			delivery_stream_id TEXT NOT NULL,
			delivery_sequence BIGINT NOT NULL,
			observed_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL,
			PRIMARY KEY (session_id, provider, native_session_id, compaction_attempt_id),
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		);
`

// initSessionCompactionsSchema creates the received-attempt ledger after the
// session/continuity tables it references exist.
func (r *Repository) initSessionCompactionsSchema() error {
	_, err := r.db.ExecContext(r.migrationContext(), sessionCompactionsSchema)
	return err
}
