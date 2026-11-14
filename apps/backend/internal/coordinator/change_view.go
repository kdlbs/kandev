package coordinator

import (
	"context"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// Reading labels carried to the Queue and the turn context. Unknown is never
// presented as passing.
const (
	ReadingMet         = "met"
	ReadingUnmet       = "unmet"
	ReadingUnavailable = "unavailable"
)

// ChangeView is one linked change as the Queue and the coordinator turn
// context show it. When the latest read failed, the last successful values stay
// in place with their observation time and Unavailable is set.
type ChangeView struct {
	Key          string `json:"key"`
	Provider     string `json:"provider"`
	Host         string `json:"host"`
	RepositoryID string `json:"repository_id"`
	Number       int    `json:"number"`
	TargetBranch string `json:"target_branch"`
	HeadSHA      string `json:"head_sha"`
	State        string `json:"state"`
	// Pipeline is green, red, pending, none ("No checks") or unavailable.
	Pipeline string `json:"pipeline"`
	// Mergeable, Conflict, Approvals and Draft are met, unmet or unavailable.
	// Draft is met when the change is not a draft.
	Mergeable string `json:"mergeable"`
	Conflict  string `json:"conflict"`
	Approvals string `json:"approvals"`
	Draft     string `json:"draft"`
	// UnresolvedThreads and Behind are nil when they could not be read.
	UnresolvedThreads *int `json:"unresolved_threads"`
	Behind            *int `json:"behind"`
	// Unavailable is set when the latest read failed or nothing was ever read.
	Unavailable bool `json:"unavailable"`
	// ObservedAt is when the shown values were read; zero when none were.
	ObservedAt time.Time `json:"observed_at"`
}

func triLabel(t mergestate.Tri) string {
	switch t {
	case mergestate.Met:
		return ReadingMet
	case mergestate.Unmet:
		return ReadingUnmet
	}
	return ReadingUnavailable
}

func conflictLabel(c mergestate.ConflictState) string {
	switch c {
	case mergestate.ConflictClear:
		return ReadingMet
	case mergestate.ConflictPresent:
		return ReadingUnmet
	}
	return ReadingUnavailable
}

func countPtr(n int) *int {
	if n < 0 {
		return nil
	}
	return &n
}

func viewOf(key mergestate.ChangeKey, snap mergestate.Snapshot, at time.Time, failed bool) ChangeView {
	return ChangeView{
		Key: key.String(), Provider: key.Provider, Host: key.Host, RepositoryID: key.RepoID, Number: key.Number,
		TargetBranch: snap.TargetBranch, HeadSHA: snap.HeadSHA, State: string(snap.State),
		Pipeline: string(snap.Pipeline), Mergeable: triLabel(snap.Mergeable.Mergeable),
		Conflict: conflictLabel(snap.Conflict), Approvals: triLabel(snap.Approvals), Draft: triLabel(snap.Draft),
		UnresolvedThreads: countPtr(snap.UnresolvedThreads), Behind: countPtr(snap.Behind),
		Unavailable: failed, ObservedAt: at,
	}
}

func unreadView(key mergestate.ChangeKey) ChangeView {
	v := viewOf(key, mergestate.Snapshot{Pipeline: mergestate.PipelineUnavailable, UnresolvedThreads: -1, Behind: -1}, time.Time{}, true)
	v.State = ""
	return v
}

// ChangeViewsAs is ChangeViews for a workspace reader of an existing coordinator.
func (s *Service) ChangeViewsAs(ctx context.Context, workspaceID, coordinatorID, taskID string, retry bool) ([]ChangeView, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if _, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID); err != nil {
		return nil, err
	}
	return s.ChangeViews(ctx, workspaceID, taskID, retry)
}

// ChangeViews returns the task's linked changes ordered by provider, host,
// repository ID and number, each read through ObserveChange. retry bypasses the
// 60-second reuse. A task with no linked change returns nil.
func (s *Service) ChangeViews(ctx context.Context, workspaceID, taskID string, retry bool) ([]ChangeView, error) {
	links := s.changeLinkReader()
	if !s.merge || links == nil {
		return nil, nil
	}
	raw, err := links.ChangesLinkedToTask(ctx, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	keys := make([]mergestate.ChangeKey, 0, len(raw))
	for _, r := range raw {
		if k, ok := mergestate.ParseChangeKey(r); ok {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
	views := make([]ChangeView, 0, len(keys))
	for _, k := range keys {
		res, err := s.ObserveChange(ctx, workspaceID, k, retry)
		switch {
		case err == nil:
			views = append(views, viewOf(k, res.Snapshot, res.StartedAt, false))
		case res.LastGood != nil:
			views = append(views, viewOf(k, *res.LastGood, res.LastGoodAt, true))
		default:
			views = append(views, unreadView(k))
		}
	}
	return views, nil
}
