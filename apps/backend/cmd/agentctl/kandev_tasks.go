package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
)

// runTasksCmd dispatches `agentctl kandev tasks <subcmd>`. The
// singular `agentctl kandev task <subcmd>` group (get/update/create)
// remains the back-compat path; this plural group adds the verbs
// the office runtime needs without breaking older scripts.
func runTasksCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev tasks <list|move|archive|message|conversation> [flags]")
		return 1
	}
	switch args[0] {
	case subcmdList:
		return tasksList(args[1:])
	case "move":
		return tasksMove(args[1:])
	case "archive":
		return tasksArchive(args[1:])
	case "message":
		return tasksMessage(args[1:])
	case "conversation":
		return tasksConversation(args[1:])
	default:
		cliError("unknown tasks subcommand: %s", args[0])
		return 1
	}
}

// tasksList returns workspace tasks, optionally filtered by status
// or assignee. Hits the office dashboard list endpoint so the
// returned shape matches what the Tasks page sees.
func tasksList(args []string) int {
	fs := flag.NewFlagSet("tasks list", flag.ContinueOnError)
	status := fs.String("status", "", "Filter by status (todo, in_progress, done, …)")
	assignee := fs.String("assignee", "", "Filter by assignee agent ID")
	project := fs.String("project", "", "Filter by project ID")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	wsID := os.Getenv("KANDEV_WORKSPACE_ID")
	path := fmt.Sprintf("/api/v1/office/workspaces/%s/tasks", wsID)
	return getWithParams(path, "KANDEV_WORKSPACE_ID", wsID, map[string]string{
		"status":   *status,
		"assignee": *assignee,
		"project":  *project,
	})
}

// tasksMove fails closed because Office runs do not have a signed runtime
// capability for arbitrary workflow-step moves.
func tasksMove(args []string) int {
	fs := flag.NewFlagSet("tasks move", flag.ContinueOnError)
	_ = fs.String("id", "", "Task ID (defaults to $KANDEV_TASK_ID)")
	_ = fs.String("step", "", "Destination workflow step ID")
	_ = fs.String("prompt", "", "Optional handoff prompt")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	cliError("tasks move is unavailable to Office agents; use `kandev task update --status <status>` for signed status changes, or ask a human or admin to move the task between workflow steps")
	return 1
}

// tasksArchive fails closed because Office runs do not have a signed runtime
// capability for archival.
func tasksArchive(args []string) int {
	fs := flag.NewFlagSet("tasks archive", flag.ContinueOnError)
	_ = fs.String("id", "", "Task ID (defaults to $KANDEV_TASK_ID)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	cliError("tasks archive is unavailable to Office agents; ask a human or admin to archive the task")
	return 1
}

// tasksMessage posts a comment through the signed Office runtime surface.
// The runtime validates task scope and derives attribution from the run token.
func tasksMessage(args []string) int {
	fs := flag.NewFlagSet("tasks message", flag.ContinueOnError)
	id := fs.String("id", "", "Task ID (required; defaults to $KANDEV_TASK_ID)")
	prompt := fs.String("prompt", "", "Message body (required; use \"-\" for stdin)")
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
	message, err := resolveBody(*prompt)
	if err != nil {
		cliError("read prompt: %v", err)
		return 1
	}
	if taskID == "" || message == "" {
		cliError("--id (or KANDEV_TASK_ID) and --prompt are required")
		return 1
	}
	payload := map[string]string{
		"task_id": taskID,
		"body":    message,
	}
	body, status, err := client.do(http.MethodPost,
		"/api/v1/office/runtime/comments", payload)
	return handleResponse(body, status, err)
}

// tasksConversation lists comments on a task. Useful for catching
// up after a wake when the prepended-summary slice is empty.
func tasksConversation(args []string) int {
	fs := flag.NewFlagSet("tasks conversation", flag.ContinueOnError)
	id := fs.String("id", "", "Task ID (required; defaults to $KANDEV_TASK_ID)")
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
		cliError("--id (or KANDEV_TASK_ID) required")
		return 1
	}
	body, status, err := client.do(http.MethodGet,
		fmt.Sprintf("/api/v1/office/tasks/%s/comments", taskID), nil)
	return handleResponse(body, status, err)
}
