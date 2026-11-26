package coordinator

import (
	"context"
	"sort"

	"github.com/kandev/kandev/internal/authz"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// ProtectionReader reads what the provider enforces for merging into a branch
// of a workspace repository; an empty branch is the default branch. A failed
// or unsupported read is an error.
type ProtectionReader interface {
	ReadProtection(ctx context.Context, workspaceID, repositoryID, branch string) (mergestate.Protection, error)
}

// SetProtectionReader wires the provider protection reads; nil until then and
// every read records unknown.
func (s *Service) SetProtectionReader(r ProtectionReader) {
	s.wakeMu.Lock()
	s.protection = r
	s.wakeMu.Unlock()
}

// ObserveProtection reads and records the branch's protections. A failed read
// records unknown and returns the observation; it never infers enforcement
// and never changes a repository's policy.
func (s *Service) ObserveProtection(ctx context.Context, workspaceID, repositoryID, branch string) (ProtectionObservation, error) {
	s.wakeMu.Lock()
	reader := s.protection
	s.wakeMu.Unlock()
	p := mergestate.UnknownProtection()
	if reader != nil {
		if got, err := reader.ReadProtection(ctx, workspaceID, repositoryID, branch); err == nil {
			p = got
		}
	}
	obs := ProtectionObservation{RepositoryID: repositoryID, Branch: branch, Pipeline: p.Pipeline, Approvals: p.Approvals, Threads: p.Threads, ObservedAt: s.store.now().UTC()}
	return obs, s.store.writeProtection(ctx, obs, workspaceID)
}

// RefreshProtectionAs is RefreshProtection for a workspace manager.
func (s *Service) RefreshProtectionAs(ctx context.Context, workspaceID, coordinatorID string) ([]ProtectionObservation, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	if !s.merge {
		return nil, errMergeUnavailable()
	}
	if _, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID); err != nil {
		return nil, err
	}
	return s.RefreshProtection(ctx, workspaceID, coordinatorID)
}

// RefreshProtection re-reads the protections of every repository that is not
// denied for each target branch of its open changes (the default branch when
// none), ascending by branch name, and returns the observations.
func (s *Service) RefreshProtection(ctx context.Context, workspaceID, coordinatorID string) ([]ProtectionObservation, error) {
	stored, err := s.store.loadRepoPolicies(ctx, s.store.ro, coordinatorID)
	if err != nil {
		return nil, err
	}
	branches, err := s.targetBranches(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(stored))
	for id, row := range stored {
		if row.Policy != RepoDenied && row.Policy.valid() {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var out []ProtectionObservation
	for _, id := range ids {
		bs := branches[id]
		if len(bs) == 0 {
			bs = []string{""}
		}
		for _, b := range bs {
			obs, err := s.ObserveProtection(ctx, workspaceID, id, b)
			if err != nil {
				return out, err
			}
			out = append(out, obs)
		}
	}
	return out, nil
}

// targetBranches maps each repository ID to the sorted target branches of its
// open changes, read through the observer.
func (s *Service) targetBranches(ctx context.Context, workspaceID string) (map[string][]string, error) {
	links := s.changeLinkReader()
	out := map[string][]string{}
	if links == nil || s.observer() == nil {
		return out, nil
	}
	keys, err := links.OpenChanges(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	seen := map[string]map[string]struct{}{}
	for _, k := range keys {
		key, ok := mergestate.ParseChangeKey(k)
		if !ok {
			continue
		}
		res, err := s.observer().Read(ctx, workspaceID, key, false)
		snap := res.Snapshot
		if err != nil && res.LastGood != nil {
			snap = *res.LastGood
		} else if err != nil {
			continue
		}
		if snap.TargetBranch == "" {
			continue
		}
		if seen[key.RepoID] == nil {
			seen[key.RepoID] = map[string]struct{}{}
		}
		seen[key.RepoID][snap.TargetBranch] = struct{}{}
	}
	for repo, set := range seen {
		for b := range set {
			out[repo] = append(out[repo], b)
		}
		sort.Strings(out[repo])
	}
	return out, nil
}
