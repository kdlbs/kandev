// Package office provides the office domain for autonomous agent management.
package office

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// Provide creates the office SQLite repository using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB, log *logger.Logger) (*sqlite.Repository, func() error, error) {
	repo, err := sqlite.NewWithDB(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	_ = log // reserved for future use
	return repo, func() error { return nil }, nil
}
