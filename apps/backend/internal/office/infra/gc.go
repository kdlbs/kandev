// Package infra provides infrastructure-level background jobs for the office
// domain, including garbage collection and reconciliation.
package infra

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// DefaultGCInterval is the default interval between GC sweeps.
const DefaultGCInterval = 3 * time.Hour

// terminalTaskStates lists task states that are considered terminal.
var terminalTaskStates = map[string]bool{
	"COMPLETED": true,
	"CANCELLED": true,
	"FAILED":    true,
}

// DockerClient is the interface used by GarbageCollector for container
// operations. It is satisfied by *docker.Client and can be mocked in tests.
type DockerClient interface {
	ListContainers(ctx context.Context, labels map[string]string) ([]GCContainerInfo, error)
	RemoveContainer(ctx context.Context, containerID string, force bool) error
}

// GCContainerInfo holds the minimal container data needed by the GC sweep.
// This decouples GC from the docker package's ContainerInfo type.
type GCContainerInfo struct {
	ID     string
	Name   string
	State  string
	Labels map[string]string
}

// GCSweepResult holds the outcome of a single GC sweep.
type GCSweepResult struct {
	WorktreesDeleted  int
	WorktreesKept     int
	ContainersRemoved int
	ContainersKept    int
	Errors            []string
}

// GarbageCollector periodically sweeps orphaned worktrees and stale Docker
// containers that are no longer needed by any active task.
type GarbageCollector struct {
	repo         *sqlite.Repository
	worktreeBase string
	dockerClient DockerClient
	interval     time.Duration
	logger       *logger.Logger
}

// NewGarbageCollector creates a new GarbageCollector.
// If dockerClient is nil, container sweeps are skipped.
func NewGarbageCollector(
	repo *sqlite.Repository,
	log *logger.Logger,
	worktreeBase string,
	dockerClient DockerClient,
	interval time.Duration,
) *GarbageCollector {
	if interval <= 0 {
		interval = DefaultGCInterval
	}
	return &GarbageCollector{
		repo:         repo,
		worktreeBase: worktreeBase,
		dockerClient: dockerClient,
		interval:     interval,
		logger:       log.WithFields(zap.String("component", "office-gc")),
	}
}

// Start runs an initial sweep immediately, then repeats every interval
// until the context is cancelled. It should be called in a goroutine.
func (gc *GarbageCollector) Start(ctx context.Context) {
	gc.logger.Info("GC sweep loop starting",
		zap.Duration("interval", gc.interval),
		zap.String("worktree_base", gc.worktreeBase))

	// Run immediately on startup.
	result := gc.Sweep(ctx)
	gc.logResult(result)

	ticker := time.NewTicker(gc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			gc.logger.Info("GC sweep loop stopping")
			return
		case <-ticker.C:
			result := gc.Sweep(ctx)
			gc.logResult(result)
		}
	}
}

// Sweep performs a single GC sweep: worktrees first, then containers.
func (gc *GarbageCollector) Sweep(ctx context.Context) GCSweepResult {
	var result GCSweepResult

	wtResult := gc.sweepWorktrees(ctx)
	result.WorktreesDeleted = wtResult.WorktreesDeleted
	result.WorktreesKept = wtResult.WorktreesKept
	result.Errors = append(result.Errors, wtResult.Errors...)

	ctrResult := gc.sweepContainers(ctx)
	result.ContainersRemoved = ctrResult.ContainersRemoved
	result.ContainersKept = ctrResult.ContainersKept
	result.Errors = append(result.Errors, ctrResult.Errors...)

	return result
}

// sweepWorktrees is temporarily a no-op. The previous algorithm passed the
// worktree directory slug to GetTaskBasicInfo (which keys on tasks.id UUID),
// so every directory was classified as orphaned and removed. A redesigned,
// inventory-driven implementation lands in a follow-up commit.
// See docs/specs/office-gc-worktree-safety/spec.md.
func (gc *GarbageCollector) sweepWorktrees(_ context.Context) GCSweepResult {
	return GCSweepResult{}
}

// sweepContainers lists kandev-managed Docker containers and removes
// orphaned or stale ones.
func (gc *GarbageCollector) sweepContainers(ctx context.Context) GCSweepResult {
	var result GCSweepResult

	if gc.dockerClient == nil {
		return result
	}

	containers, err := gc.dockerClient.ListContainers(ctx, map[string]string{
		"kandev.managed": "true",
	})
	if err != nil {
		result.Errors = append(result.Errors, "list containers: "+err.Error())
		return result
	}

	for _, ctr := range containers {
		if gc.shouldRemoveContainer(ctx, ctr) {
			gc.removeContainer(ctx, ctr, &result)
		} else {
			result.ContainersKept++
		}
	}

	return result
}

// shouldRemoveContainer decides if a container should be removed.
func (gc *GarbageCollector) shouldRemoveContainer(ctx context.Context, ctr GCContainerInfo) bool {
	sessionID := ctr.Labels["kandev.session_id"]
	taskID := ctr.Labels["kandev.task_id"]

	if taskID == "" {
		return false
	}

	fields, err := gc.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		gc.logger.Info("container references unknown task (orphan)",
			zap.String("container_id", ctr.ID),
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		return true
	}

	if !terminalTaskStates[fields.State] {
		return false
	}

	if ctr.State != "running" {
		return true
	}

	return false
}

// removeContainer forcefully removes a container and logs the action.
func (gc *GarbageCollector) removeContainer(
	ctx context.Context, ctr GCContainerInfo, result *GCSweepResult,
) {
	gc.logger.Info("removing stale container",
		zap.String("container_id", ctr.ID),
		zap.String("container_name", ctr.Name),
		zap.String("task_id", ctr.Labels["kandev.task_id"]),
		zap.String("session_id", ctr.Labels["kandev.session_id"]))

	if err := gc.dockerClient.RemoveContainer(ctx, ctr.ID, true); err != nil {
		result.Errors = append(result.Errors, "remove container "+ctr.ID+": "+err.Error())
		return
	}
	result.ContainersRemoved++
}

// logResult logs the GC sweep outcome.
func (gc *GarbageCollector) logResult(result GCSweepResult) {
	gc.logger.Info("GC sweep completed",
		zap.Int("worktrees_deleted", result.WorktreesDeleted),
		zap.Int("worktrees_kept", result.WorktreesKept),
		zap.Int("containers_removed", result.ContainersRemoved),
		zap.Int("containers_kept", result.ContainersKept),
		zap.Int("errors", len(result.Errors)))

	for _, e := range result.Errors {
		gc.logger.Warn("GC sweep error", zap.String("error", e))
	}
}
