package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

var dockerStorageProfileKeys = []string{
	"docker_tmp_size",
	"docker_log_max_size",
	"docker_log_max_files",
}

// @covers AC-EXECUTORS-STORAGE-001.4
func TestResolveExecutorConfig_DockerStorageProfileIsAuthoritative(t *testing.T) {
	for _, key := range dockerStorageProfileKeys {
		t.Run(key, func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			repo.executors["exec-docker-storage"] = &models.Executor{
				ID:   "exec-docker-storage",
				Type: models.ExecutorTypeLocalDocker,
			}
			repo.executorProfiles["profile-docker-storage"] = &models.ExecutorProfile{
				ID:         "profile-docker-storage",
				ExecutorID: "exec-docker-storage",
				Config:     map[string]string{key: "profile-value"},
			}

			cfg := exec.resolveExecutorConfig(context.Background(), "exec-docker-storage", "ws-1",
				map[string]interface{}{
					"executor_profile_id": "profile-docker-storage",
					key:                   "task-value",
				})

			if got, _ := cfg.Metadata[key].(string); got != "profile-value" {
				t.Fatalf("metadata[%q] = %q, want the profile value", key, got)
			}
		})
	}
}

// @covers AC-EXECUTORS-STORAGE-001.4
func TestResolveExecutorConfig_DockerStorageProfileClearsTaskValues(t *testing.T) {
	for _, key := range dockerStorageProfileKeys {
		t.Run(key, func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			repo.executors["exec-docker-storage"] = &models.Executor{
				ID:   "exec-docker-storage",
				Type: models.ExecutorTypeLocalDocker,
			}
			repo.executorProfiles["profile-docker-storage"] = &models.ExecutorProfile{
				ID:         "profile-docker-storage",
				ExecutorID: "exec-docker-storage",
				Config:     map[string]string{},
			}

			cfg := exec.resolveExecutorConfig(context.Background(), "exec-docker-storage", "ws-1",
				map[string]interface{}{
					"executor_profile_id": "profile-docker-storage",
					key:                   "task-value",
				})

			if got, _ := cfg.Metadata[key].(string); got != "" {
				t.Fatalf("metadata[%q] = %q, want the empty profile value", key, got)
			}
		})
	}
}

// @covers AC-EXECUTORS-STORAGE-001.4
func TestResolveExecutorConfig_DockerStorageClearedWithoutProfile(t *testing.T) {
	for _, key := range dockerStorageProfileKeys {
		t.Run(key, func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			repo.executors["exec-docker-storage"] = &models.Executor{
				ID:   "exec-docker-storage",
				Type: models.ExecutorTypeLocalDocker,
			}

			cfg := exec.resolveExecutorConfig(context.Background(), "exec-docker-storage", "ws-1",
				map[string]interface{}{key: "task-value"})

			if got, _ := cfg.Metadata[key].(string); got != "" {
				t.Fatalf("metadata[%q] = %q, want it cleared without a profile", key, got)
			}
		})
	}
}
