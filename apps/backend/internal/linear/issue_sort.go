package linear

import (
	"sort"
	"time"
)

// sortIssues reorders matched issues in place so the highest-priority /
// most-relevant issues are published (and therefore dispatched) first under
// the watch's in-flight cap. An empty/unknown sort key leaves the slice in the
// order Linear returned it (updatedAt asc). Stable so equal-key issues keep
// Linear's relative order.
func sortIssues(issues []*LinearIssue, by IssueSortBy) {
	switch by {
	case SortByPriorityDesc:
		sort.SliceStable(issues, func(i, j int) bool {
			return priorityRank(issues[i].Priority) < priorityRank(issues[j].Priority)
		})
	case SortByPriorityAsc:
		sort.SliceStable(issues, func(i, j int) bool {
			return priorityRank(issues[i].Priority) > priorityRank(issues[j].Priority)
		})
	case SortByCreatedDesc:
		sort.SliceStable(issues, func(i, j int) bool {
			return parseLinearTime(issues[i].Created).After(parseLinearTime(issues[j].Created))
		})
	case SortByCreatedAsc:
		sort.SliceStable(issues, func(i, j int) bool {
			return parseLinearTime(issues[i].Created).Before(parseLinearTime(issues[j].Created))
		})
	case SortByUpdatedDesc:
		sort.SliceStable(issues, func(i, j int) bool {
			return parseLinearTime(issues[i].Updated).After(parseLinearTime(issues[j].Updated))
		})
	case SortByUpdatedAsc:
		sort.SliceStable(issues, func(i, j int) bool {
			return parseLinearTime(issues[i].Updated).Before(parseLinearTime(issues[j].Updated))
		})
	}
}

// priorityRank maps Linear's priority encoding (0=none,1=urgent,2=high,
// 3=medium,4=low) onto an importance rank where LOWER = more important:
// urgent(1) < high(2) < medium(3) < low(4) < none. "None" (0) is treated as
// least important, so it ranks after low rather than before urgent.
func priorityRank(p int) int {
	if p == 0 {
		return 5
	}
	return p
}

// parseLinearTime parses a Linear ISO-8601 timestamp, returning the zero time
// on empty/unparseable input so such issues sort to a consistent end.
func parseLinearTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
