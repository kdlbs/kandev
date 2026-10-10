package orchestrator

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

// Idle-session reaper.
//
// The reaper is a single owner of one background goroutine on Service. It
// The reaper runs two separate policies over executors_running. The existing
// disconnected-owner pass reclaims dead rows after a minimum age and never
// stops a live process. The workspace ACP policy separately suspends settled,
// resumable sessions when their workspace opts in; it stops only the selected
// ACP process and preserves task-owned resources.
//
// Two constants:
//   - idleReaperInterval: how often the reaper scans. Short enough to
//     reclaim idle rows within a minute, long enough to avoid per-tick
//     amplification when no rows are candidates.
//   - idleReaperMinIdle: the minimum row age (UpdatedAt → now) before a
//     row is eligible. New rows are never reaped even if their state
//     already looks idle — this gives the system a settling window
//     after a normal turn ends so a row that is "WaitingForInput
//     since 1ms" does not get reclaimed on the very next tick.
//
// Both constants are intentional configuration knobs in code source —
// not env vars — because the orchestrator has no other runtime config
// of this shape, and the values are bounded by fail-closed invariants
// rather than deployment shape. Operators who need to tune them can
// edit and re-deploy; consumers do not.
//
// Disconnected-owner reclaim invariants:
//   - Never call StopAgent / StopAgentWithReason / Kill. reclaimIdleSession
//     routes through repairDeadRowLiveness (status=stopped, LocalPID=0,
//     resume_token preserved); a live executor is short-circuited inside
//     reclaimIdleSession before any side effect.
//   - reclaimIdleSession is fail-closed: an uncertain guard is a skip,
//     never a force. The reaper just calls the primitive in a loop,
//     so the same invariant carries.
//   - Rows whose UpdatedAt is unset / zero / in the future are treated
//     as ineligible; they would otherwise be reaped on the first tick.
//   - The reaper scans regardless of session lifecycle: it does NOT
//     depend on session.ensure / session.launch / agent.ready. The
//     startup reconciliation covers fresh restarts; the reaper covers
//     ongoing drift.
const (
	// idleReaperInterval is the cadence at which the reaper scans
	// executors_running for reclaim candidates. Short enough to keep
	// idle windows tight, long enough that no tick overlaps with the
	// previous tick on a busy system.
	idleReaperInterval = 30 * time.Second

	// idleReaperMinIdle is the minimum age (now - row.UpdatedAt)
	// before a row is eligible. Below this, the system is still
	// settling — on_enter / on_turn_complete / agent.ready may have
	// just transitioned the row and the next event has not fired yet.
	idleReaperMinIdle = 60 * time.Second

	// idleSuspensionOperationTimeout bounds executor teardown after candidate
	// locks have been released.
	idleSuspensionOperationTimeout = 30 * time.Second
	// idleSuspensionRevalidationTimeout bounds admission and candidate reads
	// performed while session locks are held.
	idleSuspensionRevalidationTimeout = 5 * time.Second
)

// idleSessionReaper owns the lifecycle of one background goroutine.
// All methods are nil-safe: a Service with reaper == nil treats the
// reaper as off (useful for tests that don't want a loop).
type idleSessionReaper struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	workers  sync.WaitGroup
	started  bool
	interval time.Duration
	minIdle  time.Duration
}

type idleParkingCandidateKey struct {
	workspaceID    string
	sessionID      string
	executionID    string
	policyUpdated  int64
	timeoutMinutes int
	idleSince      int64
	promptGen      uint64
	activityEpoch  uint64
}

type idlePromptActivityReader interface {
	GetPromptActivityForSession(context.Context, string) (string, uint64, uint64, time.Time, error)
}

type idleSuspender interface {
	SuspendIdle(context.Context, agentruntime.IdleSuspensionIdentity) error
}

type idleSuspensionStateCAS interface {
	CompareAndSetExecutorRunningIdleSuspension(context.Context, string, string, time.Time, string, string) error
}

type workspaceIdleCandidate struct {
	key      idleParkingCandidateKey
	identity agentruntime.IdleSuspensionIdentity
	timeout  time.Duration
}

type idleParkingSkipReason string

const (
	idleParkingSkipIdentityUnavailable  idleParkingSkipReason = "identity_unavailable"
	idleParkingSkipWorkspaceUnavailable idleParkingSkipReason = "workspace_unavailable"
	idleParkingSkipPolicyDisabled       idleParkingSkipReason = "policy_disabled"
	idleParkingSkipOwnershipProtected   idleParkingSkipReason = "ownership_protected"
	idleParkingSkipSessionState         idleParkingSkipReason = "session_state"
	idleParkingSkipKnownWork            idleParkingSkipReason = "known_work"
	idleParkingSkipRestoreUnavailable   idleParkingSkipReason = "restore_unavailable"
	idleParkingSkipActivityUnavailable  idleParkingSkipReason = "activity_unavailable"
	idleParkingSkipStaleActivity        idleParkingSkipReason = "stale_activity"
	idleParkingSkipSuspensionFailed     idleParkingSkipReason = "suspension_failed"
)

var idleParkingSkipReasonOrder = [...]idleParkingSkipReason{
	idleParkingSkipIdentityUnavailable,
	idleParkingSkipWorkspaceUnavailable,
	idleParkingSkipPolicyDisabled,
	idleParkingSkipOwnershipProtected,
	idleParkingSkipSessionState,
	idleParkingSkipKnownWork,
	idleParkingSkipRestoreUnavailable,
	idleParkingSkipActivityUnavailable,
	idleParkingSkipStaleActivity,
	idleParkingSkipSuspensionFailed,
}

func newIdleSessionReaper() *idleSessionReaper {
	return &idleSessionReaper{
		interval: idleReaperInterval,
		minIdle:  idleReaperMinIdle,
	}
}

// start launches the reaper loop with the given tick callback.
// Idempotent: a second call on a running reaper is a no-op. Tests
// inject a custom tick to observe invocation count; production
// wires Service.reclaimIdleSessionsOnce as the tick.
func (r *idleSessionReaper) start(parent context.Context, tick func(ctx context.Context)) bool {
	if r == nil {
		return false
	}
	if tick == nil {
		return false
	}
	if parent == nil {
		parent = context.Background()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return false
	}
	loopCtx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.started = true
	r.workers.Add(1)
	go func() {
		defer r.workers.Done()
		r.runLoop(loopCtx, tick)
	}()
	return true
}

// stop signals the reaper to exit and waits for it. Safe to call on
// a never-started / already-stopped reaper. Blocks until the loop
// observes cancellation; tests and Service.Stop rely on this for
// clean teardown.
func (r *idleSessionReaper) stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()
	r.workers.Wait()
	r.mu.Lock()
	r.cancel = nil
	r.started = false
	r.mu.Unlock()
}

// runLoop selects on the ticker + the loop context. Tick callback runs
// under the loop context, so a slow tick is cancelled on shutdown.
func (r *idleSessionReaper) runLoop(ctx context.Context, tick func(ctx context.Context)) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick(ctx)
		}
	}
}

// Service-level hooks. The reaper is owned by Service so the lifecycle
// matches the rest of the orchestration: Service.Start launches it,
// Service.Stop joins it.

// startIdleSessionReaper wires the reaper into Service.Start. The
// reaper captures the canonical fail-closed reclaim primitive so
// the per-tick work is a thin scan + loop, not a new policy. The tick also
// drives reclaimStuckSignalSessionsOnce (stuck_signal_watchdog.go),
// detectOfficeDecisionWaitingOnce (office_decision_stall_watchdog.go) and
// reapStalePendingMovesOnce (pending_move_reaper.go) — further scans
// sharing this ticker rather than background goroutines of their own,
// preserving the reaper's single-goroutine-owner invariant. The 30s
// cadence is far finer than either of those needs; they are here for the
// goroutine budget, not for the interval.
func (s *Service) startIdleSessionReaper(ctx context.Context) {
	if s.idleReaper == nil {
		return
	}
	if !s.idleReaper.start(ctx, func(tickCtx context.Context) {
		s.reclaimIdleSessionsOnce(tickCtx)
		s.suspendWorkspaceIdleSessionsOnce(tickCtx)
		s.reclaimStuckSignalSessionsOnce(tickCtx)
		s.detectOfficeDecisionWaitingOnce(tickCtx)
		s.reapStalePendingMovesOnce(tickCtx)
	}) {
		return
	}
	s.logger.Info("idle-session reaper started",
		zap.Duration("interval", s.idleReaper.interval),
		zap.Duration("min_idle", s.idleReaper.minIdle))
}

// stopIdleSessionReaper joins the reaper. Must be called before
// Service.Stop returns to honour the goroutine-ownership invariant.
func (s *Service) stopIdleSessionReaper() {
	if s.idleReaper == nil {
		return
	}
	s.idleReaper.stop()
	s.logger.Info("idle-session reaper stopped")
}

// reclaimIdleSessionsOnce is the per-tick scan. It reads every
// executors_running row, applies the minimum-idle filter (UpdatedAt
// age), and delegates each candidate to reclaimIdleSession. The
// primitive enforces the fail-closed guard (state, live agent,
// active turn, resume token); this scan only adds the age filter that
// the settle-point callers do not need.
//
// Each row's reclaim is best-effort: a failure is logged at warn and
// skipped. The next tick will retry. The scan never aborts the loop
// on a single-row error.
func (s *Service) reclaimIdleSessionsOnce(ctx context.Context) {
	now := time.Now().UTC()
	cutoff := now.Add(-s.idleReaper.minIdle)
	var (
		rows []*models.ExecutorRunning
		err  error
	)
	if candidates, ok := s.repo.(idleExecutorCandidateLister); ok {
		rows, err = candidates.ListExecutorsRunningIdle(ctx, cutoff)
	} else {
		rows, err = s.repo.ListExecutorsRunning(ctx)
	}
	if err != nil {
		s.logger.Warn("idle reaper: list executors_running failed; tick skipped",
			zap.Error(err))
		return
	}
	if len(rows) == 0 {
		return
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.Status == models.ExecutorRunningStatusStopped {
			continue
		}
		sessionID := row.SessionID
		if sessionID == "" {
			continue
		}
		// Reject rows with zero / unset / future UpdatedAt — they would
		// otherwise look older than the threshold on the first tick.
		if row.UpdatedAt.IsZero() {
			continue
		}
		if row.UpdatedAt.After(now) || now.Sub(row.UpdatedAt) < s.idleReaper.minIdle {
			continue
		}
		// Delegate to the fail-closed primitive. It reads the session,
		// checks its guard (state × live runtime × active turn × resume
		// token), and either reclaims the row or skips it. Errors here are
		// recoverable on the next tick — log and move on.
		if err := s.reclaimIdleSession(ctx, sessionID); err != nil {
			s.logger.Warn("idle reaper: reclaim failed; row preserved for next tick",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
}

func (s *Service) suspendWorkspaceIdleSessionsOnce(ctx context.Context) {
	suspender, ok := s.agentManager.(idleSuspender)
	if !ok || suspender == nil {
		return
	}
	rows, err := s.repo.ListExecutorsRunning(ctx)
	if err != nil {
		s.logger.Warn("workspace idle suspension scan skipped; inventory read failed", zap.Error(err))
		return
	}
	now := time.Now().UTC()
	summary := idleParkingScanSummary{
		observed: make(map[idleParkingCandidateKey]struct{}),
		skipped:  make(map[idleParkingSkipReason]int),
	}
	for _, row := range rows {
		s.processWorkspaceIdleRow(ctx, row, now, suspender, &summary)
	}
	s.pruneIdleParkingCandidates(summary.observed)
	s.logWorkspaceIdleScan(summary)
}

type idleParkingScanSummary struct {
	observed                     map[idleParkingCandidateKey]struct{}
	skipped                      map[idleParkingSkipReason]int
	suspended, failed, recovered int
}

func (s *Service) processWorkspaceIdleRow(
	ctx context.Context,
	row *models.ExecutorRunning,
	now time.Time,
	suspender idleSuspender,
	summary *idleParkingScanSummary,
) {
	if row == nil || row.SessionID == "" || row.IdleSuspensionState == models.ExecutorIdleSuspensionSuspended {
		return
	}
	if row.IdleSuspensionState == models.ExecutorIdleSuspensionInProgress ||
		row.IdleSuspensionState == models.ExecutorIdleSuspensionAgentStopped {
		if s.reconcileIdleSuspensionAfterRestart(ctx, row) {
			summary.recovered++
		}
		return
	}
	candidate, skipReason, eligible := s.workspaceIdleCandidate(ctx, row, now)
	if !eligible {
		summary.skipped[skipReason]++
		return
	}
	summary.observed[candidate.key] = struct{}{}
	if !s.idleParkingCandidateDue(candidate.key, now, candidate.timeout) || !s.claimIdleParkingCandidate(candidate.key) {
		return
	}
	s.suspendWorkspaceIdleCandidate(ctx, row, candidate, suspender, summary)
}

func (s *Service) suspendWorkspaceIdleCandidate(
	ctx context.Context,
	row *models.ExecutorRunning,
	candidate workspaceIdleCandidate,
	suspender idleSuspender,
	summary *idleParkingScanSummary,
) {
	var identity agentruntime.IdleSuspensionIdentity
	revalidated := false
	revalidationCtx, cancelRevalidation := context.WithTimeout(ctx, idleSuspensionRevalidationTimeout)
	suspendErr := s.withSessionPromptAdmission(revalidationCtx, row.SessionID, func(admittedCtx context.Context) error {
		releaseLifecycleLock := s.acquireSessionLifecycleLock(row.SessionID)
		defer releaseLifecycleLock()
		current, _, currentOK := s.workspaceIdleCandidate(admittedCtx, row, time.Now().UTC())
		if !currentOK || current.key != candidate.key || current.identity != candidate.identity {
			return nil
		}
		identity = current.identity
		revalidated = true
		return nil
	})
	cancelRevalidation()
	if suspendErr == nil && revalidated {
		operationCtx, cancel := context.WithTimeout(ctx, idleSuspensionOperationTimeout)
		suspendErr = suspender.SuspendIdle(operationCtx, identity)
		cancel()
	}
	succeeded := suspendErr == nil && revalidated
	s.finishIdleParkingCandidate(candidate.key, succeeded)
	if suspendErr == nil {
		if succeeded {
			summary.suspended++
		}
		return
	}
	summary.failed++
	summary.skipped[idleParkingSkipSuspensionFailed]++
	s.logWorkspaceIdleSuspensionFailure(row.SessionID, suspendErr)
}

func (s *Service) logWorkspaceIdleSuspensionFailure(sessionID string, suspendErr error) {
	failureStage := "unclassified"
	if reporter, ok := suspendErr.(interface{ IdleSuspensionFailureStage() string }); ok {
		failureStage = reporter.IdleSuspensionFailureStage()
	}
	s.logger.Warn("workspace ACP idle suspension failed; candidate will be retried",
		zap.String("session_id", sessionID),
		zap.String("reason", string(idleParkingSkipSuspensionFailed)),
		zap.String("failure_stage", failureStage))
}

func (s *Service) logWorkspaceIdleScan(summary idleParkingScanSummary) {
	if len(summary.skipped) > 0 {
		fields := make([]zap.Field, 0, len(idleParkingSkipReasonOrder))
		for _, reason := range idleParkingSkipReasonOrder {
			if count := summary.skipped[reason]; count > 0 {
				fields = append(fields, zap.Int("skip_"+string(reason), count))
			}
		}
		s.logger.Debug("workspace ACP idle suspension scan skipped candidates", fields...)
	}
	if summary.suspended > 0 || summary.failed > 0 || summary.recovered > 0 {
		s.logger.Info("workspace ACP idle suspension scan completed",
			zap.Int("suspended", summary.suspended),
			zap.Int("failed", summary.failed),
			zap.Int("recovered", summary.recovered))
	}
}

func (s *Service) workspaceIdleCandidate(
	ctx context.Context,
	row *models.ExecutorRunning,
	now time.Time,
) (workspaceIdleCandidate, idleParkingSkipReason, bool) {
	current, reason, ok := s.currentWorkspaceIdleExecutor(ctx, row)
	if !ok {
		return workspaceIdleCandidate{}, reason, false
	}
	task, reason, ok := s.workspaceIdleTask(ctx, current)
	if !ok {
		return workspaceIdleCandidate{}, reason, false
	}
	workspace, reason, ok := s.workspaceIdlePolicy(ctx, task)
	if !ok {
		return workspaceIdleCandidate{}, reason, false
	}
	session, reason, ok := s.workspaceIdleSession(ctx, current, task)
	if !ok {
		return workspaceIdleCandidate{}, reason, false
	}
	if s.sessionHasKnownIdleWork(ctx, current.SessionID) {
		return workspaceIdleCandidate{}, idleParkingSkipKnownWork, false
	}
	executionID, generation, activityEpoch, lastActivityAt, reason, ok := s.workspaceIdleActivity(ctx, current, session, workspace, now)
	if !ok {
		return workspaceIdleCandidate{}, reason, false
	}
	return buildWorkspaceIdleCandidate(current, task, workspace, executionID, generation, activityEpoch, lastActivityAt), "", true
}

func (s *Service) currentWorkspaceIdleExecutor(
	ctx context.Context,
	row *models.ExecutorRunning,
) (*models.ExecutorRunning, idleParkingSkipReason, bool) {
	if row == nil || row.SessionID == "" || row.TaskID == "" || row.AgentExecutionID == "" ||
		row.IdleSuspensionState != models.ExecutorIdleSuspensionNone || !idleSuspensionExecutorStatus(row.Status) {
		return nil, idleParkingSkipIdentityUnavailable, false
	}
	latest, err := s.repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	if err != nil || latest == nil || latest.AgentExecutionID != row.AgentExecutionID ||
		latest.UpdatedAt != row.UpdatedAt || latest.IdleSuspensionState != models.ExecutorIdleSuspensionNone ||
		!idleSuspensionExecutorStatus(latest.Status) {
		return nil, idleParkingSkipIdentityUnavailable, false
	}
	if !latest.Resumable || latest.ResumeToken == "" {
		return nil, idleParkingSkipRestoreUnavailable, false
	}
	return latest, "", true
}

func (s *Service) workspaceIdleTask(ctx context.Context, row *models.ExecutorRunning) (*models.Task, idleParkingSkipReason, bool) {
	task, err := s.repo.GetTask(ctx, row.TaskID)
	if err != nil || task == nil || task.ArchivedAt != nil {
		return nil, idleParkingSkipOwnershipProtected, false
	}
	if task.WorkspaceID == "" {
		return nil, idleParkingSkipWorkspaceUnavailable, false
	}
	isOfficeTask, err := s.lookupOfficeTask(ctx, row.TaskID)
	if err != nil || isOfficeTask {
		return nil, idleParkingSkipOwnershipProtected, false
	}
	return task, "", true
}

func (s *Service) workspaceIdlePolicy(ctx context.Context, task *models.Task) (*models.Workspace, idleParkingSkipReason, bool) {
	workspace, err := s.repo.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil || workspace == nil || workspace.ACPIdleTimeoutMinutes <= 0 {
		return nil, idleParkingSkipWorkspaceUnavailable, false
	}
	if !workspace.ACPIdleSuspensionEnabled {
		return nil, idleParkingSkipPolicyDisabled, false
	}
	return workspace, "", true
}

func (s *Service) workspaceIdleSession(ctx context.Context, row *models.ExecutorRunning, task *models.Task) (*models.TaskSession, idleParkingSkipReason, bool) {
	session, err := s.repo.GetTaskSession(ctx, row.SessionID)
	if err != nil || session == nil || !idleTaskSessionState(session.State) {
		return nil, idleParkingSkipSessionState, false
	}
	if _, parked := models.LoadWorkflowParking(session.Metadata); parked {
		return nil, idleParkingSkipOwnershipProtected, false
	}
	if allowed, _ := s.autoResumeEligibility(ctx, task, session); !allowed {
		return nil, idleParkingSkipOwnershipProtected, false
	}
	return session, "", true
}

func (s *Service) workspaceIdleActivity(
	ctx context.Context,
	row *models.ExecutorRunning,
	session *models.TaskSession,
	workspace *models.Workspace,
	now time.Time,
) (string, uint64, uint64, time.Time, idleParkingSkipReason, bool) {
	reader, ok := s.agentManager.(idlePromptActivityReader)
	if !ok {
		return "", 0, 0, time.Time{}, idleParkingSkipActivityUnavailable, false
	}
	executionID, generation, activityEpoch, lastActivityAt, err := reader.GetPromptActivityForSession(ctx, row.SessionID)
	if err != nil || executionID != row.AgentExecutionID || activityEpoch == 0 || lastActivityAt.IsZero() || lastActivityAt.After(now) {
		return "", 0, 0, time.Time{}, idleParkingSkipStaleActivity, false
	}
	lastActivityAt = latestIdleActivity(lastActivityAt, session.UpdatedAt, row.UpdatedAt, workspace.UpdatedAt)
	if lastActivityAt.After(now) {
		return "", 0, 0, time.Time{}, idleParkingSkipStaleActivity, false
	}
	return executionID, generation, activityEpoch, lastActivityAt, "", true
}

func latestIdleActivity(activityAt time.Time, activitySources ...time.Time) time.Time {
	for _, candidate := range activitySources {
		if candidate.After(activityAt) {
			activityAt = candidate
		}
	}
	return activityAt
}

func buildWorkspaceIdleCandidate(
	row *models.ExecutorRunning,
	task *models.Task,
	workspace *models.Workspace,
	executionID string,
	generation, activityEpoch uint64,
	lastActivityAt time.Time,
) workspaceIdleCandidate {
	return workspaceIdleCandidate{
		key: idleParkingCandidateKey{
			workspaceID: task.WorkspaceID, sessionID: row.SessionID, executionID: executionID,
			policyUpdated: workspace.UpdatedAt.UTC().UnixNano(), timeoutMinutes: workspace.ACPIdleTimeoutMinutes,
			idleSince: lastActivityAt.UTC().UnixNano(), promptGen: generation, activityEpoch: activityEpoch,
		},
		identity: agentruntime.IdleSuspensionIdentity{
			ExecutionID: executionID, SessionID: row.SessionID,
			WorkspaceID: task.WorkspaceID, PolicyUpdatedAt: workspace.UpdatedAt.UTC(),
			PromptGeneration: generation, ActivityEpoch: activityEpoch,
		},
		timeout: idleParkingTimeout(workspace.ACPIdleTimeoutMinutes),
	}
}

func (s *Service) sessionHasKnownIdleWork(ctx context.Context, sessionID string) bool {
	if s == nil || s.agentManager == nil || sessionID == "" || s.sessionHasActiveTurn(ctx, sessionID) ||
		(s.lspLeases != nil && s.lspLeases.HasActiveLSPLease(sessionID)) || s.hasKnownBackgroundWork(sessionID) {
		return true
	}
	clarifications, err := s.repo.FindActiveClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil || len(clarifications) > 0 {
		return true
	}
	permissions, err := s.agentManager.ListPendingPermissionsBySessionID(ctx, sessionID)
	if err != nil || len(permissions) > 0 {
		return true
	}
	hasPending, err := s.messageQueue.HasPendingForSession(ctx, sessionID)
	return err != nil || hasPending
}

func (s *Service) hasKnownBackgroundWork(sessionID string) bool {
	activity := s.lockTurnActivity(sessionID, false)
	if activity == nil {
		return false
	}
	defer activity.mu.Unlock()
	return len(activity.background) > 0
}

func idleParkingTimeout(minutes int) time.Duration {
	maxMinutes := time.Duration(1<<63-1) / time.Minute
	if time.Duration(minutes) > maxMinutes {
		return time.Duration(1<<63 - 1)
	}
	return time.Duration(minutes) * time.Minute
}

func idleTaskSessionState(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateWaitingForInput || state == models.TaskSessionStateIdle || state == models.TaskSessionStateCompleted
}

func idleSuspensionExecutorStatus(status string) bool {
	return status == models.ExecutorRunningStatusRunning ||
		status == models.ExecutorRunningStatusReady ||
		status == models.ExecutorRunningStatusComplete
}

func (s *Service) idleParkingCandidateDue(key idleParkingCandidateKey, now time.Time, timeout time.Duration) bool {
	s.idleParkingMu.Lock()
	defer s.idleParkingMu.Unlock()
	if s.idleParkingCandidates == nil {
		s.idleParkingCandidates = make(map[idleParkingCandidateKey]time.Time)
	}
	started, exists := s.idleParkingCandidates[key]
	if !exists {
		started = time.Unix(0, key.idleSince).UTC()
		if policyAt := time.Unix(0, key.policyUpdated).UTC(); policyAt.After(started) {
			started = policyAt
		}
		if focusedAt := s.idleParkingFocusAt[key.sessionID]; focusedAt.After(started) {
			started = focusedAt
		}
		s.idleParkingCandidates[key] = started
	}
	return !now.Before(started) && now.Sub(started) >= timeout
}

func (s *Service) claimIdleParkingCandidate(key idleParkingCandidateKey) bool {
	s.idleParkingMu.Lock()
	defer s.idleParkingMu.Unlock()
	if s.idleParkingInFlight == nil {
		s.idleParkingInFlight = make(map[idleParkingCandidateKey]struct{})
	}
	if _, exists := s.idleParkingInFlight[key]; exists {
		return false
	}
	s.idleParkingInFlight[key] = struct{}{}
	return true
}

func (s *Service) finishIdleParkingCandidate(key idleParkingCandidateKey, succeeded bool) {
	s.idleParkingMu.Lock()
	defer s.idleParkingMu.Unlock()
	delete(s.idleParkingInFlight, key)
	if succeeded {
		delete(s.idleParkingCandidates, key)
		delete(s.idleParkingFocusAt, key.sessionID)
	}
}

func (s *Service) pruneIdleParkingCandidates(observed map[idleParkingCandidateKey]struct{}) {
	s.idleParkingMu.Lock()
	defer s.idleParkingMu.Unlock()
	for key := range s.idleParkingCandidates {
		if _, exists := observed[key]; !exists {
			delete(s.idleParkingCandidates, key)
		}
	}
	observedSessions := make(map[string]struct{}, len(observed))
	for key := range observed {
		observedSessions[key.sessionID] = struct{}{}
	}
	for sessionID := range s.idleParkingFocusAt {
		if _, exists := observedSessions[sessionID]; !exists {
			delete(s.idleParkingFocusAt, sessionID)
		}
	}
}

func (s *Service) resetIdleParkingCandidates(sessionID string) {
	if sessionID == "" {
		return
	}
	now := time.Now().UTC()
	s.idleParkingMu.Lock()
	if s.idleParkingFocusAt == nil {
		s.idleParkingFocusAt = make(map[string]time.Time)
	}
	s.idleParkingFocusAt[sessionID] = now
	for key := range s.idleParkingCandidates {
		if key.sessionID == sessionID {
			s.idleParkingCandidates[key] = now
		}
	}
	s.idleParkingMu.Unlock()
}

func (s *Service) reconcileIdleSuspensionAfterRestart(ctx context.Context, row *models.ExecutorRunning) bool {
	if row == nil || (row.IdleSuspensionState != models.ExecutorIdleSuspensionInProgress &&
		row.IdleSuspensionState != models.ExecutorIdleSuspensionAgentStopped) ||
		(s.lspLeases != nil && s.lspLeases.HasActiveLSPLease(row.SessionID)) {
		return false
	}
	if s.idleParkingSessionInFlight(row.SessionID) {
		return false
	}
	live, err := s.probeAgentRunning(ctx, row.SessionID)
	if err != nil {
		return false
	}
	cas, ok := s.repo.(idleSuspensionStateCAS)
	if !ok {
		return false
	}
	policyCurrent := s.idleSuspensionClaimPolicyCurrent(ctx, row)
	if row.IdleSuspensionState == models.ExecutorIdleSuspensionInProgress && live {
		if !policyCurrent {
			return s.releaseLiveIdleSuspensionClaim(ctx, row, cas)
		}
		return s.finishRestartedIdleSuspension(ctx, row, true)
	}
	if live {
		return false
	}
	// Once liveness proves the process stopped, complete its claim. Disabling
	// policy invalidates only claims whose agent process is still live.
	return s.finishRestartedIdleSuspension(ctx, row, false)
}

func (s *Service) idleParkingSessionInFlight(sessionID string) bool {
	s.idleParkingMu.Lock()
	defer s.idleParkingMu.Unlock()
	for key := range s.idleParkingInFlight {
		if key.sessionID == sessionID {
			return true
		}
	}
	return false
}

func (s *Service) idleSuspensionClaimPolicyCurrent(ctx context.Context, row *models.ExecutorRunning) bool {
	if row == nil || row.IdleSuspensionPolicyUpdatedAt.IsZero() || row.TaskID == "" {
		return false
	}
	task, err := s.repo.GetTask(ctx, row.TaskID)
	if err != nil || task == nil || task.WorkspaceID == "" {
		return false
	}
	workspace, err := s.repo.GetWorkspace(ctx, task.WorkspaceID)
	return err == nil && workspace != nil && workspace.ACPIdleSuspensionEnabled &&
		workspace.UpdatedAt.Equal(row.IdleSuspensionPolicyUpdatedAt)
}

func (s *Service) releaseLiveIdleSuspensionClaim(
	ctx context.Context,
	row *models.ExecutorRunning,
	cas idleSuspensionStateCAS,
) bool {
	if err := cas.CompareAndSetExecutorRunningIdleSuspension(
		ctx, row.SessionID, row.AgentExecutionID, row.UpdatedAt,
		models.ExecutorIdleSuspensionInProgress, models.ExecutorIdleSuspensionNone,
	); err != nil {
		return false
	}
	recovery, ok := s.agentManager.(interface {
		CancelIdleSuspension(context.Context, string, string) error
	})
	if !ok {
		return true
	}
	if err := recovery.CancelIdleSuspension(ctx, row.SessionID, row.AgentExecutionID); err != nil {
		s.logger.Warn("failed to release in-memory idle suspension fence",
			zap.String("session_id", row.SessionID), zap.Error(err))
		return false
	}
	return true
}

func (s *Service) finishRestartedIdleSuspension(
	ctx context.Context,
	row *models.ExecutorRunning,
	requireCurrentPolicy bool,
) bool {
	suspender, ok := s.agentManager.(idleSuspender)
	if !ok || suspender == nil {
		return false
	}
	task, err := s.repo.GetTask(ctx, row.TaskID)
	if err != nil || task == nil || task.WorkspaceID == "" {
		return false
	}
	identity := agentruntime.IdleSuspensionIdentity{
		ExecutionID: row.AgentExecutionID, SessionID: row.SessionID,
		WorkspaceID: task.WorkspaceID, PolicyUpdatedAt: row.IdleSuspensionPolicyUpdatedAt,
		RequireCurrentPolicy: requireCurrentPolicy,
	}
	if reader, ok := s.agentManager.(idlePromptActivityReader); ok {
		executionID, generation, activityEpoch, _, activityErr := reader.GetPromptActivityForSession(ctx, row.SessionID)
		if activityErr == nil {
			if executionID != row.AgentExecutionID {
				return false
			}
			identity.PromptGeneration = generation
			identity.ActivityEpoch = activityEpoch
		}
	}
	operationCtx, cancel := context.WithTimeout(ctx, idleSuspensionOperationTimeout)
	defer cancel()
	if err := suspender.SuspendIdle(operationCtx, identity); err != nil {
		return false
	}
	return true
}

// idleExecutorCandidateLister is implemented by the durable repository. The
// fallback to ListExecutorsRunning keeps lightweight test doubles compatible,
// while production can filter stopped and young rows in SQL.
type idleExecutorCandidateLister interface {
	ListExecutorsRunningIdle(ctx context.Context, cutoff time.Time) ([]*models.ExecutorRunning, error)
}
