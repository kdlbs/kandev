//go:build !windows

package process

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// logChildProcesses logs all processes in the code-server's process group.
// Best-effort: failure does not affect shutdown.
func logChildProcesses(log *logger.Logger, pgid int) {
	out, err := exec.Command("pgrep", "-g", strconv.Itoa(pgid)).CombinedOutput()
	if err != nil {
		log.Debug("could not enumerate process group members", zap.Int("pgid", pgid), zap.Error(err))
		return
	}
	pids := strings.TrimSpace(string(out))
	if pids != "" {
		log.Info("code-server process group members", zap.Int("pgid", pgid), zap.String("pids", pids))
	}
}

// CleanupOrphanedCodeServers kills code-server processes left from a previous
// kandev session. On macOS there is no Pdeathsig, so if kandev crashes the
// code-server workers become orphaned (re-parented to launchd/init, ppid=1).
// Called once at startup as a safety net.
//
// Only targets processes whose direct parent is PID 1 (orphaned). Processes
// belonging to parallel kandev instances still have a live parent and are safe.
func CleanupOrphanedCodeServers(log *logger.Logger) {
	out, err := exec.Command("pgrep", "-f", "code-server.*--bind-addr").CombinedOutput()
	if err != nil {
		// pgrep returns exit 1 when no match — not an error.
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(line))
		if parseErr != nil || pid <= 0 {
			continue
		}
		ppid := getParentPID(pid)
		if ppid != 1 {
			// Parent is still alive — this code-server belongs to a running instance.
			continue
		}
		log.Warn("killing orphaned code-server process (ppid=1)", zap.Int("pid", pid))
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// getParentPID returns the parent PID of the given process, or -1 on error.
func getParentPID(pid int) int {
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).CombinedOutput()
	if err != nil {
		return -1
	}
	ppid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return -1
	}
	return ppid
}
