// Package mergestate holds the provider-neutral reading, episode and ordering
// rules for coordinator merge stewardship. It has no I/O.
package mergestate

// Tri is the three-valued reading of a merge condition.
type Tri int

const (
	// Unavailable means the provider did not give a trustworthy answer. It never
	// counts as passing.
	Unavailable Tri = iota
	// Unmet means the condition is known not to hold.
	Unmet
	// Met means the condition is known to hold.
	Met
)

// Condition names a merge condition in the closed refusal-reason vocabulary.
type Condition string

// Conditions that a provider status can explain.
const (
	CondHead        Condition = "head"
	CondPipeline    Condition = "pipeline"
	CondMergeable   Condition = "mergeable"
	CondDraft       Condition = "draft"
	CondApprovals   Condition = "approvals"
	CondThreads     Condition = "threads"
	CondPredecessor Condition = "predecessor"
	CondMethod      Condition = "method"
)

// Pipeline is the normalized pipeline state of a change head.
type Pipeline string

// Pipeline states.
const (
	PipelineGreen       Pipeline = "green"
	PipelineRed         Pipeline = "red"
	PipelinePending     Pipeline = "pending"
	PipelineNone        Pipeline = "none"
	PipelineUnavailable Pipeline = "unavailable"
)

// Tri maps a pipeline state to its condition reading: only green is met.
func (p Pipeline) Tri() Tri {
	switch p {
	case PipelineGreen:
		return Met
	case PipelineUnavailable:
		return Unavailable
	default:
		return Unmet
	}
}

// Run is one check run (or status context) of the head commit.
type Run struct {
	Status     string // queued, in_progress, completed
	Conclusion string // success, failure, neutral, cancelled, timed_out, action_required, skipped, ...
}

// PipelineInput is everything GitHub reports about a head's checks.
type PipelineInput struct {
	Runs []Run
	// CombinedStatus is the legacy combined commit status: success, failure,
	// error, pending or empty when no statuses exist.
	CombinedStatus string
	// RequiredUnreported counts required contexts that are configured by branch
	// protection but have not reported on the head.
	RequiredUnreported int
	Unavailable        bool
}

func runIsRed(r Run) bool {
	switch r.Conclusion {
	case "failure", "error", "cancelled", "timed_out", "startup_failure":
		return r.Status == "completed" || r.Status == ""
	}
	return false
}

func runIsPending(r Run) bool {
	if r.Status != "completed" && r.Status != "" {
		return true
	}
	return r.Conclusion == "action_required" || r.Conclusion == "pending" || r.Conclusion == "stale"
}

// PipelineFromChecks combines every check run and the combined commit status.
func PipelineFromChecks(in PipelineInput) Pipeline {
	if in.Unavailable {
		return PipelineUnavailable
	}
	red := in.CombinedStatus == "failure" || in.CombinedStatus == "error"
	pending := in.CombinedStatus == "pending" || in.RequiredUnreported > 0
	for _, r := range in.Runs {
		switch {
		case runIsRed(r):
			red = true
		case runIsPending(r):
			pending = true
		}
	}
	switch {
	case red:
		return PipelineRed
	case pending:
		return PipelinePending
	case len(in.Runs) == 0 && in.CombinedStatus == "":
		return PipelineNone
	}
	return PipelineGreen
}

// GitLabMR carries the GitLab merge-readiness fields the readings use.
type GitLabMR struct {
	DetailedMergeStatus string
	MergeStatus         string
	HasConflicts        *bool
}

// MergeableReading is the mergeable reading plus the condition an unmet status
// already explains (so it is not also reported as not_mergeable).
type MergeableReading struct {
	Mergeable Tri
	Explains  Condition
}

// GitLabMergeable maps detailed_merge_status to a reading. merge_status is the
// fallback only when the detailed field is absent.
func GitLabMergeable(mr GitLabMR) MergeableReading {
	switch mr.DetailedMergeStatus {
	case "":
		return gitlabFallback(mr.MergeStatus)
	case "mergeable":
		return MergeableReading{Mergeable: Met}
	case "not_approved", "approvals_syncing":
		return MergeableReading{Mergeable: Unmet, Explains: CondApprovals}
	case "discussions_not_resolved":
		return MergeableReading{Mergeable: Unmet, Explains: CondThreads}
	case "ci_must_pass", "ci_still_running":
		return MergeableReading{Mergeable: Unmet, Explains: CondPipeline}
	case "draft_status":
		return MergeableReading{Mergeable: Unmet, Explains: CondDraft}
	case "checking", "unchecked", "preparing", "locked":
		return MergeableReading{Mergeable: Unavailable}
	}
	return MergeableReading{Mergeable: Unmet}
}

func gitlabFallback(mergeStatus string) MergeableReading {
	switch mergeStatus {
	case "can_be_merged":
		return MergeableReading{Mergeable: Met}
	case "cannot_be_merged", "cannot_be_merged_recheck":
		return MergeableReading{Mergeable: Unmet}
	}
	return MergeableReading{Mergeable: Unavailable}
}

// ConflictState is the reading of a change's merge conflict.
type ConflictState int

const (
	// ConflictUnknown neither raises nor clears a conflict episode.
	ConflictUnknown ConflictState = iota
	ConflictPresent
	ConflictClear
)

// ConflictInput is the provider data the conflict reading uses.
type ConflictInput struct {
	Provider            string // github or gitlab
	Mergeable           *bool  // GitHub
	MergeableState      string // GitHub
	HasConflicts        *bool  // GitLab
	DetailedMergeStatus string // GitLab
	Unavailable         bool
}

// ReadConflict applies the per-provider conflict definition. Only an explicit
// provider value clears a conflict.
func ReadConflict(in ConflictInput) ConflictState {
	if in.Unavailable {
		return ConflictUnknown
	}
	if in.Provider == "gitlab" {
		return readGitLabConflict(in)
	}
	return readGitHubConflict(in)
}

func readGitHubConflict(in ConflictInput) ConflictState {
	if in.MergeableState == "dirty" || (in.Mergeable != nil && !*in.Mergeable) {
		return ConflictPresent
	}
	if in.Mergeable != nil && *in.Mergeable {
		return ConflictClear
	}
	return ConflictUnknown
}

func readGitLabConflict(in ConflictInput) ConflictState {
	if (in.HasConflicts != nil && *in.HasConflicts) || in.DetailedMergeStatus == "conflict" {
		return ConflictPresent
	}
	if in.HasConflicts != nil {
		return ConflictClear
	}
	return ConflictUnknown
}
