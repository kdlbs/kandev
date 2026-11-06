package models

import "fmt"

// WorktreeIdentityAmbiguousError identifies competing physical-worktree claims.
// Callers must preserve the inventory until an explicit offline repair succeeds.
type WorktreeIdentityAmbiguousError struct {
	WorktreeID string
}

func (e *WorktreeIdentityAmbiguousError) Error() string {
	return fmt.Sprintf("worktree %s has ambiguous inventory ownership", e.WorktreeID)
}
