package share

import (
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
)

// Config is the boot-time configuration for the share package. KandevVersion
// is the version string recorded in every snapshot so downstream tools can
// detect schema drift.
type Config struct {
	KandevVersion string
}

// Provide wires the share package's repository, service, and HTTP handlers.
// The taskReader is typically *sqliterepo.Repository from the task package;
// only the three methods on TaskReader are used. The GitHub resolver selects
// the owning workspace's automation principal for each operation.
// Cleanup is a no-op; the repository does not own its database connection.
func Provide(
	writer, reader *sqlx.DB,
	taskReader TaskReader,
	githubResolver GitHubClientResolver,
	log *logger.Logger,
	cfg Config,
) (*HTTPHandlers, func() error, error) {
	repo, err := NewRepository(writer, reader, log)
	if err != nil {
		return nil, nil, fmt.Errorf("share: provide repository: %w", err)
	}
	backend := NewWorkspaceGistBackend(githubResolver)
	svc := New(repo, taskReader, backend, log, cfg.KandevVersion)
	h := NewHTTPHandlers(svc, log)
	// Cleanup is a true no-op — the repository doesn't own its database
	// connection (the pool is owned by cmd/kandev).
	return h, func() error { return nil }, nil
}
