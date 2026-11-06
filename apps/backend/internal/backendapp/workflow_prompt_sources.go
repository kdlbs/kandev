package backendapp

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/workflow/stepevents"
)

func wirePromptSourceEvents(workflows *workflowservice.Service, tasks *taskservice.Service, eventBus bus.EventBus, log *logger.Logger) {
	workflows.SetPromptSourceEventPublishers(tasks.PublishWorkflowEvent, stepevents.NewPublisher(eventBus, "workflow-prompt-sources", log))
}

// Source drift uses the same scoped workspace listing as the normal task API.
func (a *workflowProviderAdapter) ListWorkspaces(ctx context.Context) ([]*taskmodels.Workspace, error) {
	return a.svc.ListWorkspaces(ctx)
}

// Use the same scope as native workflow creation; visibility alone cannot write.
func (a *workflowProviderAdapter) AuthorizePromptSourceWrite(ctx context.Context, workspaceID string) error {
	err := a.svc.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeRepositoryManage)
	if taskservice.IsForbidden(err) {
		return errors.Join(workflowmodels.ErrPromptSourceForbidden, err)
	}
	return err
}
