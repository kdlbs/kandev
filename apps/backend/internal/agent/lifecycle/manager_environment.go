package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/executor"
)

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
	cm := dockerExec.ContainerMgr()
	if cm == nil {
		return fmt.Errorf("docker container manager not initialized")
	}
	return cm.RemoveContainer(ctx, containerID, true)
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
	instance := &ExecutorInstance{
		InstanceID: executionID,
		Metadata:   map[string]interface{}{"sprite_name": sandboxID},
	}
	return spritesExec.StopInstance(ctx, instance, true)
}
