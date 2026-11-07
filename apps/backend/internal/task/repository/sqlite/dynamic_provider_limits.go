package sqlite

import (
	"context"
	"database/sql"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
)

// ListProviderLimits returns every operator-entered provider limit setting.
func (r *Repository) ListProviderLimits(ctx context.Context) ([]dynamicruntime.ProviderLimit, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT provider, monthly_reset_at, monthly_reset_timezone, block_until, updated_at
		FROM dynamic_provider_limits ORDER BY provider
	`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	limits := make([]dynamicruntime.ProviderLimit, 0)
	for rows.Next() {
		var limit dynamicruntime.ProviderLimit
		var monthly, block sql.NullTime
		if err := rows.Scan(&limit.Provider, &monthly, &limit.MonthlyResetTimezone, &block, &limit.UpdatedAt); err != nil {
			return nil, err
		}
		if monthly.Valid {
			value := monthly.Time
			limit.MonthlyResetAt = &value
		}
		if block.Valid {
			value := block.Time
			limit.BlockUntil = &value
		}
		limits = append(limits, limit)
	}
	return limits, rows.Err()
}

// SaveProviderLimit stores one provider's limit setting, replacing the
// previous value.
func (r *Repository) SaveProviderLimit(ctx context.Context, limit dynamicruntime.ProviderLimit) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO dynamic_provider_limits
			(provider, monthly_reset_at, monthly_reset_timezone, block_until, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(provider) DO UPDATE SET
			monthly_reset_at = excluded.monthly_reset_at,
			monthly_reset_timezone = excluded.monthly_reset_timezone,
			block_until = excluded.block_until,
			updated_at = excluded.updated_at
	`), limit.Provider, optionalTime(limit.MonthlyResetAt), limit.MonthlyResetTimezone,
		optionalTime(limit.BlockUntil), time.Now().UTC())
	return err
}

func optionalTime(value *time.Time) interface{} {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
}
