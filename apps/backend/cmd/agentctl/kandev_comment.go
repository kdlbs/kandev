package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
)

func runCommentCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev comment <add|list> [flags]")
		return 1
	}
	switch args[0] {
	case "add":
		return commentAdd(args[1:])
	case subcmdList:
		return commentList(args[1:])
	default:
		cliError("unknown comment subcommand: %s", args[0])
		return 1
	}
}

// commentAdd posts a comment on a task. If --body is "-", reads from stdin.
func commentAdd(args []string) int {
	fs := flag.NewFlagSet("comment add", flag.ContinueOnError)
	taskFlag := fs.String("task", "", "Task ID (defaults to $KANDEV_TASK_ID)")
	bodyFlag := fs.String("body", "", "Comment body (use \"-\" for stdin)")
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

	body, err := resolveBody(*bodyFlag)
	if err != nil {
		cliError("read body: %v", err)
		return 1
	}
	if body == "" {
		cliError("--body is required")
		return 1
	}

	payload := map[string]string{
		"body":        body,
		"author_type": "agent",
		"author_id":   client.agentID,
	}

	respBody, status, doErr := client.do(http.MethodPost,
		fmt.Sprintf("/api/v1/office/tasks/%s/comments", taskID), payload)
	return handleResponse(respBody, status, doErr)
}

// commentList retrieves comments for a task with optional limit.
func commentList(args []string) int {
	fs := flag.NewFlagSet("comment list", flag.ContinueOnError)
	taskFlag := fs.String("task", "", "Task ID (defaults to $KANDEV_TASK_ID)")
	limitFlag := fs.Int("limit", 0, "Max number of comments (0 = all)")
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

	path := fmt.Sprintf("/api/v1/office/tasks/%s/comments", taskID)
	if *limitFlag > 0 {
		path += "?limit=" + strconv.Itoa(*limitFlag)
	}

	body, status, doErr := client.do(http.MethodGet, path, nil)
	return handleResponse(body, status, doErr)
}

// resolveBody returns the body string, or reads from stdin when value is "-".
func resolveBody(value string) (string, error) {
	if value != "-" {
		return value, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return string(data), nil
}
