package backendapp

import (
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/config"
	sshhandlers "github.com/kandev/kandev/internal/ssh"
	"github.com/kandev/kandev/internal/sshconnections"
)

func newSSHConnections(cfg *config.Config, repos *Repositories) *sshconnections.Service {
	if !cfg.Features.SSHConnectionManager {
		return nil
	}
	service := sshconnections.NewService(true, repos.SSHConnections, repos.SSHIdentityVault)
	service.SetMigrationPlatformCheck(agentruntime.CheckSSHRemotePlatform)
	return service
}

func managedSSHRuntime(services *Services) agentruntime.ManagedSSHConnections {
	if services.SSHConnections == nil {
		return nil
	}
	return services.SSHConnections
}

func managedSSHManagement(services *Services) sshhandlers.ManagedConnections {
	if services.SSHConnections == nil {
		return nil
	}
	return services.SSHConnections
}

func managedSSHExecutor(services *Services) agentruntime.ManagedSSHExecutorConnection {
	if services.SSHConnections == nil {
		return nil
	}
	return services.SSHConnections
}
