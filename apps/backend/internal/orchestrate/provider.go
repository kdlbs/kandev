// Package orchestrate provides the orchestrate domain for autonomous agent management.
package orchestrate

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrate/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

// Provide creates the orchestrate repository and service using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB, log *logger.Logger) (*sqlite.Repository, *service.Service, func() error, error) {
	repo, err := sqlite.NewWithDB(writer, reader)
	if err != nil {
		return nil, nil, nil, err
	}
	svc := service.NewService(repo, log)
	return repo, svc, func() error { return nil }, nil
}
