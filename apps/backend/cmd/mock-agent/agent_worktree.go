package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/common/subproc"
)

// agentWorktreeArgs configures the two agent-created-worktree scenarios.
type agentWorktreeArgs struct {
	Branch string `json:"branch"`
}

// scenarioAgentWorktreeUnreported creates a linked worktree inside the task
// workspace and does not report it, so only passive detection can attach it.
func scenarioAgentWorktreeUnreported(e *emitter) {
	scenarioAgentWorktree(e, false)
}

// scenarioAgentWorktreeReported creates the same worktree and reports it with
// the real add_branch_to_task_kandev MCP tool.
func scenarioAgentWorktreeReported(e *emitter) {
	scenarioAgentWorktree(e, true)
}

func scenarioAgentWorktree(e *emitter, report bool) {
	args, err := parseAgentWorktreeArgs(e.scenarioArgs)
	if err != nil {
		e.text("Agent worktree scenario is misconfigured: " + err.Error())
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		e.text("Agent worktree scenario could not read the agent working directory: " + err.Error())
		return
	}
	workspaceRoot, primaryRepo, err := agentWorktreeRoots(cwd)
	if err != nil {
		e.text("Agent worktree scenario could not resolve the task workspace: " + err.Error())
		return
	}
	worktreePath := filepath.Join(workspaceRoot, ".claude", "worktrees", args.Branch)
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		e.text("Agent worktree scenario could not create the worktree parent: " + err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(e.ctx, 30*time.Second)
	defer cancel()
	if _, statErr := os.Stat(worktreePath); statErr != nil {
		if err := runAgentWorktreeGit(ctx, primaryRepo, "worktree", "add", "-b", args.Branch, worktreePath, "main"); err != nil {
			e.text("Agent worktree scenario could not create the worktree: " + err.Error())
			return
		}
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "worktree-change.txt"), []byte("agent worktree change\n"), 0o644); err != nil {
		e.text("Agent worktree scenario could not write its change: " + err.Error())
		return
	}
	e.text(fmt.Sprintf("Created a linked worktree at `%s` on branch `%s`.", worktreePath, args.Branch))
	if !report {
		return
	}
	toolID := nextToolID()
	toolArgs := map[string]any{"worktree_path": worktreePath, "base_branch": "main"}
	e.startTool(toolID, "add_branch_to_task_kandev", acp.ToolKindOther, toolArgs)
	result, err := e.callMCPTool("kandev", "add_branch_to_task_kandev", toolArgs)
	if err != nil {
		e.completeTool(toolID, map[string]any{toolKeyError: "MCP error: " + err.Error()})
		e.text("Reporting the worktree failed: " + err.Error())
		return
	}
	e.completeTool(toolID, map[string]any{toolKeyResult: result})
	e.text("Reported the worktree with add_branch_to_task_kandev.")
}

func parseAgentWorktreeArgs(raw string) (agentWorktreeArgs, error) {
	args := agentWorktreeArgs{Branch: "fix-b"}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return args, nil
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return args, fmt.Errorf("invalid JSON arguments: %w", err)
	}
	if strings.TrimSpace(args.Branch) == "" {
		args.Branch = "fix-b"
	}
	return args, nil
}

// agentWorktreeRoots returns the task workspace root and the repository whose
// worktree registry should own the new checkout. The agent CWD is either the
// single-repository checkout or the multi-repository task root.
func agentWorktreeRoots(cwd string) (string, string, error) {
	if isGitWorktreeRoot(cwd) {
		return filepath.Dir(filepath.Clean(cwd)), cwd, nil
	}
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return "", "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		candidate := filepath.Join(cwd, entry.Name())
		if isGitWorktreeRoot(candidate) {
			return cwd, candidate, nil
		}
	}
	return "", "", fmt.Errorf("no git repository found under %s", cwd)
}

func isGitWorktreeRoot(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info != nil
}

func runAgentWorktreeGit(ctx context.Context, dir string, args ...string) error {
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := subproc.NewGitCommand(ctx, cmdArgs...)
	if out, err := subproc.RunGitOutputClass(ctx, subproc.GitBackground, cmd); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
