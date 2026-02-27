package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/mock_light.svg
var mockLogoLight []byte

//go:embed logos/mock_dark.svg
var mockLogoDark []byte

var _ Agent = (*MockAgent)(nil)

type MockAgent struct {
	enabled    bool
	binaryPath string
}

func NewMockAgent() *MockAgent { return &MockAgent{} }

// SetEnabled enables or disables the mock agent at runtime.
func (a *MockAgent) SetEnabled(enabled bool) { a.enabled = enabled }

// SetBinaryPath overrides the mock-agent binary path.
func (a *MockAgent) SetBinaryPath(path string) { a.binaryPath = path }

func (a *MockAgent) ID() string          { return "mock-agent" }
func (a *MockAgent) Name() string        { return "Mock Agent" }
func (a *MockAgent) DisplayName() string { return "Mock" }
func (a *MockAgent) Description() string {
	return "Mock agent for testing. Generates simulated responses with all message types."
}
func (a *MockAgent) Enabled() bool     { return a.enabled }
func (a *MockAgent) DisplayOrder() int { return 99 }

func (a *MockAgent) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return mockLogoDark
	}
	return mockLogoLight
}

func (a *MockAgent) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	// Mock agent is always "available" when enabled (forced by settings controller).
	// SupportsMCP is true so plan mode workflow events work in E2E tests.
	return &DiscoveryResult{Available: false, SupportsMCP: true}, nil
}

func (a *MockAgent) DefaultModel() string { return "mock-default" }

func (a *MockAgent) ListModels(ctx context.Context) (*ModelList, error) {
	return &ModelList{Models: mockStaticModels(), SupportsDynamic: false}, nil
}

func (a *MockAgent) BuildCommand(opts CommandOptions) Command {
	binary := "mock-agent"
	if a.binaryPath != "" {
		binary = a.binaryPath
	}
	return Cmd(binary).
		Model(NewParam("--model", "{model}"), opts.Model).
		Build()
}

func (a *MockAgent) Runtime() *RuntimeConfig {
	canRecover := false
	return &RuntimeConfig{
		Cmd:            Cmd("mock-agent").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
		Protocol:       agent.ProtocolClaudeCode,
		ModelFlag:      NewParam("--model", "{model}"),
		SessionConfig: SessionConfig{
			CanRecover: &canRecover,
		},
	}
}

func (a *MockAgent) RemoteAuth() *RemoteAuth { return nil }

func (a *MockAgent) PermissionSettings() map[string]PermissionSetting {
	return mockPermSettings
}

var mockPermSettings = map[string]PermissionSetting{
	"auto_approve": {Supported: true, Default: true, Label: "Auto-approve", Description: "Automatically approve tool calls",
		ApplyMethod: "stdio"},
	"dangerously_skip_permissions": {Supported: true, Default: false, Label: "Skip Permissions (YOLO)", Description: "Bypass all permission checks"},
}

func mockStaticModels() []Model {
	return []Model{
		{ID: "mock-default", Name: "Mock Default", Provider: "mock", ContextWindow: 200000, IsDefault: true, Source: "static"},
		{ID: "mock-slow", Name: "Mock Slow", Description: "Longer delays", Provider: "mock", ContextWindow: 200000, Source: "static"},
		{ID: "mock-fast", Name: "Mock Fast", Description: "Minimal delays", Provider: "mock", ContextWindow: 200000, Source: "static"},
	}
}
