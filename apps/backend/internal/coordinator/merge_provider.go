package coordinator

import (
	"context"
	"slices"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// MethodSet is the merge methods a change's repository currently allows and the
// one it selects by default. For GitLab the labels are "squash" and "merge":
// the squash flag, not a method name, is what Kandev sends.
type MethodSet struct {
	Allowed []string
	Default string
}

func (m MethodSet) allows(method string) bool {
	return method != "" && slices.Contains(m.Allowed, method)
}

// MergeOutcome is the provider's definite answer to a merge request.
type MergeOutcome int

const (
	// MergeAccepted means the provider merged the change.
	MergeAccepted MergeOutcome = iota
	// MergeRejected means the provider refused the merge.
	MergeRejected
	// MergeStaleHead means the provider reports the expected head no longer
	// matches.
	MergeStaleHead
	// MergeQueued means the provider queued the merge or reports it in
	// progress; the outcome is not known.
	MergeQueued
)

// MergeResult is a provider answer. A request that gave no answer returns an
// error instead.
type MergeResult struct {
	Outcome     MergeOutcome
	MergeCommit string
	// Reason is the bounded provider reason of a rejection.
	Reason string
}

// ChangeMerger is the provider seam of the merge kind. Every call is bound to
// the workspace's credentials and the key's canonical host, and none of them
// forces, bypasses checks, pushes or enables a deferred provider merge.
type ChangeMerger interface {
	// ReadLive reads the change from the provider, bypassing every cache.
	ReadLive(ctx context.Context, workspaceID string, key mergestate.ChangeKey) (mergestate.Snapshot, error)
	// Methods reads the merge methods the repository allows.
	Methods(ctx context.Context, workspaceID string, key mergestate.ChangeKey) (MethodSet, error)
	// Merge sends one merge request that carries the expected head and method.
	Merge(ctx context.Context, workspaceID string, key mergestate.ChangeKey, headSHA, method string) (MergeResult, error)
	// CommitMessages lists up to limit commit messages of the change's target
	// branch after sha, oldest first, for the given page of 100.
	CommitMessages(ctx context.Context, workspaceID string, key mergestate.ChangeKey, afterSHA string, page int) (messages []string, more bool, err error)
}

// SetChangeMerger wires the provider seam; merge proposals cannot be created or
// executed without it.
func (s *Service) SetChangeMerger(m ChangeMerger) {
	s.wakeMu.Lock()
	s.merger = m
	s.wakeMu.Unlock()
}

func (s *Service) changeMerger() ChangeMerger {
	s.wakeMu.Lock()
	defer s.wakeMu.Unlock()
	return s.merger
}
