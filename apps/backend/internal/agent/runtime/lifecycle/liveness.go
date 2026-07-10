package lifecycle

import (
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// RowProcessLiveness reports the liveness of the OS process backing a row,
// branching on the row's runtime so that a host-local process check is NEVER
// applied to a remote row (#1597: executors_running.pid is the agentctl PID
// on the REMOTE host for SSH rows — never overload it with a local pid).
//
// It returns models.ProcessLiveness — a low-dependency type the orchestrator and
// task service can consume without importing runtime/lifecycle — so cleanup and
// reconciliation paths can classify a row through a small adapter method while
// the actual host-local probe stays in this platform-split package.
//
//   - local/standalone: judged by local_pid on this host.
//   - SSH/remote: Unknown here. The agent process lives on another host and its
//     liveness is owned by the SSH executor's remote `kill -0` path; the SSH
//     `pid` column is a REMOTE pid and must never be probed with a local
//     os.FindProcess.
//   - docker / anything else: Unknown for now (out of scope for this batch).
func RowProcessLiveness(row *models.ExecutorRunning) models.ProcessLiveness {
	if row == nil {
		return models.ProcessLivenessUnknown
	}
	if !isLocalRuntime(row.Runtime) {
		// Never run a local-process existence check against a remote row.
		return models.ProcessLivenessUnknown
	}
	if row.LocalPID <= 0 {
		// A local row without a populated handle can't be judged — don't call it
		// dead (that would risk pruning a row before its handle lands).
		return models.ProcessLivenessUnknown
	}
	if isLocalPIDAlive(row.LocalPID) {
		return models.ProcessLivenessAlive
	}
	return models.ProcessLivenessDead
}

// isLocalRuntime reports whether a row's runtime runs its agent process on THIS
// host, so a local os.FindProcess check is meaningful. Standalone is the only
// local runtime; SSH/docker/remote-docker/sprites run the process elsewhere.
// An empty/unknown runtime is treated as non-local (Unknown), never probed.
func isLocalRuntime(runtime agentruntime.Runtime) bool {
	return runtime == agentruntime.RuntimeStandalone
}
