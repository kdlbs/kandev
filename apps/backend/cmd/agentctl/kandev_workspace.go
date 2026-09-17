package main

import (
	"flag"
	"net/http"
	"net/url"
)

func workspaceCatalog() int {
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	body, status, err := client.do(http.MethodGet, "/api/v1/office/runtime/workspace", nil)
	return handleResponse(body, status, err)
}

func taskManage(args []string) int {
	fs := flag.NewFlagSet("task manage", flag.ContinueOnError)
	operation := registerAssistantMutation(fs)
	id := fs.String("id", "", "Existing task ID")
	action := fs.String("action", "adopt", "adopt, assign, start, stop, or message")
	assignee := fs.String(kandevAssigneeKey, "", "Workspace worker persona ID")
	session := fs.String("session", "", "Existing task session ID")
	prompt := fs.String("prompt", "", "Message for the worker")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *id == "" {
		cliError("--id is required")
		return 1
	}
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	payload := map[string]any{"action": *action, kandevAssigneeKey: *assignee, "session_id": *session, "prompt": *prompt}
	operation.add(payload)
	body, status, err := client.do(http.MethodPost, "/api/v1/office/runtime/tasks/"+url.PathEscape(*id)+"/manage", payload)
	return handleResponse(body, status, err)
}

func taskInspect(args []string) int {
	fs := flag.NewFlagSet("task inspect", flag.ContinueOnError)
	id := fs.String("id", "", "Task ID")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *id == "" {
		cliError("--id is required")
		return 1
	}
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	body, status, err := client.do(http.MethodGet, "/api/v1/office/runtime/tasks/"+url.PathEscape(*id)+"/details", nil)
	return handleResponse(body, status, err)
}
