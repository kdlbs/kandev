package executor

import mcpproviders "github.com/kandev/kandev/internal/mcp/providers"

// deriveMCPProviders returns the normalized provider capability union for the
// task's resolved repositories. It deliberately reads only the persisted
// repository provider, never a filesystem remote, so launch behavior is stable
// for local checkouts and provider-backed clones alike.
func deriveMCPProviders(repositories []*repoInfo) []string {
	values := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		if repository == nil || repository.Repository == nil {
			continue
		}
		values = append(values, repository.Repository.Provider)
	}
	return mcpproviders.Normalize(values)
}
