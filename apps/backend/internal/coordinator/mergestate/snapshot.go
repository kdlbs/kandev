package mergestate

import "time"

// ChangeState is the lifecycle state of a pull request or merge request.
type ChangeState string

// Change lifecycle states.
const (
	StateOpen   ChangeState = "open"
	StateClosed ChangeState = "closed"
	StateMerged ChangeState = "merged"
)

// Snapshot is one provider read of a change, normalized across providers. A
// field that could not be read carries Unavailable (or a negative count); it
// is never presented as passing.
type Snapshot struct {
	Key          ChangeKey
	State        ChangeState
	Draft        Tri // Met means not a draft
	TargetBranch string
	HeadSHA      string
	// MergedHeadSHA is the head the provider merged, when State is merged.
	MergedHeadSHA string
	MergeCommit   string
	Pipeline      Pipeline
	Mergeable     MergeableReading
	Conflict      ConflictState
	// UnresolvedThreads is negative when it could not be read.
	UnresolvedThreads int
	Approvals         Tri
	// Behind is negative when it could not be read.
	Behind int
	// InProgress is set when the provider reports a merge in flight (GitLab locked).
	InProgress bool
}

// Observation converts the snapshot to the input of the episode state machine.
func (s Snapshot) Observation(startedAt time.Time) Observation {
	return Observation{
		StartedAt: startedAt, HeadSHA: s.HeadSHA, Pipeline: s.Pipeline,
		Conflict: s.Conflict, UnresolvedThreads: s.UnresolvedThreads,
	}
}
