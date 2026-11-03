package acpdbg

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/pkg/agent"
)

// AgentSpec is the minimal info acpdbg needs to spawn a registered agent.
type AgentSpec struct {
	ID          string
	DisplayName string
	Command     []string
	Env         []string
}

// ListACPAgents returns every enabled inference agent whose runtime
// protocol is ACP, sorted by ID. This is the same filter the host utility
// manager uses, so `acpdbg list` matches `/api/v1/agent-capabilities`.
func ListACPAgents(reg *registry.Registry) []AgentSpec {
	var out []AgentSpec
	for _, ia := range reg.ListInferenceAgents() {
		ag, ok := ia.(agents.Agent)
		if !ok {
			continue
		}
		rt := ag.Runtime()
		if rt == nil || rt.Protocol != agent.ProtocolACP {
			continue
		}
		cfg := ia.InferenceConfig()
		if cfg == nil || cfg.Command.IsEmpty() {
			continue
		}
		command, env := resolveAgentLaunch(ia, cfg.Command.Args())
		out = append(out, AgentSpec{
			ID:          ag.ID(),
			DisplayName: ag.DisplayName(),
			Command:     command,
			Env:         env,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LookupAgent returns the spec for a single agent ID, or an error if the
// agent is not registered or not ACP-capable.
func resolveAgentLaunch(ia agents.InferenceAgent, base []string) ([]string, []string) {
	resolver, ok := ia.(agents.LocalRuntimeAgent)
	if !ok {
		return base, nil
	}
	command, values, err := resolver.ResolveLocalRuntime(context.Background())
	if err != nil {
		return base, nil
	}
	env := acpcompat.MergeEnvMap(os.Environ(), values)
	harness := acpcompat.EnvValue(env, acpcompat.AntigravityHarnessPathEnv)
	prepared, err := acpcompat.PrepareLocalAntigravityEnv(env, harness)
	if err != nil {
		return command.Args(), env
	}
	return command.Args(), prepared
}

func LookupAgent(reg *registry.Registry, agentID string) (AgentSpec, error) {
	for _, spec := range ListACPAgents(reg) {
		if spec.ID == agentID {
			if spec.ID == acpcompat.AntigravityAgentID && spec.Env == nil {
				return AgentSpec{}, fmt.Errorf("agent %q has no complete local runtime", agentID)
			}
			return spec, nil
		}
	}
	return AgentSpec{}, fmt.Errorf("agent %q not found or not ACP-capable", agentID)
}
