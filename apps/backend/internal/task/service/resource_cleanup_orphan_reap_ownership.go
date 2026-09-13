package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// applyOrphanReapOwnership filters attributed candidates down to the ones
// this task may signal. Every check fails closed at the narrowest unit it
// governs: a repository error is phase-wide inconclusive across every
// currently active root, a stored-path resolution failure is inconclusive
// across every root (the system cannot rule out "containing" for an
// unresolvable path), a blocked root excludes only that root's candidates,
// and a protected or otherwise-owned PID excludes only that one candidate.
func (s *Service) applyOrphanReapOwnership(
	ctx context.Context,
	taskID string,
	snap []hostProcess,
	byRoot map[string][]orphanReapCandidate,
	snapshot *taskResourceCleanupSnapshot,
) []orphanReapCandidate {
	ppidByPID := make(map[int]int, len(snap))
	for _, p := range snap {
		ppidByPID[p.PID] = p.PPID
	}
	protected, protectedInconclusive := orphanReapProtectedPIDs(ppidByPID)
	if protectedInconclusive {
		// An unresolved hop in the backend's own ancestry cannot be ruled
		// out as hiding a protected ancestor, so every root this attempt
		// found is inconclusive: the same posture as the checks below.
		s.skipEveryOrphanReapRoot(snapshot, byRoot, "ownership check inconclusive: could not resolve the backend's own ancestry")
		return nil
	}

	otherExecutors, execErr := s.executors.ListExecutorsRunning(ctx)
	if execErr != nil {
		s.skipEveryOrphanReapRoot(snapshot, byRoot, "ownership check failed: "+execErr.Error())
		return nil
	}
	otherSessions, sessErr := s.sessions.ListLiveWorkspaceSessions(ctx)
	if sessErr != nil {
		s.skipEveryOrphanReapRoot(snapshot, byRoot, "ownership check failed: "+sessErr.Error())
		return nil
	}

	localPIDOwner, liveWorktreeRoots, worktreeResolutionFailed := s.orphanReapOtherExecutorOwnership(ctx, otherExecutors, taskID)
	if worktreeResolutionFailed {
		// An unresolvable other-task live executor worktree path cannot be
		// ruled out as "containing" any root, so every root this attempt
		// found is inconclusive: the same posture otherTaskSessionPaths
		// already applies below.
		s.skipEveryOrphanReapRoot(snapshot, byRoot, "ownership check inconclusive: could not resolve another task's live executor worktree path")
		return nil
	}

	otherSessionPaths, resolutionFailed := s.otherTaskSessionPaths(ctx, otherSessions, taskID)
	if resolutionFailed {
		// An unresolvable stored path cannot be ruled out as "containing"
		// any root, so every root this attempt found is inconclusive.
		s.skipEveryOrphanReapRoot(snapshot, byRoot, "ownership check inconclusive: could not resolve another task's session workspace path")
		return nil
	}

	var toSignal []orphanReapCandidate
	for root, candidates := range byRoot {
		if owner, blocked := orphanReapFindOverlap(otherSessionPaths, root); blocked {
			s.recordOrphanReapRootSkip(snapshot, root, "workspace not exclusively owned: session of task "+owner)
			continue
		}
		if owner, blocked := orphanReapFindContainment(liveWorktreeRoots, root); blocked {
			s.recordOrphanReapRootSkip(snapshot, root, "another task's live recorded execution occupies this workspace: task "+owner)
			continue
		}
		for _, cand := range candidates {
			toSignal = s.applyOrphanReapPerCandidateOwnership(
				snapshot, taskID, cand, protected, ppidByPID, localPIDOwner, toSignal,
			)
		}
	}
	return toSignal
}

// orphanReapOtherExecutorOwnership indexes every other task's recorded
// executions into a local_pid ownership map and the set of live worktree
// roots another task's recorded execution occupies. resolutionFailed is
// true when any live executor's worktree path could not be resolved:
// mirrors otherTaskSessionPaths, since an unresolvable path can silently
// fail to match a process's real (resolved) cwd.
func (s *Service) orphanReapOtherExecutorOwnership(
	ctx context.Context, otherExecutors []*models.ExecutorRunning, taskID string,
) (localPIDOwner map[int]string, liveWorktreeRoots []orphanReapOwnedPath, resolutionFailed bool) {
	localPIDOwner = map[int]string{}
	for _, ex := range otherExecutors {
		if ex == nil || ex.TaskID == "" || ex.TaskID == taskID {
			continue
		}
		if ex.LocalPID != 0 {
			localPIDOwner[ex.LocalPID] = ex.TaskID
		}
		if ex.WorktreePath == "" || !orphanReapExecutorIsLive(ex.Status) {
			continue
		}
		hostOwned, known := orphanReapRuntimeRunsOnHost(ex.Runtime)
		if !known && ex.ExecutorID != "" {
			var err error
			hostOwned, known, err = s.orphanReapExecutorIDRunsOnHost(ctx, ex.ExecutorID)
			if err != nil {
				resolutionFailed = true
				continue
			}
		}
		if !known {
			resolutionFailed = true
			continue
		}
		resolved, err := filepath.EvalSymlinks(ex.WorktreePath)
		if err != nil {
			// A known remote runtime stores its cwd in another filesystem
			// namespace. Absence on this host is expected. If the path does
			// resolve locally, keep it as a real host mount and protect it.
			if !hostOwned && errors.Is(err, os.ErrNotExist) {
				continue
			}
			resolutionFailed = true
			continue
		}
		liveWorktreeRoots = append(liveWorktreeRoots, orphanReapOwnedPath{taskID: ex.TaskID, path: resolved})
	}
	return localPIDOwner, liveWorktreeRoots, resolutionFailed
}

func (s *Service) applyOrphanReapPerCandidateOwnership(
	snapshot *taskResourceCleanupSnapshot,
	taskID string,
	cand orphanReapCandidate,
	protected map[int]bool,
	ppidByPID map[int]int,
	localPIDOwner map[int]string,
	toSignal []orphanReapCandidate,
) []orphanReapCandidate {
	if cand.PID <= 1 || protected[cand.PID] {
		s.recordOrphanReapCandidate(snapshot, taskID, orphanReapCandidateRecord{
			PID: cand.PID, Cwd: cand.Cwd, Root: cand.Root, Command: cand.Command,
			Outcome: orphanReapOutcomeSkipped, Reason: "protected process",
		})
		return toSignal
	}
	owner, blocked, inconclusive := orphanReapAncestorOwner(cand.PID, ppidByPID, localPIDOwner)
	if inconclusive {
		s.recordOrphanReapCandidate(snapshot, taskID, orphanReapCandidateRecord{
			PID: cand.PID, Cwd: cand.Cwd, Root: cand.Root, Command: cand.Command,
			Outcome: orphanReapOutcomeSkipped, Reason: "ownership ancestry unresolvable",
		})
		return toSignal
	}
	if blocked {
		s.recordOrphanReapCandidate(snapshot, taskID, orphanReapCandidateRecord{
			PID: cand.PID, Cwd: cand.Cwd, Root: cand.Root, Command: cand.Command,
			Outcome: orphanReapOutcomeSkipped,
			Reason:  "owned by another task's recorded execution: " + owner,
		})
		return toSignal
	}
	return append(toSignal, cand)
}

// skipEveryOrphanReapRoot is only ever called for a detection failure (a
// repository error, or an unresolvable other-task stored path), never for a
// completed check that found real ownership, so every skip it records uses
// the detection-failure severity.
func (s *Service) skipEveryOrphanReapRoot(
	snapshot *taskResourceCleanupSnapshot, byRoot map[string][]orphanReapCandidate, reason string,
) {
	for root := range byRoot {
		s.recordOrphanReapRootSkipDetectionFailure(snapshot, root, reason)
	}
}

type orphanReapOwnedPath struct {
	taskID string
	path   string
}

// otherTaskSessionPaths resolves every other task's live session workspace
// path. A session with an empty workspace_path names no path.
// resolutionFailed is true when any non-empty path could not be resolved.
func (s *Service) otherTaskSessionPaths(
	ctx context.Context, sessions []*models.TaskSession, taskID string,
) (paths []orphanReapOwnedPath, resolutionFailed bool) {
	for _, sess := range sessions {
		if sess == nil || sess.TaskID == "" || sess.TaskID == taskID {
			continue
		}
		path := strings.TrimSpace(sess.WorkspacePath)
		if path == "" {
			continue
		}
		hostOwned, known, err := s.orphanReapSessionPathRunsOnHost(ctx, sess)
		if err != nil {
			resolutionFailed = true
			continue
		}
		if !known {
			resolutionFailed = true
			continue
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			// Remote and container paths are not visible from the backend.
			// An absent path is therefore not an ownership-check failure. A
			// path that does resolve locally is a real host mount and remains
			// protected by the normal containment check.
			if !hostOwned && errors.Is(err, os.ErrNotExist) {
				continue
			}
			resolutionFailed = true
			continue
		}
		paths = append(paths, orphanReapOwnedPath{taskID: sess.TaskID, path: resolved})
	}
	return paths, resolutionFailed
}

func orphanReapRuntimeRunsOnHost(runtime agentruntime.Runtime) (hostOwned, known bool) {
	switch runtime {
	case agentruntime.RuntimeStandalone:
		return true, true
	case agentruntime.RuntimeDocker,
		agentruntime.RuntimeRemoteDocker,
		agentruntime.RuntimeSprites,
		agentruntime.RuntimeSSH,
		agentruntime.RuntimeKubernetes:
		return false, true
	default:
		return false, false
	}
}

func orphanReapExecutorTypeRunsOnHost(executorType string) (hostOwned, known bool) {
	switch models.ExecutorType(executorType) {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree:
		return true, true
	case models.ExecutorTypeLocalDocker,
		models.ExecutorTypeRemoteDocker,
		models.ExecutorTypeSprites,
		models.ExecutorTypeSSH,
		models.ExecutorTypeKubernetes,
		models.ExecutorTypeMockRemote:
		return false, true
	default:
		return false, false
	}
}

func (s *Service) orphanReapExecutorIDRunsOnHost(
	ctx context.Context, executorID string,
) (hostOwned, known bool, err error) {
	if executorID == "" || s.executors == nil {
		return false, false, nil
	}
	executor, err := s.executors.GetExecutor(ctx, executorID)
	if err != nil {
		if errors.Is(err, models.ErrExecutorNotFound) {
			return false, false, nil
		}
		return false, false, err
	}
	if executor == nil {
		return false, false, nil
	}
	hostOwned, known = orphanReapExecutorTypeRunsOnHost(string(executor.Type))
	return hostOwned, known, nil
}

func (s *Service) orphanReapSessionPathRunsOnHost(
	ctx context.Context, session *models.TaskSession,
) (hostOwned, known bool, err error) {
	if session == nil {
		return false, false, nil
	}
	environmentID := session.TaskEnvironmentID
	if environmentID == "" {
		environmentID = session.EnvironmentID
	}
	if environmentID != "" {
		return s.orphanReapTaskEnvironmentRunsOnHost(ctx, environmentID)
	}
	if session.ExecutorID != "" {
		return s.orphanReapExecutorIDRunsOnHost(ctx, session.ExecutorID)
	}
	// Legacy repo-less sessions predate the environment/runtime identity and
	// store a direct host folder. Keep resolving those paths as before; an
	// absent path still fails closed in the caller.
	return true, true, nil
}

func (s *Service) orphanReapTaskEnvironmentRunsOnHost(
	ctx context.Context, environmentID string,
) (hostOwned, known bool, err error) {
	if s.taskEnvironments == nil {
		return false, false, nil
	}
	environment, getErr := s.taskEnvironments.GetTaskEnvironment(ctx, environmentID)
	if getErr != nil {
		if errors.Is(getErr, repository.ErrTaskEnvironmentNotFound) {
			return false, false, nil
		}
		return false, false, getErr
	}
	if environment == nil {
		return false, false, nil
	}
	if environment.ExecutorType != "" {
		hostOwned, known = orphanReapExecutorTypeRunsOnHost(environment.ExecutorType)
		return hostOwned, known, nil
	}
	if environment.ExecutorID == "" {
		return false, false, nil
	}
	return s.orphanReapExecutorIDRunsOnHost(ctx, environment.ExecutorID)
}

// orphanReapFindOverlap reports whether root is equal to, inside, or
// contains any of paths.
func orphanReapFindOverlap(paths []orphanReapOwnedPath, root string) (owner string, found bool) {
	for _, p := range paths {
		if p.path == root || orphanReapPathWithinRoot(root, p.path) || orphanReapPathWithinRoot(p.path, root) {
			return p.taskID, true
		}
	}
	return "", false
}

// orphanReapFindContainment reports whether any of paths is equal to or
// inside root (AC-TASKS-ORPHAN-REAP-003.4: another task's live worktree
// nested inside this reap root blocks it), a narrower test than
// orphanReapFindOverlap's bidirectional "equal to, inside, or containing".
func orphanReapFindContainment(paths []orphanReapOwnedPath, root string) (owner string, found bool) {
	for _, p := range paths {
		if p.path == root || orphanReapPathWithinRoot(root, p.path) {
			return p.taskID, true
		}
	}
	return "", false
}

// orphanReapExecutorIsLive applies a fail-closed posture to an undefined
// term: "another task's live recorded execution" has no definition of live
// on its own. A row is treated as live unless its status is one of the
// three the executors_running state model uses for a row that has finished
// running.
func orphanReapExecutorIsLive(status string) bool {
	switch status {
	case models.ExecutorRunningStatusFailed,
		models.ExecutorRunningStatusStopped,
		models.ExecutorRunningStatusComplete:
		return false
	default:
		return true
	}
}

// orphanReapAncestorOwner walks pid's ancestry (including pid itself) over
// the ppid chain from one host snapshot and reports the owning task if any
// hop is the local_pid of another task's recorded execution. A process
// reparented away from its launcher has no ancestry left to walk, so this
// can miss it by design; the root-level worktree-containment check covers
// that case instead. inconclusive is true when the walk reaches a hop whose
// ancestry the host snapshot could not resolve (orphanReapUnresolvedPPID):
// the walk cannot rule out ownership further up an unknown chain, so the
// caller must fail that candidate closed rather than read this as "not
// owned". A hop simply absent from ppidByPID (never seen by either host
// command at all) still ends the walk as "not owned", a narrower and rarer
// gap the root-level worktree-containment check also backstops.
func orphanReapAncestorOwner(pid int, ppidByPID map[int]int, owners map[int]string) (owner string, blocked bool, inconclusive bool) {
	seen := make(map[int]bool)
	for pid > 0 && !seen[pid] {
		seen[pid] = true
		if owner, ok := owners[pid]; ok {
			return owner, true, false
		}
		parent, ok := ppidByPID[pid]
		if !ok || parent == pid {
			break
		}
		if parent == orphanReapUnresolvedPPID {
			return "", false, true
		}
		pid = parent
	}
	return "", false, false
}

// orphanReapProtectedPIDs computes the protected set: the backend process
// (which is also "the process running the reap phase", since the phase
// runs in-process) and every ancestor of the backend process, walked over
// the same parent identifiers as every other check in this phase.
// inconclusive is true when the walk reaches a hop whose ancestry the host
// snapshot could not resolve (orphanReapUnresolvedPPID): mirrors
// orphanReapAncestorOwner's own sentinel handling, since a gap in the
// backend's own ancestry cannot be ruled out as hiding a protected
// ancestor any more than a gap in a candidate's ancestry can be ruled out
// as hiding an owning task.
func orphanReapProtectedPIDs(ppidByPID map[int]int) (protected map[int]bool, inconclusive bool) {
	protected = make(map[int]bool)
	self := os.Getpid()
	protected[self] = true
	pid := self
	seen := map[int]bool{pid: true}
	for {
		parent, ok := ppidByPID[pid]
		if !ok || parent == pid || seen[parent] {
			break
		}
		if parent == orphanReapUnresolvedPPID {
			return protected, true
		}
		if parent <= 0 {
			break
		}
		protected[parent] = true
		seen[parent] = true
		pid = parent
	}
	return protected, false
}
