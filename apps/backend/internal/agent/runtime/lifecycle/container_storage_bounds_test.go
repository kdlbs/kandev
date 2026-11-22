package lifecycle

import (
	"strings"
	"testing"
)

// @covers AC-EXECUTORS-STORAGE-001.4
// @covers AC-EXECUTORS-STORAGE-001.6
func TestBuildDockerContainerConfigCarriesStorageBounds(t *testing.T) {
	for _, executorType := range []string{"local_docker", "remote_docker"} {
		t.Run(executorType, func(t *testing.T) {
			cfg, err := buildDockerContainerConfig(&ExecutorCreateRequest{
				Metadata: map[string]interface{}{
					"docker_tmp_size":      " 2g ",
					"docker_log_max_size":  " 100m ",
					"docker_log_max_files": " 3 ",
				},
			}, executorType)
			if err != nil {
				t.Fatalf("buildDockerContainerConfig() error = %v", err)
			}
			if cfg.DockerTmpSizeBytes != 2*1024*1024*1024 {
				t.Errorf("DockerTmpSizeBytes = %d, want %d", cfg.DockerTmpSizeBytes, 2*1024*1024*1024)
			}
			if cfg.DockerLogMaxSize != "100m" || cfg.DockerLogMaxFiles != "3" {
				t.Errorf("Docker log overrides = (%q, %q), want trimmed profile values", cfg.DockerLogMaxSize, cfg.DockerLogMaxFiles)
			}
		})
	}

	t.Run("blank values leave Docker defaults", func(t *testing.T) {
		cfg, err := buildDockerContainerConfig(&ExecutorCreateRequest{
			Metadata: map[string]interface{}{
				"docker_tmp_size":      "  ",
				"docker_log_max_size":  "  ",
				"docker_log_max_files": "",
			},
		}, "local_docker")
		if err != nil {
			t.Fatalf("buildDockerContainerConfig() error = %v", err)
		}
		if cfg.DockerTmpSizeBytes != 0 || cfg.DockerLogMaxSize != "" || cfg.DockerLogMaxFiles != "" {
			t.Fatalf("blank values produced storage overrides: %+v", cfg)
		}
	})
}

// @covers AC-EXECUTORS-STORAGE-001.7
func TestBuildDockerContainerConfigRejectsInvalidTmpSize(t *testing.T) {
	for _, executorType := range []string{"local_docker", "remote_docker"} {
		for _, size := range []string{"not-a-size", "0", "-1"} {
			t.Run(executorType+"/"+size, func(t *testing.T) {
				_, err := buildDockerContainerConfig(&ExecutorCreateRequest{
					Metadata: map[string]interface{}{"docker_tmp_size": size},
				}, executorType)
				if err == nil {
					t.Fatal("buildDockerContainerConfig() error = nil, want an invalid-size error")
				}
				if !strings.Contains(err.Error(), "docker_tmp_size") {
					t.Fatalf("error = %q, want it to identify docker_tmp_size", err)
				}
			})
		}
	}
}

// @covers AC-EXECUTORS-STORAGE-001.6
func TestBuildContainerConfigCarriesStorageBounds(t *testing.T) {
	cm := newCMTest(t)
	got, err := cm.buildContainerConfig(ContainerConfig{
		AgentConfig:        newConfigStubAgent(),
		InstanceID:         "0123456789abcdef",
		TaskID:             "task-1",
		DockerTmpSizeBytes: 2 * 1024 * 1024 * 1024,
		DockerLogMaxSize:   "100m",
		DockerLogMaxFiles:  "3",
	})
	if err != nil {
		t.Fatalf("buildContainerConfig() error = %v", err)
	}
	if got.TmpfsSizeBytes != 2*1024*1024*1024 || got.LogMaxSize != "100m" || got.LogMaxFiles != "3" {
		t.Fatalf("Docker storage overrides = (%d, %q, %q), want configured values", got.TmpfsSizeBytes, got.LogMaxSize, got.LogMaxFiles)
	}
}
