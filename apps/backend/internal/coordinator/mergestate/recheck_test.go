package mergestate

import "testing"

func okInput() RecheckInput {
	return RecheckInput{
		Authority: true, RepoAvailable: Met, ExpectedHead: "h", CurrentHead: "h",
		Pipeline: PipelineGreen, Mergeable: MergeableReading{Mergeable: Met}, Conflict: ConflictClear,
		NotDraft: Met, Approvals: Met, Threads: Met, Predecessors: Met, Method: Met,
	}
}

func TestRecheckOrderedReasons(t *testing.T) {
	cases := []struct {
		name   string
		mut    func(*RecheckInput)
		reason string
		out    Outcome
	}{
		{"all met", func(*RecheckInput) {}, "", Proceed},
		{"authority", func(i *RecheckInput) { i.Authority = false }, "authority", Refuse},
		{"authority beats paused", func(i *RecheckInput) { i.Authority = false; i.Automatic, i.Paused = true, true }, "authority", Refuse},
		{"paused automatic", func(i *RecheckInput) { i.Automatic, i.Paused = true, true }, "paused", ManagerOnly},
		{"paused ignored for manager", func(i *RecheckInput) { i.Paused = true }, "", Proceed},
		{"repo unavailable", func(i *RecheckInput) { i.RepoAvailable = Unavailable }, "repository_unavailable", Refuse},
		{"closed", func(i *RecheckInput) { i.Closed = true }, "closed", Refuse},
		{"merged elsewhere", func(i *RecheckInput) { i.MergedElsewhere = true }, "merged_elsewhere", Refuse},
		{"stale head", func(i *RecheckInput) { i.CurrentHead = "x" }, "stale_head", Refuse},
		{"empty head is unavailable", func(i *RecheckInput) { i.CurrentHead = "" }, "unavailable:head", Wait},
		{"pipeline red", func(i *RecheckInput) { i.Pipeline = PipelineRed }, "pipeline", Refuse},
		{"pipeline pending waits", func(i *RecheckInput) { i.Pipeline = PipelinePending }, "pipeline", Wait},
		{"pipeline none waits", func(i *RecheckInput) { i.Pipeline = PipelineNone }, "pipeline", Wait},
		{"pipeline unavailable", func(i *RecheckInput) { i.Pipeline = PipelineUnavailable }, "unavailable:pipeline", Wait},
		{"conflict", func(i *RecheckInput) { i.Conflict = ConflictPresent }, "not_mergeable", Refuse},
		{"not mergeable", func(i *RecheckInput) { i.Mergeable = MergeableReading{Mergeable: Unmet} }, "not_mergeable", Refuse},
		{"mergeable unavailable", func(i *RecheckInput) { i.Mergeable = MergeableReading{Mergeable: Unavailable} }, "unavailable:mergeable", Wait},
		{"draft waits", func(i *RecheckInput) { i.NotDraft = Unmet }, "draft", Wait},
		{"approvals wait", func(i *RecheckInput) { i.Approvals = Unmet }, "approvals", Wait},
		{"approvals unavailable", func(i *RecheckInput) { i.Approvals = Unavailable }, "unavailable:approvals", Wait},
		{"threads wait", func(i *RecheckInput) { i.Threads = Unmet }, "threads", Wait},
		{"predecessor waits", func(i *RecheckInput) { i.Predecessors = Unmet }, "predecessor", Wait},
		{"predecessor unavailable", func(i *RecheckInput) { i.Predecessors = Unavailable }, "unavailable:predecessor", Wait},
		{"method refuses", func(i *RecheckInput) { i.Method = Unmet }, "method", Refuse},
		{"method unavailable", func(i *RecheckInput) { i.Method = Unavailable }, "unavailable:method", Wait},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := okInput()
			c.mut(&in)
			got := Recheck(in)
			if got.Reason != c.reason || got.Outcome != c.out {
				t.Fatalf("got %+v want reason=%q outcome=%v", got, c.reason, c.out)
			}
		})
	}
}

func TestStatusExplainedByLaterConditionIsNotNotMergeable(t *testing.T) {
	in := okInput()
	in.Mergeable = MergeableReading{Mergeable: Unmet, Explains: CondApprovals}
	in.Approvals = Unmet
	if got := Recheck(in); got.Reason != "approvals" || got.Outcome != Wait {
		t.Fatalf("got %+v", got)
	}
	in = okInput()
	in.Mergeable = MergeableReading{Mergeable: Unmet, Explains: CondThreads}
	in.Threads = Unmet
	if got := Recheck(in); got.Reason != "threads" {
		t.Fatalf("got %+v", got)
	}
}

func TestManagerApprovalRefusesOnWaitingReason(t *testing.T) {
	in := okInput()
	in.Approvals = Unmet
	d := Recheck(in)
	if d.Outcome != Wait {
		t.Fatalf("got %+v", d)
	}
	if got := d.ForAttempt(false); got != Refuse {
		t.Fatalf("manager attempt: %v", got)
	}
	if got := d.ForAttempt(true); got != Wait {
		t.Fatalf("automatic attempt: %v", got)
	}
	if got := (Decision{Reason: "paused", Outcome: ManagerOnly}).ForAttempt(true); got != ManagerOnly {
		t.Fatalf("paused automatic: %v", got)
	}
}

func TestExplainedNotMergeableNeverProceeds(t *testing.T) {
	in := okInput()
	in.Mergeable = MergeableReading{Mergeable: Unmet, Explains: CondApprovals}
	if got := Recheck(in); got.Outcome != Refuse || got.Reason != "not_mergeable" {
		t.Fatalf("provider says unmergeable but own reads pass: %+v", got)
	}
}
