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

var (
	_ Agent            = (*MockAgent)(nil)
	_ PassthroughAgent = (*MockAgent)(nil)
	_ InferenceAgent   = (*MockAgent)(nil)
)

type MockAgent struct {
	StandardPassthrough
	enabled     bool
	binaryPath  string
	supportsMCP bool
}

func NewMockAgent() *MockAgent {
	return &MockAgent{
		StandardPassthrough: StandardPassthrough{
			Cfg: PassthroughConfig{
				Supported:         true,
				Label:             "TUI Passthrough",
				Description:       "Terminal UI mode for testing",
				PassthroughCmd:    NewCommand("mock-agent", "--tui"),
				ModelFlag:         NewParam("--model", "{model}"),
				PromptFlag:        NewParam("--prompt", "{prompt}"),
				SessionResumeFlag: NewParam("--resume"),
				ResumeFlag:        NewParam("-c"),
				IdleTimeout:       2 * time.Second,
				BufferMaxBytes:    DefaultBufferMaxBytes,
			},
		},
		supportsMCP: true,
	}
}

// SetEnabled enables or disables the mock agent at runtime.
func (a *MockAgent) SetEnabled(enabled bool) { a.enabled = enabled }

// SetBinaryPath overrides the mock-agent binary path.
// Also updates the passthrough command to use the same binary.
func (a *MockAgent) SetBinaryPath(path string) {
	a.binaryPath = path
	a.Cfg.PassthroughCmd = NewCommand(path, "--tui")
}

// SetSupportsMCP controls whether the mock agent reports MCP support.
// Defaults to true so plan mode workflow events work in E2E tests.
func (a *MockAgent) SetSupportsMCP(v bool) { a.supportsMCP = v }

// SupportsMCPEnabled reports the current MCP support setting.
func (a *MockAgent) SupportsMCPEnabled() bool { return a.supportsMCP }

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
	return &DiscoveryResult{Available: true, SupportsMCP: a.supportsMCP}, nil
}

func (a *MockAgent) BuildCommand(opts CommandOptions) Command {
	binary := "mock-agent"
	if a.binaryPath != "" {
		binary = a.binaryPath
	}
	return Cmd(binary).Build()
}

func (a *MockAgent) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:            Cmd("mock-agent").Build(),
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: ResourceLimits{MemoryMB: 512, CPUCores: 0.5, Timeout: time.Hour},
		Protocol:       agent.ProtocolACP,
		SessionConfig: SessionConfig{
			CanRecover: &canRecover,
		},
	}
}

func (a *MockAgent) RemoteAuth() *RemoteAuth { return nil }

func (a *MockAgent) InstallScript() string { return "" }

func (a *MockAgent) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

// InferenceConfig enables one-shot inference via ACP. The mock-agent binary
// advertises its available models in the session/new response, so the host
// utility capability probe populates them into the cache without any static
// model list here.
func (a *MockAgent) InferenceConfig() *InferenceConfig {
	binary := "mock-agent"
	if a.binaryPath != "" {
		binary = a.binaryPath
	}
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(binary),
	}
}
