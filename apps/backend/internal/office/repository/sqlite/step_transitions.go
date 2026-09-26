package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/kandev/kandev/internal/steptelemetry"
)

// StepTransitionActor is the subset of one task_step_transitions ledger
// row a run-causation carrier resolver needs: who (or what) caused the
// transition, which session it happened on (for the agent case), and
// when — used to find the run that was claimed on that session at that
// moment (AC-OFFICE-RUN-CAUSATION-001.25). This table is owned by
// internal/task/repository/sqlite; office reads it directly the same way
// GetTaskMetadata reads the tasks table.
type StepTransitionActor struct {
	ActorKind  steptelemetry.ActorKind
	ActorID    string
	SessionID  string
	OccurredAt time.Time
}

// GetStepTransitionActor reads the actor attribution recorded on ledger
// row transitionID. Returns (nil, sql.ErrNoRows) when the row does not
// exist, so a resolver can fall back to the ordinary task-boundary carrier
// without distinguishing that case from any other lookup failure.
func (r *Repository) GetStepTransitionActor(ctx context.Context, transitionID int64) (*StepTransitionActor, error) {
	var actorKind string
	var actorID, sessionID sql.NullString
	var occurredAt time.Time
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT actor_kind, actor_id, session_id, occurred_at
		FROM task_step_transitions
		WHERE id = ?
	`), transitionID).Scan(&actorKind, &actorID, &sessionID, &occurredAt)
	if err != nil {
		return nil, err
	}
	return &StepTransitionActor{
		ActorKind:  steptelemetry.ActorKind(actorKind),
		ActorID:    actorID.String,
		SessionID:  sessionID.String,
		OccurredAt: occurredAt,
	}, nil
}
