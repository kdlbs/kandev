package agents

import (
	"context"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

// CustomACPAgentConfig is the declarative specification for a user-defined
// agent whose CLI speaks ACP on stdin/stdout.
type CustomACPAgentConfig struct {
	// Required
	AgentID   string
	AgentName string
	Command   string // binary name, e.g. "my-agent"
	Desc      string

	// Optional (zero values get sensible defaults)
	Display     string         // defaults to AgentName
	Order       *int           // defaults to 99 when nil; use a pointer so 0 is a valid explicit value
	CommandArgs []string       // extra args after Command, e.g. "--acp"
	DetectOpts  []DetectOption // defaults to WithCommand(Command)
}

// CustomACPAgent implements Agent for a user-defined CLI that kandev drives
// over ACP.
//
// It is deliberately not a TUIAgent. IsPassthroughOnly is a type assertion on
// that type, so being one would seed the default profile as terminal
// passthrough and hide the agent from the host-utility capability probe.
//
// It offers no passthrough mode either. A custom definition carries one
// command, and an ACP server command is not the command that renders an
// interactive terminal: launching `my-agent --acp` under a PTY shows the user
// JSON-RPC frames.
type CustomACPAgent struct {
	cfg CustomACPAgentConfig
	cmd Command // cached command built once from cfg.Command + cfg.CommandArgs
}

// Compile-time interface checks.
var (
	_ Agent          = (*CustomACPAgent)(nil)
	_ InferenceAgent = (*CustomACPAgent)(nil)
)

// NewCustomACPAgent creates a CustomACPAgent from a declarative config,
// applying defaults.
func NewCustomACPAgent(cfg CustomACPAgentConfig) *CustomACPAgent {
	if cfg.Display == "" {
		cfg.Display = cfg.AgentName
	}
	if cfg.Order == nil {
		defaultOrder := 99
		cfg.Order = &defaultOrder
	}
	if len(cfg.DetectOpts) == 0 {
		cfg.DetectOpts = []DetectOption{WithCommand(cfg.Command)}
	}
	return &CustomACPAgent{
		cfg: cfg,
		cmd: Cmd(cfg.Command).Flag(cfg.CommandArgs...).Build(),
	}
}

func (a *CustomACPAgent) ID() string          { return a.cfg.AgentID }
func (a *CustomACPAgent) Name() string        { return a.cfg.AgentName }
func (a *CustomACPAgent) DisplayName() string { return a.cfg.Display }
func (a *CustomACPAgent) Description() string { return a.cfg.Desc }
func (a *CustomACPAgent) Enabled() bool       { return true }
func (a *CustomACPAgent) DisplayOrder() int   { return *a.cfg.Order }

// Logo returns nil: a user-defined agent ships no artwork, and the frontend
// renders a placeholder.
func (a *CustomACPAgent) Logo(LogoVariant) []byte { return nil }

func (a *CustomACPAgent) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, a.cfg.DetectOpts...)
	if err != nil {
		return result, err
	}
	// Resolved MCP servers travel in ACP session/new, so support does not
	// depend on a per-CLI config-file strategy the way passthrough does.
	result.SupportsMCP = true
	return result, nil
}

func (a *CustomACPAgent) BuildCommand(_ CommandOptions) Command { return a.cmd }

func (a *CustomACPAgent) Runtime() *RuntimeConfig {
	return &RuntimeConfig{
		Cmd:            a.cmd,
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: DefaultResourceLimits,
		Protocol:       agent.ProtocolACP,
	}
}

// InferenceConfig opts the agent into the host-utility capability probe, which
// is where its models and modes come from.
func (a *CustomACPAgent) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{Supported: true, Command: a.cmd, OperatorDefined: true}
}

func (a *CustomACPAgent) InstallScript() string          { return "" }
func (a *CustomACPAgent) RemoteAuth() *RemoteAuth        { return nil }
func (a *CustomACPAgent) BillingType() usage.BillingType { return defaultBillingType() }

// PermissionSettings has no CLI-driven entries: permission stance is expressed
// through ACP session modes and per-tool-call prompts, as for built-in ACP
// agents.
func (a *CustomACPAgent) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}
