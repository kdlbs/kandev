package coordinator

import (
	"context"
	"encoding/json"
)

const maxMergeRepositories = 500

// parseMergeMember reads the optional merge member and the policy_revision
// that guards it. A body naming merge while the flag is off is refused.
func parseMergeMember(body []byte, enabled bool, req *settingsRequest) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return nil // parseSettingsBody already refused it
	}
	if raw, ok := presentMember(top, "policy_revision"); ok {
		var n int
		if err := json.Unmarshal(raw, &n); err != nil || n < 0 {
			return bodyErr("policy_revision", "policy_revision must be a non-negative integer")
		}
		req.haveRevision, req.revision = true, n
	}
	raw, ok := presentMember(top, "merge")
	if !ok {
		return nil
	}
	if !enabled {
		return &SettingsError{Code: codeMergeUnavailable, Field: "merge", Message: "merge settings are not available"}
	}
	var member struct {
		Repositories []struct {
			RepositoryID     string `json:"repository_id"`
			Policy           string `json:"policy"`
			ReplacePrincipal bool   `json:"replace_principal"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(raw, &member); err != nil {
		return bodyErr("merge", "merge must be an object with a repositories list")
	}
	if len(member.Repositories) > maxMergeRepositories {
		return bodyErr("merge.repositories", "too many repositories")
	}
	for _, r := range member.Repositories {
		if r.RepositoryID == "" || !RepoPolicy(r.Policy).valid() {
			return bodyErr("merge.repositories", "each entry needs a repository_id and a policy of denied, requires_approval or automatic")
		}
		req.merge = append(req.merge, RepoPolicyChange{RepositoryID: r.RepositoryID, Policy: RepoPolicy(r.Policy), ReplacePrincipal: r.ReplacePrincipal})
	}
	return nil
}

// planMergePolicies runs inside the coordinator lock. It validates that every
// listed repository is a live repository of the workspace (a deleted or foreign
// one cannot be set to anything but denied), then plans the change.
func (s *Service) planMergePolicies(ctx context.Context, tx coordinatorExec, workspaceID, coordinatorID string, req settingsRequest, storedRevision int) (policyPlan, error) {
	if len(req.merge) == 0 {
		return policyPlan{}, nil
	}
	stored, err := s.store.loadRepoPolicies(ctx, tx, coordinatorID)
	if err != nil {
		return policyPlan{}, err
	}
	live, err := s.store.liveRepositories(ctx, tx, workspaceID)
	if err != nil {
		return policyPlan{}, err
	}
	liveIDs := make(map[string]struct{}, len(live))
	for _, r := range live {
		liveIDs[r.ID] = struct{}{}
	}
	for _, c := range req.merge {
		if _, ok := liveIDs[c.RepositoryID]; !ok && c.Policy != RepoDenied {
			return policyPlan{}, &SettingsError{Code: codeRepositoryUnknown, Field: "merge.repositories", Message: "repository is not available in this workspace"}
		}
	}
	validity := map[string]bool{}
	for id, row := range stored {
		if row.Policy == RepoAutomatic {
			validity[id] = s.principalValid(ctx, workspaceID, row.SelectedBy)
		}
	}
	return planRepoPolicies(stored, req.merge, decidingUserID(ctx), req.haveRevision, req.revision, storedRevision, func(id string) bool { return validity[id] })
}

// principalValid reads an execution principal as authorized only when the
// validator confirms it; an empty principal, a missing validator and a failed
// read are not authorized.
func (s *Service) principalValid(ctx context.Context, workspaceID, userID string) bool {
	s.wakeMu.Lock()
	v := s.principals
	s.wakeMu.Unlock()
	if v == nil || userID == "" {
		return false
	}
	ok, err := v.ManagerAuthorized(ctx, workspaceID, userID)
	return err == nil && ok
}

// SetPrincipalValidator wires the execution-principal check.
func (s *Service) SetPrincipalValidator(v PrincipalValidator) {
	s.wakeMu.Lock()
	s.principals = v
	s.wakeMu.Unlock()
}

// MergeSettingsDTO is the settings read's repository merge policies.
type MergeSettingsDTO struct {
	Repositories []MergeRepoDTO `json:"repositories"`
}

// MergeRepoDTO is one workspace repository and its effective merge policy.
// AutomaticActive is false when the policy is automatic but its execution
// principal is no longer an authorized manager.
type MergeRepoDTO struct {
	RepositoryID           string     `json:"repository_id"`
	Name                   string     `json:"name"`
	Policy                 RepoPolicy `json:"policy"`
	AutomaticSinceRevision int        `json:"automatic_since_revision,omitempty"`
	AutomaticActive        bool       `json:"automatic_active"`
}

// mergeView lists every live repository of the workspace, ordered by name then
// ID, with its policy (denied when none is stored). It is nil while merge is
// off.
func (s *Service) mergeView(ctx context.Context, workspaceID, coordinatorID string) (*MergeSettingsDTO, error) {
	if !s.merge {
		return nil, nil
	}
	stored, err := s.store.loadRepoPolicies(ctx, s.store.ro, coordinatorID)
	if err != nil {
		return nil, err
	}
	repos, err := s.store.liveRepositories(ctx, s.store.ro, workspaceID)
	if err != nil {
		return nil, err
	}
	out := &MergeSettingsDTO{Repositories: make([]MergeRepoDTO, 0, len(repos))}
	for _, r := range repos {
		row := stored[r.ID]
		dto := MergeRepoDTO{RepositoryID: r.ID, Name: r.Name, Policy: storedPolicy(stored, r.ID)}
		if dto.Policy == RepoAutomatic {
			dto.AutomaticSinceRevision = row.AutomaticSinceRevision
			dto.AutomaticActive = s.principalValid(ctx, workspaceID, row.SelectedBy)
		}
		out.Repositories = append(out.Repositories, dto)
	}
	return out, nil
}
