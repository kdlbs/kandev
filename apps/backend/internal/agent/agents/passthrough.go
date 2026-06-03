package agents

// StandardPassthrough provides a data-driven implementation of PassthroughAgent.
// Agents embed this struct and configure it declaratively to get passthrough support.
type StandardPassthrough struct {
	Cfg          PassthroughConfig
	PermSettings map[string]PermissionSetting
}

// PassthroughConfig returns the passthrough configuration.
func (p *StandardPassthrough) PassthroughConfig() PassthroughConfig {
	return p.Cfg
}

// BuildPassthroughCommand builds a CLI command for passthrough mode.
func (p *StandardPassthrough) BuildPassthroughCommand(opts PassthroughOptions) Command {
	b := p.Cfg.PassthroughCmd.With().
		Model(p.Cfg.ModelFlag, opts.Model).
		Settings(p.PermSettings, opts.PermissionValues).
		Flag(opts.CLIFlagTokens...)

	switch {
	case opts.SessionID != "" && !p.Cfg.SessionResumeFlag.IsEmpty():
		b.Resume(p.Cfg.SessionResumeFlag, opts.SessionID, false)
	case opts.Resume && !p.Cfg.ResumeFlag.IsEmpty():
		b.Flag(p.Cfg.ResumeFlag.Args()...)
	case opts.Prompt != "":
		b.Prompt(p.Cfg.PromptFlag, opts.Prompt)
	}

	// MCP injection args go last so Claude Code's variadic --mcp-config does not
	// swallow a positional prompt as an extra config path. Codex's `-c` overrides
	// are order-insensitive, so trailing placement is safe for them too.
	b.Flag(opts.MCPArgs...)

	return b.Build()
}
