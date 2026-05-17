// Package executor defines the agent executor types shared across lifecycle and policy logic.
package executor

import (
	"github.com/kandev/kandev/internal/task/models"
)

// Name identifies the execution backend.
type Name string

const (
	NameUnknown      Name = ""
	NameDocker       Name = "docker"
	NameStandalone   Name = "standalone"
	NameLocal        Name = "local"
	NameRemoteDocker Name = "remote_docker"
	NameSprites      Name = "sprites"
	NameSSH          Name = "ssh"
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
	case models.ExecutorTypeSSH:
		return NameSSH
	case models.ExecutorTypeMockRemote:
		return NameStandalone
	default:
		return NameStandalone
	}
}
