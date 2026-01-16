package registry

import "github.com/kandev/kandev/pkg/agent"

// DefaultAgents returns the default agent configurations
func DefaultAgents() []*AgentTypeConfig {
	return []*AgentTypeConfig{
		{
			ID:            "auggie-agent",
			Name:          "Augment Coding Agent",
			Description:   "Auggie CLI-powered autonomous coding agent. Requires AUGMENT_SESSION_AUTH for authentication.",
			Image:         "kandev/augment-agent",
			Tag:           "latest",
			Cmd:           []string{"auggie", "--acp"},
			WorkingDir:    "/workspace",
			RequiredEnv:   []string{"AUGMENT_SESSION_AUTH"},
			Env: map[string]string{
				"AGENTCTL_AUTO_APPROVE_PERMISSIONS": "true",
			},
			Mounts: []MountTemplate{
				{Source: "{workspace}", Target: "/workspace", ReadOnly: false},
			},
			ResourceLimits: ResourceLimits{
				MemoryMB:       4096,
				CPUCores:       2.0,
				TimeoutSeconds: 3600,
			},
			Capabilities:  []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
			Enabled:       true,
			ModelFlag:     "--model",
			WorkspaceFlag: "--workspace-root",
			Protocol:      agent.ProtocolACP,
			SessionConfig: SessionConfig{
				ResumeViaACP:       false, // Auggie handles resume via CLI flag
				ResumeFlag:         "--resume",
				SessionDirTemplate: "{home}/.augment/sessions",
				SessionDirTarget:   "/root/.augment/sessions",
			},
		},
		{
			ID:          "claude-agent",
			Name:        "Claude Code Agent",
			Description: "Claude Code CLI-powered autonomous coding agent.",
			Image:       "kandev/claude-agent",
			Tag:         "latest",
			Cmd:         []string{"claude", "--acp"},
			WorkingDir:  "/workspace",
			RequiredEnv: []string{},
			Env:         map[string]string{},
			Mounts: []MountTemplate{
				{Source: "{workspace}", Target: "/workspace", ReadOnly: false},
			},
			ResourceLimits: ResourceLimits{
				MemoryMB:       4096,
				CPUCores:       2.0,
				TimeoutSeconds: 3600,
			},
			Capabilities: []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
			Enabled:      true,
			ModelFlag:    "--model",
			Protocol:     agent.ProtocolACP,
			SessionConfig: SessionConfig{
				ResumeViaACP: true, // Claude uses ACP session/load for resume
			},
		},
		{
			ID:          "codex-agent",
			Name:        "OpenAI Codex Agent",
			Description: "OpenAI Codex CLI-powered autonomous coding agent using the Codex app-server protocol.",
			Image:       "kandev/codex-agent",
			Tag:         "latest",
			Cmd:         []string{"codex", "app-server"},
			WorkingDir:  "/workspace",
			RequiredEnv: []string{},
			Env:         map[string]string{},
			Mounts: []MountTemplate{
				{Source: "{workspace}", Target: "/workspace", ReadOnly: false},
			},
			ResourceLimits: ResourceLimits{
				MemoryMB:       4096,
				CPUCores:       2.0,
				TimeoutSeconds: 3600,
			},
			Capabilities: []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
			Enabled:      true,
			ModelFlag:    "--model",
			Protocol:     agent.ProtocolCodex,
			SessionConfig: SessionConfig{
				ResumeViaACP: true, // Codex uses thread/resume via ACP
			},
		},
		{
			ID:          "gemini-agent",
			Name:        "Gemini CLI Agent",
			Description: "Google Gemini CLI-powered autonomous coding agent.",
			Image:       "kandev/gemini-agent",
			Tag:         "latest",
			Cmd:         []string{"gemini", "--acp"},
			WorkingDir:  "/workspace",
			RequiredEnv: []string{},
			Env:         map[string]string{},
			Mounts: []MountTemplate{
				{Source: "{workspace}", Target: "/workspace", ReadOnly: false},
			},
			ResourceLimits: ResourceLimits{
				MemoryMB:       4096,
				CPUCores:       2.0,
				TimeoutSeconds: 3600,
			},
			Capabilities: []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
			Enabled:      true,
			ModelFlag:    "--model",
			Protocol:     agent.ProtocolACP,
			SessionConfig: SessionConfig{
				ResumeViaACP: true, // Gemini uses ACP session/load for resume
			},
		},
		{
			ID:          "opencode-agent",
			Name:        "OpenCode Agent",
			Description: "OpenCode CLI-powered autonomous coding agent.",
			Image:       "kandev/opencode-agent",
			Tag:         "latest",
			Cmd:         []string{"opencode", "--acp"},
			WorkingDir:  "/workspace",
			RequiredEnv: []string{},
			Env:         map[string]string{},
			Mounts: []MountTemplate{
				{Source: "{workspace}", Target: "/workspace", ReadOnly: false},
			},
			ResourceLimits: ResourceLimits{
				MemoryMB:       4096,
				CPUCores:       2.0,
				TimeoutSeconds: 3600,
			},
			Capabilities: []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
			Enabled:      true,
			ModelFlag:    "--model",
			Protocol:     agent.ProtocolACP,
			SessionConfig: SessionConfig{
				ResumeViaACP: true, // OpenCode uses ACP session/load for resume
			},
		},
		{
			ID:          "copilot-agent",
			Name:        "GitHub Copilot Agent",
			Description: "GitHub Copilot CLI-powered autonomous coding agent.",
			Image:       "kandev/copilot-agent",
			Tag:         "latest",
			Cmd:         []string{"copilot", "--acp"},
			WorkingDir:  "/workspace",
			RequiredEnv: []string{},
			Env:         map[string]string{},
			Mounts: []MountTemplate{
				{Source: "{workspace}", Target: "/workspace", ReadOnly: false},
			},
			ResourceLimits: ResourceLimits{
				MemoryMB:       4096,
				CPUCores:       2.0,
				TimeoutSeconds: 3600,
			},
			Capabilities: []string{"code_generation", "code_review", "refactoring", "testing", "shell_execution"},
			Enabled:      true,
			ModelFlag:    "--model",
			Protocol:     agent.ProtocolACP,
			SessionConfig: SessionConfig{
				ResumeViaACP: true, // Copilot uses ACP session/load for resume
			},
		},
	}
}
