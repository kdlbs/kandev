package linear

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

// maxHeldIssues bounds how many held issues a watch re-checks by ID on each
// poll. It matches the paged search window, so a watch adds at most
// issueWatchMaxPages extra requests for held issues.
const maxHeldIssues = issueWatchMaxPages * issueWatchSearchPageSize

// holdBlockedIssues re-checks the issues a watch held on earlier polls that
// the paged search did not return, removes issues that still have an open
// blocker, and records them as held for the next poll. Held issues are not
// reserved, so they take no in-flight slot until they are released.
func (s *Service) holdBlockedIssues(
	ctx context.Context, client Client, w *IssueWatch,
	issues []*LinearIssue, fetched, seen map[string]struct{},
) ([]*LinearIssue, error) {
	held, listErr := s.store.ListHeldIssueIDs(ctx, w.ID)
	if listErr != nil {
		s.log.Warn("linear: held issue list failed",
			zap.String("watch_id", w.ID), zap.Error(listErr))
	}
	revisit := make([]string, 0, len(held))
	for _, id := range held {
		if _, ok := fetched[id]; !ok {
			revisit = append(revisit, id)
		}
	}
	revisited, unchecked, err := s.revisitHeldIssues(ctx, client, w, revisit)
	if err != nil {
		return nil, err
	}
	for _, issue := range revisited {
		if _, ok := seen[issue.Identifier]; !ok {
			issues = append(issues, issue)
		}
	}
	kept, blocked := splitBlockedIssues(issues)
	if len(blocked) > 0 {
		s.log.Debug("linear: issue watch holding issues with open blockers",
			zap.String("watch_id", w.ID), zap.Int("held", len(blocked)))
	}
	// A failed read leaves the stored set alone rather than replacing it with
	// the window's view, which would forget held issues outside the window.
	if listErr == nil {
		next := heldIssueIDs(unchecked, blocked)
		if err := s.store.ReplaceHeldIssueIDs(ctx, w.ID, next); err != nil {
			s.log.Warn("linear: held issue update failed",
				zap.String("watch_id", w.ID), zap.Error(err))
		}
	}
	return kept, nil
}

// revisitHeldIssues fetches held issues by ID, still restricted by the watch
// filter, in pages of issueWatchSearchPageSize. An issue that no longer
// matches is absent from the result and so stops being held. When a request
// fails, the IDs it did not check are returned as unchecked so they stay
// held; a canceled poll returns the error.
func (s *Service) revisitHeldIssues(
	ctx context.Context, client Client, w *IssueWatch, ids []string,
) ([]*LinearIssue, []string, error) {
	var out []*LinearIssue
	for start := 0; start < len(ids); start += issueWatchSearchPageSize {
		chunk := ids[start:min(start+issueWatchSearchPageSize, len(ids))]
		filter := w.Filter
		filter.IssueIDs = chunk
		res, err := client.SearchIssues(ctx, filter, "", len(chunk))
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, nil, err
			}
			s.log.Debug("linear: held issue re-check stopped early",
				zap.String("watch_id", w.ID), zap.Error(err))
			return out, ids[start:], nil
		}
		for i := range res.Issues {
			issue := res.Issues[i]
			out = append(out, &issue)
		}
	}
	return out, nil, nil
}

// splitBlockedIssues separates issues that can start from issues that still
// have an open blocker.
func splitBlockedIssues(issues []*LinearIssue) (kept, blocked []*LinearIssue) {
	kept = make([]*LinearIssue, 0, len(issues))
	for _, issue := range issues {
		if issueBlocked(issue) {
			blocked = append(blocked, issue)
			continue
		}
		kept = append(kept, issue)
	}
	return kept, blocked
}

// issueBlocked reports whether an issue has a blocker that is not completed.
// An issue whose relation list was truncated counts as blocked, because an
// unseen blocker may still be open.
func issueBlocked(issue *LinearIssue) bool {
	if issue.BlockersTruncated {
		return true
	}
	for _, b := range issue.Blockers {
		if !b.Done() {
			return true
		}
	}
	return false
}

// heldIssueIDs lists the IDs to hold for the next poll, unchecked ones first,
// without duplicates and at most maxHeldIssues.
func heldIssueIDs(unchecked []string, blocked []*LinearIssue) []string {
	out := make([]string, 0, min(len(unchecked)+len(blocked), maxHeldIssues))
	added := make(map[string]struct{}, cap(out))
	add := func(id string) {
		if _, ok := added[id]; ok || id == "" || len(out) >= maxHeldIssues {
			return
		}
		added[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range unchecked {
		add(id)
	}
	for _, issue := range blocked {
		add(issue.ID)
	}
	return out
}
