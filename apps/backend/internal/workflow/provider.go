// Package workflow provides workflow management functionality.
package workflow

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/workflow/service"
)

// Provide creates the workflow repository and service using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB, log *logger.Logger) (*repository.Repository, *service.Service, func() error, error) {
	repo, err := repository.NewWithDB(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	svc := service.NewService(repo, log)
	return repo, svc, func() error { return nil }, nil
}
