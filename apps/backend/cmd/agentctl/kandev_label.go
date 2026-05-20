package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
)

// runLabelCmd dispatches label sub-commands: add, remove, list.
func runLabelCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev label <add|remove|list> [flags]")
		return 1
	}
	switch args[0] {
	case "add":
		return labelAdd(args[1:])
	case "remove":
		return labelRemove(args[1:])
	case subcmdList:
		return labelList(args[1:])
	default:
		cliError("unknown label subcommand: %s", args[0])
		return 1
	}
}

// labelAdd adds a named label to a task.
func labelAdd(args []string) int {
	fs := flag.NewFlagSet("label add", flag.ContinueOnError)
	taskFlag := fs.String("task", "", "Task ID (defaults to $KANDEV_TASK_ID)")
	nameFlag := fs.String("name", "", "Label name (required)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	taskID := resolveTaskID(*taskFlag, client.taskID)
	if taskID == "" {
		cliError("task ID required: use --task or set KANDEV_TASK_ID")
		return 1
	}
	if *nameFlag == "" {
		cliError("--name is required")
		return 1
	}
	if client.workspaceID == "" {
		cliError("KANDEV_WORKSPACE_ID must be set")
		return 1
	}

	payload := map[string]string{"name": *nameFlag}
	path := fmt.Sprintf("/api/v1/office/workspaces/%s/tasks/%s/labels",
		client.workspaceID, taskID)
	body, status, doErr := client.do(http.MethodPost, path, payload)
	return handleResponse(body, status, doErr)
}

// labelRemove removes a named label from a task.
func labelRemove(args []string) int {
	fs := flag.NewFlagSet("label remove", flag.ContinueOnError)
	taskFlag := fs.String("task", "", "Task ID (defaults to $KANDEV_TASK_ID)")
	nameFlag := fs.String("name", "", "Label name (required)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	taskID := resolveTaskID(*taskFlag, client.taskID)
	if taskID == "" {
		cliError("task ID required: use --task or set KANDEV_TASK_ID")
		return 1
	}
	if *nameFlag == "" {
		cliError("--name is required")
		return 1
	}
	if client.workspaceID == "" {
		cliError("KANDEV_WORKSPACE_ID must be set")
		return 1
	}

	path := fmt.Sprintf("/api/v1/office/workspaces/%s/tasks/%s/labels/%s",
		client.workspaceID, taskID, url.PathEscape(*nameFlag))
	body, status, doErr := client.do(http.MethodDelete, path, nil)
	return handleResponse(body, status, doErr)
}

// labelList lists all labels attached to a task.
func labelList(args []string) int {
	fs := flag.NewFlagSet("label list", flag.ContinueOnError)
	taskFlag := fs.String("task", "", "Task ID (defaults to $KANDEV_TASK_ID)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	taskID := resolveTaskID(*taskFlag, client.taskID)
	if taskID == "" {
		cliError("task ID required: use --task or set KANDEV_TASK_ID")
		return 1
	}
	if client.workspaceID == "" {
		cliError("KANDEV_WORKSPACE_ID must be set")
		return 1
	}

	path := fmt.Sprintf("/api/v1/office/workspaces/%s/tasks/%s/labels",
		client.workspaceID, taskID)
	body, status, doErr := client.do(http.MethodGet, path, nil)
	return handleResponse(body, status, doErr)
}
