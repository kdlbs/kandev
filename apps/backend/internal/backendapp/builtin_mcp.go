package backendapp

import (
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentsettingshandlers "github.com/kandev/kandev/internal/agent/settings/handlers"
	httpmw "github.com/kandev/kandev/internal/common/httpmw"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
)

func wireBuiltinMCPActions(params routeParams) {
	params.lifecycleMgr.SetBuiltinActionResolver(func(profile p.Context) ([]string, error) {
		return mcpserver.ResolveBuiltinActions(profile, params.log)
	})
}

func configureBuiltinMCPServices(params routeParams) {
	repo, ok := params.agentSettingsRepo.(mcpconfig.BuiltinPolicyRepository)
	if !ok || params.orchestratorSvc == nil || params.taskSvc == nil || params.router == nil {
		return
	}
	params.orchestratorSvc.SetBuiltinPolicyReader(repo)
	service := mcpconfig.NewBuiltinPolicyService(repo, params.taskSvc.AuthorizeWorkspaceScope, params.orchestratorSvc.BuiltinTaskScope)
	params.taskSvc.SetBuiltinCreationWriter(service)
	agentsettingshandlers.RegisterBuiltinMCPRoutes(params.router, service, httpmw.InterimSettingsInterlock(params.interimSettingsInterlockToken), params.orchestratorSvc)
}
