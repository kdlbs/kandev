package lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
)

// ContainerLiveStatus is a runtime snapshot of a Docker container's state,
// suitable for surfacing in the Executor Settings popover.
type ContainerLiveStatus struct {
	ContainerID string     `json:"container_id"`
	State       string     `json:"state"`  // running, exited, paused, restarting, removing, dead, created
	Status      string     `json:"status"` // human-readable, e.g. "Up 5 minutes"
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	ExitCode    int        `json:"exit_code,omitempty"`
	Health      string     `json:"health,omitempty"`
	Missing     bool       `json:"missing,omitempty"` // true when the container no longer exists
}

// DestroyContainer removes a Docker container (forcefully, along with its filesystem).
// Used by Reset Environment to tear down the container layer without touching worktrees
// or sprite sandboxes.
func (m *Manager) DestroyContainer(ctx context.Context, containerID string) error {
	if containerID == "" {
		return nil
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameDocker)
	if err != nil {
		return fmt.Errorf("docker backend unavailable: %w", err)
	}
	dockerExec, ok := backend.(*DockerExecutor)
	if !ok {
		return fmt.Errorf("docker backend has unexpected type %T", backend)
	}
	// Force lazy initialization here. After a backend restart this method
	// can be the first caller to touch Docker, and a missing daemon should
	// surface as a clear "docker unavailable" error rather than a generic
	// "container manager not initialized".
	if _, _, err := dockerExec.ensureClient(); err != nil {
		return fmt.Errorf("initialize docker backend: %w", err)
	}
	cm := dockerExec.ContainerMgr()
	if cm == nil {
		return fmt.Errorf("docker container manager not initialized")
	}
	return cm.RemoveContainer(ctx, containerID, true)
}

// GetContainerLiveStatus inspects a Docker container and returns a live
// snapshot. Returns Missing=true when the container has been removed.
func (m *Manager) GetContainerLiveStatus(ctx context.Context, containerID string) (*ContainerLiveStatus, error) {
	if containerID == "" {
		return nil, nil
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameDocker)
	if err != nil {
		return nil, fmt.Errorf("docker backend unavailable: %w", err)
	}
	dockerExec, ok := backend.(*DockerExecutor)
	if !ok {
		return nil, fmt.Errorf("docker backend has unexpected type %T", backend)
	}
	if _, _, err := dockerExec.ensureClient(); err != nil {
		return nil, fmt.Errorf("initialize docker backend: %w", err)
	}
	cm := dockerExec.ContainerMgr()
	if cm == nil {
		return nil, fmt.Errorf("docker container manager not initialized")
	}
	info, err := cm.GetContainerInfo(ctx, containerID)
	if err != nil {
		// Treat any inspect failure as "missing" so the UI can surface it
		// without bubbling a 500 to the popover.
		return &ContainerLiveStatus{ContainerID: containerID, State: "missing", Missing: true}, nil
	}
	out := &ContainerLiveStatus{
		ContainerID: containerID,
		State:       info.State,
		Status:      info.Status,
		ExitCode:    info.ExitCode,
		Health:      info.Health,
	}
	if !info.StartedAt.IsZero() {
		t := info.StartedAt
		out.StartedAt = &t
	}
	if !info.FinishedAt.IsZero() {
		t := info.FinishedAt
		out.FinishedAt = &t
	}
	return out, nil
}

// DestroySandbox destroys a Sprites sandbox by name. The executionID is used to
// resolve a cached API token when one exists; if no token is cached, the sprite's
// metadata-resolver fallback kicks in.
func (m *Manager) DestroySandbox(ctx context.Context, sandboxID, executionID string) error {
	if sandboxID == "" {
		return nil
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameSprites)
	if err != nil {
		return fmt.Errorf("sprites backend unavailable: %w", err)
	}
	spritesExec, ok := backend.(*SpritesExecutor)
	if !ok {
		return fmt.Errorf("sprites backend has unexpected type %T", backend)
	}
	// StopInstance only runs sandbox cleanup when StopReason matches a
	// terminal reason (see shouldRunExecutorCleanup). DestroySandbox IS the
	// destruction path, so set StopReasonTaskDeleted explicitly — the
	// empty StopReason this previously sent was silently treated as a
	// preserve-on-stop signal and the sandbox was never destroyed.
	instance := &ExecutorInstance{
		InstanceID: executionID,
		Metadata:   map[string]interface{}{MetadataKeySpriteName: sandboxID},
		StopReason: StopReasonTaskDeleted,
	}
	return spritesExec.StopInstance(ctx, instance, true)
}
