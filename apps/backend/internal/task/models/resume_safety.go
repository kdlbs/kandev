package models

// ProcessLiveness classifies whether the OS process backing an executors_running
// row is still alive, judged in a runtime-aware way. It is a low-dependency
// mirror of the runtime/lifecycle package's liveness probe result, so the
// orchestrator and task service can reason about a row's liveness without
// importing runtime/lifecycle (#1597 startup reconciliation,
// #1597 runtime-aware liveness).
type ProcessLiveness int

const (
	// ProcessLivenessUnknown means the row cannot be judged by a host-local
	// process check — a remote (SSH) runtime whose process lives on another host,
	// a containerized/docker runtime, or a local row whose liveness handle is not
	// yet populated. Callers MUST NOT treat Unknown as dead.
	ProcessLivenessUnknown ProcessLiveness = iota
	// ProcessLivenessAlive means the row's local process handle refers to a
	// process that currently exists on this host.
	ProcessLivenessAlive
	// ProcessLivenessDead means the row's local process handle refers to a
	// process that no longer exists on this host — the row is stale. Pruning it
	// is safe only subject to the resume-safety invariant (RowMustBePreserved),
	// not from liveness alone.
	ProcessLivenessDead
)

// IsResumableSessionState reports whether a session is in an open, resumable
// state whose executors_running row must be preserved even without a
// resume_token — the agent may still be reachable or the conversation continued
// on the next turn. The resumable states are Starting, Running, WaitingForInput,
// and Idle (office between-turns).
//
// Explicitly NOT resumable: Created (a never-started placeholder with no
// conversation to lose) and the terminal states (Completed/Failed/Cancelled).
// Those may be pruned when the row also carries no resume_token.
func IsResumableSessionState(state TaskSessionState) bool {
	switch state {
	case TaskSessionStateStarting, TaskSessionStateRunning,
		TaskSessionStateWaitingForInput, TaskSessionStateIdle:
		return true
	default:
		return false
	}
}

// RowMustBePreserved is the resume-safety deletion invariant: a row holding a
// resume_token, or backing a session in an open/resumable state, is repaired in
// place and never deleted. Only a row that is neither (a finished or
// never-started session with no resume_token) may be pruned.
//
// This is the single source of truth for "is it safe to delete this
// executors_running row" across every cleanup path (cancel cleanup, startup
// reconciliation, on-demand stale cleanup, task teardown). The guarantee is
// expressed as a deletion invariant rather than by duplicating resume_token into
// a second table, which would reintroduce the divergence risk this effort
// removes (#1597: resume_token must not be duplicated into a second table).
//
// One documented deviation: the lifecycle tier's stale-execution cleanup
// (lifecycle.Manager deleteExecutorRunning) gates on the resume_token alone,
// without the session-state half — a tokenless row has no agent-side session to
// resume, so preserving it there buys nothing. See the rationale on that
// function before "fixing" the divergence.
func RowMustBePreserved(running *ExecutorRunning, sessionState TaskSessionState) bool {
	if running == nil {
		return false
	}
	if running.ResumeToken != "" {
		return true
	}
	return IsResumableSessionState(sessionState)
}
