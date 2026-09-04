package sqlite

import (
	"context"
	"database/sql"
	"sort"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Reorder band discriminators (REQ-TASKS-KANBAN-TASK-REORDERING-001.17): the
// wire contract's explicit "admitted" | "queued" values.
const (
	ReorderBandAdmitted = "admitted"
	ReorderBandQueued   = "queued"
)

// ReorderStepTasks rewrites stepID's named band to orderedTaskIDs and
// densely renumbers the whole step from 0, admitted band first
// (REQ-TASKS-KANBAN-TASK-REORDERING-001.15). It returns the step's full
// non-hidden task list in its new order plus the step's order_revision as of
// commit.
//
// Validation is two-tiered. A submitted id that names no current non-hidden
// task of stepID at all (never existed, belongs to a different step, or has
// been archived/hidden since the client last read the band) is a
// structurally malformed request: ErrInvalidReorder, alongside an invalid
// band value, an empty list, or a duplicate id
// (REQ-TASKS-KANBAN-TASK-REORDERING-001.18). Once every id resolves to a
// current member of stepID, whether that member set exactly equals the named
// band's current membership is a race, not a malformed request — a task that
// moved to the step's other band, or a band that has gained members the
// caller doesn't know about, is exactly the drift
// REQ-TASKS-KANBAN-TASK-REORDERING-001.26 says to treat as the
// REQ-TASKS-KANBAN-TASK-REORDERING-001.19 conflict, ErrStepChanged, whatever
// caused it.
func (r *Repository) ReorderStepTasks(
	ctx context.Context, stepID, band string, orderedTaskIDs []string,
) ([]*models.Task, int64, error) {
	if band != ReorderBandAdmitted && band != ReorderBandQueued {
		return nil, 0, repoerrors.ErrInvalidReorder
	}
	if err := validateReorderIDList(orderedTaskIDs); err != nil {
		return nil, 0, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockWorkflowStepForWrite(ctx, tx, r.db.DriverName(), r.db.Rebind, stepID); err != nil {
		return nil, 0, err
	}
	tasks, err := r.listStepTasksInTx(ctx, tx, stepID)
	if err != nil {
		return nil, 0, err
	}
	admitted, queued := partitionReorderBands(tasks, stepID)
	named := admitted
	other := queued
	if band == ReorderBandQueued {
		named, other = queued, admitted
	}

	orderedNamed, err := resolveReorderStepMembership(tasks, orderedTaskIDs)
	if err != nil {
		return nil, 0, err
	}
	if !sameTaskSet(orderedNamed, named) {
		revision, revErr := r.stepOrderRevisionInTx(ctx, tx, stepID)
		if revErr != nil {
			return nil, 0, revErr
		}
		return sortStepOrder(tasks), revision, repoerrors.ErrStepChanged
	}

	renumbered := renumberReorderedStep(band, orderedNamed, other)
	for _, task := range renumbered {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET position = ?, updated_at = ? WHERE id = ?`),
			task.Position, r.nowUTC(), task.ID); err != nil {
			return nil, 0, err
		}
	}
	revision, err := r.bumpStepOrderRevisionInTx(ctx, tx, stepID)
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return renumbered, revision, nil
}

func validateReorderIDList(orderedTaskIDs []string) error {
	if len(orderedTaskIDs) == 0 {
		return repoerrors.ErrInvalidReorder
	}
	seen := make(map[string]struct{}, len(orderedTaskIDs))
	for _, id := range orderedTaskIDs {
		if _, dup := seen[id]; dup {
			return repoerrors.ErrInvalidReorder
		}
		seen[id] = struct{}{}
	}
	return nil
}

// listStepTasksInTx reads stepID's non-hidden tasks inside tx, so the read
// cannot straddle a concurrent writer's renumbering.
func (r *Repository) listStepTasksInTx(ctx context.Context, tx *sql.Tx, stepID string) ([]*models.Task, error) {
	rows, err := tx.QueryContext(ctx, r.db.Rebind(`
		SELECT `+taskSelectColumns("t")+`
		FROM tasks t
		WHERE t.workflow_step_id = ? AND t.archived_at IS NULL AND t.is_ephemeral = 0`+andNotAutomationOriginT+`
	`), stepID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// partitionReorderBands splits stepID's non-hidden tasks into its admitted
// and queued bands (Terminology: the queued band is
// !wip_admitted && queued_for_step_id == stepID; everything else in the step
// is the admitted band).
func partitionReorderBands(tasks []*models.Task, stepID string) (admitted, queued []*models.Task) {
	for _, task := range tasks {
		if !task.WIPAdmitted && task.QueuedForStepID == stepID {
			queued = append(queued, task)
		} else {
			admitted = append(admitted, task)
		}
	}
	return admitted, queued
}

// resolveReorderStepMembership resolves each of orderedIDs against stepTasks
// (both bands). An id with no match is a structurally malformed request:
// repoerrors.ErrInvalidReorder.
func resolveReorderStepMembership(stepTasks []*models.Task, orderedIDs []string) ([]*models.Task, error) {
	byID := make(map[string]*models.Task, len(stepTasks))
	for _, task := range stepTasks {
		byID[task.ID] = task
	}
	resolved := make([]*models.Task, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		task, ok := byID[id]
		if !ok {
			return nil, repoerrors.ErrInvalidReorder
		}
		resolved = append(resolved, task)
	}
	return resolved, nil
}

// sameTaskSet reports whether resolved (already deduplicated, one entry per
// submitted id) names exactly the same tasks as currentBand, regardless of
// order.
func sameTaskSet(resolved, currentBand []*models.Task) bool {
	if len(resolved) != len(currentBand) {
		return false
	}
	ids := make(map[string]struct{}, len(currentBand))
	for _, task := range currentBand {
		ids[task.ID] = struct{}{}
	}
	for _, task := range resolved {
		if _, ok := ids[task.ID]; !ok {
			return false
		}
	}
	return true
}

// renumberReorderedStep assigns dense positions 0..N-1 across the whole
// step, admitted band first (REQ-TASKS-KANBAN-TASK-REORDERING-001.15): the
// named band in the caller's submitted sequence, the other band in its
// existing step order.
func renumberReorderedStep(band string, orderedNamed, other []*models.Task) []*models.Task {
	sort.SliceStable(other, func(i, j int) bool { return models.StepOrderLess(other[i], other[j]) })
	finalAdmitted, finalQueued := orderedNamed, other
	if band == ReorderBandQueued {
		finalAdmitted, finalQueued = other, orderedNamed
	}
	renumbered := make([]*models.Task, 0, len(finalAdmitted)+len(finalQueued))
	position := 0
	for _, task := range finalAdmitted {
		task.Position = position
		renumbered = append(renumbered, task)
		position++
	}
	for _, task := range finalQueued {
		task.Position = position
		renumbered = append(renumbered, task)
		position++
	}
	return renumbered
}

// sortStepOrder returns tasks sorted by the current, uncommitted step order —
// used only to populate an ErrStepChanged response with the authoritative
// order the caller reconciles to.
func sortStepOrder(tasks []*models.Task) []*models.Task {
	sorted := make([]*models.Task, len(tasks))
	copy(sorted, tasks)
	sort.SliceStable(sorted, func(i, j int) bool { return models.StepOrderLess(sorted[i], sorted[j]) })
	return sorted
}

func (r *Repository) stepOrderRevisionInTx(ctx context.Context, tx *sql.Tx, stepID string) (int64, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT order_revision FROM workflow_steps WHERE id = ?`), stepID).Scan(&revision)
	return revision, err
}

func (r *Repository) bumpStepOrderRevisionInTx(ctx context.Context, tx *sql.Tx, stepID string) (int64, error) {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(
		`UPDATE workflow_steps SET order_revision = order_revision + 1, updated_at = ? WHERE id = ?`,
	), r.nowUTC(), stepID); err != nil {
		return 0, err
	}
	return r.stepOrderRevisionInTx(ctx, tx, stepID)
}
