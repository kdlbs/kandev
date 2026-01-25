// Package runtime defines the agent runtime types shared across lifecycle and policy logic.
package runtime

import (
	"github.com/kandev/kandev/internal/task/models"
)

// Name identifies the execution runtime.
type Name string

const (
	NameUnknown    Name = ""
	NameDocker     Name = "docker"
	NameStandalone Name = "standalone"
	NameLocal      Name = "local"
)

// ExecutorTypeToRuntime maps an ExecutorType to its corresponding runtime Name.
func ExecutorTypeToRuntime(execType models.ExecutorType) Name {
	switch execType {
	case models.ExecutorTypeLocalPC:
		return NameStandalone
	case models.ExecutorTypeLocalDocker:
		return NameDocker
	case models.ExecutorTypeRemoteDocker:
		// For now, remote docker uses the same runtime as local docker.
		// Future: separate runtime for remote docker.
		return NameDocker
	default:
		return NameStandalone
	}
}
