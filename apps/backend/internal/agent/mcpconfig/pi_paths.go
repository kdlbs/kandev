package mcpconfig

import (
	"fmt"
	"path/filepath"
	"strings"
)

func piMCPConfigPath(paths PassthroughPaths) (string, error) {
	if !paths.Isolated {
		if paths.WorkspaceDir == "" {
			return "", nil
		}
		return paths.projectPath(".pi", "mcp.json"), nil
	}
	home := paths.IsolatedHomeDir
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("pi MCP requires isolated agent state")
	}
	directory := paths.Environment["PI_CODING_AGENT_DIR"]
	switch {
	case directory == "":
		directory = filepath.Join(home, ".pi", "agent")
	case directory == "~":
		directory = home
	case strings.HasPrefix(directory, "~/"):
		directory = filepath.Join(home, directory[2:])
	case !filepath.IsAbs(directory):
		directory = filepath.Join(paths.WorkspaceDir, directory)
	}
	relative, err := filepath.Rel(home, directory)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("pi MCP configuration must use isolated agent state")
	}
	return filepath.Join(directory, "mcp.json"), nil
}
