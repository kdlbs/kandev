package shared

// RunProvenance classifies a run as attended or unattended, per
// REQ-OFFICE-BUDGET-007 (docs/specs/office/requirements/budget-run-provenance.md).
// A person plausibly caused an attended run and is plausibly waiting on it.
type RunProvenance int

const (
	// RunProvenanceUnattended is the safe default for any reason not on the
	// attended allowlist, including one the build has never seen.
	RunProvenanceUnattended RunProvenance = iota
	RunProvenanceAttended
)

// Attended reports whether p classifies as attended.
func (p RunProvenance) Attended() bool { return p == RunProvenanceAttended }

// attendedRunReasons is the exact 20-literal allowlist of
// AC-OFFICE-BUDGET-007.2: 16 current reasons plus 4 legacy literals. Every
// other value, including one the build does not recognize, classifies as
// unattended (RunProvenanceUnattended's zero value).
//
// Polarity warning: this is the OPPOSITE safe default from
// IsPeriodicTasklessWake, which reads the same runs.reason column for the
// idle-skip gate. Do not reuse that classifier here — see the glossary
// (docs/specs/office/glossary.md, "Budget enforcement").
var attendedRunReasons = map[string]struct{}{
	"task_assigned":               {},
	"task_comment":                {},
	"task_review_requested":       {},
	"task_changes_requested":      {},
	"task_blockers_resolved":      {},
	"task_children_completed":     {},
	"approval_resolved":           {},
	"routine_dispatch_event":      {},
	"manual_resume_after_failure": {},
	"task_mentioned":              {},
	"task_reopened":               {},
	"task_reopened_via_comment":   {},
	"task_unblocked":              {},
	"task_ready_to_close":         {},
	"stage_pending":               {},
	"stage_changes_requested":     {},
	// Legacy literals, still reachable on rows queued before the named
	// constants above existed.
	"review_started":     {},
	"approval_started":   {},
	"blockers_resolved":  {},
	"children_completed": {},
}

// ClassifyRunProvenance implements AC-OFFICE-BUDGET-007.1/.2: a total
// function of reason, computed once per run and never revised by any gate.
func ClassifyRunProvenance(reason string) RunProvenance {
	if _, ok := attendedRunReasons[reason]; ok {
		return RunProvenanceAttended
	}
	return RunProvenanceUnattended
}
