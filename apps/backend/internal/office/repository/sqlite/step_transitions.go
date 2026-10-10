package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/kandev/kandev/internal/steptelemetry"
)

// StepTransitionActor is the subset of one task_step_transitions ledger
// row a run-causation carrier resolver needs: who (or what) caused the
// transition, which session it happened on, its causing run ID, and when
// (for legacy rows without a causing run ID). This table is owned by
// internal/task/repository/sqlite; office reads it directly the same way
// GetTaskMetadata reads the tasks table.
type StepTransitionActor struct {
	ActorKind    steptelemetry.ActorKind
	ActorID      string
	SessionID    string
	CausingRunID string
	OccurredAt   time.Time
}

// GetStepTransitionActor reads the actor attribution recorded on ledger
// row transitionID. Returns (nil, sql.ErrNoRows) when the row does not
// exist, so a resolver can fall back to the ordinary task-boundary carrier
// without distinguishing that case from any other lookup failure.
func (r *Repository) GetStepTransitionActor(ctx context.Context, transitionID int64) (*StepTransitionActor, error) {
	var actorKind string
	var actorID, sessionID, causingRunID sql.NullString
	var occurredAt time.Time
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT actor_kind, actor_id, session_id, causing_run_id, occurred_at
		FROM task_step_transitions
		WHERE id = ?
	`), transitionID).Scan(&actorKind, &actorID, &sessionID, &causingRunID, &occurredAt)
	if err != nil {
		return nil, err
	}
	return &StepTransitionActor{
		ActorKind:    steptelemetry.ActorKind(actorKind),
		ActorID:      actorID.String,
		SessionID:    sessionID.String,
		CausingRunID: causingRunID.String,
		OccurredAt:   occurredAt,
	}, nil
}
