package agents

import (
	"context"
	"time"

	"github.com/kandev/kandev/pkg/agent"
)

// TUIAgentConfig is a declarative specification for CLI passthrough / TUI agents.
// TUI tools like k9s or lazydocker can be defined in ~20 lines instead of ~90.
type TUIAgentConfig struct {
	// Required
	AgentID   string
	AgentName string
	Command   string // binary name, e.g. "claude"
	Desc      string

	// Optional (zero values get sensible defaults)
	Display     string         // defaults to AgentName
	Order       *int           // defaults to 99 when nil; use a pointer so 0 is a valid explicit value
	Protocol    agent.Protocol // defaults to ProtocolACP
	LogoLight   []byte         // nil = no logo (frontend shows a blank placeholder)
	LogoDark    []byte         // nil = no logo
	IdleTimeout time.Duration  // defaults to 3s
	BufferMax   int64          // defaults to DefaultBufferMaxBytes
	WaitForTerm bool           // delay process start until the terminal WebSocket connects and sends its first resize
	ModelFlag   Param          // e.g. NewParam("--model", "{model}") — lets users pick a model in profile settings
	CommandArgs []string       // extra args after Command
	DetectOpts  []DetectOption // defaults to WithCommand(Command)
}

// TUIAgent implements Agent + PassthroughAgent for CLI passthrough TUI tools.
type TUIAgent struct {
	StandardPassthrough
	cfg TUIAgentConfig
	cmd Command // cached command built once from cfg.Command + cfg.CommandArgs
}

// NewTUIAgent creates a TUIAgent from a declarative config, applying defaults.
func NewTUIAgent(cfg TUIAgentConfig) *TUIAgent {
	if cfg.Display == "" {
		cfg.Display = cfg.AgentName
	}
	if cfg.Order == nil {
		defaultOrder := 99
		cfg.Order = &defaultOrder
	}
	if cfg.Protocol == "" {
		cfg.Protocol = agent.ProtocolACP
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 3 * time.Second
	}
	if cfg.BufferMax == 0 {
		cfg.BufferMax = DefaultBufferMaxBytes
	}
	if len(cfg.DetectOpts) == 0 {
		cfg.DetectOpts = []DetectOption{WithCommand(cfg.Command)}
	}

	cmdArgs := append([]string{cfg.Command}, cfg.CommandArgs...)

	cmd := Cmd(cfg.Command).Flag(cfg.CommandArgs...).Build()

	a := &TUIAgent{
		StandardPassthrough: StandardPassthrough{
			Cfg: PassthroughConfig{
				Supported:       true,
				Label:           "CLI Passthrough",
				Description:     cfg.Desc,
				PassthroughCmd:  NewCommand(cmdArgs...),
				ModelFlag:       cfg.ModelFlag,
				IdleTimeout:     cfg.IdleTimeout,
				BufferMaxBytes:  cfg.BufferMax,
				WaitForTerminal: cfg.WaitForTerm,
			},
		},
		cfg: cfg,
		cmd: cmd,
	}
	return a
}

// Compile-time interface checks.
var (
	_ Agent            = (*TUIAgent)(nil)
	_ PassthroughAgent = (*TUIAgent)(nil)
)

func (a *TUIAgent) ID() string          { return a.cfg.AgentID }
func (a *TUIAgent) Name() string        { return a.cfg.AgentName }
func (a *TUIAgent) DisplayName() string { return a.cfg.Display }
func (a *TUIAgent) Description() string { return a.cfg.Desc }
func (a *TUIAgent) Enabled() bool       { return true }
func (a *TUIAgent) DisplayOrder() int   { return *a.cfg.Order }

func (a *TUIAgent) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return a.cfg.LogoDark
	}
	return a.cfg.LogoLight
}

func (a *TUIAgent) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	return Detect(ctx, a.cfg.DetectOpts...)
}

func (a *TUIAgent) DefaultModel() string { return "" }

func (a *TUIAgent) ListModels(_ context.Context) (*ModelList, error) {
	return &ModelList{Models: nil, SupportsDynamic: false}, nil
}

func (a *TUIAgent) BuildCommand(_ CommandOptions) Command {
	return a.cmd
}

func (a *TUIAgent) PermissionSettings() map[string]PermissionSetting { return nil }

func (a *TUIAgent) Runtime() *RuntimeConfig {
	return &RuntimeConfig{
		Cmd:            a.cmd,
		WorkingDir:     "{workspace}",
		Env:            map[string]string{},
		ResourceLimits: DefaultResourceLimits,
		Protocol:       a.cfg.Protocol,
	}
}
