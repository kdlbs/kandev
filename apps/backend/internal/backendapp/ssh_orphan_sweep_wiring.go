package backendapp

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// sshOrphanSweepRepo is the narrow store surface startSSHOrphanSweepScheduler
// needs. It is declared independently rather than referencing the runtime
// seam's own store interfaces — an interface value typed this way still
// satisfies agentruntime.OrphanSweepExecutorStore and
// agentruntime.OrphanSweepTaskStore structurally at the
// agentruntime.StartOrphanSweepScheduler call site. *sqliterepo.Repository
// satisfies it, the same way it already satisfies reachabilitypkg.Repository
// for the poller wired alongside it; a lightweight fake can satisfy it too,
// keeping this helper independently testable.
type sshOrphanSweepRepo interface {
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
	ListSSHExecutorsForReachability(ctx context.Context) ([]*models.Executor, error)
	GetExecutorReachability(ctx context.Context, executorID string) (*models.ExecutorReachability, error)
	GetTask(ctx context.Context, id string) (*models.Task, error)
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	ListExecutorsRunningByTaskID(ctx context.Context, taskID string) ([]*models.ExecutorRunning, error)
	ListExecutorProfiles(ctx context.Context, executorID string) ([]*models.ExecutorProfile, error)
}

// startSSHOrphanSweepScheduler launches the SSH orphaned-agentctl sweep
// scheduler behind the internal/agent/runtime seam and registers its Stop on
// addCleanup, mirroring startSSHReachabilityPoller's wiring shape alongside
// it in startAgentInfrastructure.
func startSSHOrphanSweepScheduler(
	ctx context.Context,
	repo sshOrphanSweepRepo,
	eventBus bus.EventBus,
	log *logger.Logger,
	addCleanup func(func() error) func() error,
) *agentruntime.OrphanSweepScheduler {
	scheduler := agentruntime.StartOrphanSweepScheduler(ctx, repo, repo, 0, log, eventBus)
	addCleanup(func() error { scheduler.Stop(); return nil })
	return scheduler
}
