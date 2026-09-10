package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
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
	protected := orphanReapProtectedPIDs(ppidByPID)

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

	localPIDOwner, liveWorktreeRoots, worktreeResolutionFailed := orphanReapOtherExecutorOwnership(otherExecutors, taskID)
	if worktreeResolutionFailed {
		// An unresolvable other-task live executor worktree path cannot be
		// ruled out as "containing" any root, so every root this attempt
		// found is inconclusive: the same posture otherTaskSessionPaths
		// already applies below.
		s.skipEveryOrphanReapRoot(snapshot, byRoot, "ownership check inconclusive: could not resolve another task's live executor worktree path")
		return nil
	}

	otherSessionPaths, resolutionFailed := s.otherTaskSessionPaths(otherSessions, taskID)
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
func orphanReapOtherExecutorOwnership(
	otherExecutors []*models.ExecutorRunning, taskID string,
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
		resolved, err := filepath.EvalSymlinks(ex.WorktreePath)
		if err != nil {
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
	if owner, ok := orphanReapAncestorOwner(cand.PID, ppidByPID, localPIDOwner); ok {
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
	sessions []*models.TaskSession, taskID string,
) (paths []orphanReapOwnedPath, resolutionFailed bool) {
	for _, sess := range sessions {
		if sess == nil || sess.TaskID == "" || sess.TaskID == taskID {
			continue
		}
		path := strings.TrimSpace(sess.WorkspacePath)
		if path == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			resolutionFailed = true
			continue
		}
		paths = append(paths, orphanReapOwnedPath{taskID: sess.TaskID, path: resolved})
	}
	return paths, resolutionFailed
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

// orphanReapFindContainment reports whether root is equal to or inside any
// of paths, a narrower test than orphanReapFindOverlap's "equal to, inside,
// or containing".
func orphanReapFindContainment(paths []orphanReapOwnedPath, root string) (owner string, found bool) {
	for _, p := range paths {
		if p.path == root || orphanReapPathWithinRoot(p.path, root) {
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
// that case instead.
func orphanReapAncestorOwner(pid int, ppidByPID map[int]int, owners map[int]string) (string, bool) {
	seen := make(map[int]bool)
	for pid > 0 && !seen[pid] {
		seen[pid] = true
		if owner, ok := owners[pid]; ok {
			return owner, true
		}
		parent, ok := ppidByPID[pid]
		if !ok || parent == pid {
			break
		}
		pid = parent
	}
	return "", false
}

// orphanReapProtectedPIDs computes the protected set: the backend process
// (which is also "the process running the reap phase", since the phase
// runs in-process) and every ancestor of the backend process, walked over
// the same parent identifiers as every other check in this phase.
func orphanReapProtectedPIDs(ppidByPID map[int]int) map[int]bool {
	protected := make(map[int]bool)
	self := os.Getpid()
	protected[self] = true
	pid := self
	seen := map[int]bool{pid: true}
	for {
		parent, ok := ppidByPID[pid]
		if !ok || parent <= 0 || parent == pid || seen[parent] {
			break
		}
		protected[parent] = true
		seen[parent] = true
		pid = parent
	}
	return protected
}
