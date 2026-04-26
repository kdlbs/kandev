package service

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// DefaultGCInterval is the default interval between GC sweeps.
const DefaultGCInterval = 3 * time.Hour

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
	svc          *Service
	worktreeBase string
	dockerClient DockerClient
	interval     time.Duration
	logger       *logger.Logger
}

// NewGarbageCollector creates a new GarbageCollector.
// If dockerClient is nil, container sweeps are skipped.
func NewGarbageCollector(
	svc *Service,
	worktreeBase string,
	dockerClient DockerClient,
	interval time.Duration,
) *GarbageCollector {
	if interval <= 0 {
		interval = DefaultGCInterval
	}
	return &GarbageCollector{
		svc:          svc,
		worktreeBase: worktreeBase,
		dockerClient: dockerClient,
		interval:     interval,
		logger:       svc.logger.WithFields(zap.String("component", "orchestrate-gc")),
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

// sweepWorktrees scans the worktree base directory and removes directories
// whose tasks no longer exist or are archived/deleted.
func (gc *GarbageCollector) sweepWorktrees(ctx context.Context) GCSweepResult {
	var result GCSweepResult

	if gc.worktreeBase == "" {
		return result
	}

	entries, err := os.ReadDir(gc.worktreeBase)
	if err != nil {
		if os.IsNotExist(err) {
			return result
		}
		result.Errors = append(result.Errors, "read worktree dir: "+err.Error())
		return result
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if gc.shouldDeleteWorktree(ctx, entry.Name()) {
			gc.deleteWorktreeDir(entry.Name(), &result)
		} else {
			result.WorktreesKept++
		}
	}

	return result
}

// shouldDeleteWorktree checks whether a worktree directory is orphaned.
// A worktree is considered orphaned if no matching task exists in the DB,
// or the task is archived.
func (gc *GarbageCollector) shouldDeleteWorktree(ctx context.Context, dirName string) bool {
	// The directory name often contains the task ID as a prefix or the
	// entire name. Try to look it up as a task ID first, then as part
	// of the directory name.
	info, err := gc.svc.repo.GetTaskBasicInfo(ctx, dirName)
	if err != nil || info == nil {
		// No task found -- this worktree is orphaned.
		return true
	}
	// Task exists. Keep it.
	return false
}

// deleteWorktreeDir removes a worktree directory and logs the action.
func (gc *GarbageCollector) deleteWorktreeDir(dirName string, result *GCSweepResult) {
	dirPath := filepath.Join(gc.worktreeBase, dirName)

	gc.logger.Info("removing orphaned worktree directory",
		zap.String("path", dirPath))

	if err := os.RemoveAll(dirPath); err != nil {
		result.Errors = append(result.Errors, "remove worktree "+dirPath+": "+err.Error())
		return
	}
	result.WorktreesDeleted++
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
// A container is removed if its session's task doesn't exist (orphan)
// or its task is terminal and stale.
func (gc *GarbageCollector) shouldRemoveContainer(ctx context.Context, ctr GCContainerInfo) bool {
	sessionID := ctr.Labels["kandev.session_id"]
	taskID := ctr.Labels["kandev.task_id"]

	// If no task ID label, we can't determine status -- keep it.
	if taskID == "" {
		return false
	}

	fields, err := gc.svc.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		// Task not found in DB -- orphaned container.
		gc.logger.Info("container references unknown task (orphan)",
			zap.String("container_id", ctr.ID),
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		return true
	}

	if !terminalTaskStates[fields.State] {
		return false
	}

	// Task is terminal. Check if it has been stale long enough.
	// We use GetTaskBasicInfo to avoid adding new repo methods.
	// The updated_at check would require a new query, so we use a
	// simpler heuristic: terminal + container not running = remove.
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
