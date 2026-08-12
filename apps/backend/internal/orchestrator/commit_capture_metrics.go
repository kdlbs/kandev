package orchestrator

import "expvar"

// expvar maps published at package init under /debug/vars (dev profile, see
// apps/backend/AGENTS.md Observability), mirroring the office scheduler's
// routing_* counters (internal/office/scheduler/metrics_vars.go). Each
// observed commit increments "observed" plus exactly one of
// inserted/duplicate/failed, broken down by which write path saw it -
// commitCaptureTriggerLive (handleGitCommitCreated), ...Sweep (the
// agent-completed reconcile pass), or ...Archive (archive capture,
// unchanged as the final reconciliation pass). A trigger that stops
// incrementing while the others keep moving is the writer-health signal:
// one path silently stopped without the whole feature going dark.
var (
	commitCaptureObserved  = expvar.NewMap("commit_capture_observed")
	commitCaptureInserted  = expvar.NewMap("commit_capture_inserted")
	commitCaptureDuplicate = expvar.NewMap("commit_capture_duplicate")
	commitCaptureFailed    = expvar.NewMap("commit_capture_failed")
)

// commitCaptureTrigger identifies which write path observed a commit.
type commitCaptureTrigger string

const (
	commitCaptureTriggerLive    commitCaptureTrigger = "live"
	commitCaptureTriggerSweep   commitCaptureTrigger = "sweep"
	commitCaptureTriggerArchive commitCaptureTrigger = "archive"
)

func recordCommitCaptureObserved(trigger commitCaptureTrigger) {
	commitCaptureObserved.Add(string(trigger), 1)
}

func recordCommitCaptureInserted(trigger commitCaptureTrigger) {
	commitCaptureInserted.Add(string(trigger), 1)
}

func recordCommitCaptureDuplicate(trigger commitCaptureTrigger) {
	commitCaptureDuplicate.Add(string(trigger), 1)
}

func recordCommitCaptureFailed(trigger commitCaptureTrigger) {
	commitCaptureFailed.Add(string(trigger), 1)
}
