package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

// dockerNetworkAuthoritativeKeys are the profile-owned network keys. Network
// placement is a containment boundary: a task that could supply its own value
// could leave an internal network the profile confined it to, or join a LAN
// segment the profile never granted.
var dockerNetworkAuthoritativeKeys = []string{
	lifecycle.MetadataKeyDockerNetwork,
	lifecycle.MetadataKeyDockerNetworkGwPriority,
	lifecycle.MetadataKeyDockerAdditionalNetworks,
}

// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.5
func TestResolveExecutorConfig_ProfileNetworkOverridesTaskMetadata(t *testing.T) {
	for _, key := range dockerNetworkAuthoritativeKeys {
		t.Run(key, func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			repo.executors["exec-docker-1"] = &models.Executor{
				ID:   "exec-docker-1",
				Type: models.ExecutorTypeLocalDocker,
			}
			repo.executorProfiles["prof-1"] = &models.ExecutorProfile{
				ID:     "prof-1",
				Config: map[string]string{key: "profile-value"},
			}

			cfg := exec.resolveExecutorConfig(context.Background(), "exec-docker-1", "ws-1",
				map[string]interface{}{
					"executor_profile_id": "prof-1",
					key:                   "task-supplied",
				})

			if got, _ := cfg.Metadata[key].(string); got != "profile-value" {
				t.Fatalf("metadata[%q] = %q, want the profile value to win", key, got)
			}
		})
	}
}

// An empty profile value must clear a task-supplied one too. Without this an
// unconfigured profile leaves the task's own network in place, which is the
// same escape by a different route.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.5
func TestResolveExecutorConfig_EmptyProfileNetworkClearsTaskMetadata(t *testing.T) {
	for _, key := range dockerNetworkAuthoritativeKeys {
		t.Run(key, func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			repo.executors["exec-docker-1"] = &models.Executor{
				ID:   "exec-docker-1",
				Type: models.ExecutorTypeLocalDocker,
			}
			repo.executorProfiles["prof-1"] = &models.ExecutorProfile{ID: "prof-1", Config: map[string]string{}}

			cfg := exec.resolveExecutorConfig(context.Background(), "exec-docker-1", "ws-1",
				map[string]interface{}{
					"executor_profile_id": "prof-1",
					key:                   "task-supplied",
				})

			if got, _ := cfg.Metadata[key].(string); got != "" {
				t.Fatalf("metadata[%q] = %q, want it cleared by the empty profile value", key, got)
			}
		})
	}
}

// A launch that attaches no profile must not inherit a task-supplied network
// either; the clearing path runs whether or not a profile resolves.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.5
func TestResolveExecutorConfig_NetworkClearedWithoutProfile(t *testing.T) {
	for _, key := range dockerNetworkAuthoritativeKeys {
		t.Run(key, func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			repo.executors["exec-docker-1"] = &models.Executor{
				ID:   "exec-docker-1",
				Type: models.ExecutorTypeLocalDocker,
			}

			cfg := exec.resolveExecutorConfig(context.Background(), "exec-docker-1", "ws-1",
				map[string]interface{}{key: "task-supplied"})

			if got, _ := cfg.Metadata[key].(string); got != "" {
				t.Fatalf("metadata[%q] = %q, want it cleared when no profile supplies it", key, got)
			}
		})
	}
}
