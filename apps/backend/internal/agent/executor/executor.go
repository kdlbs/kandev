// Package executor defines the agent executor types shared across lifecycle and policy logic.
package executor

import (
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// Name identifies the execution backend. It aliases agentruntime.Runtime
// so the executor and runtime layers share a single typed vocabulary
// without forcing every existing consumer to switch import paths.
type Name = agentruntime.Runtime

const (
	NameUnknown      Name = ""
	NameDocker            = agentruntime.RuntimeDocker
	NameStandalone        = agentruntime.RuntimeStandalone
	NameLocal        Name = "local"
	NameRemoteDocker      = agentruntime.RuntimeRemoteDocker
	NameSprites           = agentruntime.RuntimeSprites
)

// ExecutorTypeToBackend maps an ExecutorType to its corresponding executor Name.
func ExecutorTypeToBackend(execType models.ExecutorType) Name {
	switch execType {
	case models.ExecutorTypeLocal:
		return NameStandalone
	case models.ExecutorTypeWorktree:
		return NameStandalone
	case models.ExecutorTypeLocalDocker:
		return NameDocker
	case models.ExecutorTypeRemoteDocker:
		return NameRemoteDocker
	case models.ExecutorTypeSprites:
		return NameSprites
	case models.ExecutorTypeMockRemote:
		return NameStandalone
	default:
		return NameStandalone
	}
}
