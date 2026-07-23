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

// taskUpdate changes task status through the authenticated Office runtime API.
func taskUpdate(args []string) int {
	fs := flag.NewFlagSet("task update", flag.ContinueOnError)
	id := fs.String("id", "", "Task ID (defaults to $KANDEV_TASK_ID)")
	status := fs.String("status", "", "New status")
	comment := fs.String("comment", "", "Comment to add")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	if strings.TrimSpace(*status) == "" {
		if strings.TrimSpace(*comment) != "" {
			cliError("--status is required; use `kandev tasks message` for comment-only updates")
		} else {
			cliError("nothing to update: pass --status")
		}
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

	payload := map[string]string{"status": *status}
	if *comment != "" {
		payload["comment"] = *comment
	}

	body, statusCode, err := client.do(http.MethodPost,
		fmt.Sprintf("/api/v1/office/runtime/tasks/%s/status", taskID), payload)
	return handleResponse(body, statusCode, err)
}

// taskCreate creates a new task through the authenticated Office runtime API.
func taskCreate(args []string) int {
	fs := flag.NewFlagSet("task create", flag.ContinueOnError)
	title := fs.String("title", "", "Task title (required)")
	description := fs.String("description", "", "Task description")
	parent := fs.String("parent", "", "Parent task ID")
	assignee := fs.String("assignee", "", "Assignee agent ID")
	priority := fs.String("priority", "", "Priority value")
	project := fs.String("project", "", "Project ID")
	blockedBy := fs.String("blocked-by", "", "Comma-separated task IDs that must complete before this task")
	workspaceMode := fs.String("workspace-mode", "", "Workspace mode for this task: inherit_parent, new_workspace, or shared_group")
	workspaceGroupID := fs.String("workspace-group-id", "", "Workspace group ID to join (required when --workspace-mode=shared_group)")
	defaultChildWorkspace := fs.String("default-child-workspace", "", "Parent-only: default workspace mode for children (inherit_parent or new_workspace)")
	defaultChildOrdering := fs.String("default-child-ordering", "", "Parent-only ordering policy for children. 'sequential' records dependency edges between siblings; 'parallel' records none. Note: recording an edge does NOT by itself defer when a child starts.")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	normalizedTitle := strings.TrimSpace(*title)
	if normalizedTitle == "" {
		cliError("--title is required")
		return 1
	}
	unsupported := []struct {
		name  string
		value string
	}{
		{"priority", *priority},
		{"blocked-by", *blockedBy},
		{"workspace-mode", *workspaceMode},
		{"workspace-group-id", *workspaceGroupID},
		{"default-child-workspace", *defaultChildWorkspace},
		{"default-child-ordering", *defaultChildOrdering},
	}
	for _, field := range unsupported {
		if strings.TrimSpace(field.value) != "" {
			cliError("--%s is not supported by Office runtime task create", field.name)
			return 1
		}
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	payload := map[string]interface{}{
		"title": normalizedTitle,
	}
	if *description != "" {
		payload["description"] = *description
	}
	if *parent != "" {
		payload["parent_id"] = *parent
	}
	if *assignee != "" {
		payload["assignee"] = *assignee
	}
	if *project != "" {
		payload["project_id"] = *project
	}

	body, status, err := client.do(http.MethodPost, "/api/v1/office/runtime/tasks", payload)
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
