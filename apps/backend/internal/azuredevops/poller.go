package azuredevops

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/integrations/healthpoll"
)

// NewPoller returns the shared auth-health poller for Azure DevOps. Task PR
// refresh remains request-driven in v1.
func NewPoller(service *Service, log *logger.Logger) *healthpoll.Poller {
	return healthpoll.New("azure devops", authProber{service: service}, log)
}

type authProber struct {
	service *Service
}

func (p authProber) HasConfig(ctx context.Context) (bool, error) {
	workspaceIDs, err := p.service.store.ListConfigWorkspaceIDs(ctx)
	return len(workspaceIDs) > 0, err
}

func (p authProber) RecordAuthHealth(ctx context.Context) {
	p.service.RecordAuthHealth(ctx)
}
