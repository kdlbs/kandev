package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// UpdateWorkflowFields writes only supplied columns and returns the row observed
// by that statement. Unrelated columns never enter the assignment list.
func (r *Repository) UpdateWorkflowFields(ctx context.Context, id string, update models.WorkflowFieldUpdate) (*models.Workflow, error) {
	assignments := []string{"updated_at = ?"}
	args := []any{time.Now().UTC()}
	appendString := func(column string, value *string) {
		if value != nil {
			assignments = append(assignments, column+" = ?")
			args = append(args, *value)
		}
	}
	appendString("name", update.Name)
	appendString("description", update.Description)
	appendString("prompt", update.Prompt)
	appendString("agent_profile_id", update.AgentProfileID)
	if update.Hidden != nil {
		assignments = append(assignments, "hidden = ?")
		args = append(args, dialect.BoolToInt(*update.Hidden))
	}
	if update.Source != nil {
		source := normalizeWorkflowSource(*update.Source)
		appendString("source", &source)
	}
	appendString("source_path", update.SourcePath)
	args = append(args, id)
	query := "UPDATE workflows SET " + strings.Join(assignments, ", ") + " WHERE id = ? RETURNING " + workflowSelectColumns
	workflow, err := scanWorkflowRow(r.db.QueryRowContext(ctx, r.db.Rebind(query), args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", repoerrors.ErrWorkflowNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	return workflow, nil
}
