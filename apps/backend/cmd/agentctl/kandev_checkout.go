package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
)

// runCheckoutCmd attempts to claim a task for the current agent.
// Returns a clear message on 409 (already claimed by another agent).
func runCheckoutCmd(args []string) int {
	fs := flag.NewFlagSet("checkout", flag.ContinueOnError)
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

	payload := map[string]string{
		"agent_id": client.agentID,
	}

	body, status, doErr := client.do(http.MethodPost,
		fmt.Sprintf("/api/v1/office/tasks/%s/checkout", taskID), payload)
	if doErr != nil {
		cliError("%v", doErr)
		return 1
	}

	if status == http.StatusConflict {
		return handleCheckoutConflict(body)
	}

	return handleResponse(body, status, nil)
}

// handleCheckoutConflict parses a 409 response and outputs a clear error.
func handleCheckoutConflict(body []byte) int {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err == nil {
		if msg, ok := resp["error"].(string); ok {
			cliError("task already claimed: %s", msg)
			return 1
		}
	}
	cliError("task already claimed by another agent")
	return 1
}
