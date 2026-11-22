package changes

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

func (c *Coordinator) drainTurnChangeSetCheckpointRefs(ctx context.Context, changeSetID string, client CheckpointClient) error {
	readCtx, cancel := context.WithTimeout(ctx, turnChangePersistenceTimeout)
	rows, err := c.repository.ListTurnRepositoryChanges(readCtx, changeSetID)
	cancel()
	if err != nil {
		return err
	}
	values := make([]models.TurnRepositoryChangeSet, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			values = append(values, *row)
		}
	}
	return c.cleanupRepositoryCheckpointRefs(ctx, values, client)
}
