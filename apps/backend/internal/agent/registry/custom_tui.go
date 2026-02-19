package registry

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
)

// RegisterCustomTUIAgent creates a TUIAgent from user-provided parameters and registers it.
// The command string is split into binary + args. Any {{model}} placeholder in the command
// is replaced with the model value.
func (r *Registry) RegisterCustomTUIAgent(slug, displayName, command, description, model string, commandArgs []string) error {
	// Replace {{model}} template in the command string
	resolvedCommand := command
	if model != "" {
		resolvedCommand = strings.ReplaceAll(command, "{{model}}", model)
	}

	// Split command into binary + args
	parts := strings.Fields(resolvedCommand)
	if len(parts) == 0 {
		return fmt.Errorf("command is empty")
	}
	binary := parts[0]
	args := parts[1:]
	if len(commandArgs) > 0 {
		args = append(args, commandArgs...)
	}

	cfg := agents.TUIAgentConfig{
		AgentID:     slug,
		AgentName:   slug,
		Command:     binary,
		Desc:        description,
		Display:     displayName,
		WaitForTerm: true,
		CommandArgs: args,
	}
	tuiAgent := agents.NewTUIAgent(cfg)
	return r.Register(tuiAgent)
}
