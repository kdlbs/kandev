package gitlab

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// TriggerMRSync re-fetches every linked MR for a task and updates the stored
// rows. Returns the refreshed rows for the caller to broadcast.
func (s *Service) TriggerMRSync(ctx context.Context, taskID string) ([]*TaskMR, error) {
	store := s.requireStore()
	if store == nil {
		return nil, fmt.Errorf("gitlab store not configured")
	}
	rows, err := store.ListTaskMRsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*TaskMR, 0, len(rows))
	for _, r := range rows {
		updated, err := s.SyncTaskMR(ctx, r.TaskID, r.RepositoryID, r.ProjectPath, r.MRIID)
		if err != nil {
			s.logger.Warn("trigger MR sync",
				zap.String("task_id", r.TaskID),
				zap.String("project", r.ProjectPath),
				zap.Int("iid", r.MRIID),
				zap.Error(err))
			continue
		}
		out = append(out, updated)
	}
	return out, nil
}
