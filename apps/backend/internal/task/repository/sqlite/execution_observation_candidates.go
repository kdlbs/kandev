package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
)

type ExecutionObservationCandidate struct {
	OrgID    string
	Snapshot *models.ExecutionSnapshot
}

// ListExecutionObservationCandidates returns bounded pages of current sandbox attempts.
func (r *Repository) ListExecutionObservationCandidates(ctx context.Context, after string, limit int) ([]ExecutionObservationCandidate, error) {
	if limit < 1 || limit > 100 {
		return nil, errExecutionSnapshotConflict
	}
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT snap.payload,s.id,s.task_id,COALESCE(w.org_id,'')
  FROM agent_execution_observations o
  JOIN agent_execution_snapshots snap ON snap.attempt_id=o.attempt_id
  JOIN task_sessions s ON s.id=snap.session_id
  JOIN tasks t ON t.id=s.task_id
  JOIN workspaces w ON w.id=t.workspace_id
  WHERE o.status NOT IN ('stopped','failed') AND snap.attempt_id>?
   AND NOT EXISTS (SELECT 1 FROM agent_execution_snapshots newer WHERE newer.session_id=snap.session_id AND newer.sequence>snap.sequence)
  ORDER BY snap.attempt_id LIMIT ?`), after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]ExecutionObservationCandidate, 0)
	for rows.Next() {
		var raw string
		var session models.TaskSession
		var value ExecutionObservationCandidate
		if err = rows.Scan(&raw, &session.ID, &session.TaskID, &value.OrgID); err != nil {
			return nil, err
		}
		value.Snapshot = decodeExecutionSnapshot(raw, &session)
		if value.Snapshot == nil || value.Snapshot.Sandbox == nil {
			return nil, errExecutionSnapshotConflict
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
