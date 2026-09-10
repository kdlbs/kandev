package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// Reap outcomes persisted per candidate.
const (
	orphanReapOutcomeTerminated = "terminated"
	orphanReapOutcomeKilled     = "killed"
	orphanReapOutcomeSkipped    = "skipped"
	orphanReapOutcomeSurvived   = "survived"
)

// orphanReapCandidateRecord is the durable, per-PID outcome record. A
// re-detected PID supersedes its earlier record.
type orphanReapCandidateRecord struct {
	PID     int    `json:"pid"`
	Cwd     string `json:"cwd,omitempty"`
	Root    string `json:"root,omitempty"`
	Command string `json:"command,omitempty"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

// orphanReapSkipRecord is a root-level or phase-level skip with no
// per-candidate record to carry its reason. An empty Root names a
// phase-level skip.
type orphanReapSkipRecord struct {
	Root   string `json:"root,omitempty"`
	Reason string `json:"reason"`
}

// orphanReapSnapshotTimeout bounds every read the host process snapshot
// makes, matching the lsof timeout already used by
// internal/agentctl/server/api/port_listener.go.
const orphanReapSnapshotTimeout = 5 * time.Second

// orphanReapGraceDelay matches internal/agentctl/server/process/runner.go's
// SIGTERM->SIGKILL escalation window. Only the duration is borrowed: that
// code waits on an owned child and kills a process group, both of which this
// capability forbids.
const orphanReapGraceDelay = 2 * time.Second

// orphanReapSettleDelay is the uninterruptible-sleep settle window after
// SIGKILL.
const orphanReapSettleDelay = 1 * time.Second

// orphanReapMaxCandidates bounds one job's signalled candidates.
const orphanReapMaxCandidates = 256

var errOrphanReapCandidateBoundReached = errors.New("orphan reap: candidate bound reached; remainder deferred to a later attempt")

var errOrphanReapCancelledMidPhase = errors.New("orphan reap: cancelled after the phase began")

var errOrphanReapCandidateSurvived = errors.New("orphan reap: one or more candidates survived SIGKILL")

// hostProcess is one process entry from a whole-host snapshot. Cwd is
// resolved and cleaned; an entry that could not be parsed is dropped by the
// platform reader rather than appearing here with a zero value.
type hostProcess struct {
	PID     int
	PPID    int
	Cwd     string
	Command string
}

// orphanReapHostSnapshotter reads one host-wide process snapshot. Bound by
// orphanReapSnapshotTimeout by the caller. Platform implementations live in
// resource_cleanup_orphan_reap_host_*.go.
type orphanReapHostSnapshotter interface {
	Snapshot(ctx context.Context) ([]hostProcess, error)
}

// errOrphanReapUnsupportedPlatform is returned by the platform snapshotter on
// an OS with no supported detection mechanism.
var errOrphanReapUnsupportedPlatform = errors.New("orphan reap: unsupported platform " + runtime.GOOS)

// orphanReapVerifier re-reads one process's resolved working directory
// immediately before a signal. Deliberately not the whole-host snapshotter:
// this read is bounded independently (orphanReapVerifyTimeout) and falls
// outside the whole-host snapshot's own combined timeout budget. Platform
// implementations live in resource_cleanup_orphan_reap_host_*.go.
type orphanReapVerifier interface {
	VerifyCwd(ctx context.Context, pid int) (cwd string, err error)
}

// gatherOrphanReapRootCandidates resolves every local path this attempt might
// remove WHILE IT STILL EXISTS — worktree directories, each worktree's
// per-task container directory, and quick-chat session directories. Call
// this BEFORE performTaskCleanup runs; pass the result to
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
// the ones this attempt actually removed. This runs every attempt, so
// whichever attempt actually removes and confirms a path records it, not
// only the first.
func confirmOrphanReapRootsRemoved(candidates []string) []string {
	var removed []string
	for _, path := range candidates {
		_, err := os.Lstat(path)
		if !errors.Is(err, os.ErrNotExist) {
			// Either the path still exists (err == nil) or the stat itself
			// failed inconclusively (e.g. ENOTDIR): neither confirms removal.
			continue
		}
		removed = append(removed, path)
	}
	return removed
}

// mergeOrphanReapRoots adds newlyRemoved paths to the durable root list,
// deduplicated, preserving existing order: a root persists for the life of
// the job once recorded.
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
// job. It is the job's last phase; the caller gates it on a clean stop and on
// the context not already being cancelled, mirroring reclaimSSHTaskDirs. The
// caller records newly removed roots into snapshot.OrphanReapRoots before
// calling this, unconditionally: recording a root and signalling against it
// are gated separately, so an attempt whose stop failed still keeps the root
// it removed even though it never reaches the signalling step below.
func (s *Service) runOrphanReapPhase(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
	snapshot *taskResourceCleanupSnapshot,
) []error {
	if len(snapshot.OrphanReapRoots) == 0 {
		// Nothing could be holding a root open if there are no roots; skip
		// the host snapshot read entirely rather than pay its cost for
		// nothing.
		return nil
	}

	resolvedRoots := resolveOrphanReapRoots(snapshot.OrphanReapRoots)
	activeRoots := make([]string, 0, len(resolvedRoots))
	for _, root := range resolvedRoots {
		_, err := os.Lstat(root)
		switch {
		case err == nil:
			// The root exists again; skip it for this attempt, but keep it
			// recorded for a later one.
			s.recordOrphanReapRootSkip(snapshot, root, "reap root exists again at reap time")
		case errors.Is(err, os.ErrNotExist):
			activeRoots = append(activeRoots, root)
		default:
			// The stat itself failed inconclusively: this alone cannot
			// confirm absence, so fail this root closed rather than guess.
			s.recordOrphanReapRootSkipDetectionFailure(snapshot, root, "cannot confirm reap root state: "+err.Error())
		}
	}
	if len(activeRoots) == 0 {
		return nil
	}

	snap, err := s.takeOrphanReapHostSnapshot(ctx)
	if err != nil {
		if errors.Is(err, errOrphanReapUnsupportedPlatform) {
			s.recordOrphanReapPhaseSkip(snapshot, "unsupported platform "+runtime.GOOS)
			return nil
		}
		// An unreadable, unavailable, or timed-out snapshot fails the whole
		// phase closed, not the job.
		s.recordOrphanReapPhaseSkipDetectionFailure(snapshot, "host process snapshot unavailable: "+err.Error())
		return nil
	}

	byRoot := attributeOrphanReapCandidates(snap, activeRoots)
	if len(byRoot) == 0 {
		// No candidates: empty result, no warning.
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
	s.recordOrphanReapAggregateOutcome(job.TaskID, snapshot, toSignal)
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
// (the root exists again, or another task genuinely owns it).
// Use recordOrphanReapRootSkipDetectionFailure instead when the check itself
// could not run: a fail-closed detection failure gets an operator-facing
// severity distinct from a benign ownership skip, because the host
// conditions causing the former are exactly what this feature exists to
// prevent.
func (s *Service) recordOrphanReapRootSkip(snapshot *taskResourceCleanupSnapshot, root, reason string) {
	snapshot.OrphanReapSkips = append(snapshot.OrphanReapSkips, orphanReapSkipRecord{Root: root, Reason: reason})
	orphanReapCounters.Add(orphanReapCounterSkippedRoot, 1)
	if s.logger != nil {
		s.logger.Info("orphan reap: root skipped", zap.String("root", root), zap.String("reason", reason))
	}
}

// recordOrphanReapRootSkipDetectionFailure records a root skip caused by the
// ownership check's own fail-closed posture firing (a repository error, or
// an other task's stored path that could not be resolved) rather than by a
// completed check finding real ownership. Logged at Warn so an operator
// scanning logs sees a detection failure, not just an inconclusive-by-design
// skip.
func (s *Service) recordOrphanReapRootSkipDetectionFailure(snapshot *taskResourceCleanupSnapshot, root, reason string) {
	snapshot.OrphanReapSkips = append(snapshot.OrphanReapSkips, orphanReapSkipRecord{Root: root, Reason: reason})
	orphanReapCounters.Add(orphanReapCounterSkippedRoot, 1)
	if s.logger != nil {
		s.logger.Warn("orphan reap: root skipped after an ownership detection failure",
			zap.String("root", root), zap.String("reason", reason))
	}
}

// recordOrphanReapPhaseSkip records a benign or expected phase-level skip
// (an unsupported platform is expected, not a failure).
func (s *Service) recordOrphanReapPhaseSkip(snapshot *taskResourceCleanupSnapshot, reason string) {
	snapshot.OrphanReapSkips = append(snapshot.OrphanReapSkips, orphanReapSkipRecord{Reason: reason})
	orphanReapCounters.Add(orphanReapCounterSkippedPhase, 1)
	if s.logger != nil {
		s.logger.Info("orphan reap: phase skipped", zap.String("reason", reason))
	}
}

// recordOrphanReapPhaseSkipDetectionFailure records the case where the host
// process snapshot itself was unreadable, unavailable, or timed out. Logged
// at Warn for the same operator-severity reason as
// recordOrphanReapRootSkipDetectionFailure.
func (s *Service) recordOrphanReapPhaseSkipDetectionFailure(snapshot *taskResourceCleanupSnapshot, reason string) {
	snapshot.OrphanReapSkips = append(snapshot.OrphanReapSkips, orphanReapSkipRecord{Reason: reason})
	orphanReapCounters.Add(orphanReapCounterSkippedPhase, 1)
	if s.logger != nil {
		s.logger.Warn("orphan reap: phase skipped after a detection failure", zap.String("reason", reason))
	}
}

// recordOrphanReapCandidate supersedes any earlier record for the same PID
// and logs/counts the outcome.
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
	// Any reap (terminated/killed/survived) is warn-level so an operator
	// scanning logs sees it without opting in.
	s.logger.Warn("orphan reap: candidate signalled", fields...)
}

// recordOrphanReapAggregateOutcome emits one aggregate warn log naming the
// task, the count, and each pid/cwd this attempt terminated or killed, in
// addition to the per-candidate logs recordOrphanReapCandidate already
// emits. Silent when nothing was terminated or killed this attempt.
func (s *Service) recordOrphanReapAggregateOutcome(
	taskID string, snapshot *taskResourceCleanupSnapshot, attempted []orphanReapCandidate,
) {
	if s.logger == nil {
		return
	}
	recordByPID := make(map[int]orphanReapCandidateRecord, len(snapshot.OrphanReapRecords))
	for _, rec := range snapshot.OrphanReapRecords {
		recordByPID[rec.PID] = rec
	}
	var processes []string
	for _, cand := range attempted {
		rec, ok := recordByPID[cand.PID]
		if !ok {
			continue
		}
		if rec.Outcome == orphanReapOutcomeTerminated || rec.Outcome == orphanReapOutcomeKilled {
			processes = append(processes, fmt.Sprintf("%d:%s", rec.PID, rec.Cwd))
		}
	}
	if len(processes) == 0 {
		return
	}
	s.logger.Warn("orphan reap: phase reaped candidates",
		zap.String("task_id", taskID),
		zap.Int("count", len(processes)),
		zap.String("processes", strings.Join(processes, ",")),
	)
}

// persistOrphanReapProgressBestEffort saves reap outcomes recorded during a
// failed cleanup attempt so cross-attempt supersession has a durable record
// to supersede. Without this, an attempt that fails for an unrelated reason
// (e.g. a worktree removal error) would silently discard reap outcomes from
// the same attempt on every retry. The caller's ctx is frequently the reason
// this attempt failed in the first place (a mid-phase cancellation), so this
// write runs on a context detached from that cancellation — the same pattern
// retryTaskResourceCleanupJob already uses for its own transition — or every
// cancelled attempt would silently lose this write before it reaches the DB.
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
	persistCtx, cancel := detachedCleanupTransitionContext(ctx)
	defer cancel()
	if _, err := s.resourceCleanups.UpdateClaimedTaskResourceCleanupSnapshot(
		persistCtx, job.ID, job.Attempts, string(encoded),
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

// resolveOrphanReapPathBestEffort fully resolves path. When the path no
// longer exists, EvalSymlinks cannot resolve it; fall back to an absolute,
// cleaned form of the path as it was captured, which is exactly what was
// compared going forward. An empty or unresolvable path yields "".
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
