package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

// Scope constants for office_provider_health.scope. The triple
// (workspace_id, provider_id, scope, scope_value) is the primary key
// so a tier-specific failure does not take the whole provider down.
const (
	HealthScopeProvider = "provider"
	HealthScopeModel    = "model"
	HealthScopeTier     = "tier"
)

// State constants for office_provider_health.state. Mirrors the spec's
// three high-level states.
const (
	HealthStateHealthy            = "healthy"
	HealthStateDegraded           = "degraded"
	HealthStateUserActionRequired = "user_action_required"
)

// Routing error codes recognised by ScopeFromCode. Kept here as
// constants (rather than imported from a routingerr package) to keep
// Phase 1 self-contained — routingerr will land in Phase 3 and will
// alias these strings. The set must stay in sync with the spec's
// normalized codes.
const (
	codeAuthRequired          = "auth_required"
	codeMissingCredentials    = "missing_credentials"
	codeSubscriptionRequired  = "subscription_required"
	codeQuotaLimited          = "quota_limited"
	codeRateLimited           = "rate_limited"
	codeProviderUnavailable   = "provider_unavailable"
	codeProviderNotConfigured = "provider_not_configured"
	codeModelUnavailable      = "model_unavailable"
)

// ScopeFromCode maps a normalized routing error code to the health
// scope it should degrade. model_unavailable → ("model", <model>);
// provider_not_configured for a missing tier mapping → ("tier", <tier>);
// every other auth/quota/rate/provider failure → ("provider", "").
//
// The function is intentionally simple — adding new codes is a
// classifier-level change, not a schema change.
func ScopeFromCode(code, model string, tier routing.Tier) (scope, value string) {
	switch code {
	case codeModelUnavailable:
		return HealthScopeModel, model
	case codeProviderNotConfigured:
		return HealthScopeTier, string(tier)
	case codeAuthRequired, codeMissingCredentials, codeSubscriptionRequired,
		codeQuotaLimited, codeRateLimited, codeProviderUnavailable:
		return HealthScopeProvider, ""
	}
	return HealthScopeProvider, ""
}

// GetProviderHealth returns the health row for a single
// (workspace, provider, scope, scopeValue) tuple. Returns
// (nil, nil) when the row does not exist so callers can treat
// "no record" as "healthy" without checking sentinel errors.
func (r *Repository) GetProviderHealth(
	ctx context.Context, workspaceID, providerID, scope, scopeValue string,
) (*models.ProviderHealth, error) {
	var h models.ProviderHealth
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT workspace_id, provider_id, scope, scope_value, state,
			COALESCE(error_code,'') AS error_code,
			retry_at, backoff_step,
			last_failure, last_success,
			COALESCE(raw_excerpt,'') AS raw_excerpt,
			updated_at
		FROM office_provider_health
		WHERE workspace_id = ? AND provider_id = ? AND scope = ? AND scope_value = ?
	`), workspaceID, providerID, scope, scopeValue).StructScan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("provider_health: get: %w", err)
	}
	return &h, nil
}

// ListProviderHealth returns every non-healthy row for the workspace.
// Healthy rows are filtered out at the SQL layer because callers (the
// inbox composer, the resolver) only need to know about routes that
// are currently restricted.
func (r *Repository) ListProviderHealth(
	ctx context.Context, workspaceID string,
) ([]models.ProviderHealth, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT workspace_id, provider_id, scope, scope_value, state,
			COALESCE(error_code,'') AS error_code,
			retry_at, backoff_step,
			last_failure, last_success,
			COALESCE(raw_excerpt,'') AS raw_excerpt,
			updated_at
		FROM office_provider_health
		WHERE workspace_id = ? AND state != ?
		ORDER BY provider_id, scope, scope_value
	`), workspaceID, HealthStateHealthy)
	if err != nil {
		return nil, fmt.Errorf("provider_health: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]models.ProviderHealth, 0)
	for rows.Next() {
		var h models.ProviderHealth
		if err := rows.StructScan(&h); err != nil {
			return nil, fmt.Errorf("provider_health: scan: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("provider_health: iterate: %w", err)
	}
	return out, nil
}

// MarkProviderDegraded upserts a degraded (or user-action-required)
// row. On a degraded→degraded transition (the previous row was already
// non-healthy) the stored backoff_step is incremented by one — so
// repeated probe failures escalate even when the caller passes the
// same h. The caller-supplied BackoffStep is used as the floor for
// the first transition into non-healthy.
//
// last_failure is stamped to now if the caller did not set it.
func (r *Repository) MarkProviderDegraded(
	ctx context.Context, h models.ProviderHealth,
) error {
	if h.WorkspaceID == "" || h.ProviderID == "" {
		return fmt.Errorf("provider_health: workspace and provider ids required")
	}
	if h.State == "" || h.State == HealthStateHealthy {
		return fmt.Errorf("provider_health: degraded mark with state %q", h.State)
	}
	now := time.Now().UTC()
	lastFailure := h.LastFailure
	if lastFailure == nil {
		lastFailure = &now
	}
	step, err := r.nextBackoffStep(ctx, h)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_provider_health (
			workspace_id, provider_id, scope, scope_value, state,
			error_code, retry_at, backoff_step,
			last_failure, last_success, raw_excerpt, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, provider_id, scope, scope_value) DO UPDATE SET
			state = excluded.state,
			error_code = excluded.error_code,
			retry_at = excluded.retry_at,
			backoff_step = excluded.backoff_step,
			last_failure = excluded.last_failure,
			raw_excerpt = excluded.raw_excerpt,
			updated_at = excluded.updated_at
	`), h.WorkspaceID, h.ProviderID, h.Scope, h.ScopeValue, h.State,
		nullableString(h.ErrorCode), h.RetryAt, step,
		lastFailure, h.LastSuccess, nullableString(h.RawExcerpt), now)
	if err != nil {
		return fmt.Errorf("provider_health: upsert: %w", err)
	}
	return nil
}

// nextBackoffStep returns h.BackoffStep on a fresh row or transition
// from healthy, and existingStep+1 when the prior row was already
// degraded / user_action_required.
func (r *Repository) nextBackoffStep(
	ctx context.Context, h models.ProviderHealth,
) (int, error) {
	var existingStep int
	var existingState string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT backoff_step, state
		FROM office_provider_health
		WHERE workspace_id = ? AND provider_id = ? AND scope = ? AND scope_value = ?
	`), h.WorkspaceID, h.ProviderID, h.Scope, h.ScopeValue).Scan(&existingStep, &existingState)
	if errors.Is(err, sql.ErrNoRows) {
		return h.BackoffStep, nil
	}
	if err != nil {
		return 0, fmt.Errorf("provider_health: read prior step: %w", err)
	}
	if existingState == HealthStateHealthy {
		return h.BackoffStep, nil
	}
	return existingStep + 1, nil
}

// MarkProviderHealthy flips an existing row back to healthy, clearing
// the backoff, retry deadline, and error code. last_success is stamped
// to now. A no-op when the row does not exist — there is nothing to
// transition out of in that case.
func (r *Repository) MarkProviderHealthy(
	ctx context.Context, workspaceID, providerID, scope, scopeValue string,
) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_provider_health
		SET state = ?, backoff_step = 0, retry_at = NULL,
			error_code = NULL, last_success = ?, updated_at = ?
		WHERE workspace_id = ? AND provider_id = ? AND scope = ? AND scope_value = ?
	`), HealthStateHealthy, now, now,
		workspaceID, providerID, scope, scopeValue)
	if err != nil {
		return fmt.Errorf("provider_health: mark healthy: %w", err)
	}
	return nil
}
