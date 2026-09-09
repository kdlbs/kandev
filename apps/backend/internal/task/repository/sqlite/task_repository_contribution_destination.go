package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/kandev/kandev/internal/task/models"
)

const contributionDestinationBindAttempts = 4

// BindTaskRepositoryContributionDestination atomically adds a verified
// destination to one exact task/repository attachment. It updates metadata
// only and retries when another writer changes the row between read and CAS.
func (r *Repository) BindTaskRepositoryContributionDestination(
	ctx context.Context,
	id, taskID, repositoryID string,
	destination *models.ContributionDestination,
) (*models.TaskRepository, bool, error) {
	if destination == nil {
		return nil, false, errors.New("contribution destination is required")
	}
	if err := destination.Validate(); err != nil {
		return nil, false, err
	}
	for attempt := 0; attempt < contributionDestinationBindAttempts; attempt++ {
		current, err := r.GetTaskRepository(ctx, id)
		if err != nil {
			return nil, false, err
		}
		if current.TaskID != taskID || current.RepositoryID != repositoryID {
			return nil, false, errors.New("task repository contribution binding changed")
		}
		bound, err := taskRepositoryHasContributionBinding(current.Metadata)
		if err != nil {
			return nil, false, err
		}
		if bound {
			return current, false, nil
		}
		metadata := maps.Clone(current.Metadata)
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		if err := models.PutContributionDestination(metadata, destination); err != nil {
			return nil, false, err
		}
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return nil, false, fmt.Errorf("serialize contribution destination metadata: %w", err)
		}
		updatedAt := r.nowUTC()
		result, err := r.db.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_repositories SET metadata = ?, updated_at = ?
			WHERE id = ? AND task_id = ? AND repository_id = ? AND updated_at = ?
		`), string(metadataJSON), updatedAt, id, taskID, repositoryID, current.UpdatedAt)
		if err != nil {
			return nil, false, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return nil, false, err
		}
		if rows == 1 {
			current.Metadata = metadata
			current.UpdatedAt = updatedAt
			return current, true, nil
		}
	}
	return nil, false, errors.New("task repository changed while binding contribution destination")
}

func taskRepositoryHasContributionBinding(metadata map[string]interface{}) (bool, error) {
	if _, ok, err := models.LoadRemoteContribution(metadata); err != nil || ok {
		return ok, err
	}
	if _, ok, err := models.LoadContributionDestination(metadata); err != nil || ok {
		return ok, err
	}
	return false, nil
}
