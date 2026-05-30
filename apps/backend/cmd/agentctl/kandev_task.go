package main

import (
	"flag"
	"fmt"
	"net/http"
	"strings"
)

func runTaskCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev task <get|update|create> [flags]")
		return 1
	}
	switch args[0] {
	case subcmdGet:
		return taskGet(args[1:])
	case "update":
		return taskUpdate(args[1:])
	case subcmdCreate:
		return taskCreate(args[1:])
	default:
		cliError("unknown task subcommand: %s", args[0])
		return 1
	}
}

// taskGet fetches a task by ID. Defaults to $KANDEV_TASK_ID when --id is omitted.
func taskGet(args []string) int {
	fs := flag.NewFlagSet("task get", flag.ContinueOnError)
	id := fs.String("id", "", "Task ID (defaults to $KANDEV_TASK_ID)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	taskID := resolveTaskID(*id, client.taskID)
	if taskID == "" {
		cliError("task ID required: use --id or set KANDEV_TASK_ID")
		return 1
	}

	body, status, err := client.do(http.MethodGet,
		fmt.Sprintf("/api/v1/office/tasks/%s", taskID), nil)
	return handleResponse(body, status, err)
}

// taskUpdate patches a task with optional status and comment fields.
func taskUpdate(args []string) int {
	fs := flag.NewFlagSet("task update", flag.ContinueOnError)
	id := fs.String("id", "", "Task ID (defaults to $KANDEV_TASK_ID)")
	status := fs.String("status", "", "New status")
	comment := fs.String("comment", "", "Comment to add")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	taskID := resolveTaskID(*id, client.taskID)
	if taskID == "" {
		cliError("task ID required: use --id or set KANDEV_TASK_ID")
		return 1
	}

	payload := make(map[string]string)
	if *status != "" {
		payload["status"] = *status
	}
	if *comment != "" {
		payload["comment"] = *comment
	}
	if len(payload) == 0 {
		cliError("nothing to update: pass --status and/or --comment")
		return 1
	}

	body, statusCode, err := client.do(http.MethodPatch,
		fmt.Sprintf("/api/v1/office/tasks/%s", taskID), payload)
	return handleResponse(body, statusCode, err)
}

// taskCreate creates a new task via the kanban task API.
func taskCreate(args []string) int {
	fs := flag.NewFlagSet("task create", flag.ContinueOnError)
	title := fs.String("title", "", "Task title (required)")
	parent := fs.String("parent", "", "Parent task ID")
	assignee := fs.String("assignee", "", "Assignee agent ID")
	priority := fs.String("priority", "", "Priority value")
	blockedBy := fs.String("blocked-by", "", "Comma-separated task IDs that must complete before this task")
	workspaceMode := fs.String("workspace-mode", "", "Workspace mode for this task: inherit_parent, new_workspace, or shared_group")
	workspaceGroupID := fs.String("workspace-group-id", "", "Workspace group ID to join (required when --workspace-mode=shared_group)")
	defaultChildWorkspace := fs.String("default-child-workspace", "", "Parent-only: default workspace mode for children (inherit_parent or new_workspace)")
	defaultChildOrdering := fs.String("default-child-ordering", "", "Parent-only: ordering for children — 'sequential' adds dependency edges between siblings, 'parallel' lets them run concurrently")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	if *title == "" {
		cliError("--title is required")
		return 1
	}
	if strings.TrimSpace(*workspaceMode) == workspaceModeSharedGroup && strings.TrimSpace(*workspaceGroupID) == "" {
		cliError("--workspace-group-id is required when --workspace-mode=%s", workspaceModeSharedGroup)
		return 1
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	payload := map[string]interface{}{"title": *title}
	if *parent != "" {
		payload["parent_id"] = *parent
	}
	if *assignee != "" {
		payload["assignee"] = *assignee
	}
	if *priority != "" {
		payload["priority"] = *priority
	}
	if *blockedBy != "" {
		ids := strings.Split(*blockedBy, ",")
		for i, id := range ids {
			ids[i] = strings.TrimSpace(id)
		}
		payload["blocked_by"] = ids
	}
	// Workspace-policy fields (office task-handoffs). Empty values fall
	// through so the backend resolves defaults / parent inheritance.
	if m := strings.TrimSpace(*workspaceMode); m != "" {
		payload["workspace_mode"] = m
	}
	if gid := strings.TrimSpace(*workspaceGroupID); gid != "" {
		payload["workspace_group_id"] = gid
	}
	if *defaultChildWorkspace != "" {
		payload["default_child_workspace"] = *defaultChildWorkspace
	}
	if *defaultChildOrdering != "" {
		payload["default_child_ordering"] = *defaultChildOrdering
	}

	body, status, err := client.do(http.MethodPost, "/api/v1/tasks", payload)
	return handleResponse(body, status, err)
}

// resolveTaskID returns the explicit ID if provided, otherwise falls back to
// the default (from KANDEV_TASK_ID env var).
func resolveTaskID(explicit, defaultID string) string {
	if explicit != "" {
		return explicit
	}
	return defaultID
}
