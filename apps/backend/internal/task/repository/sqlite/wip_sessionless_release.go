package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// wipRowQueryer is the single-row read shared by *sql.Tx, *sqlx.Tx and
// *sqlx.DB, so a WIP count reads its step's release policy on the same
// connection (and, inside a write transaction, the same snapshot) as the count.
type wipRowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// stepWIPReleasePolicy is the workspace's sessionless WIP release policy as
// it applies to one workflow step.
type stepWIPReleasePolicy struct {
	StepID       string
	WorkspaceID  string
	WIPLimit     int
	Enabled      bool
	GraceMinutes int
}

func (p stepWIPReleasePolicy) active() bool {
	return p.Enabled && p.GraceMinutes > 0
}

// sessionlessReleasedPredicate matches an admitted `tasks` row that the
// opt-in policy no longer counts toward its step's WIP: the task is
// IN_PROGRESS or FAILED, no session of it is live (every session is
// COMPLETED, FAILED or CANCELLED, or it has none), and neither the task nor
// any of its sessions changed after the cutoff. It takes the cutoff twice.
//
// A live session in any non-terminal state (CREATED, STARTING, RUNNING,
// IDLE, WAITING_FOR_INPUT) keeps the slot, so a task that re-enters work
// counts again on the next admission check.
func sessionlessReleasedPredicate() string {
	return `(tasks.state IN ('` + string(v1.TaskStateInProgress) + `', '` + string(v1.TaskStateFailed) + `')
		AND tasks.updated_at <= ?
		AND NOT EXISTS (
			SELECT 1 FROM task_sessions wip_rs
			WHERE wip_rs.task_id = tasks.id
			  AND (wip_rs.state NOT IN ('` + string(models.TaskSessionStateCompleted) + `', '` +
		string(models.TaskSessionStateFailed) + `', '` + string(models.TaskSessionStateCancelled) + `')
			       OR wip_rs.updated_at > ?)
		))`
}

// occupancyFilter returns the SQL appended to an admitted-occupant count.
// With the policy off it is empty, so every count is byte-for-byte the query
// that existed before the policy.
func (p stepWIPReleasePolicy) occupancyFilter(now time.Time) (string, []any) {
	if !p.active() {
		return "", nil
	}
	cutoff := p.cutoff(now)
	return " AND NOT " + sessionlessReleasedPredicate(), []any{cutoff, cutoff}
}

func (p stepWIPReleasePolicy) cutoff(now time.Time) time.Time {
	return now.UTC().Add(-time.Duration(p.GraceMinutes) * time.Minute)
}

// loadStepWIPReleasePolicy reads the policy of the workspace that owns
// stepID. A step that is not found, or a store that cannot answer, reports
// the policy as off: the count then keeps every admitted task, which is the
// behavior that predates the policy, rather than releasing slots on a guess.
func (r *Repository) loadStepWIPReleasePolicy(ctx context.Context, q wipRowQueryer, stepID string) stepWIPReleasePolicy {
	policy, err := r.readStepWIPReleasePolicy(ctx, q, stepID)
	if err != nil {
		return stepWIPReleasePolicy{StepID: stepID}
	}
	return policy
}

func (r *Repository) readStepWIPReleasePolicy(ctx context.Context, q wipRowQueryer, stepID string) (stepWIPReleasePolicy, error) {
	policy := stepWIPReleasePolicy{StepID: stepID}
	if stepID == "" {
		return policy, wfmodels.ErrWorkflowStepNotFound
	}
	err := q.QueryRowContext(ctx, r.db.Rebind(`
		SELECT w.id, s.wip_limit, w.wip_sessionless_release_enabled, w.wip_sessionless_release_grace_minutes
		FROM workflow_steps s
		JOIN workflows f ON f.id = s.workflow_id
		JOIN workspaces w ON w.id = f.workspace_id
		WHERE s.id = ?
	`), stepID).Scan(&policy.WorkspaceID, &policy.WIPLimit, &policy.Enabled, &policy.GraceMinutes)
	if errors.Is(err, sql.ErrNoRows) {
		return stepWIPReleasePolicy{StepID: stepID}, wfmodels.ErrWorkflowStepNotFound
	}
	return policy, err
}

// admittedOccupancyFilter is the policy-aware suffix for a WIP count of
// stepID, read through q.
func (r *Repository) admittedOccupancyFilter(ctx context.Context, q wipRowQueryer, stepID string) (string, []any) {
	return r.loadStepWIPReleasePolicy(ctx, q, stepID).occupancyFilter(time.Now())
}

// ListSessionlessWIPReleaseSteps returns the WIP-limited steps of workspaces
// with the policy on that hold at least one admitted IN_PROGRESS or FAILED
// task with no live session. It does not apply the grace: the caller runs the
// normal pull for each step, and that pull's policy-aware count decides
// whether a slot is actually free yet.
func (r *Repository) ListSessionlessWIPReleaseSteps(ctx context.Context) ([]string, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT DISTINCT tasks.workflow_step_id
		FROM tasks
		JOIN workflow_steps s ON s.id = tasks.workflow_step_id
		JOIN workspaces w ON w.id = tasks.workspace_id
		WHERE w.wip_sessionless_release_enabled = TRUE
		  AND s.wip_limit > 0
		  AND tasks.wip_admitted = 1
		  AND tasks.archived_at IS NULL
		  AND tasks.is_ephemeral = 0
		  AND tasks.state IN (?, ?)
		  AND NOT EXISTS (
			SELECT 1 FROM task_sessions wip_rs
			WHERE wip_rs.task_id = tasks.id
			  AND wip_rs.state NOT IN (?, ?, ?)
		  )`+andNotAutomationOrigin+`
		ORDER BY tasks.workflow_step_id
	`), v1.TaskStateInProgress, v1.TaskStateFailed,
		models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var stepIDs []string
	for rows.Next() {
		var stepID string
		if err := rows.Scan(&stepID); err != nil {
			return nil, err
		}
		stepIDs = append(stepIDs, stepID)
	}
	return stepIDs, rows.Err()
}

// GetWorkflowStepWIPOccupancy explains who holds stepID's WIP slots and why.
// Each admitted task is listed with whether it counts toward the limit right
// now; Counted equals the number the admission checks use.
func (r *Repository) GetWorkflowStepWIPOccupancy(ctx context.Context, stepID string) (*models.WIPStepOccupancy, error) {
	policy, err := r.readStepWIPReleasePolicy(ctx, r.ro, stepID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	releasedExpr := "0"
	var args []any
	if policy.active() {
		cutoff := policy.cutoff(now)
		releasedExpr = "CASE WHEN " + sessionlessReleasedPredicate() + " THEN 1 ELSE 0 END"
		args = append(args, cutoff, cutoff)
	}
	args = append(args, stepID)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT tasks.id, tasks.title, tasks.state, tasks.updated_at, `+releasedExpr+`
		FROM tasks
		WHERE tasks.workflow_step_id = ?
		  AND tasks.wip_admitted = 1
		  AND tasks.archived_at IS NULL
		  AND tasks.is_ephemeral = 0`+andNotAutomationOrigin+`
		ORDER BY tasks.position, tasks.id
	`), args...)
	if err != nil {
		return nil, err
	}
	occupants, err := scanWIPOccupants(rows)
	if err != nil {
		return nil, err
	}
	if err := r.attachWIPOccupantSessions(ctx, occupants); err != nil {
		return nil, err
	}
	result := &models.WIPStepOccupancy{
		StepID:              stepID,
		WorkspaceID:         policy.WorkspaceID,
		WIPLimit:            policy.WIPLimit,
		ReleaseEnabled:      policy.Enabled,
		ReleaseGraceMinutes: policy.GraceMinutes,
		Occupants:           make([]models.WIPStepOccupant, 0, len(occupants)),
	}
	for _, occupant := range occupants {
		classifyWIPOccupant(&occupant.WIPStepOccupant, occupant.released, policy, now)
		if occupant.HoldsSlot {
			result.Counted++
		}
		result.Occupants = append(result.Occupants, occupant.WIPStepOccupant)
	}
	return result, nil
}

type wipOccupantRow struct {
	models.WIPStepOccupant
	released bool
}

func scanWIPOccupants(rows *sql.Rows) ([]*wipOccupantRow, error) {
	defer func() { _ = rows.Close() }()
	var occupants []*wipOccupantRow
	for rows.Next() {
		row := &wipOccupantRow{}
		var released int
		if err := rows.Scan(&row.TaskID, &row.Title, &row.State, &row.LastActivityAt, &released); err != nil {
			return nil, err
		}
		row.LastActivityAt = row.LastActivityAt.UTC()
		row.released = released == 1
		occupants = append(occupants, row)
	}
	return occupants, rows.Err()
}

func (r *Repository) attachWIPOccupantSessions(ctx context.Context, occupants []*wipOccupantRow) error {
	if len(occupants) == 0 {
		return nil
	}
	byID := make(map[string]*wipOccupantRow, len(occupants))
	placeholders := make([]string, 0, len(occupants))
	args := make([]any, 0, len(occupants))
	for _, occupant := range occupants {
		byID[occupant.TaskID] = occupant
		placeholders = append(placeholders, "?")
		args = append(args, occupant.TaskID)
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT task_id, state, updated_at FROM task_sessions
		WHERE task_id IN (`+strings.Join(placeholders, ", ")+`)
	`), args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var taskID string
		var state models.TaskSessionState
		var updatedAt time.Time
		if err := rows.Scan(&taskID, &state, &updatedAt); err != nil {
			return err
		}
		occupant := byID[taskID]
		if occupant == nil {
			continue
		}
		occupant.SessionCount++
		if !wipTerminalSessionState(state) {
			occupant.LiveSessionCount++
		}
		if updatedAt.UTC().After(occupant.LastActivityAt) {
			occupant.LastActivityAt = updatedAt.UTC()
		}
	}
	return rows.Err()
}

func wipTerminalSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return true
	default:
		return false
	}
}

// classifyWIPOccupant names why an admitted task does or does not hold a
// slot. released comes from the same SQL predicate the admission counts use,
// so the explanation cannot disagree with admission about who counts.
func classifyWIPOccupant(occupant *models.WIPStepOccupant, released bool, policy stepWIPReleasePolicy, now time.Time) {
	switch {
	case released:
		occupant.HoldsSlot = false
		occupant.Reason = models.WIPOccupantReasonReleasedNoLiveSession
	case occupant.LiveSessionCount > 0:
		occupant.HoldsSlot = true
		occupant.Reason = models.WIPOccupantReasonLiveSession
	case policy.active() && wipReleaseEligibleState(occupant.State):
		occupant.HoldsSlot = true
		occupant.Reason = models.WIPOccupantReasonReleaseGracePending
		releaseAt := occupant.LastActivityAt.Add(time.Duration(policy.GraceMinutes) * time.Minute)
		if releaseAt.Before(now) {
			// The row changed between the two reads; the next read settles it.
			releaseAt = now
		}
		occupant.ReleaseAt = &releaseAt
	default:
		occupant.HoldsSlot = true
		occupant.Reason = models.WIPOccupantReasonNoLiveSession
	}
}

func wipReleaseEligibleState(state string) bool {
	return state == string(v1.TaskStateInProgress) || state == string(v1.TaskStateFailed)
}
