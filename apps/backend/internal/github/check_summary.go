package github

import "strings"

// maxCheckSummaryObserved bounds the accepted selected-check snapshot size. A
// provider payload beyond this bound is treated as malformed metadata rather
// than persisted, because the summary is display-only evidence.
const maxCheckSummaryObserved = 10000

// CheckSummary is the bounded, head-scoped description of the last selected
// detailed check snapshot. It distinguishes an all-skipped observation from an
// empty one without changing pass-rate or repair semantics.
type CheckSummary struct {
	HeadSHA  string `json:"head_sha"`
	Observed int    `json:"observed"`
	Skipped  int    `json:"skipped"`
}

// summarizeSelectedChecks records how many selected check records were
// observed and how many of them completed as skipped. Skipped checks stay out
// of the pass-rate counts, so this metadata is the only way a consumer can
// recognize an all-skipped snapshot.
func summarizeSelectedChecks(headSHA string, checks []CheckRun) *CheckSummary {
	summary := &CheckSummary{HeadSHA: strings.TrimSpace(headSHA), Observed: len(checks)}
	for _, check := range checks {
		if check.Status == checkStatusCompleted && check.Conclusion == checkConclusionSkipped {
			summary.Skipped++
		}
	}
	return summary
}

// validCheckSummary rejects malformed or wrong-head metadata. A nil result
// means the observation is unknown, not empty.
func validCheckSummary(summary *CheckSummary, headSHA string) *CheckSummary {
	if summary == nil {
		return nil
	}
	headSHA = strings.TrimSpace(headSHA)
	if headSHA == "" || summary.HeadSHA != headSHA {
		return nil
	}
	if summary.Observed < 0 || summary.Skipped < 0 || summary.Skipped > summary.Observed {
		return nil
	}
	if summary.Observed > maxCheckSummaryObserved {
		return nil
	}
	return summary
}

// AllSkipped reports a nonempty snapshot whose every selected record completed
// as skipped.
func (s *CheckSummary) AllSkipped() bool {
	return s != nil && s.Observed > 0 && s.Skipped == s.Observed
}

func cloneCheckSummary(summary *CheckSummary) *CheckSummary {
	if summary == nil {
		return nil
	}
	copy := *summary
	return &copy
}

func checkSummarySemanticEqual(left, right *CheckSummary) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

// resolveTaskPRCheckSummary applies the observation-preservation rules at the
// storage boundary. A lightweight rollup-only sync preserves same-head
// metadata, an authoritative detailed observation replaces it, and a head
// change discards metadata that described the previous head.
func resolveTaskPRCheckSummary(previous *TaskPR, status *PRStatus, nextHeadSHA string) *CheckSummary {
	previousSummary := (*CheckSummary)(nil)
	if previous != nil && previous.HeadSHA != "" && previous.HeadSHA == nextHeadSHA {
		previousSummary = validCheckSummary(previous.CheckSummary, nextHeadSHA)
	}
	if status == nil || !status.ChecksPopulated {
		return cloneCheckSummary(previousSummary)
	}
	if incoming := validCheckSummary(status.CheckSummary, nextHeadSHA); incoming != nil {
		return cloneCheckSummary(incoming)
	}
	// A populated path that carries no valid detailed metadata is not an
	// observation of an empty snapshot; keep the same-head baseline instead.
	return cloneCheckSummary(previousSummary)
}
