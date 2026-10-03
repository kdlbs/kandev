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
	NameSSH               = agentruntime.RuntimeSSH
	NameKubernetes        = agentruntime.RuntimeKubernetes
	NameCursorCloud       = agentruntime.RuntimeCursorCloud
	NamePluginRemote      = agentruntime.RuntimePluginRemote
)

// ExecutorTypeToBackend maps an ExecutorType to its corresponding executor Name.
func ExecutorTypeToBackend(execType models.ExecutorType) Name {
	switch execType {
	case models.ExecutorTypeLocal, models.ExecutorType("local_pc"):
		return NameStandalone
	case models.ExecutorTypeWorktree:
		return NameStandalone
	case models.ExecutorTypeLocalDocker:
		return NameDocker
	case models.ExecutorTypeRemoteDocker:
		return NameRemoteDocker
	case models.ExecutorTypeSprites:
		return NameSprites
	case models.ExecutorTypeSSH:
		return NameSSH
	case models.ExecutorTypeKubernetes:
		return NameKubernetes
	case models.ExecutorTypeMockRemote:
		return NameStandalone
	case models.ExecutorTypeCursorCloud:
		return NameCursorCloud
	case models.ExecutorTypePluginRemote:
		return NamePluginRemote
	default:
		return NameUnknown
	}
}
