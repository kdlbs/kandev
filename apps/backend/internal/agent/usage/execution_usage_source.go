package usage

import (
	"path/filepath"
	"runtime"
	"strings"
)

func resolveExecutionUsageSource(input ExecutionUsageInput) (executionUsageSource, string) {
	if input.ExecutionID == "" || input.Environment == nil {
		return executionUsageSource{}, "execution_unknown"
	}
	if !localUsageExecution(input.ExecutorType) {
		return executionUsageSource{}, "unsupported_executor"
	}
	env := input.Environment
	switch input.AgentID {
	case executionAgentCodex:
		if env["OPENAI_API_KEY"] != "" || env["CODEX_API_KEY"] != "" || env["OPENAI_BASE_URL"] != "" {
			return executionUsageSource{}, executionSelectionUnknown
		}
		dir := env["CODEX_HOME"]
		if dir == "" {
			dir = filepath.Join(executionUsageHome(env), ".codex")
		}
		if !filepath.IsAbs(dir) {
			return executionUsageSource{}, "credential_source_unknown"
		}
		return executionUsageSource{provider: executionProviderOpenAI, path: filepath.Join(dir, "auth.json")}, ""
	case executionAgentClaude:
		return resolveClaudeExecutionUsageSource(env)
	default:
		return executionUsageSource{}, "unsupported_agent"
	}
}

func executionUsageHome(env map[string]string) string {
	if runtime.GOOS == "windows" {
		return env["USERPROFILE"]
	}
	return env["HOME"]
}

func resolveClaudeExecutionUsageSource(env map[string]string) (executionUsageSource, string) {
	for key, value := range env {
		if value != "" && (strings.HasPrefix(key, "CLAUDE_CODE_USE_") || key == "ANTHROPIC_API_KEY" || key == "ANTHROPIC_AUTH_TOKEN" || key == "ANTHROPIC_PROFILE" || key == "ANTHROPIC_ORGANIZATION_ID" || key == "ANTHROPIC_WORKSPACE_ID") {
			return executionUsageSource{}, executionSelectionUnknown
		}
	}
	if base := strings.TrimRight(env["ANTHROPIC_BASE_URL"], "/"); base != "" && base != "https://api.anthropic.com" {
		return executionUsageSource{}, executionSelectionUnknown
	}
	if token := env["CLAUDE_CODE_OAUTH_TOKEN"]; token != "" {
		return executionUsageSource{provider: "anthropic", token: token}, ""
	}
	dir := env["CLAUDE_CONFIG_DIR"]
	if dir == "" {
		dir = filepath.Join(executionUsageHome(env), ".claude")
	}
	if !filepath.IsAbs(dir) {
		return executionUsageSource{}, "credential_source_unknown"
	}
	return executionUsageSource{provider: "anthropic", path: filepath.Join(dir, ".credentials.json")}, ""
}
