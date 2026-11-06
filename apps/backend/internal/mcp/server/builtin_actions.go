package mcp

import (
	"errors"
	"slices"

	"github.com/kandev/kandev/internal/common/logger"
	p "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// ResolveBuiltinActions uses the same native registration path as agentctl.
// It creates no transport, starts no goroutine, and performs no backend calls.
// Plugin invocation retains its independent approval and resource checks.
func ResolveBuiltinActions(profile p.Context, log *logger.Logger) ([]string, error) {
	if profile.BuiltinPackages == nil || profile.BuiltinPackages.Validate() != nil || profile.Surface != profile.BuiltinPackages.Surface {
		return nil, errors.New("invalid built-in MCP profile")
	}
	s := newServerWithProfile(nil, "", profile.BuiltinPackages.TaskID, log, "", profile)
	actions := []string{ws.ActionMCPListPluginTools, ws.ActionMCPInvokePluginTool}
	for _, names := range s.BuiltinCatalog() {
		for _, name := range names {
			actions = append(actions, p.BuiltinToolActions(name)...)
		}
	}
	slices.Sort(actions)
	return slices.Compact(actions), nil
}
