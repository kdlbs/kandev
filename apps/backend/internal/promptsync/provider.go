package promptsync

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/reposync"
)

// Provide creates the prompt sync store and service.
func Provide(
	writer, reader *sqlx.DB, githubClients reposync.GitHubClientProvider, gitlabClients reposync.GitLabClientProvider,
	prompts PromptApplier, refs ReferenceSource, log *logger.Logger,
) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	return NewService(store, githubClients, gitlabClients, prompts, refs, log), func() error { return nil }, nil
}
