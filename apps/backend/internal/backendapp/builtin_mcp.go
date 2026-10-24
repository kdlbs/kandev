package backendapp

import (
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
)

func wireBuiltinMCPActions(params routeParams) {
	params.lifecycleMgr.SetBuiltinActionResolver(func(profile p.Context) ([]string, error) {
		return mcpserver.ResolveBuiltinActions(profile, params.log)
	})
}
