package executor

import "github.com/kandev/kandev/internal/task/models"

// RepositoryCloneURL exposes the clone-locator derivation shared by the launch
// and resume paths, so recovery rebuilds byte-identical locators.
func RepositoryCloneURL(repo *models.Repository) string {
	if repo == nil {
		return ""
	}
	return repositoryCloneURL(repo)
}
