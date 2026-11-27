package gitlab

import (
	"context"

	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func (s *Store) migrateExecutorCreators() error {
	migrate := db.NewRequiredMigrateLogger(s.db, nil)
	for _, table := range []string{"gitlab_review_watches", "gitlab_issue_watches"} {
		if err := migrate.Apply(table+".created_by_user_id", "ALTER TABLE "+table+" ADD COLUMN created_by_user_id TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	return nil
}

// SetExecutorContext installs the current-identity policy before pollers start.
func (s *Service) SetExecutorContext(resolve func(context.Context, string, string) (context.Context, error)) {
	s.executorContext = resolve
}

func (s *Service) executorActorContext(ctx context.Context, creator, profile string, workspace ...string) (context.Context, error) {
	if s.executorContext == nil {
		return ctx, nil
	}
	if len(workspace) > 0 {
		ctx = taskmodels.WithExecutorWorkspace(ctx, workspace[0])
	}
	return s.executorContext(ctx, creator, profile)
}
