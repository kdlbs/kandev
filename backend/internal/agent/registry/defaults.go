package registry

// DefaultAgents returns the default agent configurations
func DefaultAgents() []*AgentTypeConfig {
	return []*AgentTypeConfig{
		{
			ID:          "augment-agent",
			Name:        "Augment Coding Agent",
			Description: "Auggie CLI-powered autonomous coding agent. Requires AUGMENT_SESSION_AUTH for authentication.",
			Image:       "kandev/augment-agent",
			Tag:         "latest",
			WorkingDir:  "/workspace",
			RequiredEnv: []string{"AUGMENT_SESSION_AUTH"},
			Mounts: []MountTemplate{
				{Source: "{workspace}", Target: "/workspace", ReadOnly: false},
				{Source: "{augment_sessions}", Target: "/root/.augment/sessions", ReadOnly: false},
			},
			ResourceLimits: ResourceLimits{
				MemoryMB:       4096,
				CPUCores:       2.0,
				TimeoutSeconds: 3600, // 1 hour
			},
			Capabilities: []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
			Enabled:      true,
		},
	}
}

