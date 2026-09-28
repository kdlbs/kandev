package models

import "fmt"

// PRBase is the provider observation used to prepare a PR-linked worktree.
// Target keeps repository and branch identity together; OID is a transient
// consistency check against the fetched target branch.
type PRBase struct {
	Target ComparisonTarget `json:"target"`
	OID    string           `json:"oid,omitempty"`
}

// Validate rejects malformed target identity and provider object IDs.
func (b PRBase) Validate() error {
	if err := b.Target.Validate(); err != nil {
		return fmt.Errorf("PR base target: %w", err)
	}
	if b.OID != "" && !remoteContributionSHA.MatchString(b.OID) {
		return fmt.Errorf("PR base OID is invalid")
	}
	return nil
}
