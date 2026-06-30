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
			return timeLess(parseLinearTime(issues[i].Created), parseLinearTime(issues[j].Created), false)
		})
	case SortByCreatedAsc:
		sort.SliceStable(issues, func(i, j int) bool {
			return timeLess(parseLinearTime(issues[i].Created), parseLinearTime(issues[j].Created), true)
		})
	case SortByUpdatedDesc:
		sort.SliceStable(issues, func(i, j int) bool {
			return timeLess(parseLinearTime(issues[i].Updated), parseLinearTime(issues[j].Updated), false)
		})
	case SortByUpdatedAsc:
		sort.SliceStable(issues, func(i, j int) bool {
			return timeLess(parseLinearTime(issues[i].Updated), parseLinearTime(issues[j].Updated), true)
		})
	}
}

// timeLess orders two Linear timestamps for the date sort modes. Valid times
// order normally (ascending or descending per `asc`); an invalid/zero time
// always sorts AFTER a valid one so malformed values never take the earliest
// dispatch slots. Two invalids keep their input order (stable sort).
func timeLess(a, b time.Time, asc bool) bool {
	if a.IsZero() || b.IsZero() {
		return !a.IsZero() && b.IsZero() // valid before invalid; else keep order
	}
	if asc {
		return a.Before(b)
	}
	return a.After(b)
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
// on empty/unparseable input. timeLess treats that zero time as "sort last"
// in both directions so malformed values never grab the earliest slots.
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
