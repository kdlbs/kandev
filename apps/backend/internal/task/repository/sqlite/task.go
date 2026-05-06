package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// taskSelectColumns returns the column projection (with runner subquery)
// for a SELECT against tasks aliased as `alias`. The output column order
// matches scanSingleTask / scanTasks.
func taskSelectColumns(alias string) string {
	if alias == "" {
		alias = "tasks"
	}
	prefix := alias + "."
	return prefix + "id, " + prefix + "workspace_id, " + prefix + "workflow_id, " + prefix + "workflow_step_id, " +
		prefix + "title, " + prefix + "description, " + prefix + "state, " + prefix + "priority, " + prefix + "position, " +
		prefix + "metadata, " + prefix + "is_ephemeral, " + prefix + "parent_id, " + prefix + "archived_at, " +
		prefix + "created_at, " + prefix + "updated_at, " +
		runnerProjection(alias) + ` AS assignee_agent_profile_id, ` +
		prefix + "origin, " + prefix + "project_id, " + prefix + "labels, " + prefix + "identifier"
}

// runnerProjection produces the correlated subquery (without alias) that
// resolves the runner for the row of `tasks` with the given alias. Used
// inline in SELECT projections.
func runnerProjection(alias string) string {
	if alias == "" {
		alias = "tasks"
	}
	return `COALESCE(
		(SELECT wsp.agent_profile_id FROM workflow_step_participants wsp
		 WHERE wsp.step_id = ` + alias + `.workflow_step_id
		   AND wsp.task_id = ` + alias + `.id
		   AND wsp.role = 'runner'
		 ORDER BY wsp.position ASC, wsp.id ASC LIMIT 1),
		(SELECT ws.agent_profile_id FROM workflow_steps ws WHERE ws.id = ` + alias + `.workflow_step_id),
		''
	)`
}

// CreateTask creates a new task. The assignee column has been removed
// (ADR 0005 Wave F); when the request carries AssigneeAgentProfileID we
// upsert a 'runner' row in workflow_step_participants instead.
func (r *Repository) CreateTask(ctx context.Context, task *models.Task) error {
	if task.ID == "" {
		task.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now

	metadata, err := json.Marshal(task.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}
	if task.Labels == "" {
		task.Labels = "[]"
	}
	// Office migrations enforce a CHECK constraint on priority; default
	// the empty zero value to 'medium' to match the column DEFAULT.
	if task.Priority == "" {
		task.Priority = "medium"
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, description, state, priority, position, metadata, is_ephemeral, parent_id, created_at, updated_at, origin, project_id, labels, identifier)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), task.ID, task.WorkspaceID, task.WorkflowID, task.WorkflowStepID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.IsEphemeral, task.ParentID, task.CreatedAt, task.UpdatedAt, task.Origin, task.ProjectID, task.Labels, task.Identifier)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("failed to rollback task insert: %w", rollbackErr)
		}
		return err
	}

	if task.AssigneeAgentProfileID != "" && task.WorkflowStepID != "" {
		if err := upsertRunnerInTx(ctx, tx, task.WorkflowStepID, task.ID, task.AssigneeAgentProfileID); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to rollback after runner write: %w", rbErr)
			}
			return err
		}
	}

	return tx.Commit()
}

// upsertRunnerInTx writes (or replaces) a 'runner' participant row for
// (stepID, taskID) inside the provided transaction. Mirrors
// workflow.Repository.SetTaskRunner but reuses the caller's tx.
func upsertRunnerInTx(ctx context.Context, tx *sql.Tx, stepID, taskID, agentProfileID string) error {
	if stepID == "" || taskID == "" || agentProfileID == "" {
		return nil
	}
	var existing string
	err := tx.QueryRowContext(ctx, `SELECT id FROM workflow_step_participants
		WHERE step_id = ? AND task_id = ? AND role = 'runner' LIMIT 1`,
		stepID, taskID).Scan(&existing)
	if err == nil {
		_, uerr := tx.ExecContext(ctx,
			`UPDATE workflow_step_participants SET agent_profile_id = ? WHERE id = ?`,
			agentProfileID, existing)
		return uerr
	}
	if err != sql.ErrNoRows {
		return err
	}
	id := uuid.New().String()
	_, ierr := tx.ExecContext(ctx, `INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, ?, ?, 'runner', ?, 0, 0)`,
		id, stepID, taskID, agentProfileID)
	return ierr
}

// clearRunnerInTx removes any 'runner' participant row for (stepID, taskID).
func clearRunnerInTx(ctx context.Context, tx *sql.Tx, stepID, taskID string) error {
	if stepID == "" || taskID == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		`DELETE FROM workflow_step_participants
		 WHERE step_id = ? AND task_id = ? AND role = 'runner'`,
		stepID, taskID)
	return err
}

// syncRunnerInTx upserts the runner participant when agentProfileID is set,
// otherwise clears it. No-op when stepID is empty.
func syncRunnerInTx(ctx context.Context, tx *sql.Tx, stepID, taskID, agentProfileID string) error {
	if stepID == "" {
		return nil
	}
	if agentProfileID != "" {
		return upsertRunnerInTx(ctx, tx, stepID, taskID, agentProfileID)
	}
	return clearRunnerInTx(ctx, tx, stepID, taskID)
}

// GetTask retrieves a task by ID
func (r *Repository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+taskSelectColumns("t")+` FROM tasks t WHERE t.id = ?`), id)
	task, err := r.scanSingleTask(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	return task, err
}

// UpdateTask updates an existing task. The runner write lands as an
// upsert/clear on workflow_step_participants inside the same tx as the
// task UPDATE.
func (r *Repository) UpdateTask(ctx context.Context, task *models.Task) error {
	task.UpdatedAt = time.Now().UTC()

	metadata, err := json.Marshal(task.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET workspace_id = ?, workflow_id = ?, workflow_step_id = ?, title = ?, description = ?, state = ?, priority = ?, position = ?, metadata = ?, parent_id = ?, updated_at = ?, origin = ?, project_id = ?, labels = ?, identifier = ?
		WHERE id = ?
	`), task.WorkspaceID, task.WorkflowID, task.WorkflowStepID, task.Title, task.Description, task.State, task.Priority, task.Position, string(metadata), task.ParentID, task.UpdatedAt, task.Origin, task.ProjectID, task.Labels, task.Identifier, task.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", task.ID)
	}

	if err := syncRunnerInTx(ctx, tx, task.WorkflowStepID, task.ID, task.AssigneeAgentProfileID); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteTask deletes a task by ID
func (r *Repository) DeleteTask(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM tasks WHERE id = ?`), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

// ListTasks returns all non-archived, non-ephemeral tasks for a workflow
func (r *Repository) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListTasks")
	defer span.End()
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskSelectColumns("t")+`
		FROM tasks t
		WHERE t.workflow_id = ? AND t.archived_at IS NULL AND t.is_ephemeral = 0
		ORDER BY t.created_at ASC
	`), workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// CountTasksByWorkflow returns the number of non-archived, non-ephemeral tasks in a workflow
func (r *Repository) CountTasksByWorkflow(ctx context.Context, workflowID string) (int, error) {
	var count int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT COUNT(*) FROM tasks WHERE workflow_id = ? AND archived_at IS NULL AND is_ephemeral = 0`), workflowID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountTasksByWorkflowStep returns the number of non-archived, non-ephemeral tasks in a workflow step
func (r *Repository) CountTasksByWorkflowStep(ctx context.Context, stepID string) (int, error) {
	var count int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT COUNT(*) FROM tasks WHERE workflow_step_id = ? AND archived_at IS NULL AND is_ephemeral = 0`), stepID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ListChildren returns non-archived, non-ephemeral children of parentID.
// Returns an empty list when parentID is empty (so root tasks resolve to
// "no children" cleanly).
func (r *Repository) ListChildren(ctx context.Context, parentID string) ([]*models.Task, error) {
	if parentID == "" {
		return []*models.Task{}, nil
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskSelectColumns("t")+`
		FROM tasks t
		WHERE t.parent_id = ? AND t.archived_at IS NULL AND t.is_ephemeral = 0
		ORDER BY t.created_at ASC, t.id ASC
	`), parentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// ListChildrenIncludingArchived returns every child task of parentID
// regardless of archived state. Used by the office task-handoffs
// unarchive cascade (phase 6) to walk a previously-archived descendant
// subtree.
func (r *Repository) ListChildrenIncludingArchived(ctx context.Context, parentID string) ([]*models.Task, error) {
	if parentID == "" {
		return []*models.Task{}, nil
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskSelectColumns("t")+`
		FROM tasks t
		WHERE t.parent_id = ? AND t.is_ephemeral = 0
		ORDER BY t.created_at ASC, t.id ASC
	`), parentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// ListSiblings returns non-archived, non-ephemeral sibling tasks. A task
// is a sibling when parent_id matches AND the parent_id is non-empty AND
// the workspace matches. Root tasks (empty parent_id) deliberately return
// an empty list so unrelated workspace roots don't surface as siblings.
func (r *Repository) ListSiblings(ctx context.Context, taskID string) ([]*models.Task, error) {
	self, err := r.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if self == nil || self.ParentID == "" {
		return []*models.Task{}, nil
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskSelectColumns("t")+`
		FROM tasks t
		WHERE t.parent_id = ?
		  AND t.workspace_id = ?
		  AND t.id != ?
		  AND t.archived_at IS NULL
		  AND t.is_ephemeral = 0
		ORDER BY t.created_at ASC, t.id ASC
	`), self.ParentID, self.WorkspaceID, self.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// ListTasksByWorkflowStep returns all non-archived, non-ephemeral tasks in a workflow step
func (r *Repository) ListTasksByWorkflowStep(ctx context.Context, workflowStepID string) ([]*models.Task, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskSelectColumns("t")+`
		FROM tasks t
		WHERE t.workflow_step_id = ? AND t.archived_at IS NULL AND t.is_ephemeral = 0 ORDER BY t.created_at ASC
	`), workflowStepID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

// ListTasksByWorkspace returns paginated tasks for a workspace with total count
// If query is non-empty, filters by task title, description, repository name, or repository path
// If includeArchived is false, archived tasks are excluded
// If includeEphemeral is false, ephemeral tasks are excluded
// If onlyEphemeral is true, only ephemeral tasks are returned
func (r *Repository) ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*models.Task, int, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListTasksByWorkspace")
	defer span.End()
	// Calculate offset
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	// Build filter conditions
	filter := ""
	if onlyEphemeral {
		// Only ephemeral tasks
		filter += " AND is_ephemeral = 1"
	} else if !includeEphemeral {
		// Exclude ephemeral tasks
		filter += " AND is_ephemeral = 0"
	}
	// If includeEphemeral is true and onlyEphemeral is false, include both

	if !includeArchived {
		filter += " AND archived_at IS NULL"
	}

	if excludeConfig {
		filter += " AND (metadata IS NULL OR json_extract(metadata, '$.config_mode') IS NOT 1)"
	}

	var rows *sql.Rows
	var total int
	var err error

	if query == "" {
		rows, total, err = r.queryAllTasks(ctx, workspaceID, filter, workflowID, repositoryID, pageSize, offset)
	} else {
		rows, total, err = r.searchTasks(ctx, workspaceID, query, filter, workflowID, repositoryID, pageSize, offset, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig)
	}

	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	tasks, err := r.scanTasks(rows)
	if err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

// queryAllTasks fetches all tasks (no search) for a workspace with pagination.
func (r *Repository) queryAllTasks(ctx context.Context, workspaceID, taskFilter, workflowID, repositoryID string, pageSize, offset int) (*sql.Rows, int, error) {
	args := []interface{}{workspaceID}
	if workflowID != "" {
		taskFilter += " AND workflow_id = ?"
		args = append(args, workflowID)
	}
	if repositoryID != "" {
		taskFilter += " AND id IN (SELECT task_id FROM task_repositories WHERE repository_id = ?)"
		args = append(args, repositoryID)
	}
	var total int
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT COUNT(*) FROM tasks WHERE workspace_id = ?`+taskFilter), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskSelectColumns("t")+`
		FROM tasks t
		WHERE t.workspace_id = ?`+rewriteFilterForAlias(taskFilter, "t")+`
		ORDER BY t.updated_at DESC
		LIMIT ? OFFSET ?
	`), append(append([]interface{}{}, args...), pageSize, offset)...)
	return rows, total, err
}

// rewriteFilterForAlias prefixes bare column references in `filter` with
// the given alias. Only used for the small, controlled fragments built
// in queryAllTasks and searchTasks.
func rewriteFilterForAlias(filter, alias string) string {
	if alias == "" {
		return filter
	}
	out := filter
	for _, col := range []string{"is_ephemeral", "archived_at", "metadata", "workflow_id"} {
		// Replace " <col>" only when not already prefixed by `alias.`.
		out = simplePrefixCol(out, col, alias)
	}
	return out
}

// simplePrefixCol replaces standalone occurrences of column names in
// SQL fragments with their aliased form. Naïve string substitution but
// adequate for the filters built locally above.
func simplePrefixCol(s, col, alias string) string {
	if s == "" {
		return s
	}
	prefix := alias + "."
	out := ""
	i := 0
	for i < len(s) {
		idx := indexFrom(s, col, i)
		if idx == -1 {
			out += s[i:]
			break
		}
		// Boundary check: char before must not be alphanumeric or '.'.
		if idx > 0 {
			c := s[idx-1]
			if c == '.' || isWordByte(c) {
				out += s[i : idx+len(col)]
				i = idx + len(col)
				continue
			}
		}
		// Boundary after: char after must not be alphanumeric.
		end := idx + len(col)
		if end < len(s) && isWordByte(s[end]) {
			out += s[i : idx+len(col)]
			i = idx + len(col)
			continue
		}
		out += s[i:idx] + prefix + col
		i = end
	}
	return out
}

func indexFrom(s, sub string, from int) int {
	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// searchTasks fetches tasks matching a search query for a workspace with pagination.
func (r *Repository) searchTasks(ctx context.Context, workspaceID, query, filter, workflowID, repositoryID string, pageSize, offset int, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) (*sql.Rows, int, error) {
	searchPattern := "%" + query + "%"
	like := dialect.Like(r.ro.DriverName())

	// Build task filter
	tFilter := ""
	if onlyEphemeral {
		tFilter += " AND t.is_ephemeral = 1"
	} else if !includeEphemeral {
		tFilter += " AND t.is_ephemeral = 0"
	}
	if !includeArchived {
		tFilter += " AND t.archived_at IS NULL"
	}
	if excludeConfig {
		tFilter += " AND (t.metadata IS NULL OR json_extract(t.metadata, '$.config_mode') IS NOT 1)"
	}

	// Collect extra filter args in query-argument order
	var extraArgs []interface{}
	if workflowID != "" {
		tFilter += " AND t.workflow_id = ?"
		extraArgs = append(extraArgs, workflowID)
	}
	if repositoryID != "" {
		tFilter += " AND tr.repository_id = ?"
		extraArgs = append(extraArgs, repositoryID)
	}

	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT t.id) FROM tasks t
		LEFT JOIN task_repositories tr ON t.id = tr.task_id
		LEFT JOIN repositories r ON tr.repository_id = r.id
		WHERE t.workspace_id = ?%s
		AND (
			t.title %s ? OR
			t.description %s ? OR
			r.name %s ? OR
			r.local_path %s ?
		)
	`, tFilter, like, like, like, like)
	countArgs := append(append([]interface{}{workspaceID}, extraArgs...), searchPattern, searchPattern, searchPattern, searchPattern)
	var total int
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(countQuery), countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	selectQuery := fmt.Sprintf(`
		SELECT DISTINCT %s
		FROM tasks t
		LEFT JOIN task_repositories tr ON t.id = tr.task_id
		LEFT JOIN repositories r ON tr.repository_id = r.id
		WHERE t.workspace_id = ?%s
		AND (
			t.title %s ? OR
			t.description %s ? OR
			r.name %s ? OR
			r.local_path %s ?
		)
		ORDER BY t.updated_at DESC
		LIMIT ? OFFSET ?
	`, taskSelectColumns("t"), tFilter, like, like, like, like)
	selectArgs := append(append([]interface{}{}, countArgs...), pageSize, offset)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(selectQuery), selectArgs...)
	return rows, total, err
}

// scanSingleTask scans a single row into a Task.
func (r *Repository) scanSingleTask(row *sql.Row) (*models.Task, error) {
	task := &models.Task{}
	var metadata string
	var archivedAt sql.NullTime
	var identifier sql.NullString
	err := row.Scan(
		&task.ID, &task.WorkspaceID, &task.WorkflowID, &task.WorkflowStepID,
		&task.Title, &task.Description, &task.State, &task.Priority, &task.Position,
		&metadata, &task.IsEphemeral, &task.ParentID, &archivedAt,
		&task.CreatedAt, &task.UpdatedAt,
		&task.AssigneeAgentProfileID, &task.Origin, &task.ProjectID,
		&task.Labels, &identifier,
	)
	if err != nil {
		return nil, err
	}
	if archivedAt.Valid {
		task.ArchivedAt = &archivedAt.Time
	}
	if identifier.Valid {
		task.Identifier = identifier.String
	}
	_ = json.Unmarshal([]byte(metadata), &task.Metadata)
	return task, nil
}

// scanTasks is a helper to scan task rows
func (r *Repository) scanTasks(rows *sql.Rows) ([]*models.Task, error) {
	var result []*models.Task
	for rows.Next() {
		task := &models.Task{}
		var metadata string
		var archivedAt sql.NullTime
		var identifier sql.NullString
		err := rows.Scan(
			&task.ID, &task.WorkspaceID, &task.WorkflowID, &task.WorkflowStepID,
			&task.Title, &task.Description, &task.State, &task.Priority, &task.Position,
			&metadata, &task.IsEphemeral, &task.ParentID, &archivedAt,
			&task.CreatedAt, &task.UpdatedAt,
			&task.AssigneeAgentProfileID, &task.Origin, &task.ProjectID,
			&task.Labels, &identifier,
		)
		if err != nil {
			return nil, err
		}
		if archivedAt.Valid {
			task.ArchivedAt = &archivedAt.Time
		}
		if identifier.Valid {
			task.Identifier = identifier.String
		}
		_ = json.Unmarshal([]byte(metadata), &task.Metadata)
		result = append(result, task)
	}
	return result, rows.Err()
}

// ArchiveTask sets the archived_at timestamp on a task
func (r *Repository) ArchiveTask(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET archived_at = ?, updated_at = ? WHERE id = ?`), now, now, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

// ArchiveTaskIfActive is the CAS variant used by office task-handoffs
// cascade archives. The update only fires when the task is currently
// active (archived_at IS NULL); this lets the cascade walk all
// descendants and archive only the ones not already archived by an
// earlier (manual or cascade) archive. Returns whether the row was
// actually updated.
//
// cascadeID is stamped on the archived row so UnarchiveTaskByCascade
// can scope its restoration to exactly the tasks this cascade owned.
// Pass empty cascadeID to opt out of cascade tracking (single-task
// manual archive); the column will simply not get set.
func (r *Repository) ArchiveTaskIfActive(ctx context.Context, id, cascadeID string) (bool, error) {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET archived_at = ?, archived_by_cascade_id = ?, updated_at = ?
		WHERE id = ? AND archived_at IS NULL
	`), now, cascadeID, now, id)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// UnarchiveTaskByCascade clears archived_at + archived_by_cascade_id only
// when the row was archived by the named cascade. Manually-archived tasks
// (empty cascade id) and tasks owned by a different cascade are left
// untouched — fixing the resurrection bug where unarchiving a parent
// would also un-archive descendants the user had archived manually
// before the cascade ran.
func (r *Repository) UnarchiveTaskByCascade(ctx context.Context, id, cascadeID string) (bool, error) {
	if cascadeID == "" {
		return false, fmt.Errorf("cascadeID is required")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET archived_at = NULL, archived_by_cascade_id = '', updated_at = ?
		WHERE id = ? AND archived_by_cascade_id = ?
	`), time.Now().UTC(), id, cascadeID)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// ListTasksForAutoArchive returns tasks eligible for auto-archiving based on workflow step settings
func (r *Repository) ListTasksForAutoArchive(ctx context.Context) ([]*models.Task, error) {
	drv := r.ro.DriverName()
	query := fmt.Sprintf(`
		SELECT %s
		FROM tasks t
		JOIN workflow_steps ws ON ws.id = t.workflow_step_id
		WHERE ws.auto_archive_after_hours > 0
			AND t.archived_at IS NULL
			AND t.updated_at <= %s
	`, taskSelectColumns("t"), dialect.NowMinusHours(drv, "ws.auto_archive_after_hours"))
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// UpdateTaskState updates the state of a task
func (r *Repository) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET state = ?, updated_at = ? WHERE id = ?`), state, time.Now().UTC(), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}
	return nil
}

// ListTasksByProject returns all non-archived, non-ephemeral tasks for a project.
func (r *Repository) ListTasksByProject(ctx context.Context, projectID string) ([]*models.Task, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskSelectColumns("t")+`
		FROM tasks t
		WHERE t.project_id = ? AND t.archived_at IS NULL AND t.is_ephemeral = 0
		ORDER BY t.created_at ASC
	`), projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// ListTasksByAssignee returns all non-archived, non-ephemeral tasks assigned to an agent.
// The lookup goes through the runner participant projection so it picks
// up both per-task overrides and step-primary fallbacks (ADR 0005 Wave F).
func (r *Repository) ListTasksByAssignee(ctx context.Context, agentInstanceID string) ([]*models.Task, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+taskSelectColumns("t")+`
		FROM tasks t
		WHERE `+runnerProjection("t")+` = ?
		  AND t.archived_at IS NULL AND t.is_ephemeral = 0
		ORDER BY t.created_at ASC
	`), agentInstanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// ListTaskTree returns a flat list of non-archived tasks for a workspace, suitable for
// building a tree using each task's ParentID field.
func (r *Repository) ListTaskTree(ctx context.Context, workspaceID string, filters models.TaskTreeFilters) ([]*models.Task, error) {
	query := `SELECT ` + taskSelectColumns("t") + ` FROM tasks t WHERE t.workspace_id = ? AND t.archived_at IS NULL AND t.is_ephemeral = 0`
	args := []interface{}{workspaceID}

	if filters.ProjectID != "" {
		query += " AND t.project_id = ?"
		args = append(args, filters.ProjectID)
	}
	if filters.AssigneeID != "" {
		query += " AND " + runnerProjection("t") + " = ?"
		args = append(args, filters.AssigneeID)
	}
	if filters.WorkflowID != "" {
		query += " AND t.workflow_id = ?"
		args = append(args, filters.WorkflowID)
	}
	if filters.Origin != "" {
		query += " AND t.origin = ?"
		args = append(args, filters.Origin)
	}
	query += " ORDER BY t.created_at ASC"

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// IncrementTaskSequence atomically increments the workspace task_sequence and returns the new value.
func (r *Repository) IncrementTaskSequence(ctx context.Context, workspaceID string) (int, error) {
	var seq int
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		UPDATE workspaces SET task_sequence = task_sequence + 1
		WHERE id = ?
		RETURNING task_sequence
	`), workspaceID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("increment task sequence for workspace %s: %w", workspaceID, err)
	}
	return seq, nil
}

// GetWorkspaceTaskPrefix returns the task prefix and office workflow ID for a workspace.
func (r *Repository) GetWorkspaceTaskPrefix(ctx context.Context, workspaceID string) (prefix, officeWorkflowID string, err error) {
	err = r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(task_prefix, 'KAN'), COALESCE(office_workflow_id, '')
		FROM workspaces WHERE id = ?
	`), workspaceID).Scan(&prefix, &officeWorkflowID)
	return
}
