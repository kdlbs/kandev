// Package infra provides infrastructure-level background jobs for the office
// domain, including garbage collection and reconciliation.
package infra

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// DefaultGCInterval is the default interval between GC sweeps.
const DefaultGCInterval = 3 * time.Hour

// WorktreeGracePeriod is the minimum age a directory must have before the
// worktree sweep will consider it for deletion. It covers the race where a
// worktree directory is created on disk before its DB row is inserted, plus
// operator-created scratch directories.
const WorktreeGracePeriod = 24 * time.Hour

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

// WorktreeInventory provides the authoritative list of live worktree paths
// the GC must not delete. Implemented by *worktree.Manager.
type WorktreeInventory interface {
	ListActiveWorktreePaths(ctx context.Context) ([]string, error)
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
	worktreeInv  WorktreeInventory
	worktreeBase string
	dockerClient DockerClient
	interval     time.Duration
	logger       *logger.Logger
}

// NewGarbageCollector creates a new GarbageCollector.
// If dockerClient is nil, container sweeps are skipped.
// If worktreeInv is nil, worktree sweeps are skipped (defensive fail-closed:
// without an authoritative inventory the GC cannot safely classify dirs).
func NewGarbageCollector(
	repo *sqlite.Repository,
	worktreeInv WorktreeInventory,
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
		worktreeInv:  worktreeInv,
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

// sweepWorktrees scans the worktree base and deletes directories that
// neither appear in the authoritative live-worktrees inventory nor sit
// above one. Deletion requires a positive orphan signal: the directory is
// absent from the inventory (and any ancestor set) AND its mtime is older
// than WorktreeGracePeriod. Any error or uncertain signal keeps the
// directory.
func (gc *GarbageCollector) sweepWorktrees(ctx context.Context) GCSweepResult {
	var result GCSweepResult
	if gc.worktreeBase == "" {
		return result
	}
	if gc.worktreeInv == nil {
		gc.logger.Warn("worktree inventory not configured; skipping worktree sweep")
		return result
	}

	base, err := filepath.Abs(filepath.Clean(gc.worktreeBase))
	if err != nil {
		result.Errors = append(result.Errors, "abs worktree base: "+err.Error())
		return result
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return result
		}
		result.Errors = append(result.Errors, "read worktree dir: "+err.Error())
		return result
	}

	livePaths, err := gc.worktreeInv.ListActiveWorktreePaths(ctx)
	if err != nil {
		// Fail-closed: without a trusted inventory we never delete.
		result.Errors = append(result.Errors, "list active worktrees: "+err.Error())
		return result
	}

	liveSet, ancestorSet := buildLiveAndAncestorSets(base, livePaths)
	cutoff := time.Now().Add(-WorktreeGracePeriod)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		absPath := filepath.Join(base, entry.Name())
		if gc.shouldKeepWorktreeDir(absPath, entry, liveSet, ancestorSet, cutoff, &result) {
			result.WorktreesKept++
			continue
		}
		gc.deleteWorktreeDir(absPath, &result)
	}

	return result
}

// shouldKeepWorktreeDir returns true if the directory must be retained.
// Deletion requires a positive orphan signal — absence from both the live
// set and the ancestor set, plus an mtime older than the grace period. Any
// stat error keeps the directory (fail-closed).
func (gc *GarbageCollector) shouldKeepWorktreeDir(
	absPath string, entry os.DirEntry,
	liveSet, ancestorSet map[string]struct{}, cutoff time.Time,
	result *GCSweepResult,
) bool {
	if _, ok := liveSet[absPath]; ok {
		return true
	}
	if _, ok := ancestorSet[absPath]; ok {
		return true
	}
	info, err := entry.Info()
	if err != nil {
		result.Errors = append(result.Errors, "stat "+absPath+": "+err.Error())
		return true
	}
	return info.ModTime().After(cutoff)
}

// buildLiveAndAncestorSets normalizes each live path to absolute+clean form
// and returns the set of live paths plus the set of every ancestor directory
// of every live path (walked upward until — but not including — the
// worktree base). The ancestor set covers the multi-repo layout
// {base}/{taskDir}/{repoName} where the sweep iterates at {base} depth-1
// and must keep {taskDir} alive when {taskDir}/{repoName} is live.
func buildLiveAndAncestorSets(base string, livePaths []string) (map[string]struct{}, map[string]struct{}) {
	liveSet := make(map[string]struct{}, len(livePaths))
	ancestorSet := make(map[string]struct{})
	for _, p := range livePaths {
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(filepath.Clean(p))
		if err != nil {
			continue
		}
		liveSet[abs] = struct{}{}
		parent := filepath.Dir(abs)
		for parent != base && parent != "/" && parent != filepath.Dir(parent) {
			ancestorSet[parent] = struct{}{}
			parent = filepath.Dir(parent)
		}
	}
	return liveSet, ancestorSet
}

// deleteWorktreeDir removes a worktree directory and logs the action.
func (gc *GarbageCollector) deleteWorktreeDir(absPath string, result *GCSweepResult) {
	gc.logger.Info("removing orphaned worktree directory", zap.String("path", absPath))
	if err := os.RemoveAll(absPath); err != nil {
		result.Errors = append(result.Errors, "remove worktree "+absPath+": "+err.Error())
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
// Fail-closed: a DB lookup error is treated as "unknown — keep", not
// "orphan — remove." Removal requires a positive signal: either the task
// row is provably absent (sqlite.ErrTaskNotFound) or the task is in a
// terminal state and the container is no longer running.
func (gc *GarbageCollector) shouldRemoveContainer(ctx context.Context, ctr GCContainerInfo) bool {
	sessionID := ctr.Labels["kandev.session_id"]
	taskID := ctr.Labels["kandev.task_id"]

	if taskID == "" {
		return false
	}

	fields, err := gc.repo.GetTaskExecutionFields(ctx, taskID)
	if errors.Is(err, sqlite.ErrTaskNotFound) {
		gc.logger.Info("container references unknown task (orphan)",
			zap.String("container_id", ctr.ID),
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		return true
	}
	if err != nil {
		gc.logger.Warn("container task lookup failed; keeping container",
			zap.String("container_id", ctr.ID),
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return false
	}
	if fields == nil {
		return false
	}
	if !terminalTaskStates[fields.State] {
		return false
	}
	return ctr.State != "running"
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
