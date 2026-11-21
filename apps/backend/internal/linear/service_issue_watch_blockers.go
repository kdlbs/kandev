package linear

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

// maxPendingIssues bounds how many pending issues a watch re-checks by ID on
// each poll. It matches the paged search window, so a watch adds at most
// issueWatchMaxPages extra requests for pending issues.
const maxPendingIssues = issueWatchMaxPages * issueWatchSearchPageSize

// holdBlockedIssues removes issues that still have an open blocker. Held
// issues are not reserved, so they take no in-flight slot.
//
// A held issue does not change when its blocker completes, so it can leave
// the paged search window while it waits. The watch therefore keeps its
// matched but unstarted issues as pending: each poll re-checks the pending
// IDs the window did not return, and an issue stays pending until dispatch
// reserves it or it stops matching the filter. A released issue whose event is
// deferred by the in-flight cap is thus found again on the next poll.
func (s *Service) holdBlockedIssues(
	ctx context.Context, client Client, w *IssueWatch,
	issues []*LinearIssue, fetched, seen map[string]struct{},
) ([]*LinearIssue, error) {
	pending, listErr := s.store.ListPendingIssueIDs(ctx, w.ID)
	if listErr != nil {
		s.log.Warn("linear: pending issue list failed",
			zap.String("watch_id", w.ID), zap.Error(listErr))
	}
	revisit := make([]string, 0, len(pending))
	for _, id := range pending {
		if _, ok := fetched[id]; !ok {
			revisit = append(revisit, id)
		}
	}
	revisited, unchecked, err := s.revisitPendingIssues(ctx, client, w, revisit)
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
	// the window's view, which would forget pending issues outside the window.
	if listErr == nil {
		next := pendingIssueIDs(unchecked, blocked, kept)
		if err := s.store.ReplacePendingIssueIDs(ctx, w.ID, next); err != nil {
			s.log.Warn("linear: pending issue update failed",
				zap.String("watch_id", w.ID), zap.Error(err))
		}
	}
	return kept, nil
}

// revisitPendingIssues fetches pending issues by ID, still restricted by the
// watch filter, in pages of issueWatchSearchPageSize. An issue that no longer
// matches is absent from the result and so stops being pending. When a
// request fails, the IDs it did not check are returned as unchecked so they
// stay pending; a canceled poll returns the error.
func (s *Service) revisitPendingIssues(
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
			s.log.Debug("linear: pending issue re-check stopped early",
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

// pendingIssueIDs lists the IDs to re-check on the next poll: unchecked ones,
// then held issues, then released issues not yet reserved, without
// duplicates and at most maxPendingIssues.
func pendingIssueIDs(unchecked []string, groups ...[]*LinearIssue) []string {
	out := make([]string, 0, maxPendingIssues)
	added := make(map[string]struct{}, maxPendingIssues)
	add := func(id string) {
		if _, ok := added[id]; ok || id == "" || len(out) >= maxPendingIssues {
			return
		}
		added[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range unchecked {
		add(id)
	}
	for _, group := range groups {
		for _, issue := range group {
			add(issue.ID)
		}
	}
	return out
}
