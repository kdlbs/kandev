package coordinator

import (
	"context"
	"sort"
)

// RepoPolicy is a coordinator's merge authority over one repository.
type RepoPolicy string

// Repository merge policies; a missing entry is RepoDenied.
const (
	RepoDenied           RepoPolicy = "denied"
	RepoRequiresApproval RepoPolicy = "requires_approval"
	RepoAutomatic        RepoPolicy = "automatic"
)

func (p RepoPolicy) valid() bool {
	return p == RepoDenied || p == RepoRequiresApproval || p == RepoAutomatic
}

// Settings error codes of the merge repository policy save.
const (
	codeRevisionRequired  = "revision_required"
	codeRevisionConflict  = "revision_conflict"
	codePrincipalValid    = "principal_valid"
	codeMergeUnavailable  = "merge_unavailable"
	codeRepositoryUnknown = "repository_unknown"
)

// Merge policy audit kinds.
const (
	auditPolicyChanged     = "policy_changed"
	auditPrincipalReplaced = "principal_replaced"
	auditAutomaticLowered  = "automatic_lowered"
)

// MergeConflictError is a 409 of the settings save: a stale policy revision or
// an attempt to replace a principal that is still authorized.
type MergeConflictError struct{ Code string }

func (e *MergeConflictError) Error() string { return e.Code }

// RepoPolicyRow is one stored repository policy. SelectedBy is the execution
// principal and AutomaticSinceRevision the policy revision at which the
// repository became automatic; both are empty for any other policy.
type RepoPolicyRow struct {
	RepositoryID           string     `json:"repository_id"`
	Policy                 RepoPolicy `json:"policy"`
	SelectedBy             string     `json:"selected_by,omitempty"`
	AutomaticSinceRevision int        `json:"automatic_since_revision,omitempty"`
}

// RepoPolicyChange is one listed repository of a settings save.
type RepoPolicyChange struct {
	RepositoryID     string
	Policy           RepoPolicy
	ReplacePrincipal bool
}

// PolicyAudit is one audited policy change.
type PolicyAudit struct {
	Kind     string
	Subject  string // repository ID
	Before   string
	After    string
	Revision int
}

// PrincipalValidator says whether a user is still an authorized manager of the
// workspace. An error or a missing validator is read as not authorized.
type PrincipalValidator interface {
	ManagerAuthorized(ctx context.Context, workspaceID, userID string) (bool, error)
}

// policyPlan is what a settings save changes among the repository policies.
type policyPlan struct {
	Rows   []RepoPolicyRow
	Audits []PolicyAudit
}

func (p policyPlan) changed() bool { return len(p.Rows) > 0 }

func storedPolicy(stored map[string]RepoPolicyRow, id string) RepoPolicy {
	if r, ok := stored[id]; ok && r.Policy.valid() {
		return r.Policy
	}
	return RepoDenied
}

// repoPolicyDiffers reports whether a listed entry changes the stored state,
// counting a replace_principal marker on an automatic repository.
func repoPolicyDiffers(stored map[string]RepoPolicyRow, c RepoPolicyChange) bool {
	cur := storedPolicy(stored, c.RepositoryID)
	if c.Policy != cur {
		return true
	}
	return c.Policy == RepoAutomatic && c.ReplacePrincipal
}

// planRepoPolicies resolves a save against the stored policies. nextRevision
// is the policy revision the save will have. principalValid says whether the
// stored principal of a repository is still an authorized manager. The
// revision check precedes the principal check, so a stale save is a
// revision_conflict even when the principal is valid.
func planRepoPolicies(stored map[string]RepoPolicyRow, req []RepoPolicyChange, actor string, haveRevision bool, revision int, storedRevision int, principalValid func(string) bool) (policyPlan, error) {
	var differing []RepoPolicyChange
	seen := map[string]struct{}{}
	for _, c := range req {
		if _, dup := seen[c.RepositoryID]; dup {
			return policyPlan{}, bodyErr("merge.repositories", "repository listed twice")
		}
		seen[c.RepositoryID] = struct{}{}
		if repoPolicyDiffers(stored, c) {
			differing = append(differing, c)
		}
	}
	if len(differing) == 0 {
		return policyPlan{}, nil
	}
	if !haveRevision {
		return policyPlan{}, &SettingsError{Code: codeRevisionRequired, Field: "policy_revision", Message: "policy_revision is required to change a repository policy"}
	}
	if revision != storedRevision {
		return policyPlan{}, &MergeConflictError{Code: codeRevisionConflict}
	}
	sort.Slice(differing, func(i, j int) bool { return differing[i].RepositoryID < differing[j].RepositoryID })
	var plan policyPlan
	next := storedRevision + 1
	for _, c := range differing {
		row, audit, err := planOne(stored, c, actor, next, principalValid)
		if err != nil {
			return policyPlan{}, err
		}
		plan.Rows = append(plan.Rows, row)
		plan.Audits = append(plan.Audits, audit)
	}
	return plan, nil
}

func planOne(stored map[string]RepoPolicyRow, c RepoPolicyChange, actor string, next int, principalValid func(string) bool) (RepoPolicyRow, PolicyAudit, error) {
	cur := storedPolicy(stored, c.RepositoryID)
	if c.Policy == cur {
		// replace_principal on an automatic repository.
		if principalValid(c.RepositoryID) {
			return RepoPolicyRow{}, PolicyAudit{}, &MergeConflictError{Code: codePrincipalValid}
		}
		row := stored[c.RepositoryID]
		row.SelectedBy = actor
		return row, PolicyAudit{Kind: auditPrincipalReplaced, Subject: c.RepositoryID, Before: row.SelectedBy, After: actor, Revision: next}, nil
	}
	row := RepoPolicyRow{RepositoryID: c.RepositoryID, Policy: c.Policy}
	if c.Policy == RepoAutomatic {
		row.SelectedBy, row.AutomaticSinceRevision = actor, next
	}
	return row, PolicyAudit{Kind: auditPolicyChanged, Subject: c.RepositoryID, Before: string(cur), After: string(c.Policy), Revision: next}, nil
}
