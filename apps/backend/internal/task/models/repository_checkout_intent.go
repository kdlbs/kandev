package models

// RepositoryCheckoutIntent carries supplied ordinary repository updates to
// persistence. Besides the checkout choices, it optionally carries a validated
// clone URL and the identity observed before validation so an ordinary save
// cannot overwrite a concurrent repair or an identity change.
type RepositoryCheckoutIntent struct {
	DefaultBranch      *string
	PullBeforeWorktree *bool
	RemoteURL          *string
	ObservedIdentity   *RepositoryIdentityFence
}

// RepositoryIdentityFence is the saved repository identity a validated clone
// URL replacement was authorized against. It is internal to the service and
// store; transports cannot submit one.
type RepositoryIdentityFence struct {
	ID             string
	WorkspaceID    string
	SourceType     string
	LocalPath      string
	Provider       string
	ProviderHost   string
	ProviderScope  string
	ProviderRepoID string
	ProviderOwner  string
	ProviderName   string
}

// NewRepositoryIdentityFence captures the identity fields of a repository.
func NewRepositoryIdentityFence(repository *Repository) *RepositoryIdentityFence {
	if repository == nil {
		return nil
	}
	return &RepositoryIdentityFence{
		ID:             repository.ID,
		WorkspaceID:    repository.WorkspaceID,
		SourceType:     repository.SourceType,
		LocalPath:      repository.LocalPath,
		Provider:       repository.Provider,
		ProviderHost:   repository.ProviderHost,
		ProviderScope:  repository.ProviderScope,
		ProviderRepoID: repository.ProviderRepoID,
		ProviderOwner:  repository.ProviderOwner,
		ProviderName:   repository.ProviderName,
	}
}

// Matches reports whether the fence still describes the supplied repository.
func (f *RepositoryIdentityFence) Matches(repository *Repository) bool {
	if f == nil || repository == nil {
		return false
	}
	return f.ID == repository.ID &&
		f.WorkspaceID == repository.WorkspaceID &&
		f.SourceType == repository.SourceType &&
		f.LocalPath == repository.LocalPath &&
		f.Provider == repository.Provider &&
		f.ProviderHost == repository.ProviderHost &&
		f.ProviderScope == repository.ProviderScope &&
		f.ProviderRepoID == repository.ProviderRepoID &&
		f.ProviderOwner == repository.ProviderOwner &&
		f.ProviderName == repository.ProviderName
}
