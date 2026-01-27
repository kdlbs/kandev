// Package workflow provides workflow management functionality.
package workflow

import (
	"database/sql"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/workflow/service"
)

// Provide creates the workflow repository and service using the shared database connection.
func Provide(db *sql.DB, log *logger.Logger) (*repository.Repository, *service.Service, func() error, error) {
	repo, err := repository.NewWithDB(db)
	if err != nil {
		return nil, nil, nil, err
	}
	svc := service.NewService(repo, log)
	return repo, svc, func() error { return nil }, nil
}

