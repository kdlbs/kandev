package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

// RecordCausationRefusal appends one durable office_causation_refusal row
// for an enqueue refused by the causation-depth or a self-trigger gate
// (AC-OFFICE-LAUNCH-SAFETY-003.5, -004.3, -004.8). Standalone (no caller
// transaction): the refusal already unwound the enqueue transaction — see
// runs/service.causation.go's refusalRecorder doc comment — so this opens
// its own transaction on the writer pool rather than reusing one that has
// already committed or rolled back.
//
// Callers must never let an error from this method affect an admission
// decision (AC-OFFICE-BACKPRESSURE-003.4): log and continue.
func (r *Repository) RecordCausationRefusal(
	ctx context.Context, workspaceID, gate, agentProfileID, causationID string, causationDepth int, reason string,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_causation_refusal (
			id, workspace_id, gate, agent_profile_id, causation_id, causation_depth, reason, refused_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), uuid.New().String(), workspaceID, gate, agentProfileID, causationID, causationDepth, reason, time.Now().UTC())
	return err
}

// ListCausationRefusals returns every durable causation-refusal record for
// workspaceID, most recent first. Operator-visibility read for
// AC-OFFICE-LAUNCH-SAFETY-003.5's durable record; not used on any
// admission path.
func (r *Repository) ListCausationRefusals(ctx context.Context, workspaceID string) ([]models.CausationRefusalEntry, error) {
	var entries []models.CausationRefusalEntry
	err := r.db.SelectContext(ctx, &entries, r.db.Rebind(`
		SELECT id, workspace_id, gate, agent_profile_id, causation_id, causation_depth, reason, refused_at
		FROM office_causation_refusal
		WHERE workspace_id = ?
		ORDER BY refused_at DESC
	`), workspaceID)
	return entries, err
}
