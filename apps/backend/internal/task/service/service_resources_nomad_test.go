package service

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestNomadExecutorValidatesDedicatedNativeProfile(t *testing.T) {
	config := map[string]string{
		"address": "https://nomad.example.test:4646", "namespace": "default", "region": "global",
		"datacenter": "dc1", "node_id": "node-b", "platform": "darwin/arm64",
		"ssh_target": "kandev@worker-b.example", "ssh_port": "22",
		"ssh_host_fingerprint": "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		"workspace_root":       "/var/lib/kandev/workspaces", "allocation_root": "/var/lib/kandev/allocations",
		"cpu_mhz": "1500", "memory_mb": "4096", "nomad_token": "$kandev-secret:nomad-token",
	}
	if err := validateExecutorForType(models.ExecutorTypeNomad, config); err != nil {
		t.Fatalf("valid Nomad profile rejected: %v", err)
	}

	for _, mutate := range []func(map[string]string){
		func(cfg map[string]string) { cfg["platform"] = "linux/arm64" },
		func(cfg map[string]string) { cfg["nomad_token"] = "plaintext" },
		func(cfg map[string]string) { cfg["workspace_mode"] = "shared" },
	} {
		invalid := make(map[string]string, len(config)+1)
		for key, value := range config {
			invalid[key] = value
		}
		mutate(invalid)
		if err := validateExecutorForType(models.ExecutorTypeNomad, invalid); !errors.Is(err, ErrInvalidExecutorConfig) {
			t.Fatalf("invalid Nomad profile error = %v, want ErrInvalidExecutorConfig", err)
		}
	}
}
