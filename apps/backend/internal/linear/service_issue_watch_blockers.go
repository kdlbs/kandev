package linear

import (
	"context"
	"errors"
	"slices"

	"go.uber.org/zap"
)

// maxPendingIssues bounds how many pending issues a watch re-checks by ID on
// each poll. It matches the paged search window, so a watch adds at most
// issueWatchMaxPages extra requests for pending issues.
const maxPendingIssues = issueWatchMaxPages * issueWatchSearchPageSize

// holdBlockedIssues returns dispatch candidates and the pending issue IDs for
// the next poll. The pending set covers held issues and dispatch attempts
// without a task ID.
func (s *Service) holdBlockedIssues(
	ctx context.Context, client Client, w *IssueWatch,
	issues []*LinearIssue, reservations map[string]bool,
) ([]*LinearIssue, []string, error) {
	pending, err := s.store.ListPendingIssueIDs(ctx, w.ID)
	if err != nil {
		return nil, nil, err
	}
	revisited, unchecked, err := s.revisitPendingIssues(ctx, client, w, unobservedIssueIDs(pending, issues))
	if err != nil {
		return nil, nil, err
	}
	kept, blocked := splitBlockedIssues(append(issues, revisited...), reservations)
	if len(blocked) > 0 {
		s.log.Debug("linear: issue watch holding issues with open blockers",
			zap.String("watch_id", w.ID), zap.Int("held", len(blocked)))
	}
	next := pendingIssueIDs(unchecked, blocked, kept)
	return unreservedIssues(kept, reservations), next, nil
}

// Only a complete batch can establish that its missing IDs no longer match.
func (s *Service) revisitPendingIssues(
	ctx context.Context, client Client, w *IssueWatch, ids []string,
) ([]*LinearIssue, []string, error) {
	var out []*LinearIssue
	budget := issueWatchMaxPages
	for len(ids) > 0 && budget > 0 {
		size := min(issueWatchSearchPageSize, len(ids))
		filter := w.Filter
		filter.IssueIDs = ids[:size]
		batch, requests, complete, err := searchPendingIssueBatch(ctx, client, filter, budget)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, nil, err
		}
		if err != nil {
			s.log.Debug("linear: pending issue re-check stopped early",
				zap.String("watch_id", w.ID), zap.Error(err))
		}
		out = append(out, batch...)
		ids = ids[size:]
		if !complete {
			return out, append(unobservedIssueIDs(filter.IssueIDs, batch), ids...), nil
		}
		budget -= requests
	}
	return out, ids, nil
}

func searchPendingIssueBatch(
	ctx context.Context, client Client, filter SearchFilter, budget int,
) ([]*LinearIssue, int, bool, error) {
	var issues []*LinearIssue
	var cursor string
	seenCursors := make(map[string]struct{})
	for requests := 1; requests <= budget; requests++ {
		res, err := client.SearchIssues(ctx, filter, cursor, len(filter.IssueIDs))
		if err != nil {
			return issues, requests, false, err
		}
		for i := range res.Issues {
			issue := res.Issues[i]
			issues = append(issues, &issue)
		}
		if res.IsLast {
			return issues, requests, true, nil
		}
		cursor = res.NextPageToken
		if _, seen := seenCursors[cursor]; cursor == "" || seen {
			return issues, requests, false, errors.New("linear: empty or repeated pending issue cursor")
		}
		seenCursors[cursor] = struct{}{}
	}
	return issues, budget, false, nil
}

func unobservedIssueIDs(ids []string, issues []*LinearIssue) []string {
	observed := make(map[string]struct{}, len(issues))
	for _, issue := range issues {
		observed[issue.ID] = struct{}{}
	}
	var out []string
	for _, id := range ids {
		if _, ok := observed[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

func unreservedIssues(issues []*LinearIssue, reservations map[string]bool) []*LinearIssue {
	return slices.DeleteFunc(issues, func(issue *LinearIssue) bool {
		_, reserved := reservations[issue.Identifier]
		return reserved
	})
}

// splitBlockedIssues classifies matches that have no recorded task.
func splitBlockedIssues(issues []*LinearIssue, reservations map[string]bool) (kept, blocked []*LinearIssue) {
	kept = make([]*LinearIssue, 0, len(issues))
	for _, issue := range issues {
		if reservations[issue.Identifier] {
			continue
		}
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
// then held issues, then released issues without a recorded task, without
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
