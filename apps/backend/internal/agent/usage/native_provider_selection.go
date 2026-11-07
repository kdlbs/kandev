package usage

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// NativeProviderSelectionKnown is a conservative negative guard over the
// actual native command, working directory and selected configuration files.
// It does not execute credential helpers or infer an account from a profile.
func NativeProviderSelectionKnown(input ExecutionUsageInput) bool {
	if !localUsageExecution(input.ExecutorType) || !filepath.IsAbs(input.WorkDir) || len(input.Command) == 0 || uncertainUsageCommand(input.AgentID, input.Command) {
		return false
	}
	var dir string
	switch input.AgentID {
	case executionAgentCodex:
		dir = input.Environment["CODEX_HOME"]
		if dir == "" {
			dir = filepath.Join(executionUsageHome(input.Environment), ".codex")
		}
	case executionAgentClaude:
		dir = input.Environment["CLAUDE_CONFIG_DIR"]
		if dir == "" {
			dir = filepath.Join(executionUsageHome(input.Environment), ".claude")
		}
	default:
		return false
	}
	if !filepath.IsAbs(dir) {
		return false
	}
	paths := usageConfigPaths(input.AgentID, dir, input.WorkDir)
	for _, path := range paths {
		if !usageConfigSelectionKnown(path, input.AgentID) {
			return false
		}
	}
	return true
}

func uncertainUsageCommand(agentID string, command []string) bool {
	program := strings.ToLower(strings.TrimSuffix(filepath.Base(command[0]), ".exe"))
	switch program {
	case "cmd", "powershell", "pwsh", "bash", "sh", "zsh", "env":
		return true
	}
	for index, arg := range command {
		key, _, _ := strings.Cut(arg, "=")
		if key == "-p" && agentID == executionAgentCodex {
			return true
		}
		switch key {
		case "--profile", "--settings", "--setting-sources", "--api-key", "--base-url", "--provider", "--model-provider", "--oss", "--local-provider":
			return true
		}
		if key == "--config" || key == "-c" {
			value := ""
			if _, inline, ok := strings.Cut(arg, "="); ok {
				value = inline
			} else if index+1 < len(command) {
				value = command[index+1]
			}
			setting, _, _ := strings.Cut(value, "=")
			if usageProviderConfigKey(strings.TrimSpace(setting)) {
				return true
			}
		}
	}
	return false
}

func usageProviderConfigKey(key string) bool {
	for _, prefix := range []string{"model_provider", "model_providers", "profile", "openai_base_url", "chatgpt_base_url", "forced_login_method", "forced_chatgpt_account_id", "experimental_bearer_token"} {
		if key == prefix || strings.HasPrefix(key, prefix+".") {
			return true
		}
	}
	return false
}

func usageConfigPaths(agentID, dir, workDir string) []string {
	name, folder := "config.toml", ".codex"
	if agentID == executionAgentClaude {
		name, folder = "settings.json", ".claude"
	}
	paths := []string{filepath.Join(dir, name)}
	for _, project := range usageProjectDirectories(workDir) {
		paths = append(paths, filepath.Join(project, folder, name))
		if agentID == executionAgentClaude {
			paths = append(paths, filepath.Join(project, folder, "settings.local.json"))
		}
	}
	return paths
}

// Stop at the closest Git root. Repo-less executions inspect their actual
// working directory only; a parent home directory is not a project namespace.
func usageProjectDirectories(workDir string) []string {
	paths := []string{}
	for dir := filepath.Clean(workDir); len(paths) < 64; dir = filepath.Dir(dir) {
		paths = append(paths, dir)
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return paths
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return []string{workDir}
}

func usageConfigSelectionKnown(path, agentID string) bool {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return false
	}
	var fields map[string]any
	if agentID == executionAgentCodex {
		if toml.Unmarshal(data, &fields) != nil {
			return false
		}
		return codexUsageConfigKnown(fields)
	}
	if json.Unmarshal(data, &fields) != nil {
		return false
	}
	return claudeUsageConfigKnown(fields)
}

func codexUsageConfigKnown(fields map[string]any) bool {
	for key, value := range fields {
		if !usageProviderConfigKey(key) {
			continue
		}
		// An explicit provider/profile block can change routing or authorization.
		// Do not execute/flatten it to guess that its auth.json is the dispatch source.
		if key == "model_provider" && value == executionProviderOpenAI {
			continue
		}
		return false
	}
	return true
}

func claudeUsageConfigKnown(fields map[string]any) bool {
	if helper, ok := fields["apiKeyHelper"]; ok && helper != "" && helper != nil {
		return false
	}
	if method, ok := fields["forceLoginMethod"]; ok && method != "claudeai" {
		return false
	}
	if org, ok := fields["forceLoginOrgUUID"]; ok && org != "" {
		return false
	}
	if env, ok := fields["env"].(map[string]any); ok {
		for key, value := range env {
			if usageEnvironmentKey(key) && !claudeExecutionControlKey(key) && value != "" {
				return false
			}
		}
	}
	return true
}

// These execution limits do not select a provider, credential source or account.
// Unrecognized Claude settings still require a proven subscription source.
func claudeExecutionControlKey(key string) bool {
	switch usageEnvironmentName(key) {
	case "CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH", "CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS":
		return true
	default:
		return false
	}
}
