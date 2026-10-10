package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// UpdateTaskGitHubIssue changes the whole issue identity under native row
// serialization, preserving the raw values of every unrelated metadata key.
func (r *Repository) UpdateTaskGitHubIssue(ctx context.Context, id string, link *models.TaskGitHubIssueLink) (*models.Task, error) {
	tx, err := r.db.BeginTx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskMetadataMutation(ctx, tx, id); err != nil {
		return nil, err
	}
	current, err := r.currentMetadataForMerge(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	applyTaskGitHubIssue(current, link)
	encoded, err := json.Marshal(current)
	if err != nil {
		return nil, fmt.Errorf("encode task issue metadata: %w", err)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind("UPDATE tasks SET metadata = ?, updated_at = ? WHERE id = ?"), string(encoded), r.nowUTC(), id)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	query := `SELECT ` + taskSelectColumns("t") + ` FROM tasks t WHERE t.id = ?`
	task, err := r.scanSingleTask(tx.QueryRowContext(ctx, r.db.Rebind(query), id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return task, nil
}

func applyTaskGitHubIssue(metadata map[string]json.RawMessage, link *models.TaskGitHubIssueLink) {
	values := map[string]interface{}{
		"issue_url": nil, "issue_number": nil, "issue_owner": nil,
		"issue_repo": nil, "github_issue_linked": nil,
	}
	if link != nil {
		values["issue_url"] = link.URL
		values["issue_number"] = link.Number
		values["issue_owner"] = link.Owner
		values["issue_repo"] = link.Repo
		values["github_issue_linked"] = true
	}
	for key, value := range values {
		if link == nil {
			delete(metadata, key)
		} else {
			// The fixed identity contains only JSON scalar types.
			metadata[key], _ = json.Marshal(value)
		}
	}
}
