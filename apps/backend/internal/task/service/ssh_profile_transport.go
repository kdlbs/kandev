package service

import (
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

// The executor owns SSH routing. Profiles can configure the shell and workload.
func validateSSHProfileTransport(kind models.ExecutorType, config map[string]string) error {
	if kind == models.ExecutorTypeSSH {
		for key := range config {
			if agentruntime.IsSSHRuntimeRouteMetadataKey(key) {
				return fmt.Errorf("%w: %s belongs to the SSH executor connection", ErrInvalidExecutorConfig, key)
			}
		}
	}
	return nil
}
