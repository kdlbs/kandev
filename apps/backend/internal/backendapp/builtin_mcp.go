package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentsettingshandlers "github.com/kandev/kandev/internal/agent/settings/handlers"
	"github.com/kandev/kandev/internal/automation"
	httpmw "github.com/kandev/kandev/internal/common/httpmw"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
)

func wireBuiltinMCPActions(params routeParams) {
	params.lifecycleMgr.SetBuiltinActionResolver(func(profile p.Context) ([]string, error) {
		return mcpserver.ResolveBuiltinActions(profile, params.log)
	})
}

// configureBuiltinMCPServices shares package policy across task creation, automation dispatch, and
// management routes.
func configureBuiltinMCPServices(params routeParams) {
	repo, ok := params.agentSettingsRepo.(mcpconfig.BuiltinPolicyRepository)
	if !ok || params.orchestratorSvc == nil || params.taskSvc == nil || params.router == nil {
		return
	}
	if params.services != nil && params.services.Automation != nil {
		params.orchestratorSvc.SetBuiltinPolicyReader(automationBuiltinPolicyReader{BuiltinPolicyRepository: repo, automation: params.services.Automation.Service})
	} else {
		params.orchestratorSvc.SetBuiltinPolicyReader(repo)
	}
	service := mcpconfig.NewBuiltinPolicyService(repo, params.taskSvc.AuthorizeWorkspaceScope, params.orchestratorSvc.BuiltinTaskScope)
	params.taskSvc.SetBuiltinCreationWriter(service)
	agentsettingshandlers.RegisterBuiltinMCPRoutes(params.router, service, httpmw.InterimSettingsInterlock(params.interimSettingsInterlockToken), params.orchestratorSvc)
}

type automationBuiltinPolicyReader struct {
	mcpconfig.BuiltinPolicyRepository
	automation *automation.Service
}

func (r automationBuiltinPolicyReader) GetAutomationMCPBinding(ctx context.Context, workspaceID, taskID string) (*p.BuiltinManifest, error) {
	return r.automation.GetAutomationMCPBinding(ctx, workspaceID, taskID)
}
