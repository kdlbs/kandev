package sqlite

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/task/models"
)

// SetAdmittedExecutionGrade owns the admission snapshot. Ordinary full-row and
// profile writes deliberately do not update it. An active unknown stays unknown.
func (r *Repository) SetAdmittedExecutionGrade(ctx context.Context, sessionID, grade string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions
 SET admitted_execution_grade = ?, admitted_execution_grade_known = 1
 WHERE id = ? AND (state NOT IN (?, ?) OR
 (admitted_execution_grade_known = 1 AND admitted_execution_grade = ?))`),
		grade, sessionID, models.TaskSessionStateStarting, models.TaskSessionStateRunning, grade)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("execution grade reservation no longer matches session %q", sessionID)
	}
	return nil
}

// ListAdmittedExecutions reads owner, grade and known state in one SQL snapshot.
func (r *Repository) ListAdmittedExecutions(ctx context.Context) ([]models.AdmittedExecution, error) {
	var rows []models.AdmittedExecution
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT id, admitted_execution_grade, admitted_execution_grade_known FROM task_sessions WHERE state IN (?, ?) ORDER BY id`), models.TaskSessionStateStarting, models.TaskSessionStateRunning)
	return rows, err
}
