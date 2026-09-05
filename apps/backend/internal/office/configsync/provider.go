package configsync

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// Provide builds the config sync service. Cleanup is a no-op today but the
// signature mirrors other integration providers so callers can register it
// uniformly. Either client provider may be nil when that integration is not
// configured for this backend; a workspace configured for the unavailable
// one gets an actionable failure at sync time rather than a boot failure.
func Provide(
	writer, reader *sqlx.DB, repo *sqlite.Repository,
	githubClients GitHubClientProvider, gitlabClients GitLabClientProvider,
	log *logger.Logger,
) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	runner := NewRunner(githubClients, gitlabClients, repo, store)
	svc := NewService(runner, store, log)
	cleanup := func() error { return nil }
	return svc, cleanup, nil
}
