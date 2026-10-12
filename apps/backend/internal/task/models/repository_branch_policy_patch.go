package models

// RepositoryBranchPolicyPatch preserves which fields a caller supplied.
type RepositoryBranchPolicyPatch struct {
	Name              *string
	Description       *string
	BaseBranch        *string
	BranchTemplate    *string
	PullRequestTarget *string
}
