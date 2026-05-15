package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// systemTasksJoin is the JOIN onto the workflows table used by every
// task list/search query that needs to know whether a task lives in a
// kandev-managed system workflow (coordination, future routines).
const systemTasksJoin = "LEFT JOIN workflows w ON w.id = t.workflow_id"

// systemTasksProjection is the boolean computed column emitted next to
// task rows so the API can mark each task as system-or-not. SQLite
// returns 1/0 which scans into a Go bool via sqlx.
const systemTasksProjection = "(CASE WHEN COALESCE(w.workflow_template_id,'') IN (%s) THEN 1 ELSE 0 END) AS is_system"

// systemTasksPlaceholders returns the comma-separated `?` placeholders
// matching the count of system template IDs and the slice of values to
// bind. Used in both the projection (CASE ... IN (?, ?)) and the
// optional NOT IN exclusion clause.
func systemTasksPlaceholders() (string, []interface{}) {
	ids := taskrepo.SystemWorkflowTemplateIDs
	if len(ids) == 0 {
		// Defensive: an empty list breaks `IN ()` SQL. Substitute a
		// sentinel that matches nothing so every task scans as
		// non-system without changing surrounding query shape.
		return "''", nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	return strings.Join(ph, ","), args
}

// TaskExecutionFields holds the execution-related fields from a task row.
// The legacy execution_policy / execution_state columns were dropped in
// Phase 4 of task-model-unification; stage progression is now owned by
// the workflow engine.
type TaskExecutionFields struct {
	ID                     string `db:"id"`
	AssigneeAgentProfileID string `db:"assignee_agent_profile_id"`
	State                  string `db:"state"`
	WorkspaceID            string `db:"workspace_id"`
}

// GetTaskExecutionFields returns the execution-related fields for a task.
func (r *Repository) GetTaskExecutionFields(ctx context.Context, taskID string) (*TaskExecutionFields, error) {
	var fields TaskExecutionFields
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT tasks.id,
		       `+RunnerProjection("tasks")+` as assignee_agent_profile_id,
		       COALESCE(tasks.state, '') as state,
		       COALESCE(tasks.workspace_id, '') as workspace_id
		FROM tasks WHERE tasks.id = ?
	`), taskID).StructScan(&fields)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if err != nil {
		return nil, err
	}
	return &fields, nil
}

// GetTaskProjectID returns the project_id for a task, or an empty string if unset.
func (r *Repository) GetTaskProjectID(ctx context.Context, taskID string) (string, error) {
	var projectID string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(project_id, '') FROM tasks WHERE id = ?
	`), taskID).Scan(&projectID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("task not found: %s", taskID)
	}
	return projectID, err
}

// UpdateTaskState updates the state column on a task (e.g. "IN_PROGRESS", "COMPLETED").
func (r *Repository) UpdateTaskState(ctx context.Context, taskID, state string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET state = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`), state, taskID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", taskID)
	}
	return nil
}

// UpdateTaskAssignee writes (or clears) the per-task runner participant
// row for a task. ADR 0005 Wave F replaced the legacy
// tasks.assignee_agent_profile_id column with a 'runner' row in
// workflow_step_participants; the row is keyed by (step_id, task_id).
//
// An empty assigneeID clears the runner participant. The updated_at on
// the task is bumped so dashboards still observe a "task changed"
// timestamp.
//
// Tasks created outside a workflow (e.g. channel tasks) have no
// workflow_step_id; for those we key the runner row on (step_id="",
// task_id) so the projection's per-task runner clause still resolves it
// (the (step_id="" / step_id="") match holds because the SELECT joins
// step_id = task.workflow_step_id which is also "").
func (r *Repository) UpdateTaskAssignee(ctx context.Context, taskID, assigneeID string) error {
	var stepID string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COALESCE(workflow_step_id, '') FROM tasks WHERE id = ?`),
		taskID).Scan(&stepID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("task not found: %s", taskID)
		}
		return err
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if assigneeID == "" {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			DELETE FROM workflow_step_participants
			WHERE step_id = ? AND task_id = ? AND role = 'runner'
		`), stepID, taskID); err != nil {
			return err
		}
	} else {
		var existing string
		probeErr := tx.QueryRowxContext(ctx, tx.Rebind(`
			SELECT id FROM workflow_step_participants
			WHERE step_id = ? AND task_id = ? AND role = 'runner' LIMIT 1
		`), stepID, taskID).Scan(&existing)
		switch probeErr {
		case nil:
			if _, err := tx.ExecContext(ctx, tx.Rebind(
				`UPDATE workflow_step_participants SET agent_profile_id = ? WHERE id = ?`),
				assigneeID, existing); err != nil {
				return err
			}
		case sql.ErrNoRows:
			if _, err := tx.ExecContext(ctx, tx.Rebind(`
				INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position)
				VALUES (?, ?, ?, 'runner', ?, 0, 0)
			`), newParticipantUUID(), stepID, taskID, assigneeID); err != nil {
				return err
			}
		default:
			return probeErr
		}
	}

	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE tasks SET updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`), taskID); err != nil {
		return err
	}
	return tx.Commit()
}

// TaskBasicInfo contains the minimal task fields needed for prompt building.
type TaskBasicInfo struct {
	Title       string `db:"title"`
	Description string `db:"description"`
	Identifier  string `db:"identifier"`
	Priority    string `db:"priority"`
	ProjectID   string `db:"project_id"`
}

// GetTaskBasicInfo returns minimal task data for prompt context.
// Returns nil, nil if the task is not found.
func (r *Repository) GetTaskBasicInfo(ctx context.Context, taskID string) (*TaskBasicInfo, error) {
	var info TaskBasicInfo
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT
			COALESCE(title, '') AS title,
			COALESCE(description, '') AS description,
			COALESCE(identifier, '') AS identifier,
			COALESCE(priority, 'medium') AS priority,
			COALESCE(project_id, '') AS project_id
		FROM tasks WHERE id = ?
	`), taskID).StructScan(&info)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// TaskSearchResult contains fields returned by a task search query.
type TaskSearchResult struct {
	ID                     string `db:"id"`
	WorkspaceID            string `db:"workspace_id"`
	Identifier             string `db:"identifier"`
	Title                  string `db:"title"`
	Description            string `db:"description"`
	Status                 string `db:"status"`
	Priority               string `db:"priority"`
	ParentID               string `db:"parent_id"`
	ProjectID              string `db:"project_id"`
	AssigneeAgentProfileID string `db:"assignee_agent_profile_id"`
	Labels                 string `db:"labels"`
	CreatedAt              string `db:"created_at"`
	UpdatedAt              string `db:"updated_at"`
	// IsSystem is true when the task lives in a kandev-managed system
	// workflow (e.g. the standing coordination task; future routine
	// tasks). The Office Tasks UI hides these by default and surfaces
	// them via a dev toggle. Populated by the LEFT JOIN in list/search
	// queries; non-list callers may leave it zero.
	IsSystem bool `db:"is_system"`
}

// TaskRow is a TaskSearchResult alias used by the office tasks API.
// It uses the same columns as TaskSearchResult — execution_policy and
// execution_state are intentionally excluded because those columns are
// added by the task service migration and may not exist in all contexts.
type TaskRow = TaskSearchResult

// ListTasksByWorkspace returns all non-archived, non-ephemeral tasks for a
// workspace ordered by updated_at descending.
func (r *Repository) ListTasksByWorkspace(ctx context.Context, workspaceID string, includeSystem bool) ([]*TaskRow, error) {
	sysPh, sysArgs := systemTasksPlaceholders()
	args := []interface{}{}
	args = append(args, sysArgs...) // for the IS_SYSTEM projection
	args = append(args, workspaceID)
	where := []string{
		"t.workspace_id = ?",
		"t.archived_at IS NULL",
		"t.is_ephemeral = 0",
	}
	if !includeSystem && len(sysArgs) > 0 {
		where = append(where, "COALESCE(w.workflow_template_id,'') NOT IN ("+sysPh+")")
		args = append(args, sysArgs...)
	}
	query := `
		SELECT t.id,
		       COALESCE(t.workspace_id, '') AS workspace_id,
		       COALESCE(t.identifier, '') AS identifier,
		       COALESCE(t.title, '') AS title,
		       COALESCE(t.description, '') AS description,
		       COALESCE(t.state, '') AS status,
		       COALESCE(t.priority, 'medium') AS priority,
		       COALESCE(t.parent_id, '') AS parent_id,
		       COALESCE(t.project_id, '') AS project_id,
		       ` + RunnerProjection("t") + ` AS assignee_agent_profile_id,
		       COALESCE(t.labels, '[]') AS labels,
		       t.created_at,
		       t.updated_at,
		       ` + fmt.Sprintf(systemTasksProjection, sysPh) + `
		FROM tasks t
		` + systemTasksJoin + `
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY t.updated_at DESC
	`
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanSearchResults(rows)
}

// TaskListSortField identifies the column the tasks list is sorted by.
// Validated against an allow-list before being interpolated into SQL to
// keep ListTasksFiltered safe against ORDER BY injection.
type TaskListSortField string

const (
	TaskSortUpdatedAt TaskListSortField = "updated_at"
	TaskSortCreatedAt TaskListSortField = "created_at"
	TaskSortPriority  TaskListSortField = "priority"
)

// taskListSortColumns maps a public sort name to the SQL column expression.
// Anything not in the map is rejected at the handler tier.
var taskListSortColumns = map[TaskListSortField]string{
	TaskSortUpdatedAt: "t.updated_at",
	TaskSortCreatedAt: "t.created_at",
	TaskSortPriority:  "t.priority",
}

// ListTasksOptions carries the filter/sort/pagination parameters for
// ListTasksFiltered. Empty / zero values are treated as "no filter".
type ListTasksOptions struct {
	Status     []string          // matches tasks.state, e.g. {"TODO","IN_PROGRESS"}
	Priority   []string          // matches tasks.priority
	AssigneeID string            // exact match on the runner column
	ProjectID  string            // exact match on tasks.project_id
	SortField  TaskListSortField // defaults to TaskSortUpdatedAt
	SortDesc   bool              // descending order; defaults to true for updated_at/created_at
	// Limit caps the number of rows returned. ≤0 / >500 are clamped.
	Limit int
	// CursorValue + CursorID encode keyset pagination over (sort_value, id).
	// Both empty means "first page". Pass the prior page's tail values to
	// fetch the next page.
	CursorValue string
	CursorID    string
	// IncludeSystem keeps tasks belonging to kandev-managed system
	// workflows (coordination, future routines) in the result. When
	// false (default) those tasks are filtered out — the Office Tasks
	// UI uses this to keep its list user-task-only by default.
	IncludeSystem bool
}

// ListTasksFilteredResult carries one page of tasks plus the cursor needed
// to fetch the next page.
type ListTasksFilteredResult struct {
	Tasks      []*TaskRow
	NextCursor string // empty when this is the final page
	NextID     string // tail row id; pair with NextCursor
}

// ListTasksFiltered returns a single page of non-archived, non-ephemeral
// tasks for a workspace, optionally filtered by status / priority /
// assignee / project, sorted by an allow-listed column, with keyset
// pagination over (sort_value, id) (Stream E of office optimization).
//
// SQL safety: status / priority / cursor values are bound as ? parameters;
// the only interpolated value is the sort column expression, sourced
// from taskListSortColumns (allow-list).
func (r *Repository) ListTasksFiltered(
	ctx context.Context, workspaceID string, opts ListTasksOptions,
) (*ListTasksFilteredResult, error) {
	resolved, err := resolveListTasksOptions(opts)
	if err != nil {
		return nil, err
	}
	sysPh, sysArgs := systemTasksPlaceholders()
	args := []interface{}{}
	args = append(args, sysArgs...) // for the IS_SYSTEM projection
	whereParts, whereArgs := buildTaskWhereClause(workspaceID, opts, resolved)
	args = append(args, whereArgs...)
	if !opts.IncludeSystem && len(sysArgs) > 0 {
		whereParts = append(whereParts, "COALESCE(w.workflow_template_id,'') NOT IN ("+sysPh+")")
		args = append(args, sysArgs...)
	}
	args = append(args, resolved.limit+1) // fetch one extra to detect "has next"

	query := `SELECT t.id,
	                 COALESCE(t.workspace_id, '') AS workspace_id,
	                 COALESCE(t.identifier, '') AS identifier,
	                 COALESCE(t.title, '') AS title,
	                 COALESCE(t.description, '') AS description,
	                 COALESCE(t.state, '') AS status,
	                 COALESCE(t.priority, 'medium') AS priority,
	                 COALESCE(t.parent_id, '') AS parent_id,
	                 COALESCE(t.project_id, '') AS project_id,
	                 ` + RunnerProjection("t") + ` AS assignee_agent_profile_id,
	                 COALESCE(t.labels, '[]') AS labels,
	                 t.created_at,
	                 t.updated_at,
	                 ` + fmt.Sprintf(systemTasksProjection, sysPh) + `
	          FROM tasks t
	          ` + systemTasksJoin + `
	          WHERE ` + strings.Join(whereParts, " AND ") + `
	          ORDER BY ` + resolved.sortCol + ` ` + resolved.dir + `, t.id ` + resolved.dir + `
	          LIMIT ?`

	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks filtered: %w", err)
	}
	defer func() { _ = rows.Close() }()
	tasks, err := scanSearchResults(rows)
	if err != nil {
		return nil, err
	}

	out := &ListTasksFilteredResult{Tasks: tasks}
	if len(tasks) > resolved.limit {
		out.Tasks = tasks[:resolved.limit]
		tail := out.Tasks[len(out.Tasks)-1]
		out.NextCursor = cursorValueForTask(tail, resolved.sortField)
		out.NextID = tail.ID
	}
	return out, nil
}

// resolvedListTasksOptions captures the validated, defaulted form of
// ListTasksOptions used by buildTaskWhereClause and the SQL builder.
type resolvedListTasksOptions struct {
	limit     int
	sortField TaskListSortField
	sortCol   string
	dir       string
}

func resolveListTasksOptions(opts ListTasksOptions) (resolvedListTasksOptions, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	sortField := opts.SortField
	if sortField == "" {
		sortField = TaskSortUpdatedAt
	}
	sortCol, ok := taskListSortColumns[sortField]
	if !ok {
		return resolvedListTasksOptions{}, fmt.Errorf("invalid sort field: %s", sortField)
	}
	dir := "DESC"
	if !opts.SortDesc {
		dir = "ASC"
	}
	return resolvedListTasksOptions{limit: limit, sortField: sortField, sortCol: sortCol, dir: dir}, nil
}

func buildTaskWhereClause(
	workspaceID string, opts ListTasksOptions, resolved resolvedListTasksOptions,
) ([]string, []interface{}) {
	args := []interface{}{workspaceID}
	parts := []string{
		"t.workspace_id = ?",
		"t.archived_at IS NULL",
		"t.is_ephemeral = 0",
	}
	if len(opts.Status) > 0 {
		ph, vals := bindList(opts.Status)
		parts = append(parts, "t.state IN ("+ph+")")
		args = append(args, vals...)
	}
	if len(opts.Priority) > 0 {
		ph, vals := bindList(opts.Priority)
		parts = append(parts, "t.priority IN ("+ph+")")
		args = append(args, vals...)
	}
	if opts.AssigneeID != "" {
		parts = append(parts, RunnerProjection("t")+" = ?")
		args = append(args, opts.AssigneeID)
	}
	if opts.ProjectID != "" {
		parts = append(parts, "COALESCE(t.project_id,'') = ?")
		args = append(args, opts.ProjectID)
	}
	if opts.CursorValue != "" {
		// Keyset pagination: (sort_value, id) < (cursor_value, cursor_id)
		// for descending order, > for ascending. Equality on the sort
		// column tie-breaks by id so the cursor remains unique.
		op := "<"
		if resolved.dir == "ASC" {
			op = ">"
		}
		parts = append(parts, fmt.Sprintf(
			"(%s %s ? OR (%s = ? AND t.id %s ?))",
			resolved.sortCol, op, resolved.sortCol, op,
		))
		args = append(args, opts.CursorValue, opts.CursorValue, opts.CursorID)
	}
	return parts, args
}

// bindList returns "?,?,?" placeholders and the values cast to []interface{}
// for splatting into SelectContext args.
func bindList(vals []string) (string, []interface{}) {
	ph := make([]string, len(vals))
	args := make([]interface{}, len(vals))
	for i, v := range vals {
		ph[i] = "?"
		args[i] = v
	}
	return strings.Join(ph, ","), args
}

// cursorValueForTask extracts the sort-column value from a task row for
// the given sort field, used as the keyset cursor for pagination.
func cursorValueForTask(t *TaskRow, sortField TaskListSortField) string {
	switch sortField {
	case TaskSortCreatedAt:
		return t.CreatedAt
	case TaskSortPriority:
		return t.Priority
	default:
		return t.UpdatedAt
	}
}

// GetTaskByID returns a single task as a TaskRow.
// Returns nil, nil when the task does not exist.
func (r *Repository) GetTaskByID(ctx context.Context, taskID string) (*TaskRow, error) {
	var result TaskRow
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT t.id,
		       COALESCE(t.workspace_id, '') AS workspace_id,
		       COALESCE(t.identifier, '') AS identifier,
		       COALESCE(t.title, '') AS title,
		       COALESCE(t.description, '') AS description,
		       COALESCE(t.state, '') AS status,
		       COALESCE(t.priority, 'medium') AS priority,
		       COALESCE(t.parent_id, '') AS parent_id,
		       COALESCE(t.project_id, '') AS project_id,
		       `+RunnerProjection("t")+` AS assignee_agent_profile_id,
		       COALESCE(t.labels, '[]') AS labels,
		       t.created_at,
		       t.updated_at
		FROM tasks t
		WHERE t.id = ?
	`), taskID).StructScan(&result)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task %s: %w", taskID, err)
	}
	return &result, nil
}

// ListChildTasks returns direct, non-archived child tasks for a parent task.
func (r *Repository) ListChildTasks(ctx context.Context, parentID string) ([]*TaskRow, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT t.id,
		       COALESCE(t.workspace_id, '') AS workspace_id,
		       COALESCE(t.identifier, '') AS identifier,
		       COALESCE(t.title, '') AS title,
		       COALESCE(t.description, '') AS description,
		       COALESCE(t.state, '') AS status,
		       COALESCE(t.priority, 'medium') AS priority,
		       COALESCE(t.parent_id, '') AS parent_id,
		       COALESCE(t.project_id, '') AS project_id,
		       `+RunnerProjection("t")+` AS assignee_agent_profile_id,
		       COALESCE(t.labels, '[]') AS labels,
		       t.created_at,
		       t.updated_at
		FROM tasks t
		WHERE t.parent_id = ?
		  AND t.archived_at IS NULL
		  AND t.is_ephemeral = 0
		ORDER BY t.created_at ASC
	`), parentID)
	if err != nil {
		return nil, fmt.Errorf("list child tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanSearchResults(rows)
}

// SearchTasks performs a full-text search on tasks using FTS5, falling back to
// LIKE-based search when the FTS5 table is not available.
func (r *Repository) SearchTasks(ctx context.Context, workspaceID, query string, limit int) ([]*TaskSearchResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if r.hasFTSTable() {
		return r.searchTasksFTS(ctx, workspaceID, query, limit)
	}
	return r.searchTasksLike(ctx, workspaceID, query, limit)
}

// hasFTSTable checks whether the tasks_fts virtual table exists.
func (r *Repository) hasFTSTable() bool {
	var exists int
	err := r.ro.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name='tasks_fts'",
	).Scan(&exists)
	return err == nil && exists == 1
}

// ftsEscape quotes a user query for safe FTS5 MATCH usage and appends * for prefix matching.
// Each whitespace-separated token is wrapped in double quotes so operators like - are literal.
func ftsEscape(query string) string {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return `""`
	}
	quoted := make([]string, len(tokens))
	for i, tok := range tokens {
		escaped := strings.ReplaceAll(tok, `"`, `""`)
		quoted[i] = `"` + escaped + `"`
	}
	// Append * to the last token for prefix matching.
	last := quoted[len(quoted)-1]
	quoted[len(quoted)-1] = last + "*"
	return strings.Join(quoted, " ")
}

// searchTasksFTS uses FTS5 MATCH for full-text search with prefix matching.
func (r *Repository) searchTasksFTS(ctx context.Context, workspaceID, query string, limit int) ([]*TaskSearchResult, error) {
	ftsQuery := ftsEscape(query)
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT t.id,
		       COALESCE(t.workspace_id, '') AS workspace_id,
		       COALESCE(t.identifier, '') AS identifier,
		       COALESCE(t.title, '') AS title,
		       COALESCE(t.description, '') AS description,
		       COALESCE(t.state, '') AS status,
		       COALESCE(t.priority, 'medium') AS priority,
		       COALESCE(t.parent_id, '') AS parent_id,
		       COALESCE(t.project_id, '') AS project_id,
		       `+RunnerProjection("t")+` AS assignee_agent_profile_id,
		       COALESCE(t.labels, '[]') AS labels,
		       t.created_at,
		       t.updated_at
		FROM tasks t
		JOIN tasks_fts fts ON t.rowid = fts.rowid
		WHERE fts.tasks_fts MATCH ?
		  AND t.workspace_id = ?
		  AND t.archived_at IS NULL
		  AND t.is_ephemeral = 0
		ORDER BY rank
		LIMIT ?
	`), ftsQuery, workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanSearchResults(rows)
}

// searchTasksLike is the LIKE-based fallback when FTS5 is unavailable.
func (r *Repository) searchTasksLike(ctx context.Context, workspaceID, query string, limit int) ([]*TaskSearchResult, error) {
	pattern := "%" + query + "%"
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT t.id,
		       COALESCE(t.workspace_id, '') AS workspace_id,
		       COALESCE(t.identifier, '') AS identifier,
		       COALESCE(t.title, '') AS title,
		       COALESCE(t.description, '') AS description,
		       COALESCE(t.state, '') AS status,
		       COALESCE(t.priority, 'medium') AS priority,
		       COALESCE(t.parent_id, '') AS parent_id,
		       COALESCE(t.project_id, '') AS project_id,
		       `+RunnerProjection("t")+` AS assignee_agent_profile_id,
		       COALESCE(t.labels, '[]') AS labels,
		       t.created_at,
		       t.updated_at
		FROM tasks t
		WHERE t.workspace_id = ?
		  AND t.archived_at IS NULL
		  AND t.is_ephemeral = 0
		  AND (t.title LIKE ? OR t.description LIKE ? OR t.identifier LIKE ?)
		ORDER BY t.updated_at DESC
		LIMIT ?
	`), workspaceID, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanSearchResults(rows)
}

// scanSearchResults scans rows into TaskSearchResult slices.
func scanSearchResults(rows *sqlx.Rows) ([]*TaskSearchResult, error) {
	var results []*TaskSearchResult
	for rows.Next() {
		var r TaskSearchResult
		if err := rows.StructScan(&r); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, &r)
	}
	return results, rows.Err()
}

// CountActionableTasksForAgent returns the number of tasks assigned to
// the given agent that are in an actionable state (TODO or IN_PROGRESS)
// and not archived. Resolves the assignee through the runner projection
// so per-task overrides and step-primary fallbacks both count.
func (r *Repository) CountActionableTasksForAgent(ctx context.Context, agentID string) (int, error) {
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COUNT(*) FROM tasks t
		WHERE `+RunnerProjection("t")+` = ?
		  AND t.state IN ('TODO', 'IN_PROGRESS')
		  AND t.archived_at IS NULL
	`), agentID).Scan(&count)
	return count, err
}

// GetCheckoutAgentBySession returns the checkout_agent_id for the task
// associated with a given session. Returns "" if not found.
func (r *Repository) GetCheckoutAgentBySession(ctx context.Context, sessionID string) (string, error) {
	var agentID string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(t.checkout_agent_id, '') FROM tasks t
		JOIN task_sessions ts ON ts.task_id = t.id
		WHERE ts.id = ?
	`), sessionID).Scan(&agentID)
	if err != nil {
		return "", err
	}
	return agentID, nil
}

// CheckoutTask atomically acquires an exclusive lock on a task for an agent.
// Returns true if the lock was acquired, false if another agent holds it.
func (r *Repository) CheckoutTask(ctx context.Context, taskID, agentID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET checkout_agent_id = ?, checkout_at = datetime('now')
		WHERE id = ? AND (checkout_agent_id IS NULL OR checkout_agent_id = '' OR checkout_agent_id = ?)
	`), agentID, taskID, agentID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// ReleaseTaskCheckout releases the exclusive lock on a task.
func (r *Repository) ReleaseTaskCheckout(ctx context.Context, taskID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET checkout_agent_id = NULL, checkout_at = NULL WHERE id = ?
	`), taskID)
	return err
}

// UpdateTaskPriority sets the priority TEXT column on a task. Caller is
// responsible for validating the value against the four-value enum; the DB
// CHECK constraint catches anything that slips through.
func (r *Repository) UpdateTaskPriority(ctx context.Context, taskID, priority string) error {
	return r.execTaskScalar(ctx, taskID, "priority", priority)
}

// UpdateTaskProjectID sets the project_id column. Empty string clears it.
func (r *Repository) UpdateTaskProjectID(ctx context.Context, taskID, projectID string) error {
	return r.execTaskScalar(ctx, taskID, "project_id", projectID)
}

// UpdateTaskParentID sets the parent_id column. Empty string clears it.
func (r *Repository) UpdateTaskParentID(ctx context.Context, taskID, parentID string) error {
	return r.execTaskScalar(ctx, taskID, "parent_id", parentID)
}

// taskScalarColumns enumerates the only columns execTaskScalar may target.
// Column names are interpolated into the SQL string (SQLite does not support
// parameterised identifiers), so the allowlist prevents an accidental
// caller-controlled column ever reaching fmt.Sprintf — gosec G201 guardrail.
var taskScalarColumns = map[string]struct{}{
	"priority":   {},
	"project_id": {},
	"parent_id":  {},
}

// execTaskScalar updates a single TEXT column on a task and bumps updated_at.
func (r *Repository) execTaskScalar(ctx context.Context, taskID, column, value string) error {
	if _, ok := taskScalarColumns[column]; !ok {
		return fmt.Errorf("disallowed task column %q", column)
	}
	query := fmt.Sprintf("UPDATE tasks SET %s = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", column)
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), value, taskID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", taskID)
	}
	return nil
}

// GetProjectWorkspaceID returns the workspace_id for a project.
// Returns empty string when the project does not exist.
func (r *Repository) GetProjectWorkspaceID(ctx context.Context, projectID string) (string, error) {
	if projectID == "" {
		return "", nil
	}
	var workspaceID string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(workspace_id,'') FROM office_projects WHERE id = ?
	`), projectID).Scan(&workspaceID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return workspaceID, err
}

// GetTaskWorkspaceID returns the workspace_id for a task.
// Returns empty string when the task does not exist.
func (r *Repository) GetTaskWorkspaceID(ctx context.Context, taskID string) (string, error) {
	if taskID == "" {
		return "", nil
	}
	var workspaceID string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(workspace_id,'') FROM tasks WHERE id = ?
	`), taskID).Scan(&workspaceID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return workspaceID, err
}

// UnstartedTaskRow holds the minimal fields needed by the recovery sweep.
type UnstartedTaskRow struct {
	ID                     string `db:"id"`
	AssigneeAgentProfileID string `db:"assignee_agent_profile_id"`
	WorkspaceID            string `db:"workspace_id"`
}

// ListUnstartedTasks returns TODO tasks with an assignee, not archived,
// within the lookback window, that have no active run (queued/claimed/finished).
// CountTasksByWorkspace returns the number of non-archived, non-ephemeral tasks
// for a workspace.
func (r *Repository) CountTasksByWorkspace(ctx context.Context, workspaceID string) (int, error) {
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COUNT(*) FROM tasks
		WHERE workspace_id = ?
		  AND archived_at IS NULL
		  AND is_ephemeral = 0
	`), workspaceID).Scan(&count)
	return count, err
}

func (r *Repository) ListUnstartedTasks(
	ctx context.Context, lookbackHours int, limit int,
) ([]*UnstartedTaskRow, error) {
	var rows []*UnstartedTaskRow
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT t.id,
		       `+RunnerProjection("t")+` AS assignee_agent_profile_id,
		       t.workspace_id
		FROM tasks t
		WHERE t.state = 'TODO'
		  AND `+RunnerProjection("t")+` != ''
		  AND t.archived_at IS NULL
		  AND t.created_at >= datetime('now', '-' || ? || ' hours')
		  AND NOT EXISTS (
		      SELECT 1 FROM runs w
		      WHERE json_extract(w.payload, '$.task_id') = t.id
		        AND w.status IN ('queued', 'claimed', 'finished')
		  )
		LIMIT ?
	`), lookbackHours, limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []*UnstartedTaskRow{}
	}
	return rows, nil
}
