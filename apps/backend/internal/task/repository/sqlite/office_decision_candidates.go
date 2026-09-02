package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/task/models"
)

// officeDecisionWaitScanLimit bounds one page. The detector runs on the idle
// reaper's tick and does per-candidate follow-up reads, so an unbounded result
// set would let a large parked backlog stretch a single tick past the scan
// budget. Keyset pagination lets one tick continue with later rows until that
// budget expires.
const officeDecisionWaitScanLimit = 200

// ListOfficeDecisionWaitCandidates returns Office tasks that sit at a workflow
// step carrying a decision-required seat and have not been touched since
// quietSince. Pass after to continue after a previous ordered page. The return
// cursor is nil when the result set is complete.
//
// This is deliberately only the cheap, indexable half of the
// decision-waiting predicate (REQ-OFFICE-STALL-VISIBILITY-002). It does not
// look at workflow_step_decisions or at the runs queue: those are judged by
// the orchestrator's detector so a candidate rejected for "a decision already
// exists" is distinguishable from one rejected for "a run is still in flight",
// and so an unreadable input can fail closed with its own counted reason
// instead of silently shrinking a SQL result set.
//
// Office identity comes from IsFromOfficePredicate — the same expression that
// backs models.Task.IsFromOffice — rather than a hand-rolled project_id test,
// because IsFromOffice is a read-time projection and a second, drifting
// definition of "Office task" is exactly the bug that would reintroduce the
// exclusion this feature exists to remove.
//
// tasks.updated_at is the quiet-time anchor. It is an approximation of "when
// the task entered this step": an unrelated edit to the task row (a title
// change, a reorder) restarts the clock and delays surfacing. That error is
// one-directional — it under-surfaces, never over-surfaces — which is the
// right direction for a detector whose stated non-goal is acting on what it
// finds. The exact anchor is task_step_transitions.occurred_at, which has no
// read API yet; tightening to it is a follow-up, not a correction.
func (r *Repository) ListOfficeDecisionWaitCandidates(
	ctx context.Context,
	quietSince time.Time,
	after *models.OfficeDecisionWaitCursor,
) ([]models.OfficeDecisionWaitCandidate, *models.OfficeDecisionWaitCursor, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListOfficeDecisionWaitCandidates")
	defer span.End()

	cursorPredicate := ""
	args := []interface{}{quietSince}
	if after != nil {
		cursorPredicate = `
		  AND (t.updated_at > ? OR (t.updated_at = ? AND t.id > ?))`
		args = append(args, after.UpdatedAt, after.UpdatedAt, after.TaskID)
	}
	query := fmt.Sprintf(`
		SELECT t.id, t.workflow_step_id, t.updated_at
		FROM tasks t
		WHERE t.archived_at IS NULL
		  AND COALESCE(t.workflow_step_id, '') != ''
		  AND t.state NOT IN ('COMPLETED', 'FAILED', 'CANCELLED')
		  AND t.updated_at < ?
		%s
		  AND %s
		  AND %s
		  AND EXISTS (
			SELECT 1 FROM workflow_step_participants p
			WHERE p.step_id = t.workflow_step_id
			  AND p.decision_required = 1
			  AND (COALESCE(p.task_id, '') = '' OR p.task_id = t.id)
		  )
		ORDER BY t.updated_at ASC, t.id ASC
		LIMIT %d
	`,
		cursorPredicate,
		isFromOfficeProjection("t"),
		excludeConfigModePredicate(r.ro.DriverName(), "t.metadata"),
		officeDecisionWaitScanLimit,
	)

	var out []models.OfficeDecisionWaitCandidate
	if err := r.ro.SelectContext(ctx, &out, r.ro.Rebind(query), args...); err != nil {
		return nil, nil, fmt.Errorf("list office decision-wait candidates: %w", err)
	}
	if len(out) == officeDecisionWaitScanLimit {
		last := out[len(out)-1]
		return out, &models.OfficeDecisionWaitCursor{
			UpdatedAt: last.UpdatedAt,
			TaskID:    last.TaskID,
		}, nil
	}
	return out, nil, nil
}
