package mergestate

import "testing"

func TestPipelineFromChecks(t *testing.T) {
	cases := []struct {
		name string
		in   PipelineInput
		want Pipeline
	}{
		{"no checks", PipelineInput{}, PipelineNone},
		{"all success", PipelineInput{Runs: []Run{{Status: "completed", Conclusion: "success"}}}, PipelineGreen},
		{"skipped and neutral are success", PipelineInput{Runs: []Run{{Status: "completed", Conclusion: "skipped"}, {Status: "completed", Conclusion: "neutral"}}}, PipelineGreen},
		{"failure is red", PipelineInput{Runs: []Run{{Status: "completed", Conclusion: "failure"}, {Status: "in_progress"}}}, PipelineRed},
		{"cancelled is red", PipelineInput{Runs: []Run{{Status: "completed", Conclusion: "cancelled"}}}, PipelineRed},
		{"timed out is red", PipelineInput{Runs: []Run{{Status: "completed", Conclusion: "timed_out"}}}, PipelineRed},
		{"queued is pending", PipelineInput{Runs: []Run{{Status: "queued"}}}, PipelinePending},
		{"running with success is pending", PipelineInput{Runs: []Run{{Status: "in_progress"}, {Status: "completed", Conclusion: "success"}}}, PipelinePending},
		{"commit status failure is red", PipelineInput{CombinedStatus: "failure"}, PipelineRed},
		{"commit status error is red", PipelineInput{CombinedStatus: "error"}, PipelineRed},
		{"commit status pending", PipelineInput{CombinedStatus: "pending"}, PipelinePending},
		{"commit status success alone is green", PipelineInput{CombinedStatus: "success"}, PipelineGreen},
		{"unreported required context is pending", PipelineInput{Runs: []Run{{Status: "completed", Conclusion: "success"}}, RequiredUnreported: 1}, PipelinePending},
		{"unreported required with nothing else is pending", PipelineInput{RequiredUnreported: 2}, PipelinePending},
		{"unavailable wins", PipelineInput{Unavailable: true, Runs: []Run{{Status: "completed", Conclusion: "success"}}}, PipelineUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PipelineFromChecks(c.in); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestPipelineMetOnlyWhenGreen(t *testing.T) {
	for p, want := range map[Pipeline]Tri{
		PipelineGreen: Met, PipelineRed: Unmet, PipelinePending: Unmet, PipelineNone: Unmet, PipelineUnavailable: Unavailable,
	} {
		if got := p.Tri(); got != want {
			t.Fatalf("%q: got %v want %v", p, got, want)
		}
	}
}

func TestGitLabDetailedStatusMapping(t *testing.T) {
	cases := []struct {
		status    string
		mergeable Tri
		explains  Condition
	}{
		{"mergeable", Met, ""},
		{"not_approved", Unmet, CondApprovals},
		{"approvals_syncing", Unmet, CondApprovals},
		{"discussions_not_resolved", Unmet, CondThreads},
		{"ci_must_pass", Unmet, CondPipeline},
		{"ci_still_running", Unmet, CondPipeline},
		{"draft_status", Unmet, CondDraft},
		{"checking", Unavailable, ""},
		{"unchecked", Unavailable, ""},
		{"preparing", Unavailable, ""},
		{"locked", Unavailable, ""},
		{"need_rebase", Unmet, ""},
		{"blocked_status", Unmet, ""},
		{"conflict", Unmet, ""},
	}
	for _, c := range cases {
		t.Run(c.status, func(t *testing.T) {
			got := GitLabMergeable(GitLabMR{DetailedMergeStatus: c.status})
			if got.Mergeable != c.mergeable || got.Explains != c.explains {
				t.Fatalf("got %+v want mergeable=%v explains=%q", got, c.mergeable, c.explains)
			}
		})
	}
}

func TestGitLabMergeableFallsBackOnlyWhenDetailedAbsent(t *testing.T) {
	if got := GitLabMergeable(GitLabMR{MergeStatus: "can_be_merged"}); got.Mergeable != Met {
		t.Fatalf("fallback can_be_merged: %+v", got)
	}
	if got := GitLabMergeable(GitLabMR{MergeStatus: "cannot_be_merged"}); got.Mergeable != Unmet {
		t.Fatalf("fallback cannot_be_merged: %+v", got)
	}
	if got := GitLabMergeable(GitLabMR{DetailedMergeStatus: "need_rebase", MergeStatus: "can_be_merged"}); got.Mergeable != Unmet {
		t.Fatalf("detailed must win over fallback: %+v", got)
	}
	if got := GitLabMergeable(GitLabMR{}); got.Mergeable != Unavailable {
		t.Fatalf("nothing known: %+v", got)
	}
}

func TestConflictReading(t *testing.T) {
	tr, fl := true, false
	cases := []struct {
		name string
		in   ConflictInput
		want ConflictState
	}{
		{"github mergeable false", ConflictInput{Provider: "github", Mergeable: &fl}, ConflictPresent},
		{"github dirty", ConflictInput{Provider: "github", MergeableState: "dirty"}, ConflictPresent},
		{"github explicit true clears", ConflictInput{Provider: "github", Mergeable: &tr, MergeableState: "clean"}, ConflictClear},
		{"github unknown is unknown", ConflictInput{Provider: "github", MergeableState: "unknown"}, ConflictUnknown},
		{"github blocked is not a conflict but not clearing", ConflictInput{Provider: "github", MergeableState: "blocked"}, ConflictUnknown},
		{"gitlab has_conflicts true", ConflictInput{Provider: "gitlab", HasConflicts: &tr}, ConflictPresent},
		{"gitlab detailed conflict", ConflictInput{Provider: "gitlab", DetailedMergeStatus: "conflict"}, ConflictPresent},
		{"gitlab has_conflicts false clears", ConflictInput{Provider: "gitlab", HasConflicts: &fl}, ConflictClear},
		{"gitlab need_rebase is not a conflict", ConflictInput{Provider: "gitlab", DetailedMergeStatus: "need_rebase"}, ConflictUnknown},
		{"unavailable", ConflictInput{Provider: "github", Unavailable: true, Mergeable: &fl}, ConflictUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ReadConflict(c.in); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}
