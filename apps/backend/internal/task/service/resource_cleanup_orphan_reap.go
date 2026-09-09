package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// Reap outcomes persisted per candidate (AC-TASKS-ORPHAN-REAP-005.1).
const (
	orphanReapOutcomeTerminated = "terminated"
	orphanReapOutcomeKilled     = "killed"
	orphanReapOutcomeSkipped    = "skipped"
	orphanReapOutcomeSurvived   = "survived"
)

// orphanReapCandidateRecord is the durable, per-PID outcome record
// (AC-TASKS-ORPHAN-REAP-005.1, AC-TASKS-ORPHAN-REAP-006.5).
type orphanReapCandidateRecord struct {
	PID     int    `json:"pid"`
	Cwd     string `json:"cwd,omitempty"`
	Root    string `json:"root,omitempty"`
	Command string `json:"command,omitempty"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

// orphanReapSkipRecord is a root-level or phase-level skip with no
// per-candidate record to carry its reason (AC-TASKS-ORPHAN-REAP-005.7). An
// empty Root names a phase-level skip.
type orphanReapSkipRecord struct {
	Root   string `json:"root,omitempty"`
	Reason string `json:"reason"`
}

// orphanReapSnapshotTimeout bounds every read the host process snapshot makes
// (AC-TASKS-ORPHAN-REAP-007.2), matching the lsof timeout already used by
// internal/agentctl/server/api/port_listener.go.
const orphanReapSnapshotTimeout = 5 * time.Second

// orphanReapGraceDelay matches internal/agentctl/server/process/runner.go's
// SIGTERM->SIGKILL escalation window (AC-TASKS-ORPHAN-REAP-004.2). Only the
// duration is borrowed: that code waits on an owned child and kills a process
// group, both of which this contract forbids.
const orphanReapGraceDelay = 2 * time.Second

// orphanReapSettleDelay is the uninterruptible-sleep settle window after
// SIGKILL (AC-TASKS-ORPHAN-REAP-004.6).
const orphanReapSettleDelay = 1 * time.Second

// orphanReapMaxCandidates bounds one job's signalled candidates
// (AC-TASKS-ORPHAN-REAP-007.5).
const orphanReapMaxCandidates = 256

var errOrphanReapCandidateBoundReached = errors.New("orphan reap: candidate bound reached; remainder deferred to a later attempt")

var errOrphanReapCancelledMidPhase = errors.New("orphan reap: cancelled after the phase began")

var errOrphanReapCandidateSurvived = errors.New("orphan reap: one or more candidates survived SIGKILL")

// hostProcess is one process entry from a whole-host snapshot
// (AC-TASKS-ORPHAN-REAP-002.1). Cwd is resolved and cleaned; an entry that
// could not be parsed is dropped by the platform reader rather than appearing
// here with a zero value.
type hostProcess struct {
	PID     int
	PPID    int
	Cwd     string
	Command string
}

// orphanReapHostSnapshotter reads one host-wide process snapshot. Bound by
// orphanReapSnapshotTimeout by the caller (AC-TASKS-ORPHAN-REAP-007.2).
// Platform implementations live in resource_cleanup_orphan_reap_host_*.go.
type orphanReapHostSnapshotter interface {
	Snapshot(ctx context.Context) ([]hostProcess, error)
}

// errOrphanReapUnsupportedPlatform is returned by the platform snapshotter on
// an OS with no supported detection mechanism (AC-TASKS-ORPHAN-REAP-007.4).
var errOrphanReapUnsupportedPlatform = errors.New("orphan reap: unsupported platform " + runtime.GOOS)

// orphanReapVerifier re-reads one process's resolved working directory
// (AC-TASKS-ORPHAN-REAP-003.7). Deliberately not the whole-host snapshotter:
// this read is bounded independently (orphanReapVerifyTimeout) and falls
// outside AC-TASKS-ORPHAN-REAP-007.2's combined snapshot bound. Platform
// implementations live in resource_cleanup_orphan_reap_host_*.go.
type orphanReapVerifier interface {
	VerifyCwd(ctx context.Context, pid int) (cwd string, err error)
}

// gatherOrphanReapRootCandidates resolves every local path this attempt might
// remove WHILE IT STILL EXISTS (AC-TASKS-ORPHAN-REAP-001.1) — worktree
// directories, each worktree's per-task container directory (Terminology:
// "task ... directories"), and quick-chat session directories. Call this
// BEFORE performTaskCleanup runs; pass the result to
// confirmOrphanReapRootsRemoved afterward.
func (s *Service) gatherOrphanReapRootCandidates(
	snapshot *taskResourceCleanupSnapshot,
	sessionIDs []string,
) []string {
	seen := make(map[string]struct{})
	var candidates []string
	add := func(path string) {
		path = resolveOrphanReapPathBestEffort(path)
		if path == "" {
			return
		}
		if _, err := os.Lstat(path); err != nil {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, path)
	}
	for _, wt := range snapshot.Worktrees {
		if wt == nil || wt.Path == "" {
			continue
		}
		add(wt.Path)
		add(filepath.Dir(wt.Path))
	}
	if s.quickChatDir != "" {
		for _, sessionID := range sessionIDs {
			if sessionID == "" {
				continue
			}
			add(filepath.Join(s.quickChatDir, sessionID))
		}
	}
	return candidates
}

// confirmOrphanReapRootsRemoved re-checks each pre-cleanup candidate after
// performTaskCleanup has run and returns only the ones now confirmed absent —
// the ones this attempt actually removed (AC-TASKS-ORPHAN-REAP-001.1,
// AC-TASKS-ORPHAN-REAP-001.3). This runs every attempt, so whichever attempt
// actually removes and confirms a path records it, not only the first.
func confirmOrphanReapRootsRemoved(candidates []string) []string {
	var removed []string
	for _, path := range candidates {
		if _, err := os.Lstat(path); err == nil {
			continue
		}
		removed = append(removed, path)
	}
	return removed
}

// mergeOrphanReapRoots adds newlyRemoved paths to the durable root list,
// deduplicated, preserving existing order (a root persists for the life of
// the job — AC-TASKS-ORPHAN-REAP-001.1).
func mergeOrphanReapRoots(existing, newlyRemoved []string) []string {
	seen := make(map[string]struct{}, len(existing))
	merged := make([]string, 0, len(existing)+len(newlyRemoved))
	for _, root := range existing {
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		merged = append(merged, root)
	}
	for _, root := range newlyRemoved {
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		merged = append(merged, root)
	}
	return merged
}

// runOrphanReapPhase is the reap phase of the durable task-resource cleanup
// job (REQ-TASKS-ORPHAN-REAP-001..007). It is the job's last phase
// (AC-TASKS-ORPHAN-REAP-006.1); the caller gates it on a clean stop
// (AC-TASKS-ORPHAN-REAP-006.2) and on the context not already being cancelled
// (AC-TASKS-ORPHAN-REAP-006.3, first clause), mirroring reclaimSSHTaskDirs.
func (s *Service) runOrphanReapPhase(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
	snapshot *taskResourceCleanupSnapshot,
	newlyRemovedRoots []string,
) []error {
	snapshot.OrphanReapRoots = mergeOrphanReapRoots(snapshot.OrphanReapRoots, newlyRemovedRoots)
	if len(snapshot.OrphanReapRoots) == 0 {
		// AC-TASKS-ORPHAN-REAP-007.1: no roots, no snapshot read.
		return nil
	}

	resolvedRoots := resolveOrphanReapRoots(snapshot.OrphanReapRoots)
	activeRoots := make([]string, 0, len(resolvedRoots))
	for _, root := range resolvedRoots {
		if _, err := os.Lstat(root); err == nil {
			// AC-TASKS-ORPHAN-REAP-001.4: the root exists again; skip it for
			// this attempt, but keep it recorded for a later one.
			s.recordOrphanReapRootSkip(snapshot, root, "reap root exists again at reap time")
			continue
		}
		activeRoots = append(activeRoots, root)
	}
	if len(activeRoots) == 0 {
		return nil
	}

	snap, err := s.takeOrphanReapHostSnapshot(ctx)
	if err != nil {
		if errors.Is(err, errOrphanReapUnsupportedPlatform) {
			// AC-TASKS-ORPHAN-REAP-007.4.
			s.recordOrphanReapPhaseSkip(snapshot, "unsupported platform "+runtime.GOOS)
			return nil
		}
		// AC-TASKS-ORPHAN-REAP-002.6: unreadable/unavailable/timed-out
		// snapshot fails the whole phase closed, not the job.
		s.recordOrphanReapPhaseSkipDetectionFailure(snapshot, "host process snapshot unavailable: "+err.Error())
		return nil
	}

	byRoot := attributeOrphanReapCandidates(snap, activeRoots)
	if len(byRoot) == 0 {
		// AC-TASKS-ORPHAN-REAP-005.6: no candidates, empty result, no warning.
		return nil
	}

	toSignal := s.applyOrphanReapOwnership(ctx, job.TaskID, snap, byRoot, snapshot)
	if len(toSignal) == 0 {
		return nil
	}

	sort.Slice(toSignal, func(i, j int) bool { return toSignal[i].PID < toSignal[j].PID })
	capped := false
	if len(toSignal) > orphanReapMaxCandidates {
		toSignal = toSignal[:orphanReapMaxCandidates]
		capped = true
	}

	retErrs := s.signalOrphanReapCandidates(ctx, job.TaskID, toSignal, snapshot)
	if capped {
		retErrs = append(retErrs, errOrphanReapCandidateBoundReached)
		s.logger.Warn("orphan reap candidate bound reached; remainder deferred",
			zap.String("task_id", job.TaskID), zap.Int("bound", orphanReapMaxCandidates))
		orphanReapCounters.Add(orphanReapCounterCapReached, 1)
	}
	return retErrs
}

func (s *Service) takeOrphanReapHostSnapshot(ctx context.Context) ([]hostProcess, error) {
	snapshotter := s.orphanReapHostSnapshotter
	if snapshotter == nil {
		snapshotter = defaultOrphanReapHostSnapshotter()
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, orphanReapSnapshotTimeout)
	defer cancel()
	return snapshotter.Snapshot(timeoutCtx)
}

// recordOrphanReapRootSkip records a benign, informational root skip: a
// working ownership check concluded this root is not this task's to reap
// (e.g. AC-001.4's "exists again", or a genuine other-task ownership hit).
// Use recordOrphanReapRootSkipDetectionFailure instead when the check itself
// could not run (system design decision carried from Spec Review round 4's
// implementation seam: a fail-closed detection failure gets an
// operator-facing severity distinct from a benign ownership skip, because the
// host conditions causing the former are exactly what this feature exists to
// prevent).
func (s *Service) recordOrphanReapRootSkip(snapshot *taskResourceCleanupSnapshot, root, reason string) {
	snapshot.OrphanReapSkips = append(snapshot.OrphanReapSkips, orphanReapSkipRecord{Root: root, Reason: reason})
	orphanReapCounters.Add(orphanReapCounterSkippedRoot, 1)
	if s.logger != nil {
		s.logger.Info("orphan reap: root skipped", zap.String("root", root), zap.String("reason", reason))
	}
}

// recordOrphanReapRootSkipDetectionFailure records a root skip caused by
// AC-TASKS-ORPHAN-REAP-003.6's fail-closed posture itself firing (a
// repository error, or an other task's stored path that could not be
// resolved) rather than by a completed check finding real ownership. Logged
// at Warn so an operator scanning logs sees a detection failure, not just an
// inconclusive-by-design skip.
func (s *Service) recordOrphanReapRootSkipDetectionFailure(snapshot *taskResourceCleanupSnapshot, root, reason string) {
	snapshot.OrphanReapSkips = append(snapshot.OrphanReapSkips, orphanReapSkipRecord{Root: root, Reason: reason})
	orphanReapCounters.Add(orphanReapCounterSkippedRoot, 1)
	if s.logger != nil {
		s.logger.Warn("orphan reap: root skipped after an ownership detection failure",
			zap.String("root", root), zap.String("reason", reason))
	}
}

// recordOrphanReapPhaseSkip records a benign or expected phase-level skip
// (AC-TASKS-ORPHAN-REAP-007.4's unsupported platform is expected, not a
// failure).
func (s *Service) recordOrphanReapPhaseSkip(snapshot *taskResourceCleanupSnapshot, reason string) {
	snapshot.OrphanReapSkips = append(snapshot.OrphanReapSkips, orphanReapSkipRecord{Reason: reason})
	orphanReapCounters.Add(orphanReapCounterSkippedPhase, 1)
	if s.logger != nil {
		s.logger.Info("orphan reap: phase skipped", zap.String("reason", reason))
	}
}

// recordOrphanReapPhaseSkipDetectionFailure records AC-TASKS-ORPHAN-REAP-002.6's
// case: the host process snapshot itself was unreadable, unavailable, or
// timed out. Logged at Warn for the same operator-severity reason as
// recordOrphanReapRootSkipDetectionFailure.
func (s *Service) recordOrphanReapPhaseSkipDetectionFailure(snapshot *taskResourceCleanupSnapshot, reason string) {
	snapshot.OrphanReapSkips = append(snapshot.OrphanReapSkips, orphanReapSkipRecord{Reason: reason})
	orphanReapCounters.Add(orphanReapCounterSkippedPhase, 1)
	if s.logger != nil {
		s.logger.Warn("orphan reap: phase skipped after a detection failure", zap.String("reason", reason))
	}
}

// recordOrphanReapCandidate supersedes any earlier record for the same PID
// (AC-TASKS-ORPHAN-REAP-006.5) and logs/counts the outcome.
func (s *Service) recordOrphanReapCandidate(
	snapshot *taskResourceCleanupSnapshot, taskID string, rec orphanReapCandidateRecord,
) {
	replaced := false
	for i := range snapshot.OrphanReapRecords {
		if snapshot.OrphanReapRecords[i].PID == rec.PID {
			snapshot.OrphanReapRecords[i] = rec
			replaced = true
			break
		}
	}
	if !replaced {
		snapshot.OrphanReapRecords = append(snapshot.OrphanReapRecords, rec)
	}
	orphanReapCounters.Add(orphanReapCounterSeen, 1)
	switch rec.Outcome {
	case orphanReapOutcomeTerminated:
		orphanReapCounters.Add(orphanReapCounterTerminated, 1)
	case orphanReapOutcomeKilled:
		orphanReapCounters.Add(orphanReapCounterKilled, 1)
	case orphanReapOutcomeSurvived:
		orphanReapCounters.Add(orphanReapCounterSurvived, 1)
	case orphanReapOutcomeSkipped:
		orphanReapCounters.Add(orphanReapCounterSkippedCandidate, 1)
	}
	if s.logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("task_id", taskID),
		zap.Int("pid", rec.PID),
		zap.String("cwd", rec.Cwd),
		zap.String("root", rec.Root),
		zap.String("outcome", rec.Outcome),
	}
	if rec.Reason != "" {
		fields = append(fields, zap.String("reason", rec.Reason))
	}
	if rec.Outcome == orphanReapOutcomeSkipped {
		s.logger.Info("orphan reap: candidate skipped", fields...)
		return
	}
	// AC-TASKS-ORPHAN-REAP-005.3: any reap (terminated/killed/survived) is
	// warn-level so an operator scanning logs sees it without opting in.
	s.logger.Warn("orphan reap: candidate signalled", fields...)
}

// persistOrphanReapProgressBestEffort saves reap outcomes recorded during a
// failed cleanup attempt so AC-TASKS-ORPHAN-REAP-006.5's cross-attempt
// supersession has a durable record to supersede. Without this, an attempt
// that fails for an unrelated reason (e.g. a worktree removal error) would
// silently discard reap outcomes from the same attempt on every retry.
// Best-effort: a persistence failure here is logged, not folded into the
// attempt's error, since the attempt is already retrying for its own reason.
func (s *Service) persistOrphanReapProgressBestEffort(
	ctx context.Context, job *models.TaskResourceCleanupJob, snapshot *taskResourceCleanupSnapshot,
) {
	if len(snapshot.OrphanReapRoots) == 0 && len(snapshot.OrphanReapRecords) == 0 && len(snapshot.OrphanReapSkips) == 0 {
		return
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		s.logger.Warn("encode resource snapshot after failed cleanup attempt",
			zap.String("job_id", job.ID), zap.Error(err))
		return
	}
	if _, err := s.resourceCleanups.UpdateClaimedTaskResourceCleanupSnapshot(
		ctx, job.ID, job.Attempts, string(encoded),
	); err != nil {
		s.logger.Warn("persist resource snapshot after failed cleanup attempt",
			zap.String("job_id", job.ID), zap.Error(err))
	}
}

func resolveOrphanReapRoots(roots []string) []string {
	resolved := make([]string, len(roots))
	for i, root := range roots {
		resolved[i] = resolveOrphanReapPathBestEffort(root)
	}
	return resolved
}

// resolveOrphanReapPathBestEffort fully resolves path (AC-TASKS-ORPHAN-REAP-002.3).
// When the path no longer exists, EvalSymlinks cannot resolve it; fall back to
// an absolute, cleaned form of the path as it was captured, which is exactly
// what was compared going forward. An empty or unresolvable path yields "".
func resolveOrphanReapPathBestEffort(path string) string {
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}
