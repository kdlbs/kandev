package orchestrator

import (
	"context"
	"errors"

	"go.uber.org/zap"

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
