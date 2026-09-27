package runtime

import (
	"context"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// OrphanSweepExecutorStore mirrors lifecycle's own (intentionally unexported)
// executor-read surface so callers outside the runtime seam never need to
// import internal/agent/runtime/lifecycle directly.
type OrphanSweepExecutorStore interface {
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
	ListSSHExecutorsForReachability(ctx context.Context) ([]*models.Executor, error)
	GetExecutorReachability(ctx context.Context, executorID string) (*models.ExecutorReachability, error)
}

// OrphanSweepTaskStore mirrors lifecycle's own (intentionally unexported)
// task/session-read surface used to attribute and gate orphan candidates.
type OrphanSweepTaskStore interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	ListExecutorsRunningByTaskID(ctx context.Context, taskID string) ([]*models.ExecutorRunning, error)
}

// OrphanSweepScheduler is the runtime-seam handle for the SSH orphaned-agentctl
// sweep scheduler; Stop is the only capability callers outside the runtime
// seam need.
type OrphanSweepScheduler struct {
	inner *lifecycle.OrphanSweepScheduler
}

// Stop cancels the scheduler and waits for every in-flight sweep to drain.
func (s *OrphanSweepScheduler) Stop() {
	s.inner.Stop()
}

// StartOrphanSweepScheduler builds and starts the SSH orphaned-agentctl sweep
// scheduler behind the runtime seam, per the "only internal/agent/runtime/
// may import runtime/lifecycle directly" convention: it runs
// sweepSSHExecutorOrphans against SSH executors, once off-cycle whenever an
// executor becomes reachable and on a slow interval as a backstop.
func StartOrphanSweepScheduler(
	ctx context.Context,
	executors OrphanSweepExecutorStore,
	tasks OrphanSweepTaskStore,
	intervalSeconds int,
	log *logger.Logger,
	eventBus bus.EventBus,
) *OrphanSweepScheduler {
	scheduler := lifecycle.NewOrphanSweepScheduler(executors, tasks, intervalSeconds, log)
	scheduler.Start(ctx, eventBus)
	return &OrphanSweepScheduler{inner: scheduler}
}
