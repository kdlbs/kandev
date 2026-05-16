package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// likeEscaper escapes SQLite LIKE metacharacters so a caller-supplied
// substring is matched literally. Used with `LIKE ? ESCAPE '\'`.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// SetRunRoutingDecision persists the immutable routing decision the
// resolver returned for a run: the JSON-encoded provider order and the
// requested tier. Idempotent: subsequent calls overwrite the snapshot
// (the dispatcher only calls this on the first attempt for a run).
func (r *Repository) SetRunRoutingDecision(
	ctx context.Context, runID, providerOrderJSON, requestedTier string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET logical_provider_order = ?, requested_tier = ?
		WHERE id = ?
	`), providerOrderJSON, requestedTier, runID)
	if err != nil {
		return fmt.Errorf("run_routing: set decision: %w", err)
	}
	return nil
}

// SetRunResolvedProvider records the provider/model that launched a
// run. Called from the dispatch success branch.
func (r *Repository) SetRunResolvedProvider(
	ctx context.Context, runID, providerID, model string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET resolved_provider_id = ?, resolved_model = ?
		WHERE id = ?
	`), providerID, model, runID)
	if err != nil {
		return fmt.Errorf("run_routing: set resolved: %w", err)
	}
	return nil
}

// IncrementRouteAttemptSeq atomically bumps the run's
// current_route_attempt_seq and returns the new value. Used to monotonically
// number RouteAttempt rows even across post-start fallbacks.
func (r *Repository) IncrementRouteAttemptSeq(
	ctx context.Context, runID string,
) (int, error) {
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET current_route_attempt_seq = current_route_attempt_seq + 1
		WHERE id = ?
	`), runID); err != nil {
		return 0, fmt.Errorf("run_routing: bump seq: %w", err)
	}
	var n int
	if err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT current_route_attempt_seq FROM runs WHERE id = ?
	`), runID).Scan(&n); err != nil {
		return 0, fmt.Errorf("run_routing: read seq: %w", err)
	}
	return n, nil
}

// UpdateRouteAttemptOutcome updates the in-flight attempt row keyed by
// (run_id, seq) with the classifier outcome. Called after a launch
// fails or after a post-start classification.
func (r *Repository) UpdateRouteAttemptOutcome(
	ctx context.Context, attempt *models.RouteAttempt,
) error {
	if attempt == nil || attempt.RunID == "" {
		return fmt.Errorf("run_routing: nil attempt")
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_run_route_attempts
		SET outcome = ?, error_code = ?, error_confidence = ?,
		    adapter_phase = ?, classifier_rule = ?, exit_code = ?,
		    raw_excerpt = ?, reset_hint = ?, finished_at = ?
		WHERE run_id = ? AND seq = ?
	`), attempt.Outcome, nullableString(attempt.ErrorCode),
		nullableString(attempt.ErrorConfidence), nullableString(attempt.AdapterPhase),
		nullableString(attempt.ClassifierRule), attempt.ExitCode,
		nullableString(attempt.RawExcerpt), attempt.ResetHint, attempt.FinishedAt,
		attempt.RunID, attempt.Seq)
	if err != nil {
		return fmt.Errorf("run_routing: update attempt: %w", err)
	}
	return nil
}

// ParkRunForProviderCapacity flips a run into the "waiting for provider
// capacity" parked state. retry_count is NOT incremented — parking is
// not a retry, just a deferral. The next eligible-claim pass will
// re-pick the run after earliest_retry_at.
func (r *Repository) ParkRunForProviderCapacity(
	ctx context.Context, runID, blockedStatus string,
	earliestRetryAt time.Time,
) error {
	var retryAt interface{}
	if !earliestRetryAt.IsZero() {
		retryAt = earliestRetryAt
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'queued',
		    routing_blocked_status = ?,
		    earliest_retry_at = ?,
		    scheduled_retry_at = ?,
		    claimed_at = NULL,
		    finished_at = NULL
		WHERE id = ?
	`), blockedStatus, retryAt, retryAt, runID)
	if err != nil {
		return fmt.Errorf("run_routing: park: %w", err)
	}
	return nil
}

// ClearRoutingBlock clears the routing-block columns on a run. Called
// from the wake-up path when a parked run becomes eligible again.
// scheduled_retry_at is also cleared so the run is immediately eligible
// — leaving it set would re-block the run at the eligibility filter even
// after the routing block clears (manifests as a "Retry now" UI that
// silently waits for the original retry time).
func (r *Repository) ClearRoutingBlock(ctx context.Context, runID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET routing_blocked_status = NULL,
		    earliest_retry_at = NULL,
		    scheduled_retry_at = NULL
		WHERE id = ?
	`), runID)
	if err != nil {
		return fmt.Errorf("run_routing: clear block: %w", err)
	}
	return nil
}

// BumpRouteCycleBaseline sets route_cycle_baseline_seq =
// current_route_attempt_seq in a single UPDATE so the next dispatch
// cycle does not inherit the exclusion list from prior failed attempts.
// Called when a parked run is lifted (auto wake-up, manual retry) — the
// "retry cycle" semantically restarts and every provider in the order
// becomes eligible again. Post-start fallback intentionally does NOT
// call this: within one cycle, an exhausted provider stays excluded.
func (r *Repository) BumpRouteCycleBaseline(ctx context.Context, runID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET route_cycle_baseline_seq = current_route_attempt_seq
		WHERE id = ?
	`), runID)
	if err != nil {
		return fmt.Errorf("run_routing: bump baseline: %w", err)
	}
	return nil
}

// RequeueRunForNextCandidate flips a claimed run back to queued so the
// next dispatch pass picks a fresh candidate. Preserves the
// LogicalProviderOrder / RequestedTier / CurrentRouteAttemptSeq cursor
// so dispatchWithRouting can derive the exclude-set from prior attempts.
func (r *Repository) RequeueRunForNextCandidate(
	ctx context.Context, runID string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'queued', session_id = '',
		    claimed_at = NULL, finished_at = NULL
		WHERE id = ?
	`), runID)
	if err != nil {
		return fmt.Errorf("run_routing: requeue: %w", err)
	}
	return nil
}

// ListPendingProviderCapacityRuns returns runs parked under
// waiting_for_provider_capacity whose earliest_retry_at has passed.
// Used by the scheduler wake-up tick to lift parked runs.
func (r *Repository) ListPendingProviderCapacityRuns(
	ctx context.Context, now time.Time,
) ([]models.Run, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT * FROM runs
		WHERE routing_blocked_status = 'waiting_for_provider_capacity'
		  AND status = 'queued'
		  AND (earliest_retry_at IS NULL OR earliest_retry_at <= ?)
		ORDER BY earliest_retry_at ASC
		LIMIT 50
	`), now)
	if err != nil {
		return nil, fmt.Errorf("run_routing: list pending: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]models.Run, 0)
	for rows.Next() {
		var run models.Run
		if err := rows.StructScan(&run); err != nil {
			return nil, fmt.Errorf("run_routing: scan pending: %w", err)
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// ListRunsWaitingOnProvider returns parked runs that listed the given
// provider in their logical order — those are the runs that benefit
// from re-dispatch when the provider flips back to healthy. Best-effort
// matching: the logical_provider_order column is a JSON array, so we
// LIKE-match a JSON token. Spurious hits are harmless because the
// resolver re-evaluates eligibility on the next attempt.
func (r *Repository) ListRunsWaitingOnProvider(
	ctx context.Context, workspaceID, providerID string,
) ([]models.Run, error) {
	// providerID is caller-supplied; escape LIKE metacharacters so a value
	// like "%" cannot match every parked run in the workspace.
	escaped := likeEscaper.Replace(providerID)
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT w.* FROM runs w
		JOIN agent_profiles a ON a.id = w.agent_profile_id
		WHERE w.routing_blocked_status IS NOT NULL
		  AND w.status = 'queued'
		  AND a.workspace_id = ?
		  AND w.logical_provider_order LIKE ? ESCAPE '\'
		LIMIT 50
	`), workspaceID, "%\""+escaped+"\"%")
	if err != nil {
		return nil, fmt.Errorf("run_routing: list waiting: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]models.Run, 0)
	for rows.Next() {
		var run models.Run
		if err := rows.StructScan(&run); err != nil {
			return nil, fmt.Errorf("run_routing: scan waiting: %w", err)
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// ForceProviderRetryNow sets retry_at = now for the (workspace, provider)
// rows so the next dispatch picks the provider up. Used by the HTTP
// retry endpoint when no ProviderProber is registered.
func (r *Repository) ForceProviderRetryNow(
	ctx context.Context, workspaceID, providerID string,
) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_provider_health
		SET retry_at = ?, updated_at = ?
		WHERE workspace_id = ? AND provider_id = ? AND state = 'degraded'
	`), now, now, workspaceID, providerID)
	if err != nil {
		return fmt.Errorf("run_routing: force retry: %w", err)
	}
	return nil
}

// ClearAllParkedRoutingForWorkspace clears the routing-block columns on
// every run in the workspace's parked set and sets status=queued so the
// next scheduler tick can claim them via the legacy (non-routing) path.
//
// Called by the dashboard RoutingProvider when a workspace flips
// office_workspace_routing.enabled from true → false: parked runs would
// otherwise sit forever waiting for a provider that no longer matters,
// because the dispatcher only re-evaluates blocked runs through the
// routing flow.
func (r *Repository) ClearAllParkedRoutingForWorkspace(
	ctx context.Context, workspaceID string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'queued',
		    routing_blocked_status = NULL,
		    earliest_retry_at = NULL,
		    scheduled_retry_at = NULL
		WHERE routing_blocked_status IS NOT NULL
		  AND agent_profile_id IN (
		    SELECT id FROM agent_profiles WHERE workspace_id = ?
		  )
	`), workspaceID)
	if err != nil {
		return fmt.Errorf("run_routing: clear parked workspace=%s: %w",
			workspaceID, err)
	}
	return nil
}

// GetRunByID is a thin wrapper that returns sql.ErrNoRows directly so
// callers can distinguish "not found" from generic errors. Mirrors the
// runs queue GetRun method but lives on the office repo for consumers
// that already hold an office repo handle.
func (r *Repository) GetRunByID(ctx context.Context, id string) (*models.Run, error) {
	run, err := r.GetRun(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return run, err
}
