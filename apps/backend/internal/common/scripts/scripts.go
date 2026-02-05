// Package scripts provides shared types for repository script lookups.
package scripts

import "context"

// RepositoryScript holds the minimal fields needed when looking up a script by ID.
type RepositoryScript struct {
	ID      string
	Name    string
	Command string
}

// ScriptService provides access to repository scripts.
type ScriptService interface {
	GetRepositoryScript(ctx context.Context, id string) (*RepositoryScript, error)
}
