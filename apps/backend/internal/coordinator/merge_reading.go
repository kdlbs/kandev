package coordinator

import (
	"context"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// mergeSpec is the frozen spec of a merge proposal.
type mergeSpec struct {
	TaskID       string `json:"task_id"`
	RepositoryID string `json:"repository_id"`
	ChangeKey    string `json:"change_key"`
	HeadSHA      string `json:"head_sha"`
	Method       string `json:"method"`
}

// Refusal and settlement codes of a merge proposal.
const (
	refuseStaleHead       = "stale_head"
	refuseExpired         = "expired"
	refuseNotMerged       = "not_merged"
	refuseUnconfirmed     = "unconfirmed"
	refuseProviderReject  = "provider_rejected"
	refuseAuthority       = "authority"
	refuseClosed          = "closed"
	refuseMergedElsewhere = "merged_elsewhere"
	reasonPolicyChanged   = "policy_changed"
)

// mergeReading is one uncached evaluation of the recheck conditions.
type mergeReading struct {
	Decision mergestate.Decision
	Snap     mergestate.Snapshot
	// DepRevision is the revision of the dependency set the predecessor
	// condition was evaluated against.
	DepRevision int
}

// readMerge evaluates the recheck of a merge proposal: every condition read
// again from the provider, failing closed.
func (s *Service) readMerge(ctx context.Context, c *Coordinator, m *MergeProposal, taskID string, automatic, claimed bool) (mergeReading, error) {
	in := mergestate.RecheckInput{Automatic: automatic, ExpectedHead: m.HeadSHA}
	var out mergeReading
	authority, err := s.mergeAuthority(ctx, c, m, automatic, claimed)
	if err != nil {
		return out, err
	}
	in.Authority = authority
	if automatic {
		paused, gateErr := s.gate.Active(ctx, c.ID)
		if gateErr != nil {
			out.Decision = mergestate.Decision{Reason: "unavailable:pause", Outcome: mergestate.Wait}
			return out, nil
		}
		in.Paused = paused
	}
	live, err := s.repositoryLive(ctx, c.WorkspaceID, m.RepositoryID)
	if err != nil {
		return out, err
	}
	in.RepoAvailable = mergestate.Unmet
	if live {
		in.RepoAvailable = mergestate.Met
	}
	key, ok := mergestate.ParseChangeKey(m.ChangeKey)
	if !ok {
		out.Decision = mergestate.Decision{Reason: "unavailable:head", Outcome: mergestate.Wait}
		return out, nil
	}
	s.fillProviderReading(ctx, c.WorkspaceID, key, m, &in, &out)
	in.Predecessors, out.DepRevision, err = s.predecessorState(ctx, c.WorkspaceID, c.ID, taskID)
	if err != nil {
		in.Predecessors = mergestate.Unavailable
	}
	out.Decision = mergestate.Recheck(in)
	return out, nil
}

// fillProviderReading reads the change and the allowed methods. A failed read
// leaves its conditions unavailable.
func (s *Service) fillProviderReading(ctx context.Context, workspaceID string, key mergestate.ChangeKey, m *MergeProposal, in *mergestate.RecheckInput, out *mergeReading) {
	in.Pipeline, in.Method = mergestate.PipelineUnavailable, mergestate.Unavailable
	in.Mergeable.Mergeable = mergestate.Unavailable
	merger := s.changeMerger()
	if merger == nil {
		return
	}
	if methods, err := merger.Methods(ctx, workspaceID, key); err == nil {
		in.Method = mergestate.Unmet
		if methods.allows(m.Method) {
			in.Method = mergestate.Met
		}
	}
	snap, err := merger.ReadLive(ctx, workspaceID, key)
	if err != nil {
		return
	}
	out.Snap = snap
	in.Closed = snap.State == mergestate.StateClosed
	in.MergedElsewhere = snap.State == mergestate.StateMerged
	in.CurrentHead = snap.HeadSHA
	in.Pipeline, in.Mergeable, in.Conflict = snap.Pipeline, snap.Mergeable, snap.Conflict
	in.NotDraft, in.Approvals = snap.Draft, snap.Approvals
	in.Threads = threadsTri(snap.UnresolvedThreads)
}

func threadsTri(unresolved int) mergestate.Tri {
	switch {
	case unresolved < 0:
		return mergestate.Unavailable
	case unresolved == 0:
		return mergestate.Met
	}
	return mergestate.Unmet
}

// mergeAuthority reports whether the flags and the stored repository policy
// allow the merge now. A claimed automatic attempt also needs the execution
// principal to be valid; waiting passes do not check it.
func (s *Service) mergeAuthority(ctx context.Context, c *Coordinator, m *MergeProposal, automatic, claimed bool) (bool, error) {
	if !s.merge {
		return false, nil
	}
	policies, err := s.store.loadRepoPolicies(ctx, s.store.ro, c.ID)
	if err != nil {
		return false, err
	}
	row, ok := policies[m.RepositoryID]
	if !ok || row.Policy == RepoDenied {
		return false, nil
	}
	if !automatic {
		return true, nil
	}
	if row.Policy != RepoAutomatic {
		return false, nil
	}
	return !claimed || s.principalValid(ctx, c.WorkspaceID, row.SelectedBy), nil
}

func (s *Service) repositoryLive(ctx context.Context, workspaceID, repositoryID string) (bool, error) {
	live, err := s.store.liveRepositories(ctx, s.store.ro, workspaceID)
	if err != nil {
		return false, err
	}
	for _, r := range live {
		if r.ID == repositoryID {
			return true, nil
		}
	}
	return false, nil
}
