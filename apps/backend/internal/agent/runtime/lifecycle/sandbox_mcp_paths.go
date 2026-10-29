package lifecycle

import (
	"fmt"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"path/filepath"
)

func sandboxMCPPaths(execution *AgentExecution) mcpconfig.PassthroughPaths {
	paths := mcpconfig.PassthroughPaths{WorkspaceDir: execution.WorkspacePath, Isolated: true, Environment: execution.RuntimeEnvironment()}
	execution.sandboxMu.Lock()
	defer execution.sandboxMu.Unlock()
	if execution.sandboxAttempt != nil {
		launch := execution.sandboxAttempt.Launch()
		paths.IsolatedHomeDir = launch.ChildHome
		execution.sandboxConfigHome = launch.ChildHome
		if launch.ChildHome != "" {
			paths.TempConfigPath = filepath.Join(launch.ChildHome, ".config", "kandev-mcp-"+safePassthroughMCPConfigName(launch.AttemptID)+".json")
		}
	}
	return paths
}

func sandboxMCPFileRoot(execution *AgentExecution, path string) (string, string) {
	execution.sandboxMu.Lock()
	defer execution.sandboxMu.Unlock()
	home := execution.sandboxConfigHome
	if execution.sandboxAttempt != nil {
		home = execution.sandboxAttempt.Launch().ChildHome
		execution.sandboxConfigHome = home
	}
	for _, root := range []string{home, execution.WorkspacePath} {
		if !filepath.IsAbs(root) || !filepath.IsAbs(path) {
			continue
		}
		relative, err := filepath.Rel(root, path)
		if err == nil && filepath.IsLocal(relative) && relative != "." {
			return root, relative
		}
	}
	return "", ""
}

func validateSandboxMCPArtifacts(execution *AgentExecution, serverCount int, artifacts mcpconfig.PassthroughArtifacts) error {
	if execution.SandboxSelection != nil && serverCount > 0 && len(artifacts.Files) == 0 && len(artifacts.Args) == 0 && len(artifacts.Env) == 0 {
		return fmt.Errorf("sandbox MCP configuration location is unavailable: %w", contract.ErrInvalidPolicy)
	}
	return nil
}
