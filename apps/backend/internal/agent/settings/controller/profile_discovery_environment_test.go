package controller

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type profileDiscoveryRuntimeAgent struct {
	agents.Agent
	agents.InferenceAgent
	env map[string]string
}

func (a profileDiscoveryRuntimeAgent) Runtime() *agents.RuntimeConfig {
	return &agents.RuntimeConfig{Env: a.env}
}

func TestProfileProbeEnvironmentDefinitionsUseSecretProfileOverride(t *testing.T) {
	baseAgent := agents.NewClaudeACP()
	inference := profileDiscoveryRuntimeAgent{
		Agent:          baseAgent,
		InferenceAgent: baseAgent,
		env:            map[string]string{"PROFILE_SECRET": "managed-default"},
	}
	definitions := profileProbeEnvironmentDefinitions(inference, []dto.ProfileEnvVarDTO{{
		Key: "PROFILE_SECRET", SecretID: "secret-1",
	}})
	resolved, _, err := runtimeenv.Resolve(context.Background(), definitions, func(context.Context, runtimeenv.Definition) (string, error) {
		return "profile-secret", nil
	})
	if err != nil {
		t.Fatalf("resolve profile environment: %v", err)
	}
	if got := resolved["PROFILE_SECRET"]; got != "profile-secret" {
		t.Fatalf("resolved secret override = %q, want profile-secret", got)
	}
}

func TestProfileProbeEnvironmentDefinitionsIgnoreEmptyProfileOverrides(t *testing.T) {
	baseAgent := agents.NewClaudeACP()
	inference := profileDiscoveryRuntimeAgent{
		Agent:          baseAgent,
		InferenceAgent: baseAgent,
		env:            map[string]string{"PROFILE_EMPTY": "managed-default"},
	}
	definitions := profileProbeEnvironmentDefinitions(inference, []dto.ProfileEnvVarDTO{{
		Key: "PROFILE_EMPTY",
	}})
	resolved, _, err := runtimeenv.Resolve(context.Background(), definitions, func(context.Context, runtimeenv.Definition) (string, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("resolve profile environment: %v", err)
	}
	if got := resolved["PROFILE_EMPTY"]; got != "managed-default" {
		t.Fatalf("resolved empty profile override = %q, want managed-default", got)
	}
}
