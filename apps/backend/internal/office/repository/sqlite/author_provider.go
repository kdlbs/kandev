package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/office/routing"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// GetWorkflowStepEvents returns a workflow step's decoded event actions. A
// missing step or an empty events column yields empty events. An events
// value that does not decode is an error, so a corrupt row can never read as
// "no declaration".
func (r *Repository) GetWorkflowStepEvents(ctx context.Context, stepID string) (wfmodels.StepEvents, error) {
	var events wfmodels.StepEvents
	if stepID == "" {
		return events, nil
	}
	var raw sql.NullString
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT events FROM workflow_steps WHERE id = ?`,
	), stepID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return events, nil
	}
	if err != nil {
		return events, fmt.Errorf("read workflow step events: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return events, nil
	}
	if err := json.Unmarshal([]byte(raw.String), &events); err != nil {
		return events, fmt.Errorf("decode workflow step %s events: %w", stepID, err)
	}
	return events, nil
}

// GetTaskAuthorProvider returns the provider of the task's most recently
// launched work-stage run: a run whose payload names the task and a
// workflow step with stage_type 'work' (workflow_step_id, else the
// queue-time queued_workflow_step_id), and whose launch recorded a resolved
// provider. Empty means no author is recorded.
func (r *Repository) GetTaskAuthorProvider(ctx context.Context, taskID string) (string, error) {
	if taskID == "" {
		return "", nil
	}
	driver := r.ro.DriverName()
	query := fmt.Sprintf(`
		SELECT r.resolved_provider_id FROM runs r
		JOIN workflow_steps ws ON ws.id = COALESCE(NULLIF(%s, ''), %s)
		WHERE %s = ?
		  AND ws.stage_type = ?
		  AND r.resolved_provider_id IS NOT NULL
		  AND r.resolved_provider_id <> ''
		ORDER BY COALESCE(r.claimed_at, r.requested_at) DESC, r.requested_at DESC, r.id DESC
		LIMIT 1
	`, dialect.JSONExtract(driver, "r.payload", "workflow_step_id"),
		dialect.JSONExtract(driver, "r.payload", runsmodels.QueuedWorkflowStepPayloadKey),
		dialect.JSONExtract(driver, "r.payload", "task_id"))
	var provider string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), taskID, string(wfmodels.StageTypeWork)).Scan(&provider)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read task author provider: %w", err)
	}
	return provider, nil
}

// ListAuthorExclusionDeclarations returns every step in the workspace's
// workflows whose ensure_participant_seat action sets
// exclude_author_provider, one entry per declared role. A step whose events
// do not decode is an error, so a corrupt row is reported rather than read as
// "no declaration".
func (r *Repository) ListAuthorExclusionDeclarations(
	ctx context.Context, workspaceID string,
) ([]routing.ExclusionDeclaration, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT w.id, w.name, s.id, s.name, s.events
		FROM workflow_steps s
		JOIN workflows w ON w.id = s.workflow_id
		WHERE w.workspace_id = ? AND s.events LIKE ?
		ORDER BY w.name, s.position, s.id
	`), workspaceID, "%"+wfmodels.ExcludeAuthorProviderConfigKey+"%")
	if err != nil {
		return nil, fmt.Errorf("list exclusion declarations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []routing.ExclusionDeclaration
	for rows.Next() {
		var wfID, wfName, stepID, stepName string
		var raw sql.NullString
		if err := rows.Scan(&wfID, &wfName, &stepID, &stepName, &raw); err != nil {
			return nil, fmt.Errorf("scan exclusion declaration: %w", err)
		}
		var events wfmodels.StepEvents
		if err := json.Unmarshal([]byte(raw.String), &events); err != nil {
			return nil, fmt.Errorf("decode workflow step %s events: %w", stepID, err)
		}
		for _, role := range sortedRoles(wfmodels.AuthorProviderExclusionRoles(events)) {
			out = append(out, routing.ExclusionDeclaration{
				WorkflowID: wfID, WorkflowName: wfName, StepID: stepID, StepName: stepName, Role: role,
			})
		}
	}
	return out, rows.Err()
}

func sortedRoles(roles map[string]bool) []string {
	out := make([]string, 0, len(roles))
	for role := range roles {
		out = append(out, role)
	}
	sort.Strings(out)
	return out
}
