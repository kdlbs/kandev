package sqlite_test

import (
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

var _ taskrepository.TurnChangesRepository = (*sqlite.Repository)(nil)
