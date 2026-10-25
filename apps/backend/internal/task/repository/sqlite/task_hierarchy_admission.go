package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type taskHierarchyReader struct {
	repo *Repository
	tx   *sql.Tx
}

func (h taskHierarchyReader) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := h.repo.scanSingleTask(h.tx.QueryRowContext(ctx, h.repo.db.Rebind(`SELECT `+taskSelectColumns("t")+` FROM tasks t WHERE t.id = ?`), id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	return task, err
}
func (h taskHierarchyReader) ListChildren(ctx context.Context, id string) ([]*models.Task, error) {
	rows, err := h.tx.QueryContext(ctx, h.repo.db.Rebind(`SELECT `+taskSelectColumns("t")+` FROM tasks t WHERE t.parent_id = ? AND t.archived_at IS NULL AND t.is_ephemeral = 0`+andNotAutomationOriginT+` ORDER BY t.created_at, t.id`), id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return h.repo.scanTasks(rows)
}
func (r *Repository) hierarchyTxOptions() *sql.TxOptions {
	if dialect.IsPostgres(r.db.DriverName()) {
		return &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	return nil
}
func (r *Repository) lockTaskHierarchy(ctx context.Context, tx internaldb.TaskHierarchyTx, workspaceIDs, taskIDs []string) error {
	err := internaldb.LockTaskHierarchy(ctx, tx, r.db.DriverName(), r.db.Rebind, workspaceIDs, taskIDs)
	if errors.Is(err, internaldb.ErrTaskHierarchyWorkspaceNotFound) {
		return repoerrors.ErrWorkspaceNotFound
	}
	return err
}
func (r *Repository) ValidateTaskParent(ctx context.Context, id, parent string, validate hierarchy.TaskParentValidator) error {
	tx, err := r.db.BeginTx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskHierarchy(ctx, tx, nil, []string{id, parent}); err != nil {
		return err
	}
	reader := taskHierarchyReader{r, tx}
	current, err := reader.GetTask(ctx, id)
	if err != nil {
		return err
	}
	return validate(ctx, reader, current, parent)
}
func (r *Repository) ValidateTaskCreationParent(ctx context.Context, workspace, parent string, validate hierarchy.TaskParentValidator) error {
	tx, err := r.db.BeginTx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskHierarchy(ctx, tx, []string{workspace}, []string{parent}); err != nil {
		return err
	}
	return validate(ctx, taskHierarchyReader{r, tx}, nil, parent)
}
func (r *Repository) validateCreationParentTx(ctx context.Context, tx *sql.Tx, task *models.Task) error {
	if err := r.managedDeletionBarrierTx(ctx, tx, task.ParentID); err != nil {
		return err
	}
	if task.ParentID == "" {
		return nil
	}
	if validate := hierarchy.TaskCreationParentValidator(ctx); validate != nil {
		return validate(ctx, taskHierarchyReader{r, tx}, task, task.ParentID)
	}
	// Trusted raw fixture/import creators participate in serialization while
	// retaining their existing parent policy. Canonical creation opts into the
	// named predicate above.
	return nil
}
func (r *Repository) UpdateTaskWithParentAdmission(ctx context.Context, task *models.Task, parent *string, explicitPosition bool, validate hierarchy.TaskParentValidator) (bool, error) {
	tx, err := r.db.BeginTx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	ids := []string{task.ID}
	if parent != nil {
		ids = append(ids, *parent)
	}
	if err := r.lockTaskHierarchy(ctx, tx, nil, ids); err != nil {
		return false, err
	}
	reader := taskHierarchyReader{r, tx}
	current, err := reader.GetTask(ctx, task.ID)
	if err != nil {
		return false, err
	}
	if parent != nil {
		if err := r.managedDeletionBarrierTx(ctx, tx, *parent); err != nil {
			return false, err
		}
		if err := validate(ctx, reader, current, *parent); err != nil {
			return false, err
		}
	}
	task.ParentID = current.ParentID
	preserveHierarchyWorkspace(task, current.Metadata)
	if parent != nil && *parent != current.ParentID {
		task.ParentID = *parent
		workspace, _ := current.Metadata["workspace"].(map[string]interface{})
		if workspace["mode"] == taskWorkspaceModeInheritParent {
			preserveWorkspaceIdentity(task, workspace, taskWorkspaceModeSharedGroup)
		}
	}
	// updateTaskTx normally preserves parent from the row. This narrow marker
	// admits only the parent validated above in this transaction.
	ctx = context.WithValue(ctx, admittedTaskParentKey{}, task.ParentID)
	if err := r.lockTaskUpdateSteps(ctx, tx, task); err != nil {
		return false, err
	}
	entry, marker, err := r.updateTaskTx(ctx, tx, task, "", !explicitPosition, !explicitPosition, nil)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	r.dispatchStepEntry(ctx, task.ID, task.WorkflowID, task.WorkflowStepID, entry, marker)
	return parent != nil && *parent != current.ParentID, nil
}

type admittedTaskParentKey struct{}

// A snapshot cannot undo a materialized shared workspace, including parent
// ABA. Ordinary metadata keys still use replacement/deletion semantics.
func preserveHierarchyWorkspace(task *models.Task, currentMetadata map[string]interface{}) {
	current, _ := currentMetadata["workspace"].(map[string]interface{})
	if current["mode"] == taskWorkspaceModeSharedGroup {
		preserveWorkspaceIdentity(task, current, taskWorkspaceModeSharedGroup)
	}
}
func preserveWorkspaceIdentity(task *models.Task, current map[string]interface{}, mode string) {
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	requested, _ := task.Metadata["workspace"].(map[string]interface{})
	workspace := maps.Clone(requested)
	if workspace == nil {
		workspace = make(map[string]interface{})
	}
	workspace["mode"] = mode
	if group, ok := current["group_id"]; ok {
		workspace["group_id"] = group
	} else {
		delete(workspace, "group_id")
	}
	task.Metadata["workspace"] = workspace
}

// The workspace boundary is already held. Lock source and destination steps
// in stable order before updateTaskTx takes the task row lock.
func (r *Repository) lockTaskUpdateSteps(ctx context.Context, tx *sql.Tx, task *models.Task) error {
	var source string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(workflow_step_id, '') FROM tasks WHERE id = ?`), task.ID).Scan(&source); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTaskNotFound
		}
		return err
	}
	return r.lockWorkflowStepsForAdmission(ctx, tx, source, task.WorkflowStepID)
}

func (r *Repository) preserveTaskHierarchySnapshot(ctx context.Context, tx *sql.Tx, task *models.Task) ([]byte, error) {
	query := `SELECT COALESCE(parent_id, ''), metadata FROM tasks WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var parent string
	var encoded sql.NullString
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), task.ID).Scan(&parent, &encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	if admitted, ok := ctx.Value(admittedTaskParentKey{}).(string); ok {
		task.ParentID = admitted
	} else {
		task.ParentID = parent
	}
	var metadata map[string]interface{}
	if encoded.Valid && strings.TrimSpace(encoded.String) != "" {
		if err := json.Unmarshal([]byte(encoded.String), &metadata); err != nil {
			return nil, err
		}
	}
	preserveHierarchyWorkspace(task, metadata)
	result, err := json.Marshal(task.Metadata)
	if err != nil {
		return nil, err
	}
	return result, nil
}
