package sqlite

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/runs/models"
)

// ResolveAgentProfileWorkspaceID returns the workspace_id for the given
// agent profile, so causation resolution can stamp a new run's workspace
// (AC-OFFICE-RUN-CAUSATION-001.20) before the row exists. Returns
// sql.ErrNoRows when the profile is unknown.
func (r *Repository) ResolveAgentProfileWorkspaceID(ctx context.Context, agentProfileID string) (string, error) {
	return resolveAgentProfileWorkspaceID(ctx, r.ro, agentProfileID)
}

// ResolveAgentProfileWorkspaceIDTx is ResolveAgentProfileWorkspaceID run
// against a caller-owned transaction, so it participates in the single
// enqueue transaction AC-OFFICE-LAUNCH-SAFETY-003.8 requires instead of
// racing it on a separate reader connection.
func (r *Repository) ResolveAgentProfileWorkspaceIDTx(ctx context.Context, tx *sqlx.Tx, agentProfileID string) (string, error) {
	return resolveAgentProfileWorkspaceID(ctx, tx, agentProfileID)
}

func resolveAgentProfileWorkspaceID(ctx context.Context, exec sqlExecutor, agentProfileID string) (string, error) {
	var workspaceID string
	err := exec.QueryRowxContext(ctx, exec.Rebind(`
		SELECT workspace_id FROM agent_profiles WHERE id = ?
	`), agentProfileID).Scan(&workspaceID)
	if err != nil {
		return "", err
	}
	return workspaceID, nil
}

// ResolveTaskWorkspaceIDTx returns the trusted workspace that owns a task.
// Global Kanban profiles have no workspace of their own, so a task-bound run
// for one of those profiles uses this value as its launch-safety scope.
func (r *Repository) ResolveTaskWorkspaceIDTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (string, error) {
	var workspaceID string
	err := tx.QueryRowxContext(ctx, tx.Rebind(`
		SELECT workspace_id FROM tasks WHERE id = ?
	`), taskID).Scan(&workspaceID)
	if err != nil {
		return "", err
	}
	return workspaceID, nil
}

// CountSelfTriggeredRuns counts runs queued for agentProfileID with the
// given reason, whose persisted actor kind is `agent` and actor id equals
// agentProfileID, requested strictly after since
// (AC-OFFICE-LAUNCH-SAFETY-004.7, the strict window boundary of
// AC-OFFICE-LAUNCH-SAFETY-005.8). Counts every such row regardless of its
// current status, per AC-OFFICE-LAUNCH-SAFETY-004.5: a refused wake was
// never inserted so it was never eligible to be counted, and a queued wake
// that later finished, failed, or was cancelled still counts.
func (r *Repository) CountSelfTriggeredRuns(
	ctx context.Context, agentProfileID, reason string, since time.Time,
) (int, error) {
	return countSelfTriggeredRuns(ctx, r.ro, agentProfileID, reason, since)
}

// CountSelfTriggeredRunsTx is CountSelfTriggeredRuns run against a
// caller-owned transaction (AC-OFFICE-LAUNCH-SAFETY-003.8): the count and
// the insert it gates must be serialized against every other concurrent
// enqueue for the same agent profile, which a read against the separate
// reader connection CountSelfTriggeredRuns uses cannot guarantee.
func (r *Repository) CountSelfTriggeredRunsTx(
	ctx context.Context, tx *sqlx.Tx, agentProfileID, reason string, since time.Time,
) (int, error) {
	return countSelfTriggeredRuns(ctx, tx, agentProfileID, reason, since)
}

func countSelfTriggeredRuns(ctx context.Context, exec sqlExecutor, agentProfileID, reason string, since time.Time) (int, error) {
	var count int
	err := exec.QueryRowxContext(ctx, exec.Rebind(`
		SELECT COUNT(*) FROM runs
		WHERE agent_profile_id = ? AND reason = ?
		  AND actor_kind = ? AND actor_id = ?
		  AND requested_at > ?
	`), agentProfileID, reason, string(models.ActorKindAgent), agentProfileID, since).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountSelfTriggeredRunsAnyReasonTx counts runs queued for
// agentProfileID whose persisted actor kind is `agent` and actor id
// equals agentProfileID, requested strictly after since, whatever
// reason they name (AC-OFFICE-LAUNCH-SAFETY-004.7/.8): the
// reason-independent sibling of CountSelfTriggeredRunsTx, differing
// from it in that one predicate only. Run against the caller's enqueue
// transaction for the same serialization reason CountSelfTriggeredRunsTx
// documents.
func (r *Repository) CountSelfTriggeredRunsAnyReasonTx(
	ctx context.Context, tx *sqlx.Tx, agentProfileID string, since time.Time,
) (int, error) {
	return countSelfTriggeredRunsAnyReason(ctx, tx, agentProfileID, since)
}

func countSelfTriggeredRunsAnyReason(ctx context.Context, exec sqlExecutor, agentProfileID string, since time.Time) (int, error) {
	var count int
	err := exec.QueryRowxContext(ctx, exec.Rebind(`
		SELECT COUNT(*) FROM runs
		WHERE agent_profile_id = ?
		  AND actor_kind = ? AND actor_id = ?
		  AND requested_at > ?
	`), agentProfileID, string(models.ActorKindAgent), agentProfileID, since).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
