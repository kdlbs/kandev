package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/office/routing"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// ListSeatSteps returns one row per ensure_participant_seat action in the
// workspace's workflows, hidden workflows included, ordered by workflow name,
// step position, and role. An events value that does not decode is an error.
func (r *Repository) ListSeatSteps(ctx context.Context, workspaceID string) ([]routing.SeatStep, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT w.id, w.name, s.id, s.name, s.events
		FROM workflow_steps s
		JOIN workflows w ON w.id = s.workflow_id
		WHERE w.workspace_id = ? AND s.events LIKE ?
		ORDER BY w.name, s.position, s.id
	`), workspaceID, "%"+string(wfmodels.OnEnterEnsureParticipantSeat)+"%")
	if err != nil {
		return nil, fmt.Errorf("list seat steps: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []routing.SeatStep
	for rows.Next() {
		var wfID, wfName, stepID, stepName string
		var raw sql.NullString
		if err := rows.Scan(&wfID, &wfName, &stepID, &stepName, &raw); err != nil {
			return nil, fmt.Errorf("scan seat step: %w", err)
		}
		var events wfmodels.StepEvents
		if err := json.Unmarshal([]byte(raw.String), &events); err != nil {
			return nil, fmt.Errorf("decode workflow step %s events: %w", stepID, err)
		}
		roles := wfmodels.ParticipantSeatRoles(events)
		for _, role := range sortedRoles(roles) {
			out = append(out, routing.SeatStep{
				WorkflowID: wfID, WorkflowName: wfName, StepID: stepID, StepName: stepName,
				Role: role, ExcludeAuthorProvider: roles[role],
			})
		}
	}
	return out, rows.Err()
}
