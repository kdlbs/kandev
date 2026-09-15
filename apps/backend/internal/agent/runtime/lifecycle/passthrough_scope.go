package lifecycle

import "github.com/kandev/kandev/internal/task/models"

// AC-EXECUTORS-SURVIVAL-005.3 (design 02 "Passthrough scope"): a passthrough
// session shall behave as if the capability were disabled for that session.
// The agent runs on a terminal owned by *this* backend process, so it dies
// with the backend no matter what the control server does -- there is nothing
// to survive. The two obligations that follow are implemented here and used
// by the two paths that would otherwise treat such a session as survivable:
//
//   - never detached: StopAgentWithReason must take the terminating path for
//     a passthrough execution even on a graceful backend shutdown, so the
//     runtime instance is stopped, agent.stopped is published, and the
//     executors_running row stops claiming a live agent. Detaching instead
//     leaves a record asserting a running agent that no later pass repairs
//     (AC-EXECUTORS-SURVIVAL-004.5).
//   - never re-tracked: a passthrough session still owns a real agentctl
//     instance (it is created by CreateInstance like any other execution, or
//     promoted from a workspace-only one), so that instance genuinely does
//     survive on the detached control server and would correlate to the
//     session's recovery-inventory record. Re-tracking it would publish the
//     session as running while its PTY agent is gone.

// isPassthroughExecution reports whether a tracked execution is a passthrough
// (PTY/TUI) one. Mirrors the existing in-memory idiom used elsewhere in this
// package (IsAgentReadyForPrompt, RecoverAgentPromptStream): the flag is set
// at launch and at workspace-execution promotion, and PassthroughProcessID is
// set once the terminal process exists, so either being present is the signal.
// In-memory state is authoritative here because the caller holds a live,
// tracked execution -- unlike startup, where nothing is tracked yet and the
// durable session record is the only source (SessionsToGuard).
func isPassthroughExecution(execution *AgentExecution) bool {
	return execution != nil && (execution.IsPassthrough || execution.PassthroughProcessID != "")
}

// recoverableRecords drops the confirmed-passthrough records from a recovery
// pass's inventory so re-tracking never reaches them. guarded is the set
// SessionsToGuard already computed for this pass -- "every named session minus
// the confirmed-passthrough ones" -- so a session whose passthrough mode could
// not be read stays both guarded and recoverable, which is the fail-safe
// direction AC-EXECUTORS-SURVIVAL-005.3 requires ("when that mode cannot be
// read for a record, the session shall be guarded rather than excluded").
//
// A record with no session identity is kept rather than dropped: it is not a
// session this pass could have classified either way, and correlation already
// skips it. The only records this removes are the ones confirmed passthrough.
func recoverableRecords(records []*models.ExecutorRunning, guarded []string) []*models.ExecutorRunning {
	guardedSet := make(map[string]struct{}, len(guarded))
	for _, sessionID := range guarded {
		guardedSet[sessionID] = struct{}{}
	}
	recoverable := make([]*models.ExecutorRunning, 0, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		if record.SessionID != "" {
			if _, ok := guardedSet[record.SessionID]; !ok {
				continue
			}
		}
		recoverable = append(recoverable, record)
	}
	return recoverable
}
