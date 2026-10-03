package projects

import (
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

func remoteBackedProjectRepository(repository *models.Repository) bool {
	if repository == nil || strings.EqualFold(strings.TrimSpace(repository.SourceType), "local") ||
		strings.TrimSpace(repository.DefaultBranch) == "" {
		return false
	}
	if strings.TrimSpace(repository.RemoteURL) != "" {
		return true
	}
	return strings.TrimSpace(repository.ProviderOwner) != "" && strings.TrimSpace(repository.ProviderName) != ""
}
