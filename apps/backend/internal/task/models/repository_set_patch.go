package models

// RepositorySetPatch contains validated changes to a repository set. Nil fields
// leave the persisted value untouched; Items replaces the whole ordered list.
type RepositorySetPatch struct {
	Name        *string
	Description *string
	Items       *[]RepositorySetItem
}
