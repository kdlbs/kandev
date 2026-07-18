package azuredevops

import "context"

const RepositoryProvider = "azure_devops"

// RepositoryBinding is the provider metadata for a repository linked to one
// task. ProviderOwner carries the Azure project ID and ProviderRepoID carries
// the Azure repository GUID.
type RepositoryBinding struct {
	WorkspaceID    string
	Provider       string
	ProviderOwner  string
	ProviderRepoID string
}

// RepositoryLookup resolves a repository only when it is linked to the given
// task. Backend wiring adapts the task service to this narrow contract.
type RepositoryLookup interface {
	LookupTaskRepository(ctx context.Context, taskID, repositoryID string) (*RepositoryBinding, error)
}
