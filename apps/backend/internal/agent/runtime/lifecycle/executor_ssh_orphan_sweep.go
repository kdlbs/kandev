package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// sshOrphanSweepStore is the narrow read surface the orphan sweep needs to
// classify a discovered remote agentctl process. It mirrors the
// persistedSSHCleanupStore pattern: small, structurally satisfied by
// *sqlite.Repository, and independent of the repository implementation.
type sshOrphanSweepStore interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	ListExecutorsRunningByTaskID(ctx context.Context, taskID string) ([]*models.ExecutorRunning, error)
}

// sshOrphanProcessRecord is one remote agentctl process discovered by the
// inventory command, after Go-side parsing of its --workdir argument.
type sshOrphanProcessRecord struct {
	PID     int
	PPID    int
	TaskDir string // e.g. "task-<task-id>"
	TaskID  string
}

// sshOrphanInventory is the parsed result of one inventory command run.
type sshOrphanInventory struct {
	Processes []sshOrphanProcessRecord
	// pidfilesByTask maps a task dir name to the pid->sessionID claims read
	// from that task's session pidfiles.
	pidfilesByTask map[string]map[int]string
	// taintedTasks holds task dir names whose pidfile read produced
	// unparsable or missing content, so ownership for any process under
	// that task dir cannot be proven.
	taintedTasks map[string]bool
}

// sshOrphanVerdict is the sweep's decision for one discovered process.
type sshOrphanVerdict int

const (
	sshOrphanPreserve sshOrphanVerdict = iota
	sshOrphanStop
)

// sshOrphanDecision is the outcome of evaluating one process against
// AC-EXECUTORS-SSH-EXECUTOR-001.14 and .15. SessionID is set whenever a
// pidfile attributed the process to a session, even when the verdict is
// preserve, so the caller can log which claim drove the decision.
type sshOrphanDecision struct {
	Verdict   sshOrphanVerdict
	Reason    string
	SessionID string
}

// sshOrphanTaskContext is the per-task data the decision function needs,
// cached once per sweep so processes that share a task do not repeat reads.
// A nil Task means the task is unknown to this Kandev database.
type sshOrphanTaskContext struct {
	Task     *models.Task
	Sessions []*models.TaskSession
	Running  []*models.ExecutorRunning
}

// sshOrphanSweepReport summarizes one sweep run for AC-EXECUTORS-SSH-EXECUTOR-001.16's
// reporting requirement. No environment values or credentials are ever
// carried in it.
type sshOrphanSweepReport struct {
	ExecutorID string
	Found      int
	Stopped    int
	Preserved  int
	Failed     int
}

// sweepSSHExecutorOrphans inventories remote agentctl processes under
// executor's configured workdir root, classifies each against Kandev's task
// and session state, and stops the ones that are orphaned. It never removes
// the task directory itself; only a pidfile-attributed stop reclaims the
// session runtime directory (AC-EXECUTORS-SSH-EXECUTOR-001.13-.16).
func sweepSSHExecutorOrphans(
	ctx context.Context,
	client *ssh.Client,
	store sshOrphanSweepStore,
	executorID string,
	config map[string]string,
	log *logger.Logger,
) (sshOrphanSweepReport, error) {
	report := sshOrphanSweepReport{ExecutorID: executorID}

	root := strings.TrimSpace(config["ssh_workdir_root"])
	if root == "" {
		root = sshDefaultWorkdir
	}
	resolvedRoot, err := expandRemoteHome(ctx, client, root)
	if err != nil {
		return report, fmt.Errorf("ssh orphan sweep: resolve workdir root: %w", err)
	}

	stdout, _, err := runSSHCommand(ctx, client, sshOrphanInventoryCommand(resolvedRoot))
	if err != nil {
		return report, fmt.Errorf("ssh orphan sweep: inventory: %w", err)
	}
	inventory := parseSSHOrphanInventory(stdout, resolvedRoot)
	report.Found = len(inventory.Processes)

	taskContexts := map[string]*sshOrphanTaskContext{}
	for _, proc := range inventory.Processes {
		taskCtx, err := loadSSHOrphanTaskContext(ctx, store, taskContexts, proc.TaskID)
		if err != nil {
			report.Failed++
			log.Warn("ssh orphan sweep: load task context failed",
				zap.String("executor_id", executorID), zap.Error(err))
			continue
		}

		sessionID, claimed := sshOrphanAttributeSession(inventory, proc)
		tainted := inventory.taintedTasks[proc.TaskDir]
		decision := decideSSHOrphanProcess(proc.PID, sessionID, claimed, tainted, taskCtx)

		if decision.Verdict == sshOrphanPreserve {
			report.Preserved++
			continue
		}

		if err := stopSSHOrphanProcess(ctx, client, resolvedRoot, proc, claimed, sessionID); err != nil {
			report.Failed++
			log.Warn("ssh orphan sweep: stop failed",
				zap.String("executor_id", executorID), zap.Error(err))
			continue
		}
		report.Stopped++
	}

	log.Info("ssh orphan sweep completed",
		zap.String("executor_id", executorID),
		zap.Int("found", report.Found),
		zap.Int("stopped", report.Stopped),
		zap.Int("preserved", report.Preserved),
		zap.Int("failed", report.Failed))
	return report, nil
}

// loadSSHOrphanTaskContext reads a task, its sessions, and its
// executors_running rows once per sweep, caching by task ID. A task unknown
// to Kandev is cached as a context with a nil Task rather than an error, so
// the decision function can preserve on unknown-task without special-casing
// the repository's not-found sentinel.
func loadSSHOrphanTaskContext(
	ctx context.Context,
	store sshOrphanSweepStore,
	cache map[string]*sshOrphanTaskContext,
	taskID string,
) (*sshOrphanTaskContext, error) {
	if cached, ok := cache[taskID]; ok {
		return cached, nil
	}
	task, err := store.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			taskCtx := &sshOrphanTaskContext{}
			cache[taskID] = taskCtx
			return taskCtx, nil
		}
		return nil, err
	}
	sessions, err := store.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	running, err := store.ListExecutorsRunningByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	taskCtx := &sshOrphanTaskContext{Task: task, Sessions: sessions, Running: running}
	cache[taskID] = taskCtx
	return taskCtx, nil
}

// decideSSHOrphanProcess implements AC-EXECUTORS-SSH-EXECUTOR-001.14 and .15.
// Every branch that cannot prove the process is safe to stop preserves it:
// an unknown task, a live executors_running row for the pid, a pidfile claim
// naming a session the task does not have, a non-terminal attributed
// session, or a non-terminal session among an unclaimed process's task.
func decideSSHOrphanProcess(
	pid int,
	claimedSessionID string,
	claimed bool,
	tainted bool,
	taskCtx *sshOrphanTaskContext,
) sshOrphanDecision {
	if tainted {
		return sshOrphanDecision{
			Verdict: sshOrphanPreserve,
			Reason:  "pidfile read for this task was unreadable or invalid",
		}
	}
	if taskCtx.Task == nil {
		return sshOrphanDecision{
			Verdict: sshOrphanPreserve,
			Reason:  "task is unknown to this Kandev database",
		}
	}
	if sshOrphanRunningRowBlocksStop(pid, taskCtx.Sessions, taskCtx.Running) {
		return sshOrphanDecision{
			Verdict: sshOrphanPreserve,
			Reason:  "an executors_running row for a non-terminal session names this pid",
		}
	}
	if claimed {
		session := findSSHOrphanSession(taskCtx.Sessions, claimedSessionID)
		if session == nil {
			return sshOrphanDecision{
				Verdict:   sshOrphanPreserve,
				Reason:    "pidfile names a session this task does not have",
				SessionID: claimedSessionID,
			}
		}
		if !isTerminalSSHOrphanSessionState(session.State) {
			return sshOrphanDecision{
				Verdict:   sshOrphanPreserve,
				Reason:    "attributed session is not terminal",
				SessionID: claimedSessionID,
			}
		}
		return sshOrphanDecision{
			Verdict:   sshOrphanStop,
			Reason:    "attributed session is terminal",
			SessionID: claimedSessionID,
		}
	}
	if taskCtx.Task.ArchivedAt != nil {
		return sshOrphanDecision{Verdict: sshOrphanStop, Reason: "unclaimed process's task is archived"}
	}
	if allSSHOrphanSessionsTerminal(taskCtx.Sessions) {
		return sshOrphanDecision{Verdict: sshOrphanStop, Reason: "unclaimed process's task has only terminal sessions"}
	}
	return sshOrphanDecision{Verdict: sshOrphanPreserve, Reason: "unclaimed process's task has a non-terminal session"}
}

func isTerminalSSHOrphanSessionState(state models.TaskSessionState) bool {
	switch state {
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return true
	default:
		return false
	}
}

func allSSHOrphanSessionsTerminal(sessions []*models.TaskSession) bool {
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if !isTerminalSSHOrphanSessionState(session.State) {
			return false
		}
	}
	return true
}

func findSSHOrphanSession(sessions []*models.TaskSession, id string) *models.TaskSession {
	for _, session := range sessions {
		if session != nil && session.ID == id {
			return session
		}
	}
	return nil
}

// sshOrphanRunningRowBlocksStop implements the executors_running safety net
// in AC-EXECUTORS-SSH-EXECUTOR-001.15: a live row naming this pid blocks the
// stop regardless of the pidfile/task attribution above, unless the row's
// own session is provably terminal. A row whose session cannot be found is
// treated the same as a non-terminal one — ownership is unproven either way.
func sshOrphanRunningRowBlocksStop(pid int, sessions []*models.TaskSession, rows []*models.ExecutorRunning) bool {
	for _, row := range rows {
		if row == nil || row.PID != pid {
			continue
		}
		session := findSSHOrphanSession(sessions, row.SessionID)
		if session == nil || !isTerminalSSHOrphanSessionState(session.State) {
			return true
		}
	}
	return false
}

// sshOrphanAttributeSession implements the ownership rule: a pidfile that
// names the pid attributes the process to that session; otherwise the
// process is attributed to its task only.
func sshOrphanAttributeSession(inv sshOrphanInventory, proc sshOrphanProcessRecord) (sessionID string, claimed bool) {
	claims, ok := inv.pidfilesByTask[proc.TaskDir]
	if !ok {
		return "", false
	}
	sessionID, claimed = claims[proc.PID]
	return sessionID, claimed
}

// sshOrphanInventoryCommand lists every process under resolvedRoot/tasks
// whose command line names the agentctl binary and a --workdir under a
// task-<id> directory, then reads every session pidfile under the same
// root, all in one round trip so the process table and the pidfiles reflect
// roughly the same instant. It deliberately runs without `set -e`: a single
// unreadable pidfile must not abort the rest of the survey, and callers
// treat unparsable pidfile content as a tainted task rather than a script
// failure.
//
//nolint:dupword // shell branches contain repeated `done` tokens.
func sshOrphanInventoryCommand(resolvedRoot string) string {
	root := strings.TrimSuffix(resolvedRoot, "/") + "/tasks"
	return "ROOT=" + shellQuote(root) + `
ps -eo pid=,ppid=,command= 2>/dev/null | while read -r pid ppid command; do
  case "$command" in
    *agentctl*"--workdir "*"$ROOT/task-"*)
      printf 'PROC\t%s\t%s\t%s\n' "$pid" "$ppid" "$command"
      ;;
  esac
done
for taskDir in "$ROOT"/task-*/; do
  [ -d "$taskDir" ] || continue
  taskName=$(basename "$taskDir")
  for pidFile in "$taskDir".kandev/sessions/*/agentctl.pid; do
    [ -f "$pidFile" ] || continue
    sessionDir=$(dirname "$pidFile")
    sessionID=$(basename "$sessionDir")
    pidValue=$(cat "$pidFile" 2>/dev/null | tr -d '[:space:]')
    printf 'PIDFILE\t%s\t%s\t%s\n' "$taskName" "$sessionID" "$pidValue"
  done
done
`
}

// parseSSHOrphanInventory parses sshOrphanInventoryCommand's output. Any line
// that does not match the expected shape is skipped rather than treated as
// an error — a corrupt PROC line yields one fewer discovered process, never
// a false attribution.
func parseSSHOrphanInventory(output, resolvedRoot string) sshOrphanInventory {
	inv := sshOrphanInventory{
		pidfilesByTask: map[string]map[int]string{},
		taintedTasks:   map[string]bool{},
	}
	root := strings.TrimSuffix(resolvedRoot, "/") + "/tasks"
	for _, line := range strings.Split(output, "\n") {
		fields := strings.SplitN(line, "\t", 4)
		switch fields[0] {
		case "PROC":
			parseSSHOrphanProcLine(&inv, root, fields)
		case "PIDFILE":
			parseSSHOrphanPidfileLine(&inv, fields)
		}
	}
	return inv
}

func parseSSHOrphanProcLine(inv *sshOrphanInventory, root string, fields []string) {
	if len(fields) != 4 {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil || pid <= 0 {
		return
	}
	ppid, _ := strconv.Atoi(strings.TrimSpace(fields[2]))
	command := fields[3]
	// Re-check the "agentctl" substring here rather than trusting the remote
	// shell's own case-pattern filter alone — the same defense-in-depth this
	// package already applies in remoteAgentctlCommandLineMatches.
	if !strings.Contains(command, "agentctl") {
		return
	}
	workdir, ok := remoteCommandLineFlagValue(command, "--workdir")
	if !ok {
		return
	}
	taskDir, taskID, ok := sshOrphanTaskIDFromWorkdir(root, workdir)
	if !ok {
		return
	}
	inv.Processes = append(inv.Processes, sshOrphanProcessRecord{
		PID: pid, PPID: ppid, TaskDir: taskDir, TaskID: taskID,
	})
}

func parseSSHOrphanPidfileLine(inv *sshOrphanInventory, fields []string) {
	if len(fields) != 4 {
		return
	}
	taskDir := strings.TrimSpace(fields[1])
	sessionID := strings.TrimSpace(fields[2])
	if taskDir == "" || sessionID == "" {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(fields[3]))
	if err != nil || pid <= 0 {
		inv.taintedTasks[taskDir] = true
		return
	}
	claims := inv.pidfilesByTask[taskDir]
	if claims == nil {
		claims = map[int]string{}
		inv.pidfilesByTask[taskDir] = claims
	}
	claims[pid] = sessionID
}

// remoteCommandLineFlagValue extracts the value of flag from a `ps` command
// line, ending the value at the next whitespace. Unlike
// commandLineHasFlagValue (which only confirms a known value is present),
// this is used when the value itself is not known in advance.
func remoteCommandLineFlagValue(line, flag string) (string, bool) {
	for _, sep := range []string{" ", "="} {
		needle := flag + sep
		idx := strings.Index(line, needle)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(needle):]
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			end = len(rest)
		}
		if value := rest[:end]; value != "" {
			return value, true
		}
	}
	return "", false
}

// sshOrphanTaskIDFromWorkdir requires workdir to be exactly
// "<root>/task-<id>" with no further path segments — a subdirectory of a
// task dir is not a task's own agentctl workdir and is ignored, matching
// startRemoteAgentctl's launch-time contract that --workdir is always the
// task dir itself.
func sshOrphanTaskIDFromWorkdir(root, workdir string) (taskDir, taskID string, ok bool) {
	prefix := root + "/task-"
	if !strings.HasPrefix(workdir, prefix) {
		return "", "", false
	}
	id := strings.TrimPrefix(workdir, prefix)
	if id == "" || strings.ContainsAny(id, "/ \t") {
		return "", "", false
	}
	return "task-" + id, id, true
}

// stopSSHOrphanProcess runs the stop ladder for one confirmed orphan.
// sessionDir is only populated (and only then removed) when a pidfile
// attributed the process to a session; the task directory is never touched.
func stopSSHOrphanProcess(
	ctx context.Context,
	client *ssh.Client,
	resolvedRoot string,
	proc sshOrphanProcessRecord,
	pidfileAttributed bool,
	sessionID string,
) error {
	var sessionDir string
	if pidfileAttributed && sessionID != "" {
		sessionDir = resolvedRoot + "/tasks/" + proc.TaskDir + "/.kandev/sessions/" + sessionID
	}
	_, _, err := runSSHCommand(ctx, client, sshOrphanStopCommand(proc.PID, sessionDir))
	return err
}

// sshOrphanStopCommand implements AC-EXECUTORS-SSH-EXECUTOR-001.16: SIGTERM,
// a bounded grace period, then SIGKILL of pid together with the process
// groups of pid's direct children (captured before signalling, since
// agentctl's own children run in their own group — see procattr_unix.go —
// and a SIGKILL of agentctl alone would strand them). Children are only
// signalled when SIGKILL is actually needed, mirroring the AC's wording.
// sessionDir, when non-empty, is removed only after the pid is confirmed
// gone. The final liveness check treats a zombie (STAT starting with Z) as
// gone too: kill(2) still reports success for an unreaped zombie, and this
// sweep is not the process's parent, so it can never be the one to reap it.
//
//nolint:dupword // shell branches contain repeated `fi` tokens.
func sshOrphanStopCommand(pid int, sessionDir string) string {
	cleanup := "true"
	if sessionDir != "" {
		cleanup = removeRemoteDirCommand(sessionDir)
	}
	return fmt.Sprintf(`TARGET_PID=%[1]d
CHILDREN=$(ps -eo pid=,ppid= 2>/dev/null | while read -r cpid cppid; do
  [ "$cppid" = "$TARGET_PID" ] && echo "$cpid"
done)
if kill "$TARGET_PID" 2>/dev/null; then
  attempt=0
  while kill -0 "$TARGET_PID" 2>/dev/null && [ "$attempt" -lt %[2]d ]; do
    sleep 0.1
    attempt=$((attempt + 1))
  done
fi
if kill -0 "$TARGET_PID" 2>/dev/null; then
  kill -9 "$TARGET_PID" 2>/dev/null || true
  for cpid in $CHILDREN; do
    kill -9 -- "-$cpid" 2>/dev/null || true
  done
fi
if kill -0 "$TARGET_PID" 2>/dev/null; then
  STATE=$(ps -o stat= -p "$TARGET_PID" 2>/dev/null | tr -d ' ')
  case "$STATE" in
    Z*|"") ;;
    *)
      echo "orphan sweep: remote agentctl pid %[1]d is still running" >&2
      exit 1
      ;;
  esac
fi
%[3]s`, pid, sshAgentctlStopPollAttempts, cleanup)
}
