package registry

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

// CustomTUIAgentSpec is the user-provided definition of a custom TUI agent, as
// stored on the agent row's tui_config and replayed into the registry on boot.
type CustomTUIAgentSpec struct {
	Slug        string
	DisplayName string
	Command     string
	Description string
	Model       string
	CommandArgs []string
	// MCPStrategyKey selects how kandev injects MCP servers into the wrapped
	// CLI (see mcpconfig.StrategyByKey). Empty means no injection.
	MCPStrategyKey string
}

// ErrUnknownMCPStrategy is returned when a spec names a strategy key that no
// longer resolves — a typo from an older client, or a key removed by an
// upgrade. Failing loudly beats registering the agent with MCP silently off.
var ErrUnknownMCPStrategy = fmt.Errorf("unknown MCP strategy")

// buildCustomTUIAgent turns a user-provided spec into a TUIAgent. The command
// string is split into binary + args, and any {{model}} placeholder in the
// command is replaced with the model value.
func buildCustomTUIAgent(spec CustomTUIAgentSpec) (*agents.TUIAgent, error) {
	// Replace {{model}} template in the command string
	resolvedCommand := spec.Command
	if spec.Model != "" {
		resolvedCommand = strings.ReplaceAll(spec.Command, "{{model}}", spec.Model)
	}

	// Split command into binary + args
	parts := strings.Fields(resolvedCommand)
	if len(parts) == 0 {
		return nil, fmt.Errorf("command is empty")
	}
	binary := parts[0]
	args := parts[1:]
	if len(spec.CommandArgs) > 0 {
		args = append(args, spec.CommandArgs...)
	}

	strategy, ok := mcpconfig.StrategyByKey(spec.MCPStrategyKey)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownMCPStrategy, spec.MCPStrategyKey)
	}

	return agents.NewTUIAgent(agents.TUIAgentConfig{
		AgentID:     spec.Slug,
		AgentName:   spec.Slug,
		Command:     binary,
		Desc:        spec.Description,
		Display:     spec.DisplayName,
		WaitForTerm: true,
		CommandArgs: args,
		MCPStrategy: strategy,
	}), nil
}

// RegisterCustomTUIAgent creates a TUIAgent from user-provided parameters and
// registers it, failing if the ID is already taken.
func (r *Registry) RegisterCustomTUIAgent(spec CustomTUIAgentSpec) error {
	tuiAgent, err := buildCustomTUIAgent(spec)
	if err != nil {
		return err
	}
	return r.Register(tuiAgent)
}

// ReplaceCustomTUIAgent rebuilds an already-registered custom TUI agent in
// place. The MCP strategy is baked into the TUIAgent at construction, so
// changing it needs a new instance rather than a mutation.
//
// This uses Replace rather than Unregister + Register because the pair leaves a
// window with no entry for the ID: a session launching in that window fails
// with "agent type not found", and a competing registration can leave the agent
// missing until restart.
func (r *Registry) ReplaceCustomTUIAgent(spec CustomTUIAgentSpec) error {
	tuiAgent, err := buildCustomTUIAgent(spec)
	if err != nil {
		return err
	}
	return r.Replace(tuiAgent)
}
