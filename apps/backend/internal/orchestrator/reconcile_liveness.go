package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// rowLivenessProber is the optional capability the orchestrator uses to classify
// an executors_running row's backing-process liveness in a runtime-aware way. It
// is satisfied by the lifecycle adapter (backendapp.lifecycleAdapter).
//
// Kept as a narrow optional interface — type-asserted off s.agentManager like
// executor.ExecutorTypeCapabilities — so the large AgentManagerClient interface
// and its many test mocks don't have to grow a method just for reconciliation.
type rowLivenessProber interface {
	RowLiveness(row *models.ExecutorRunning) models.ProcessLiveness
}

// agentRunningProber preserves runtime probe errors so reclaim can fail
// closed. The legacy boolean method remains the fallback for test doubles and
// adapters that do not expose the richer probe yet.
type agentRunningProber interface {
	ProbeAgentRunningForSession(ctx context.Context, sessionID string) (bool, error)
}

type startupCleanupDisposition string

const (
	startupCleanupDispositionStopped       startupCleanupDisposition = "stopped"
	startupCleanupDispositionAlreadyAbsent startupCleanupDisposition = "already_absent"
	startupCleanupDispositionPreserved     startupCleanupDisposition = "preserved"
)

type startupCleanupDecision struct {
	disposition    startupCleanupDisposition
	liveness       string
	stopErrorClass string
	proceed        bool
	expected       bool
}

// classifyStartupCleanup is the single decision matrix for startup stop
// results. Only a typed runtime absence plus confirmed-dead local liveness
// permits cleanup to continue; every uncertain result remains preserved.
func classifyStartupCleanup(err error, liveness models.ProcessLiveness) startupCleanupDecision {
	decision := startupCleanupDecision{
		disposition:    startupCleanupDispositionPreserved,
		liveness:       processLivenessClass(liveness),
		stopErrorClass: "none",
	}
	if err == nil {
		decision.disposition = startupCleanupDispositionStopped
		decision.proceed = true
		return decision
	}
	if stopReportsRuntimeAbsent(err) {
		decision.stopErrorClass = "runtime_not_found"
		if liveness == models.ProcessLivenessDead {
			decision.disposition = startupCleanupDispositionAlreadyAbsent
			decision.proceed = true
			return decision
		}
		decision.expected = true
		return decision
	}
	decision.stopErrorClass = "unexpected"
	return decision
}

func processLivenessClass(liveness models.ProcessLiveness) string {
	switch liveness {
	case models.ProcessLivenessAlive:
		return "alive"
	case models.ProcessLivenessDead:
		return "dead"
	default:
		return "unknown"
	}
}

type startupCleanupReport struct {
	expectedPreservedCount int
	expectedOutcomes       map[string]int
}

func newStartupCleanupReport() *startupCleanupReport {
	return &startupCleanupReport{expectedOutcomes: make(map[string]int)}
}

func (r *startupCleanupReport) recordExpected(running *models.ExecutorRunning, decision startupCleanupDecision) {
	if r == nil {
		return
	}
	key := fmt.Sprintf("disposition=%s,liveness=%s,stop_error=%s,local_pid=%t",
		decision.disposition, decision.liveness, decision.stopErrorClass,
		running != nil && running.LocalPID > 0)
	r.expectedPreservedCount++
	r.expectedOutcomes[key]++
}

func (r *startupCleanupReport) flush(log *logger.Logger) {
	if r == nil || r.expectedPreservedCount == 0 {
		return
	}
	log.Warn("startup runtime cleanup summary",
		zap.Int("expected_preserved_count", r.expectedPreservedCount),
		zap.Any("expected_preserved_outcomes", r.expectedOutcomes))
}

func startupCleanupFields(running *models.ExecutorRunning, decision startupCleanupDecision) []zap.Field {
	fields := []zap.Field{
		zap.String("liveness", decision.liveness),
		zap.String("stop_error_class", decision.stopErrorClass),
		zap.String("disposition", string(decision.disposition)),
		zap.Bool("local_pid_present", running != nil && running.LocalPID > 0),
	}
	if running == nil {
		return fields
	}
	return append(fields,
		zap.String("session_id", running.SessionID),
		zap.String("task_id", running.TaskID),
		zap.String("execution_id", running.AgentExecutionID),
	)
}

// stopReportsRuntimeAbsent accepts only typed runtime/executor absence
// sentinels. Generic lookup errors remain retryable failures.
func stopReportsRuntimeAbsent(err error) bool {
	return errors.Is(err, runtimeapi.ErrNotFound) ||
		errors.Is(err, executor.ErrExecutionNotFound)
}

// rowLiveness classifies a row's backing-process liveness, returning Unknown when
// no prober is wired (unit tests, degraded startup) so a caller never mistakes
// "can't probe" for "dead". The probe is runtime-aware: a local process check
// never runs against a remote/SSH row (#1597 runtime-aware liveness).
func (s *Service) rowLiveness(row *models.ExecutorRunning) models.ProcessLiveness {
	prober, ok := s.agentManager.(rowLivenessProber)
	if !ok || prober == nil {
		return models.ProcessLivenessUnknown
	}
	return prober.RowLiveness(row)
}

// pruneOrRepairExecutorRow enforces the resume-safety deletion invariant
// (#1597 resume-safety invariant) at a reconciliation cleanup site: a row backing
// a resumable session, or holding a resume_token, is repaired in place (never
// deleted); only a row that is neither is pruned.
func (s *Service) pruneOrRepairExecutorRow(ctx context.Context, running *models.ExecutorRunning, sessionState models.TaskSessionState) {
	sessionID := running.SessionID
	if models.RowMustBePreserved(running, sessionState) {
		s.repairDeadRowLiveness(ctx, running)
		return
	}
	if err := s.repo.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil &&
		!errors.Is(err, models.ErrExecutorRunningNotFound) {
		s.logger.Warn("failed to prune terminal executor row during reconciliation",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// repairDeadRowLiveness repairs a preserved row so it no longer claims a live
// process — status=stopped, local_pid cleared, resume_token/worktree preserved —
// satisfying #1597's "never leave a row claiming a dead process" expected
// behavior. Best-effort; a missing row is not an error.
func (s *Service) repairDeadRowLiveness(ctx context.Context, running *models.ExecutorRunning) error {
	var err error
	if repairer, ok := s.repo.(executionIdentityRepairer); ok {
		err = repairer.RepairExecutorRunningDeadIfCurrent(
			ctx,
			running.SessionID,
			running.AgentExecutionID,
			running.UpdatedAt,
		)
	} else {
		err = s.repo.RepairExecutorRunningDead(ctx, running.SessionID)
	}
	if errors.Is(err, models.ErrExecutorRunningNotFound) || errors.Is(err, models.ErrExecutionRotated) {
		return nil
	}
	if err != nil {
		s.logger.Warn("failed to repair executor row liveness during reconciliation",
			zap.String("session_id", running.SessionID),
			zap.Error(err))
	}
	return err
}

type executionIdentityRepairer interface {
	RepairExecutorRunningDeadIfCurrent(
		ctx context.Context,
		sessionID, expectedExecutionID string,
		expectedUpdatedAt time.Time,
	) error
}

type executionIdentityCleaner interface {
	CleanupStaleExecutionBySessionIDIfCurrent(
		ctx context.Context,
		sessionID, expectedExecutionID string,
		expectedUpdatedAt time.Time,
	) error
}

func (s *Service) probeAgentRunning(ctx context.Context, sessionID string) (bool, error) {
	if prober, ok := s.agentManager.(agentRunningProber); ok && prober != nil {
		return prober.ProbeAgentRunningForSession(ctx, sessionID)
	}
	return s.agentManager.IsAgentRunningForSession(ctx, sessionID), nil
}

// idleReclaimDisposition captures why a session is or is not eligible for the
// idle-session reclaim path. Mirrors startupCleanupDisposition's role: keep
// every decision logged and observable without leaking resume tokens.
type idleReclaimDisposition string

const (
	idleReclaimDispositionReclaimed    idleReclaimDisposition = "reclaimed"
	idleReclaimDispositionSkippedState idleReclaimDisposition = "skipped_state"
	idleReclaimDispositionSkippedLive  idleReclaimDisposition = "skipped_live_runtime"
	idleReclaimDispositionSkippedTurn  idleReclaimDisposition = "skipped_active_turn"
)

// classifyIdleReclaim is the single decision matrix for the idle-session
// reclaim primitive. Reclaim is fail-closed: only sessions in an idle-yet-
// resumable OR terminal shape AND without a live runtime AND without an
// active turn proceed. Any uncertain signal falls into a skipped
// disposition and the caller leaves the row untouched.
//
// Why include Completed in the allowed set: the subtask terminal collapse
// in setSessionWaitingForInputIfRequested writes session.state=Completed
// when the agent's last turn did not request input, and the parent task
// has no further use for the provider-runtime reservation. Failed and
// Cancelled are deliberately excluded — those have separate cancellation
// cleanup paths (handleTerminalSessionOnStartup and the cancel pipelines)
// that already reconcile executor rows.
func classifyIdleReclaim(sessionState models.TaskSessionState, agentRunning bool, hasActiveTurn bool) idleReclaimDisposition {
	switch sessionState {
	case models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateIdle,
		models.TaskSessionStateCompleted:
	default:
		return idleReclaimDispositionSkippedState
	}
	if agentRunning {
		return idleReclaimDispositionSkippedLive
	}
	if hasActiveTurn {
		return idleReclaimDispositionSkippedTurn
	}
	return idleReclaimDispositionReclaimed
}

// reclaimIdleSession releases the provider-runtime reservation backing an
// idle-yet-resumable or terminal session (state in {WaitingForInput, Idle,
// Completed}) when no live agent process and no active turn can be
// observed. Resume token and worktree path are preserved so a later resume
// can re-attach the session without losing context. Never touches a
// running executor. Synchronous settle points and the periodic reaper use
// the same fail-closed primitive.
//
// Synchronous call sites so far:
//   - setSessionWaitingForInputIfRequested: subtask terminal collapse to
//     Completed when the last agent message did not request input. The
//     child has no further use for the provider runtime.
//   - handleAgentCompleted: after completeTurnForSession, when the
//     session has settled into a non-active terminal/idle state.
//
// Idempotent: a session with no in-memory execution entry returns nil from
// CleanupStaleExecutionBySessionID, and a missing executor row is not an
// error for repairDeadRowLiveness.
func (s *Service) reclaimIdleSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	releaseLifecycleLock := s.acquireSessionLifecycleLock(sessionID)
	defer releaseLifecycleLock()
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		if isTaskSessionNotFound(err) {
			return nil
		}
		return fmt.Errorf("load session for idle reclaim: %w", err)
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		// A missing executor row means there is nothing to release; skip.
		if errors.Is(err, models.ErrExecutorRunningNotFound) {
			return nil
		}
		return fmt.Errorf("load executor row for idle reclaim: %w", err)
	}
	agentRunning, probeErr := s.probeAgentRunning(ctx, sessionID)
	if probeErr != nil {
		s.logger.Warn("idle reclaim skipped; agent liveness probe failed",
			zap.String("session_id", sessionID),
			zap.Error(probeErr))
		return nil
	}
	hasActiveTurn := s.sessionHasActiveTurn(ctx, sessionID)
	decision := classifyIdleReclaim(session.State, agentRunning, hasActiveTurn)
	if decision != idleReclaimDispositionReclaimed {
		s.logger.Debug("idle reclaim skipped",
			zap.String("session_id", sessionID),
			zap.String("disposition", string(decision)),
			zap.String("session_state", string(session.State)),
			zap.Bool("agent_running", agentRunning),
			zap.Bool("has_active_turn", hasActiveTurn))
		return nil
	}
	var cleanupErr error
	if cleaner, ok := s.agentManager.(executionIdentityCleaner); ok {
		cleanupErr = cleaner.CleanupStaleExecutionBySessionIDIfCurrent(
			ctx,
			sessionID,
			running.AgentExecutionID,
			running.UpdatedAt,
		)
	} else {
		cleanupErr = s.agentManager.CleanupStaleExecutionBySessionID(ctx, sessionID)
	}
	if cleanupErr != nil {
		s.logger.Warn("idle reclaim: cleanup failed; row preserved",
			zap.String("session_id", sessionID),
			zap.Error(cleanupErr))
		return nil
	}
	if err := s.repairDeadRowLiveness(ctx, running); err != nil {
		return fmt.Errorf("repair executor row after idle reclaim: %w", err)
	}
	s.logger.Info("idle reclaim: provider runtime released; row preserved for resume",
		zap.String("session_id", sessionID),
		zap.String("task_id", running.TaskID),
		zap.Bool("has_resume_token", running.ResumeToken != ""),
		zap.Bool("has_worktree", running.WorktreePath != ""))
	return nil
}

// sessionHasActiveTurn reports whether the orchestrator has any in-flight
// turn bound to the session. Conservative default (true on lookup error)
// keeps reclaim fail-closed.
func (s *Service) sessionHasActiveTurn(ctx context.Context, sessionID string) bool {
	if s.turnService == nil {
		return true
	}
	turn, err := s.turnService.GetActiveTurn(ctx, sessionID)
	if err != nil {
		return true
	}
	return turn != nil
}
