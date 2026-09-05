package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
)

// CreateSessionControlEvent writes an immutable authorized control outcome.
func (r *Repository) CreateSessionControlEvent(ctx context.Context, event *models.SessionControlEvent) error {
	if event == nil || event.ActorTaskID == "" || event.ActorSessionID == "" ||
		event.TargetTaskID == "" || event.TargetSessionID == "" || event.TargetTurnID == "" ||
		event.AuthorityBasis == "" || event.EvidenceCode == "" || event.Result == "" {
		return fmt.Errorf("session control event requires actor, target, authority, evidence, and result")
	}
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO session_control_events (
			id, actor_task_id, actor_session_id, target_task_id, target_session_id,
			target_turn_id, authority_basis, evidence_code, result, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), event.ID, event.ActorTaskID, event.ActorSessionID, event.TargetTaskID,
		event.TargetSessionID, event.TargetTurnID, event.AuthorityBasis,
		event.EvidenceCode, event.Result, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert session control event: %w", err)
	}
	return nil
}
