package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/github"
)

// ResolveBranchForWatch keeps checkout identity scoped to the source session.
// The watch's attributed task can be a group owner with different branches.
func (s *Service) ResolveBranchForWatch(ctx context.Context, watch *github.PRWatch) string {
	if watch == nil || watch.SessionID == "" || watch.RepositoryID == "" {
		return ""
	}
	session, err := s.repo.GetTaskSession(ctx, watch.SessionID)
	if err != nil || session == nil {
		return ""
	}
	targets := s.resolveSessionWatchTargets(ctx, session.TaskID, session.ID, "")
	return matchingWatchBranch(targets, watch.RepositoryID, watch.Branch)
}

func matchingWatchBranch(targets []sessionWatchTarget, repositoryID, current string) string {
	branches := make(map[string]struct{})
	for _, target := range targets {
		if target.RepositoryID != repositoryID || target.Branch == "" {
			continue
		}
		if target.Branch == current {
			return current
		}
		branches[target.Branch] = struct{}{}
	}
	if len(branches) == 1 {
		for branch := range branches {
			return branch
		}
	}
	return ""
}
