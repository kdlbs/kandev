package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestResolveAgentModelConfigRequiresModel(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{
		"test-agent": &testAgent{id: "test-agent"},
	})

	_, err := ctrl.ResolveAgentModelConfig(context.Background(), "test-agent", dto.ResolveAgentModelConfigRequest{})
	if !errors.Is(err, ErrModelRequired) {
		t.Fatalf("ResolveAgentModelConfig() error = %v, want ErrModelRequired", err)
	}
}

func TestResolveAgentModelConfigWithoutHostUtilityReturnsNotConfigured(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{
		"test-agent": &testAgent{id: "test-agent"},
	})

	resp, err := ctrl.ResolveAgentModelConfig(context.Background(), "test-agent", dto.ResolveAgentModelConfigRequest{
		Model: "model",
	})
	if err != nil {
		t.Fatalf("ResolveAgentModelConfig() error = %v", err)
	}
	if resp.Status != "not_configured" {
		t.Fatalf("status = %q, want not_configured", resp.Status)
	}
	if len(resp.ConfigOptions) != 0 {
		t.Fatalf("config options = %#v, want empty", resp.ConfigOptions)
	}
}

type fakeModelConfigHostUtility struct {
	resolution hostutility.ModelConfigResolution
}

func (f *fakeModelConfigHostUtility) Get(string) (hostutility.AgentCapabilities, bool) {
	return hostutility.AgentCapabilities{}, true
}

func (f *fakeModelConfigHostUtility) Refresh(context.Context, string) (hostutility.AgentCapabilities, error) {
	return hostutility.AgentCapabilities{}, nil
}

func (f *fakeModelConfigHostUtility) ResolveModelConfig(
	context.Context,
	string,
	hostutility.ModelConfigResolutionRequest,
) (hostutility.ModelConfigResolution, error) {
	return f.resolution, nil
}

func TestResolveAgentModelConfigSanitizesProviderFailure(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{
		"test-agent": &testAgent{id: "test-agent"},
	})
	ctrl.hostUtility = &fakeModelConfigHostUtility{resolution: hostutility.ModelConfigResolution{
		Status: hostutility.StatusFailed,
		Error:  "provider stderr includes token=secret-value",
	}}

	resp, err := ctrl.ResolveAgentModelConfig(context.Background(), "test-agent", dto.ResolveAgentModelConfigRequest{
		Model: "model",
	})
	if err != nil {
		t.Fatalf("ResolveAgentModelConfig() error = %v", err)
	}
	if resp.Error == nil || *resp.Error != "model option resolution failed" {
		t.Fatalf("error = %v, want sanitized failure", resp.Error)
	}
	if strings.Contains(*resp.Error, "secret-value") {
		t.Fatalf("sanitized error contains provider output: %q", *resp.Error)
	}
}

func TestResolveAgentModelConfigMapsAuthenticationRequired(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{
		"test-agent": &testAgent{id: "test-agent"},
	})
	ctrl.hostUtility = &fakeModelConfigHostUtility{resolution: hostutility.ModelConfigResolution{
		Status: hostutility.StatusAuthRequired,
		Error:  "provider asks for login",
	}}

	resp, err := ctrl.ResolveAgentModelConfig(context.Background(), "test-agent", dto.ResolveAgentModelConfigRequest{
		Model: "model",
	})
	if err != nil {
		t.Fatalf("ResolveAgentModelConfig() error = %v", err)
	}
	if resp.Error == nil || *resp.Error != "agent authentication is required" {
		t.Fatalf("error = %v, want authentication message", resp.Error)
	}
}
