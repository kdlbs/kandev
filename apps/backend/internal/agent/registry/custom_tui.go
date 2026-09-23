package registry

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

// CustomAgentProtocol selects how kandev drives a user-defined agent.
type CustomAgentProtocol string

const (
	// CustomAgentProtocolTerminal runs the command as a raw CLI under a PTY.
	// It is the zero value, so every definition stored before this field
	// existed keeps its behaviour without a migration.
	CustomAgentProtocolTerminal CustomAgentProtocol = ""
	// CustomAgentProtocolACP drives the command over ACP on stdin/stdout,
	// giving the agent structured chat, tool calls, models, and modes.
	CustomAgentProtocolACP CustomAgentProtocol = "acp"
)

// CustomTUIAgentSpec is the user-provided definition of a custom agent, as
// stored on the agent row's tui_config and replayed into the registry on boot.
type CustomTUIAgentSpec struct {
	Slug        string
	DisplayName string
	Command     string
	Description string
	Model       string
	CommandArgs []string
	// MCPStrategyKey selects how kandev injects MCP servers into the wrapped
	// CLI (see mcpconfig.StrategyByKey). Empty means no injection. It applies
	// to the terminal protocol only.
	MCPStrategyKey string
	// Protocol selects the runtime kandev drives the command with. Empty means
	// terminal passthrough.
	Protocol CustomAgentProtocol
}

// ErrUnknownMCPStrategy is returned when a spec names a strategy key that no
// longer resolves — a typo from an older client, or a key removed by an
// upgrade. Failing loudly beats registering the agent with MCP silently off.
var ErrUnknownMCPStrategy = fmt.Errorf("unknown MCP strategy")

// ErrUnknownCustomAgentProtocol is returned when a spec names a protocol that
// does not resolve. Falling back to terminal would silently ignore the user's
// choice and launch an ACP server into a PTY.
var ErrUnknownCustomAgentProtocol = fmt.Errorf("unknown custom agent protocol")

// ErrMCPStrategyNotApplicable is returned when a spec pairs a passthrough MCP
// strategy with a protocol that does not inject MCP through a config file. An
// ACP agent receives resolved servers in session/new, so a strategy there is a
// contradiction rather than a no-op.
var ErrMCPStrategyNotApplicable = fmt.Errorf("MCP strategy does not apply to this protocol")

// resolveCustomCommand splits a spec's command string into binary + args,
// substituting any {{model}} placeholder with the spec's model value.
func resolveCustomCommand(spec CustomTUIAgentSpec) (string, []string, error) {
	resolvedCommand := spec.Command
	if spec.Model != "" {
		resolvedCommand = strings.ReplaceAll(spec.Command, "{{model}}", spec.Model)
	}

	parts := strings.Fields(resolvedCommand)
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("command is empty")
	}
	args := parts[1:]
	if len(spec.CommandArgs) > 0 {
		args = append(args, spec.CommandArgs...)
	}
	return parts[0], args, nil
}

// buildCustomAgent turns a user-provided spec into the agent implementation
// its protocol calls for.
func buildCustomAgent(spec CustomTUIAgentSpec) (agents.Agent, error) {
	binary, args, err := resolveCustomCommand(spec)
	if err != nil {
		return nil, err
	}

	switch spec.Protocol {
	case CustomAgentProtocolTerminal:
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
	case CustomAgentProtocolACP:
		if spec.MCPStrategyKey != mcpconfig.StrategyKeyNone {
			return nil, fmt.Errorf("%w: %q", ErrMCPStrategyNotApplicable, spec.MCPStrategyKey)
		}
		return agents.NewCustomACPAgent(agents.CustomACPAgentConfig{
			AgentID:     spec.Slug,
			AgentName:   spec.Slug,
			Command:     binary,
			Desc:        spec.Description,
			Display:     spec.DisplayName,
			CommandArgs: args,
		}), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownCustomAgentProtocol, spec.Protocol)
	}
}

// RegisterCustomTUIAgent creates the agent the spec's protocol calls for and
// registers it, failing if the ID is already taken.
func (r *Registry) RegisterCustomTUIAgent(spec CustomTUIAgentSpec) error {
	customAgent, err := buildCustomAgent(spec)
	if err != nil {
		return err
	}
	return r.Register(customAgent)
}

// ReplaceCustomTUIAgent rebuilds an already-registered custom agent in place.
// The MCP strategy and the protocol are baked in at construction, so changing
// either needs a new instance rather than a mutation.
//
// This uses Replace rather than Unregister + Register because the pair leaves a
// window with no entry for the ID: a session launching in that window fails
// with "agent type not found", and a competing registration can leave the agent
// missing until restart.
func (r *Registry) ReplaceCustomTUIAgent(spec CustomTUIAgentSpec) error {
	customAgent, err := buildCustomAgent(spec)
	if err != nil {
		return err
	}
	return r.Replace(customAgent)
}
