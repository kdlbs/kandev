package mergestate

const reasonNotMergeable = "not_mergeable"

// Outcome is what a recheck means for the attempt that ran it.
type Outcome int

const (
	// Proceed means every condition is met.
	Proceed Outcome = iota
	// Wait means a waiting reason is unmet; an automatic proposal stays pending.
	Wait
	// Refuse means the proposal settles refused with the reason.
	Refuse
	// ManagerOnly means Pause stopped an automatic attempt; a manager decides.
	ManagerOnly
)

// Decision is the first failing check of a recheck, in the refusal-reason order.
type Decision struct {
	Reason  string
	Outcome Outcome
}

// ForAttempt applies the attempt kind: a manager's approval refuses on any
// unmet condition, including waiting ones.
func (d Decision) ForAttempt(automatic bool) Outcome {
	if !automatic && d.Outcome == Wait {
		return Refuse
	}
	return d.Outcome
}

// RecheckInput is a fresh, uncached view used immediately before dispatch.
type RecheckInput struct {
	Authority       bool // flags, policy and execution principal allow the merge
	Automatic       bool
	Paused          bool
	RepoAvailable   Tri
	Closed          bool
	MergedElsewhere bool
	ExpectedHead    string
	CurrentHead     string
	Pipeline        Pipeline
	Mergeable       MergeableReading
	Conflict        ConflictState
	NotDraft        Tri
	Approvals       Tri
	Threads         Tri
	Predecessors    Tri
	Method          Tri
}

func unavailable(c Condition) Decision {
	return Decision{Reason: "unavailable:" + string(c), Outcome: Wait}
}

// Recheck returns the first unmet or unavailable check in the closed order:
// authority, paused, repository, closed, merged elsewhere, head, pipeline,
// mergeable and conflict, draft, approvals, threads, predecessors, method.
func Recheck(in RecheckInput) Decision {
	for _, step := range []func(RecheckInput) (Decision, bool){
		checkEntry, checkChange, checkPipeline, checkMergeable,
	} {
		if d, hit := step(in); hit {
			return d
		}
	}
	return checkRemaining(in)
}

func checkEntry(in RecheckInput) (Decision, bool) {
	switch {
	case !in.Authority:
		return Decision{Reason: "authority", Outcome: Refuse}, true
	case in.Automatic && in.Paused:
		return Decision{Reason: "paused", Outcome: ManagerOnly}, true
	case in.RepoAvailable != Met:
		return Decision{Reason: "repository_unavailable", Outcome: Refuse}, true
	}
	return Decision{}, false
}

func checkChange(in RecheckInput) (Decision, bool) {
	switch {
	case in.Closed:
		return Decision{Reason: "closed", Outcome: Refuse}, true
	case in.MergedElsewhere:
		return Decision{Reason: "merged_elsewhere", Outcome: Refuse}, true
	case in.CurrentHead == "":
		return unavailable(CondHead), true
	case in.CurrentHead != in.ExpectedHead:
		return Decision{Reason: "stale_head", Outcome: Refuse}, true
	}
	return Decision{}, false
}

func checkPipeline(in RecheckInput) (Decision, bool) {
	switch in.Pipeline {
	case PipelineGreen:
		return Decision{}, false
	case PipelineUnavailable:
		return unavailable(CondPipeline), true
	case PipelineRed:
		return Decision{Reason: "pipeline", Outcome: Refuse}, true
	}
	return Decision{Reason: "pipeline", Outcome: Wait}, true
}

func checkMergeable(in RecheckInput) (Decision, bool) {
	if in.Conflict == ConflictPresent {
		return Decision{Reason: reasonNotMergeable, Outcome: Refuse}, true
	}
	switch in.Mergeable.Mergeable {
	case Unavailable:
		return unavailable(CondMergeable), true
	case Unmet:
		if in.Mergeable.Explains == "" {
			return Decision{Reason: reasonNotMergeable, Outcome: Refuse}, true
		}
	}
	return Decision{}, false
}

func checkRemaining(in RecheckInput) Decision {
	for _, c := range []struct {
		cond   Condition
		tri    Tri
		refuse bool
	}{
		{CondDraft, in.NotDraft, false},
		{CondApprovals, in.Approvals, false},
		{CondThreads, in.Threads, false},
		{CondPredecessor, in.Predecessors, false},
		{CondMethod, in.Method, true},
	} {
		switch c.tri {
		case Met:
			continue
		case Unavailable:
			return unavailable(c.cond)
		}
		if c.refuse {
			return Decision{Reason: string(c.cond), Outcome: Refuse}
		}
		return Decision{Reason: string(c.cond), Outcome: Wait}
	}
	if in.Mergeable.Mergeable != Met {
		return Decision{Reason: reasonNotMergeable, Outcome: Refuse}
	}
	return Decision{Outcome: Proceed}
}
