package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// liveRepositoryAttachmentScenarioPrefix is the prompt directive handled by
// scenarioLiveRepositoryAttachment. The JSON arguments identify the second
// repository to attach through the real in-turn legacy MCP tool.
const liveRepositoryAttachmentScenarioPrefix = "live-repository-attachment"

type liveRepositoryAttachmentArgs struct {
	RepositoryID   string `json:"repository_id"`
	LocalPath      string `json:"local_path"`
	BaseBranch     string `json:"base_branch"`
	CheckoutBranch string `json:"checkout_branch"`
}

// scenarioLiveRepositoryAttachment emits relative links before and after a real
// add_branch_to_task_kandev call during the active turn. It exercises the same
// MCP tool a production agent calls, so the browser proof cannot pass by
// simulating an idle batch attachment or a store write.
//
// Prompt shape:
//
//	/e2e:live-repository-attachment {"repository_id":"…","base_branch":"main"}
func scenarioLiveRepositoryAttachment(e *emitter) {
	args, err := parseLiveRepositoryAttachmentArgs(e.scenarioArgs)
	if err != nil {
		e.text("Live repository attachment scenario is misconfigured: " + err.Error())
		return
	}
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		e.text("Live repository attachment scenario could not read the agent working directory: " + cwdErr.Error())
		return
	}
	primaryDir := filepath.Base(cwd)

	e.text(fmt.Sprintf(
		"Before attachment, this work order lives in the agent checkout. [primary before](implementation_plans/task-00.md)\n\n`agent cwd: %s`",
		cwd,
	))

	toolID := nextToolID()
	// No task_id: the task-bound MCP server scopes the legacy tool to the
	// current session's task, so the scenario cannot name another task.
	toolArgs := map[string]any{
		"base_branch": args.BaseBranch,
	}
	if args.RepositoryID != "" {
		toolArgs["repository_id"] = args.RepositoryID
	}
	if args.LocalPath != "" {
		toolArgs["local_path"] = args.LocalPath
	}
	if args.CheckoutBranch != "" {
		toolArgs["checkout_branch"] = args.CheckoutBranch
	}
	e.startTool(toolID, "add_branch_to_task_kandev", acp.ToolKindOther, toolArgs)
	result, err := e.callMCPTool("kandev", "add_branch_to_task_kandev", toolArgs)
	if err != nil {
		e.completeTool(toolID, map[string]any{toolKeyError: "MCP error: " + err.Error()})
		e.text("Live repository attachment failed: " + err.Error())
		return
	}
	e.completeTool(toolID, map[string]any{toolKeyResult: result})

	secondaryPrefix, ok := attachmentWorkspacePrefix(result)
	if !ok {
		e.text("Attachment succeeded but the tool response omitted the materialized worktree identity; refusing to guess a source prefix.")
		return
	}
	e.text(fmt.Sprintf(
		"Attachment succeeded without restarting the agent. The same work order is still available from its original checkout. [primary after](implementation_plans/task-00.md)\n\n"+
			"An explicit source prefix keeps the primary checkout. [primary qualified](%s/README.md)\n\n"+
			"A duplicate README in the attached repository is addressable only by its source prefix. [secondary README](%s/README.md)",
		primaryDir,
		secondaryPrefix,
	))
}

// attachmentWorkspacePrefix derives the Files-root-relative directory of the
// materialized sibling from the real tool response. The tool result is the
// authority: worktree directory names include the branch slug and must not be
// guessed from the repository source name. A response without both paths is a
// hard failure so the browser scenario cannot pass with a fabricated link.
func attachmentWorkspacePrefix(result string) (string, bool) {
	var response struct {
		TaskWorkspacePath string `json:"task_workspace_path"`
		WorktreePath      string `json:"worktree_path"`
	}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		return "", false
	}
	workspace := filepath.Clean(response.TaskWorkspacePath)
	worktree := filepath.Clean(response.WorktreePath)
	if workspace == "." || worktree == "." || workspace == "" || worktree == "" {
		return "", false
	}
	relative, err := filepath.Rel(workspace, worktree)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func parseLiveRepositoryAttachmentArgs(raw string) (liveRepositoryAttachmentArgs, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return liveRepositoryAttachmentArgs{}, fmt.Errorf("missing JSON arguments")
	}
	var args liveRepositoryAttachmentArgs
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return liveRepositoryAttachmentArgs{}, fmt.Errorf("invalid JSON arguments: %w", err)
	}
	if args.RepositoryID == "" && args.LocalPath == "" {
		return liveRepositoryAttachmentArgs{}, fmt.Errorf("repository_id or local_path is required")
	}
	if args.RepositoryID != "" && args.LocalPath != "" {
		return liveRepositoryAttachmentArgs{}, fmt.Errorf("pass only one repository locator")
	}
	if args.BaseBranch == "" {
		args.BaseBranch = "main"
	}
	return args, nil
}
