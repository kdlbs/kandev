package models

// RepositoryCheckoutIntent carries supplied checkout choices to persistence.
type RepositoryCheckoutIntent struct {
	DefaultBranch      *string
	PullBeforeWorktree *bool
}
