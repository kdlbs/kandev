package orchestrator

import (
	"context"
	"errors"
	"fmt"

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
func (s *Service) repairDeadRowLiveness(ctx context.Context, running *models.ExecutorRunning) {
	if err := s.repo.RepairExecutorRunningDead(ctx, running.SessionID); err != nil &&
		!errors.Is(err, models.ErrExecutorRunningNotFound) {
		s.logger.Warn("failed to repair executor row liveness during reconciliation",
			zap.String("session_id", running.SessionID),
			zap.Error(err))
	}
}
