package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/github"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// githubTaskActivityAdapter keeps the GitHub scheduler dependent on its
// narrow integration projection while the task repository owns the query and
// its activity-source rules.
type githubTaskActivityAdapter struct {
	repo *tasksqlite.Repository
}

func (a *githubTaskActivityAdapter) LoadPRWatchTaskActivity(
	ctx context.Context, taskIDs []string,
) (map[string]github.PRWatchTaskActivity, error) {
	result := make(map[string]github.PRWatchTaskActivity, len(taskIDs))
	if a == nil || a.repo == nil || len(taskIDs) == 0 {
		return result, nil
	}
	activity, err := a.repo.LoadPRWatchTaskActivity(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for taskID, value := range activity {
		result[taskID] = github.PRWatchTaskActivity{
			LastActivityAt: value.LastActivityAt,
			Running:        value.Running,
		}
	}
	return result, nil
}
