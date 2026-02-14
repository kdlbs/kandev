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
		Settings(p.PermSettings, opts.PermissionValues)

	if opts.SessionID != "" && !p.Cfg.SessionResumeFlag.IsEmpty() {
		b.Resume(p.Cfg.SessionResumeFlag, opts.SessionID, false)
	} else if opts.Resume && !p.Cfg.ResumeFlag.IsEmpty() {
		b.Flag(p.Cfg.ResumeFlag.Args()...)
	} else if opts.Prompt != "" {
		b.Prompt(p.Cfg.PromptFlag, opts.Prompt)
	}

	return b.Build()
}
