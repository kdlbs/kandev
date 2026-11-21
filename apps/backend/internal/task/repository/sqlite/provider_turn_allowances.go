package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/db/dialect"
)

func (r *Repository) initProviderTurnAllowancesSchema() error {
	_, err := r.db.ExecContext(r.migrationContext(), `
		CREATE TABLE IF NOT EXISTS provider_turn_allowances (
			account_key TEXT PRIMARY KEY, provider TEXT NOT NULL,
			config_json TEXT NOT NULL DEFAULT '{}', learned_json TEXT NOT NULL DEFAULT '{}',
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE IF NOT EXISTS provider_turn_claims (
			id TEXT PRIMARY KEY, account_key TEXT NOT NULL REFERENCES provider_turn_allowances(account_key),
			execution_id TEXT NOT NULL, session_id TEXT NOT NULL DEFAULT '', prompt_generation BIGINT NOT NULL,
			submission_id TEXT NOT NULL DEFAULT '', state TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL, finished_at TIMESTAMP,
			UNIQUE(execution_id, prompt_generation)
		);
		CREATE INDEX IF NOT EXISTS idx_provider_turn_claims_account ON provider_turn_claims(account_key, created_at);
	`)
	return err
}

const turnAllowanceColumns = `account_key, provider, config_json, learned_json, updated_at`
const providerTurnClaimCompleted = "completed"

func readTurnAllowance(ctx context.Context, reader sqlx.QueryerContext, query, key string) (*agentusage.TurnAllowanceRecord, error) {
	var row struct {
		AccountKey string    `db:"account_key"`
		Provider   string    `db:"provider"`
		Owner      string    `db:"config_json"`
		Learned    string    `db:"learned_json"`
		UpdatedAt  time.Time `db:"updated_at"`
	}
	if err := sqlx.GetContext(ctx, reader, &row, query, key); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &agentusage.TurnAllowanceRecord{AccountKey: row.AccountKey, Provider: row.Provider, Owner: json.RawMessage(row.Owner), Learned: json.RawMessage(row.Learned), UpdatedAt: row.UpdatedAt}, nil
}

func (r *Repository) GetProviderTurnAllowance(ctx context.Context, key string) (*agentusage.TurnAllowanceRecord, error) {
	return readTurnAllowance(ctx, r.ro, r.ro.Rebind(`SELECT `+turnAllowanceColumns+` FROM provider_turn_allowances WHERE account_key=?`), key)
}

// lockTurnAllowance uses the native SQLite writer or a PostgreSQL row lock.
func (r *Repository) lockTurnAllowance(ctx context.Context, tx *sqlx.Tx, key string) (*agentusage.TurnAllowanceRecord, error) {
	query := `SELECT ` + turnAllowanceColumns + ` FROM provider_turn_allowances WHERE account_key=?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	return readTurnAllowance(ctx, tx, tx.Rebind(query), key)
}

func (r *Repository) PatchProviderTurnAllowance(ctx context.Context, key, provider string, patch json.RawMessage) (*agentusage.TurnAllowanceRecord, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO provider_turn_allowances(account_key,provider,updated_at) VALUES (?,?,?) ON CONFLICT(account_key) DO NOTHING`), key, provider, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	prior, err := r.lockTurnAllowance(ctx, tx, key)
	if err != nil {
		return nil, err
	}
	owner, err := agentusage.PatchTurnAllowance(prior.Owner, patch)
	if err != nil {
		return nil, err
	}
	prior.Owner, prior.UpdatedAt = owner, time.Now().UTC()
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE provider_turn_allowances SET config_json=?,updated_at=? WHERE account_key=?`), string(owner), prior.UpdatedAt, key)
	if err != nil {
		return nil, err
	}
	return prior, tx.Commit()
}

// HasProviderTurnAllowance is conservative when execution binding is unresolved.
func (r *Repository) HasProviderTurnAllowance(ctx context.Context, provider string) (bool, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`SELECT config_json FROM provider_turn_allowances WHERE (?='' OR provider=?)`), provider, provider)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return false, err
		}
		p, err := agentusage.DecodeTurnAllowance(json.RawMessage(raw))
		if err != nil {
			return false, err
		}
		if p.Configured(time.Now()) {
			return true, nil
		}
	}
	return false, rows.Err()
}

type ProviderTurnClaim struct {
	SessionID    string
	ID           string
	ExecutionID  string
	Generation   uint64
	SubmissionID string
}

// AcquireProviderTurn reports whether this attempt owns the unsent refund.
func (r *Repository) AcquireProviderTurn(ctx context.Context, key string, claim ProviderTurnClaim, populations ...[]ProviderTurnClaim) (bool, error) {
	var population []ProviderTurnClaim
	if len(populations) > 0 {
		population = populations[0]
	}
	return r.acquireProviderTurnAt(ctx, key, claim, time.Now, population)
}

func (r *Repository) acquireProviderTurnAt(ctx context.Context, key string, claim ProviderTurnClaim, clock func() time.Time, population []ProviderTurnClaim) (bool, error) {
	if claim.ID == "" || claim.ExecutionID == "" || claim.Generation == 0 {
		return false, agentusage.ErrTurnAllowanceUnknown
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	row, err := r.lockTurnAllowance(ctx, tx, key)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, nil
	}
	policy, err := agentusage.DecodeTurnAllowance(row.Owner)
	if err != nil {
		return false, err
	}
	reused, err := reuseProviderTurn(ctx, tx, key, &claim)
	if err != nil {
		return false, err
	}
	if reused {
		return false, tx.Commit()
	}
	now := clock().UTC()
	if !policy.Configured(now) {
		return false, nil
	}
	if err := checkTurnAllowance(ctx, tx, key, policy, now, population); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO provider_turn_claims(id,account_key,execution_id,session_id,prompt_generation,submission_id,state,created_at)
		VALUES (?,?,?,?,?,?,'claimed',?) ON CONFLICT(id) DO UPDATE SET state='claimed',execution_id=excluded.execution_id,session_id=excluded.session_id,prompt_generation=excluded.prompt_generation,submission_id=excluded.submission_id,created_at=excluded.created_at,finished_at=NULL WHERE provider_turn_claims.state='released'`), claim.ID, key, claim.ExecutionID, claim.SessionID, claim.Generation, claim.SubmissionID, now)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

type storedTurnClaim struct {
	ID      string `db:"id"`
	Session string `db:"session_id"`
	Account string `db:"account_key"`
	State   string `db:"state"`
}

func reuseProviderTurn(ctx context.Context, tx *sqlx.Tx, key string, claim *ProviderTurnClaim) (bool, error) {
	prior, err := existingTurnClaim(ctx, tx, *claim)
	if err != nil || prior == nil {
		return false, err
	}
	if prior.Account != key || prior.Session != claim.SessionID {
		return false, agentusage.ErrTurnAllowanceUnknown
	}
	claim.ID = prior.ID
	if prior.State == "released" {
		return false, nil
	}
	if prior.State != "claimed" && prior.State != providerTurnClaimCompleted {
		return false, agentusage.ErrTurnAllowanceUnknown
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE provider_turn_claims SET execution_id=?,prompt_generation=? WHERE id=?`), claim.ExecutionID, claim.Generation, prior.ID)
	return true, err
}

func existingTurnClaim(ctx context.Context, tx *sqlx.Tx, claim ProviderTurnClaim) (*storedTurnClaim, error) {
	var rows []storedTurnClaim
	err := tx.SelectContext(ctx, &rows, tx.Rebind(`SELECT id,session_id,account_key,state FROM provider_turn_claims WHERE id=? OR (execution_id=? AND prompt_generation=?)`), claim.ID, claim.ExecutionID, claim.Generation)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 {
		return nil, agentusage.ErrTurnAllowanceUnknown
	}
	return &rows[0], nil
}

func checkTurnAllowance(ctx context.Context, tx *sqlx.Tx, key string, policy agentusage.TurnAllowance, now time.Time, population []ProviderTurnClaim) error {
	if policy.BlockUntil != nil && now.Before(*policy.BlockUntil) {
		return &agentusage.TurnAllowanceLimited{Dimension: "blocked", RetryAt: *policy.BlockUntil}
	}
	if policy.ConcurrentTurns != nil && *policy.ConcurrentTurns > 0 {
		var active int64
		if err := tx.GetContext(ctx, &active, tx.Rebind(`SELECT COUNT(*) FROM provider_turn_claims WHERE account_key=? AND state NOT IN ('completed','released')`), key); err != nil {
			return err
		}
		untracked, err := untrackedProviderTurns(ctx, tx, key, population)
		if err != nil {
			return err
		}
		active += untracked
		if active >= *policy.ConcurrentTurns {
			return &agentusage.TurnAllowanceLimited{Dimension: "concurrent_turns"}
		}
	}
	if policy.TurnsPerWindow != nil && *policy.TurnsPerWindow > 0 {
		start, reset := policy.TurnWindow(now)
		var used int64
		if err := tx.GetContext(ctx, &used, tx.Rebind(`SELECT COUNT(*) FROM provider_turn_claims WHERE account_key=? AND state<>'released' AND created_at>=? AND created_at<?`), key, start, reset); err != nil {
			return err
		}
		if used >= *policy.TurnsPerWindow {
			return &agentusage.TurnAllowanceLimited{Dimension: "turns_per_window", RetryAt: reset}
		}
	}
	return nil
}

func untrackedProviderTurns(ctx context.Context, tx *sqlx.Tx, key string, population []ProviderTurnClaim) (int64, error) {
	var count int64
	for _, turn := range population {
		var found int
		if err := tx.GetContext(ctx, &found, tx.Rebind(`SELECT COUNT(*) FROM provider_turn_claims WHERE account_key=? AND state<>'released' AND (id=? OR (execution_id=? AND prompt_generation=?))`), key, turn.ID, turn.ExecutionID, turn.Generation); err != nil {
			return 0, err
		}
		if found == 0 {
			count++
		}
	}
	return count, nil
}

func (r *Repository) ProviderTurnClaimExists(ctx context.Context, key string, claim ProviderTurnClaim) (bool, error) {
	var found int
	err := r.ro.GetContext(ctx, &found, r.ro.Rebind(`SELECT COUNT(*) FROM provider_turn_claims WHERE account_key=? AND state<>'released' AND (id=? OR (execution_id=? AND prompt_generation=?))`), key, claim.ID, claim.ExecutionID, claim.Generation)
	return found > 0, err
}

// FinishProviderTurn is idempotent and never refunds a turn that completed.
func (r *Repository) FinishProviderTurn(ctx context.Context, execution string, generation uint64, release bool) error {
	state := providerTurnClaimCompleted
	if release {
		state = "released"
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE provider_turn_claims SET state=?,finished_at=? WHERE execution_id=? AND prompt_generation=? AND state='claimed'`), state, time.Now().UTC(), execution, generation)
	return err
}

func (r *Repository) CompleteProviderExecution(ctx context.Context, execution string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE provider_turn_claims SET state='completed',finished_at=? WHERE execution_id=? AND state='claimed'`), time.Now().UTC(), execution)
	return err
}
