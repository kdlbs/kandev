package sqlite

import (
	"context"
	"fmt"
	"time"
)

// UsageLimitObservation is the recorded usage of a provider account at the
// moment one of its candidates hit a usage or rate limit. Kandev keeps it so the
// point where an undisclosed limit bites can be estimated from its own ledger.
// Values are recorded-only lower bounds over trailing windows ending at the hit.
type UsageLimitObservation struct {
	WindowUsageJSON    string    `db:"window_usage_json"`
	Tier               string    `db:"tier"`
	Dimension          string    `db:"dimension"`
	RequestsMinute     float64   `db:"requests_minute"`
	RequestsDay        float64   `db:"requests_day"`
	ActiveTurns        int64     `db:"active_turns"`
	ID                 string    `db:"id"`
	ExecutionProfileID string    `db:"execution_profile_id"`
	AccountKey         string    `db:"account_key"`
	Code               string    `db:"code"`
	ObservedAt         time.Time `db:"observed_at"`
	Turns5h            int64     `db:"turns_5h"`
	Tokens5h           int64     `db:"tokens_5h"`
	TurnsDay           int64     `db:"turns_day"`
	TokensDay          int64     `db:"tokens_day"`
	CostSubcentsDay    int64     `db:"cost_subcents_day"`
	TurnsWeek          int64     `db:"turns_week"`
	TokensWeek         int64     `db:"tokens_week"`
}

// initUsageLimitObservationsSchema creates the limit observation log and the
// route attempt index that round-robin continuation reads. Both are new, so
// CREATE ... IF NOT EXISTS is complete.
func (r *Repository) initUsageLimitObservationsSchema() error {
	_, err := r.db.ExecContext(r.migrationContext(), `
		CREATE TABLE IF NOT EXISTS usage_limit_observations (
			id TEXT PRIMARY KEY,
			execution_profile_id TEXT NOT NULL,
			account_key TEXT NOT NULL DEFAULT '',
			code TEXT NOT NULL DEFAULT '',
			observed_at TIMESTAMP NOT NULL,
			turns_5h BIGINT NOT NULL DEFAULT 0,
			tokens_5h BIGINT NOT NULL DEFAULT 0,
			turns_day BIGINT NOT NULL DEFAULT 0,
			tokens_day BIGINT NOT NULL DEFAULT 0,
			cost_subcents_day BIGINT NOT NULL DEFAULT 0,
			turns_week BIGINT NOT NULL DEFAULT 0,
			tokens_week BIGINT NOT NULL DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_usage_limit_observations_account
			ON usage_limit_observations(account_key, observed_at);

		CREATE TABLE IF NOT EXISTS provider_account_limits (
			account_key TEXT PRIMARY KEY,
			config_json TEXT NOT NULL DEFAULT '{}',
			learned_json TEXT NOT NULL DEFAULT '{}',
			updated_at TIMESTAMP NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("create usage limit observations: %w", err)
	}
	for _, column := range []struct{ name, ddl string }{
		{"tier", `ALTER TABLE usage_limit_observations ADD COLUMN tier TEXT NOT NULL DEFAULT ''`},
		{"dimension", `ALTER TABLE usage_limit_observations ADD COLUMN dimension TEXT NOT NULL DEFAULT ''`},
		{"requests_minute", `ALTER TABLE usage_limit_observations ADD COLUMN requests_minute REAL NOT NULL DEFAULT 0`},
		{"requests_day", `ALTER TABLE usage_limit_observations ADD COLUMN requests_day REAL NOT NULL DEFAULT 0`},
		{"active_turns", `ALTER TABLE usage_limit_observations ADD COLUMN active_turns BIGINT NOT NULL DEFAULT 0`},
		{"window_usage_json", `ALTER TABLE usage_limit_observations ADD COLUMN window_usage_json TEXT NOT NULL DEFAULT '{}'`},
	} {
		if err := r.migrate.Apply("usage_limit_observations."+column.name, column.ddl); err != nil {
			return err
		}
	}

	return nil
}

// InsertUsageLimitObservation appends one observation.
func (r *Repository) InsertUsageLimitObservation(ctx context.Context, observation UsageLimitObservation) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO usage_limit_observations (
			id, execution_profile_id, account_key, code, observed_at,
			turns_5h, tokens_5h, turns_day, tokens_day, cost_subcents_day, turns_week, tokens_week,
			tier,dimension,requests_minute,requests_day,active_turns,window_usage_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		observation.ID, observation.ExecutionProfileID, observation.AccountKey, observation.Code,
		observation.ObservedAt.UTC(),
		observation.Turns5h, observation.Tokens5h, observation.TurnsDay, observation.TokensDay,
		observation.CostSubcentsDay, observation.TurnsWeek, observation.TokensWeek,
		observation.Tier, observation.Dimension, observation.RequestsMinute, observation.RequestsDay, observation.ActiveTurns,
		observation.WindowUsageJSON,
	)
	return err
}

// ListUsageLimitObservationsSince returns observations at or after since,
// newest first.
func (r *Repository) ListUsageLimitObservationsSince(
	ctx context.Context,
	since time.Time,
) ([]UsageLimitObservation, error) {
	var observations []UsageLimitObservation
	err := r.ro.SelectContext(ctx, &observations, r.ro.Rebind(`
		SELECT id, execution_profile_id, account_key, code, observed_at,
		       turns_5h, tokens_5h, turns_day, tokens_day, cost_subcents_day, turns_week, tokens_week,
		       tier,dimension,requests_minute,requests_day,active_turns,window_usage_json
		  FROM usage_limit_observations
		 WHERE observed_at >= ?
		 ORDER BY observed_at DESC
	`), since.UTC())
	return observations, err
}
