package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/office/models"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
)

// authorParkedPredicate matches a run parked by routing in its current retry
// cycle after skipping the author's provider. A cycle that also deferred a
// launch to the session ceiling is excluded: the ceiling's replay owns that
// launch, and re-queueing the run would launch it twice. Session-recovery
// parks are never matched. Arguments: skip outcome, deferral outcome,
// session-recovery status.
const authorParkedPredicate = `
	runs.status = 'queued'
	AND runs.routing_blocked_status IS NOT NULL
	AND runs.routing_blocked_status <> ?
	AND EXISTS (
		SELECT 1 FROM office_run_route_attempts a
		WHERE a.run_id = runs.id AND a.seq > runs.route_cycle_baseline_seq AND a.outcome = ?
	)
	AND NOT EXISTS (
		SELECT 1 FROM office_run_route_attempts a
		WHERE a.run_id = runs.id AND a.seq > runs.route_cycle_baseline_seq AND a.outcome = ?
	)`

func authorParkedArgs() []interface{} {
	return []interface{}{
		runsmodels.RoutingBlockedSessionRecoveryRequired,
		string(models.RouteAttemptOutcomeSkippedAuthorProvider),
		string(models.RouteAttemptOutcomeDeferredCapacity),
	}
}

// ReleaseAuthorExclusionParkedRuns re-queues the workspace's runs queued at
// one of stepIDs that parked after skipping the author's provider, and returns
// how many it released. The match is re-checked in the write, so a run that
// launched or changed park reason after it was listed is left alone.
func (r *Repository) ReleaseAuthorExclusionParkedRuns(
	ctx context.Context, workspaceID string, stepIDs []string,
) (int, error) {
	if workspaceID == "" || len(stepIDs) == 0 {
		return 0, nil
	}
	runIDs, err := r.listAuthorParkedRunsAtSteps(ctx, workspaceID, stepIDs)
	if err != nil || len(runIDs) == 0 {
		return 0, err
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(runIDs)), ",")
	args := []interface{}{runsmodels.PriorityClassHuman, runsmodels.PriorityClassRecovery}
	for _, id := range runIDs {
		args = append(args, id)
	}
	args = append(args, authorParkedArgs()...)
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE runs
		SET status = 'queued',
		    routing_blocked_status = NULL,
		    earliest_retry_at = NULL,
		    scheduled_retry_at = NULL,
		    priority_class = CASE WHEN priority_class = ? THEN priority_class ELSE ? END
		WHERE runs.id IN (`+placeholders+`) AND `+authorParkedPredicate), args...)
	if err != nil {
		return 0, fmt.Errorf("run_routing: release author-exclusion parks workspace=%s: %w", workspaceID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// listAuthorParkedRunsAtSteps returns the ids of the workspace's
// author-exclusion parked runs whose payload names one of stepIDs.
func (r *Repository) listAuthorParkedRunsAtSteps(
	ctx context.Context, workspaceID string, stepIDs []string,
) ([]string, error) {
	args := append([]interface{}{workspaceID}, authorParkedArgs()...)
	var rows []struct {
		ID      string `db:"id"`
		Payload string `db:"payload"`
	}
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT runs.id, COALESCE(runs.payload, '') AS payload FROM runs
		WHERE runs.agent_profile_id IN (SELECT id FROM agent_profiles WHERE workspace_id = ?)
		  AND `+authorParkedPredicate), args...); err != nil {
		return nil, fmt.Errorf("run_routing: list author-exclusion parks workspace=%s: %w", workspaceID, err)
	}
	wanted := make(map[string]bool, len(stepIDs))
	for _, id := range stepIDs {
		wanted[id] = true
	}
	var out []string
	for _, row := range rows {
		if wanted[payloadWorkflowStepID(row.Payload)] {
			out = append(out, row.ID)
		}
	}
	return out, nil
}

func payloadWorkflowStepID(payload string) string {
	if payload == "" {
		return ""
	}
	var p struct {
		WorkflowStepID string `json:"workflow_step_id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return ""
	}
	return p.WorkflowStepID
}
