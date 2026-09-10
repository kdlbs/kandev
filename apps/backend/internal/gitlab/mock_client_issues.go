package gitlab

import (
	"context"
	"net/url"
	"sort"
)

// resolveEffectiveMilestone derives the milestone MockClient.ListIssues
// filters on from the three raw search inputs the controller passes through
// unmodified — MockClient never calls buildIssueSearchQuery, so it must
// reproduce the fold precedence itself. The first matching rule wins:
//
//  1. customQuery carries a "milestone" key: its first value, even if empty
//     (an empty key still wins outright — it must not fall through to the
//     milestone parameter, matching how an empty fold reaches GitLab as an
//     empty milestone on the live path).
//  2. the milestone parameter, if non-empty.
//  3. filter carries a non-empty "milestone" key: its first value.
//  4. otherwise, empty.
func resolveEffectiveMilestone(filter, customQuery, milestone string) string {
	if parsed, err := url.ParseQuery(customQuery); err == nil && parsed.Has("milestone") {
		return parsed.Get("milestone")
	}
	if milestone != "" {
		return milestone
	}
	if parsed, err := url.ParseQuery(filter); err == nil {
		if v := parsed.Get("milestone"); v != "" {
			return v
		}
	}
	return ""
}

// ListIssues returns seeded issues, optionally narrowed to an exact
// case-sensitive milestone match. See resolveEffectiveMilestone for how
// filter/customQuery/milestone combine. Every other filter key is ignored —
// PATClient is the only client that talks to real GitLab filtering.
func (c *MockClient) ListIssues(_ context.Context, filter, customQuery, milestone string) ([]*Issue, error) {
	effective := resolveEffectiveMilestone(filter, customQuery, milestone)

	c.mu.Lock()
	defer c.mu.Unlock()
	out := []*Issue{}
	for _, i := range c.issues {
		if effective != "" && i.Milestone != effective {
			continue
		}
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].ProjectPath != out[b].ProjectPath {
			return out[a].ProjectPath < out[b].ProjectPath
		}
		return out[a].IID < out[b].IID
	})
	return out, nil
}

// ListIssuesPaged pages over ListIssues' filtered, sorted result. page < 1 is
// normalised to 1; perPage < 1 normalises to 0 (no issues, any page). There
// is no upper clamp on perPage: the echoed Page/PerPage are the normalised
// values actually used, from those two rules only. TotalCount is always the
// full filtered count, not the length of the returned slice.
func (c *MockClient) ListIssuesPaged(
	ctx context.Context, filter, customQuery, milestone string, page, perPage int,
) (*IssueSearchPage, error) {
	issues, err := c.ListIssues(ctx, filter, customQuery, milestone)
	if err != nil {
		return nil, err
	}

	normPage := page
	if normPage < 1 {
		normPage = 1
	}
	normPerPage := perPage
	if normPerPage < 1 {
		normPerPage = 0
	}

	// Guard the multiplication itself: normPage is not bounded above, so
	// (normPage-1)*normPerPage can overflow int before the "start >
	// len(issues)" clamp below ever runs. Comparing via division first is
	// safe from overflow and gives the same answer.
	var start int
	if normPerPage == 0 || normPage-1 > len(issues)/normPerPage {
		start = len(issues)
	} else {
		start = (normPage - 1) * normPerPage
	}

	// Guard start+normPerPage the same way: normPerPage is unbounded above
	// (no clamp), so the addition can itself overflow int. Comparing against
	// the remaining count first avoids ever computing the sum when it would
	// exceed len(issues).
	end := len(issues)
	if normPerPage < len(issues)-start {
		end = start + normPerPage
	}

	return &IssueSearchPage{
		Issues:     issues[start:end],
		TotalCount: len(issues),
		Page:       normPage,
		PerPage:    normPerPage,
	}, nil
}
