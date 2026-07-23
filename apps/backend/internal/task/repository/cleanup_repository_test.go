package repository_test

import (
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// The workspace-source rollback path needs the guarded cleanup operation, but
// consumers of RepositoryEntityRepository must not be forced to provide it.
var _ repository.RepositoryCleanupRepository = (*sqlite.Repository)(nil)
