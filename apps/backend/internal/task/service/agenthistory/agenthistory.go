// Package agenthistory removes the conversation history an agent CLI keeps in
// its own store for one agent session. Every removed path is derived from the
// session's identity; nothing is matched by prefix or substring.
package agenthistory

import (
	"context"
	"path/filepath"
	"regexp"
)

// Agent names whose history this package can remove.
const (
	AgentClaudeACP      = "claude-acp"
	AgentCodexACP       = "codex-acp"
	AgentCodexAppServer = "codex-app-server"
)

// Identity names one agent session's history.
type Identity struct {
	// AgentName is the agent profile's agent name, e.g. "claude-acp".
	AgentName string
	// ResumeToken is the agent's own session (Claude) or thread (Codex) ID.
	ResumeToken string
	// WorkDir is the absolute working directory the agent ran in.
	WorkDir string
	// Env holds the agent profile environment; config-dir overrides are read from it.
	Env map[string]string
	// HomeDir is the home directory the agent ran under.
	HomeDir string
}

var safeTokenPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// Remove deletes the history files that belong to id. Absent history and
// unsupported agents are not errors.
func Remove(ctx context.Context, id Identity) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if !safeTokenPattern.MatchString(id.ResumeToken) || id.WorkDir == "" || !filepath.IsAbs(id.WorkDir) {
		return nil
	}
	switch id.AgentName {
	case AgentClaudeACP:
		return removeClaude(ctx, id)
	case AgentCodexACP, AgentCodexAppServer:
		return removeCodex(ctx, id)
	default:
		return nil
	}
}

// configRoot resolves an agent's history root from an environment override or
// a directory under the home directory. It returns "" when no root is known.
func configRoot(id Identity, envKey, homeSubdir string) string {
	if dir := id.Env[envKey]; dir != "" {
		if !filepath.IsAbs(dir) {
			return ""
		}
		return filepath.Clean(dir)
	}
	if id.HomeDir == "" || !filepath.IsAbs(id.HomeDir) {
		return ""
	}
	return filepath.Join(id.HomeDir, homeSubdir)
}
