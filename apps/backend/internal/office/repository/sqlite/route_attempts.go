package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/office/models"
)

// AppendRouteAttempt persists one (run_id, seq) row. Callers are
// responsible for monotonically incrementing Seq; conflicts on the
// primary key are surfaced as errors so the scheduler's
// "current_route_attempt_seq" bookkeeping catches accidental reuse.
func (r *Repository) AppendRouteAttempt(
	ctx context.Context, a *models.RouteAttempt,
) error {
	if a == nil {
		return fmt.Errorf("route_attempts: nil attempt")
	}
	if a.RunID == "" {
		return fmt.Errorf("route_attempts: empty run id")
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_run_route_attempts (
			run_id, seq, provider_id, model, tier, outcome,
			error_code, error_confidence, adapter_phase, classifier_rule,
			exit_code, raw_excerpt, reset_hint, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), a.RunID, a.Seq, a.ProviderID, a.Model, a.Tier, a.Outcome,
		nullableString(a.ErrorCode), nullableString(string(a.ErrorConfidence)),
		nullableString(string(a.AdapterPhase)), nullableString(a.ClassifierRule),
		a.ExitCode, nullableString(a.RawExcerpt), a.ResetHint,
		a.StartedAt, a.FinishedAt)
	if err != nil {
		return fmt.Errorf("route_attempts: insert run=%s seq=%d: %w",
			a.RunID, a.Seq, err)
	}
	return nil
}

// ListRouteAttempts returns every recorded attempt for runID ordered
// by seq. Empty slice (not nil) when the run has no attempts yet so
// callers can range over the result safely.
func (r *Repository) ListRouteAttempts(
	ctx context.Context, runID string,
) ([]models.RouteAttempt, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT run_id, seq, provider_id, model, tier, outcome,
			COALESCE(error_code,'') AS error_code,
			COALESCE(error_confidence,'') AS error_confidence,
			COALESCE(adapter_phase,'') AS adapter_phase,
			COALESCE(classifier_rule,'') AS classifier_rule,
			exit_code,
			COALESCE(raw_excerpt,'') AS raw_excerpt,
			reset_hint, started_at, finished_at
		FROM office_run_route_attempts
		WHERE run_id = ?
		ORDER BY seq ASC
	`), runID)
	if err != nil {
		return nil, fmt.Errorf("route_attempts: list run=%s: %w", runID, err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]models.RouteAttempt, 0)
	for rows.Next() {
		var a models.RouteAttempt
		if err := rows.StructScan(&a); err != nil {
			return nil, fmt.Errorf("route_attempts: scan: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("route_attempts: iterate: %w", err)
	}
	return out, nil
}

// nullableString returns nil for the empty string so SQLite stores NULL
// rather than "" — keeps the optional columns honest for downstream
// filters.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
