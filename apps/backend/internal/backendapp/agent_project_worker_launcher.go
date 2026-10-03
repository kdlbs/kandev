package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator"
)

type agentProjectWorkerLauncher struct {
	service *orchestrator.Service
}

func (l agentProjectWorkerLauncher) StartAgentProjectWorker(
	ctx context.Context, taskID, profileID, executorProfileID, prompt string,
) error {
	if l.service == nil {
		return orchestrator.ErrAgentProjectLaunchUnavailable
	}
	_, err := l.service.StartTask(ctx, taskID, profileID, "", executorProfileID, "", prompt, "", false, true, nil)
	return err
}
