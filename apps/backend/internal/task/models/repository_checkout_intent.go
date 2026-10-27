package models

// RepositoryCheckoutIntent carries supplied checkout choices to persistence.
type RepositoryCheckoutIntent struct {
	DefaultBranch      *string
	PullBeforeWorktree *bool
	// RunGitHooks is set only by the repository settings REST handlers.
	RunGitHooks *bool
}
