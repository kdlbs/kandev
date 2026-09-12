package sqlite

import (
	"context"
	"database/sql"
	"sort"
	"strings"

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
// Validation is two-tiered. A submitted id matching no task row in stepID's
// own workspace is a structurally malformed request: ErrInvalidReorder,
// alongside an invalid band value, an empty list, or a duplicate id
// (REQ-TASKS-KANBAN-TASK-REORDERING-001.18) — a task in a different
// workspace was never a candidate for this band under any race, and this
// also keeps the check from leaking whether an id the caller cannot
// authorize against exists at all. A submitted id naming a real task, in the
// same workspace, that the named step's current non-hidden membership does
// not include — because it became hidden, or because it moved to another
// step, since the client read the band — is not malformed: REQ-TASKS-KANBAN-
// TASK-REORDERING-001.26 and .33 route every such case to the
// REQ-TASKS-KANBAN-TASK-REORDERING-001.19 conflict, ErrStepChanged, because
// the server has no signal to tell "moved away during this exact window"
// apart from "was already elsewhere" — only nonexistence-in-workspace is
// unambiguous. Once every id resolves to a current, non-hidden member of
// stepID, whether that member set exactly equals the named band's current
// membership is the same conflict, checked by set comparison instead of
// existence.
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

	orderedNamed, unresolvedIDs := resolveReorderStepMembership(tasks, orderedTaskIDs)
	if len(unresolvedIDs) > 0 {
		return r.reorderUnresolvedIDsResult(ctx, tx, stepID, tasks, unresolvedIDs)
	}
	if !sameTaskSet(orderedNamed, named) {
		revision, revErr := r.stepOrderRevisionInTx(ctx, tx, stepID)
		if revErr != nil {
			return nil, 0, revErr
		}
		return sortStepOrder(tasks), revision, repoerrors.ErrStepChanged
	}

	renumbered := renumberReorderedStep(band, orderedNamed, other)
	r.fireReorderPreWriteHook()
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

func (r *Repository) fireReorderPreWriteHook() {
	if r.reorderPreWriteHook != nil {
		r.reorderPreWriteHook()
	}
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

// resolveReorderStepMembership resolves each of orderedIDs against
// stepTasks, the step's current non-hidden members (both bands). An id with
// no match is returned in unresolved rather than rejected immediately: the
// caller still has to tell an id matching no task row at all apart from one
// naming a real task that has simply left stepID's current membership since
// the client read the band, which is a membership-change conflict rather
// than a malformed request (REQ-TASKS-KANBAN-TASK-REORDERING-001.26/.33).
func resolveReorderStepMembership(stepTasks []*models.Task, orderedIDs []string) (resolved []*models.Task, unresolved []string) {
	byID := make(map[string]*models.Task, len(stepTasks))
	for _, task := range stepTasks {
		byID[task.ID] = task
	}
	resolved = make([]*models.Task, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		task, ok := byID[id]
		if !ok {
			unresolved = append(unresolved, id)
			continue
		}
		resolved = append(resolved, task)
	}
	return resolved, unresolved
}

// reorderUnresolvedIDsResult resolves the two-way branch for ids that don't
// currently match any of stepID's non-hidden tasks: an id resolving to no
// task row in stepID's own workspace is a structurally malformed request,
// ErrInvalidReorder, because the server has no membership window to
// attribute it to; otherwise every one of them names a real task, in the
// same workspace, that the named step's current membership no longer
// includes — hidden inside the window, or moved to another step, whatever
// the cause — which is the REQ-TASKS-KANBAN-TASK-REORDERING-001.26/.33
// membership-change conflict, ErrStepChanged.
func (r *Repository) reorderUnresolvedIDsResult(
	ctx context.Context, tx *sql.Tx, stepID string, tasks []*models.Task, unresolvedIDs []string,
) ([]*models.Task, int64, error) {
	nonexistent, err := r.hasNonexistentReorderID(ctx, tx, stepID, unresolvedIDs)
	if err != nil {
		return nil, 0, err
	}
	if nonexistent {
		return nil, 0, repoerrors.ErrInvalidReorder
	}
	revision, err := r.stepOrderRevisionInTx(ctx, tx, stepID)
	if err != nil {
		return nil, 0, err
	}
	return sortStepOrder(tasks), revision, repoerrors.ErrStepChanged
}

// hasNonexistentReorderID reports whether any of ids matches no task row in
// stepID's own workspace. Called only for ids resolveReorderStepMembership
// could not match against the step's live non-hidden tasks, so a false
// result means every id names a real task in that same workspace — the
// server has no way to tell "left the named step during this window" apart
// from "was never in it," so both resolve to the same membership-change
// conflict rather than the malformed-request case.
//
// Scoping the existence check to stepID's workspace (rather than checking
// the whole tasks table) is not just correctness — a task could never have
// raced into or out of a band in a workspace it does not belong to — it is
// also required for per-user scoping (service.authorizeWorkflowID only
// authorizes stepID's own workflow, never the individual submitted ids): a
// task existing in a workspace the caller cannot see must read identically
// to a nonexistent one, the same "no existence leak" invariant
// authorizeTaskID/authorizeWorkflowID are built around.
func (r *Repository) hasNonexistentReorderID(ctx context.Context, tx *sql.Tx, stepID string, ids []string) (bool, error) {
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, stepID)
	query := r.db.Rebind(`
		SELECT COUNT(*) FROM tasks WHERE id IN (` + strings.Join(placeholders, ",") + `) AND workspace_id = (
			SELECT w.workspace_id FROM workflow_steps ws JOIN workflows w ON w.id = ws.workflow_id WHERE ws.id = ?
		)
	`)
	var existingCount int
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&existingCount); err != nil {
		return false, err
	}
	return existingCount != len(ids), nil
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
