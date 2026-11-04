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
	ID           string
	DisplayName  string
	Command      []string
	Env          []string
	ResolveError string
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
		command, env, resolveErr := resolveAgentLaunch(ia, cfg.Command.Args())
		spec := AgentSpec{
			ID:          ag.ID(),
			DisplayName: ag.DisplayName(),
			Command:     command,
			Env:         env,
		}
		if resolveErr != nil {
			spec.ResolveError = resolveErr.Error()
		}
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LookupAgent returns the spec for a single agent ID, or an error if the
// agent is not registered or not ACP-capable.
func resolveAgentLaunch(ia agents.InferenceAgent, base []string) ([]string, []string, error) {
	resolver, ok := ia.(agents.LocalRuntimeAgent)
	if !ok {
		return base, nil, nil
	}
	command, values, err := resolver.ResolveLocalRuntime(context.Background())
	if err != nil {
		return base, nil, err
	}
	env := acpcompat.MergeEnvMap(os.Environ(), values)
	harness := acpcompat.EnvValue(env, acpcompat.AntigravityHarnessPathEnv)
	prepared, err := acpcompat.PrepareLocalAntigravityEnv(env, harness)
	if err != nil {
		return command.Args(), nil, err
	}
	return command.Args(), prepared, nil
}

func LookupAgent(reg *registry.Registry, agentID string) (AgentSpec, error) {
	for _, spec := range ListACPAgents(reg) {
		if spec.ID == agentID {
			if spec.ResolveError != "" {
				return AgentSpec{}, fmt.Errorf("agent %q has no complete local runtime: %s", agentID, spec.ResolveError)
			}
			return spec, nil
		}
	}
	return AgentSpec{}, fmt.Errorf("agent %q not found or not ACP-capable", agentID)
}
