package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
)

// runRoutinesCmd dispatches `agentctl kandev routines <subcmd>`.
// Office routines schedule recurring work via cron or webhook; the
// CEO uses this group to install standups, weekly reports, etc.
func runRoutinesCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev routines <list|create|pause|resume|delete> [flags]")
		return 1
	}
	switch args[0] {
	case subcmdList:
		return routinesList(args[1:])
	case subcmdCreate:
		return routinesCreate(args[1:])
	case "pause":
		return routinesSetStatus(args[1:], "paused")
	case "resume":
		return routinesSetStatus(args[1:], "active")
	case "delete":
		return routinesDelete(args[1:])
	default:
		cliError("unknown routines subcommand: %s", args[0])
		return 1
	}
}

func routinesList(args []string) int {
	fs := flag.NewFlagSet("routines list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	wsID := os.Getenv("KANDEV_WORKSPACE_ID")
	path := fmt.Sprintf("/api/v1/office/workspaces/%s/routines", wsID)
	return getWithParams(path, "KANDEV_WORKSPACE_ID", wsID, nil)
}

// routinesCreate provisions a routine + cron trigger in two
// requests. The trigger is created only when --cron is set; webhook
// triggers can be added later via the UI or future subcommand.
//
// Why two calls: the backend keeps the routine and its trigger in
// separate tables so a routine can carry multiple triggers (cron +
// webhook) without redesigning the create endpoint. The cost of two
// round-trips is acceptable for an interactive CEO action.
func routinesCreate(args []string) int {
	fs := flag.NewFlagSet("routines create", flag.ContinueOnError)
	name := fs.String("name", "", "Routine name (required)")
	taskTitle := fs.String("task-title", "", "Title for tasks created by each fire (required)")
	taskDesc := fs.String("task-description", "", "Description for tasks created by each fire")
	assignee := fs.String("assignee", "", "Agent ID that receives the created task (required)")
	cron := fs.String("cron", "", "Cron expression for the fire trigger (e.g. '0 9 * * MON-FRI')")
	timezone := fs.String("timezone", "UTC", "Timezone for the cron trigger")
	concurrency := fs.String("concurrency", "coalesce_if_active", "Concurrency policy")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	if *name == "" || *taskTitle == "" || *assignee == "" {
		cliError("--name, --task-title and --assignee are required")
		return 1
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	wsID := os.Getenv("KANDEV_WORKSPACE_ID")
	if wsID == "" {
		cliError("KANDEV_WORKSPACE_ID must be set")
		return 1
	}

	template := map[string]string{"title": *taskTitle}
	if *taskDesc != "" {
		template["description"] = *taskDesc
	}
	tplJSON, _ := json.Marshal(template)

	body, status, err := client.do(http.MethodPost,
		fmt.Sprintf("/api/v1/office/workspaces/%s/routines", wsID),
		map[string]any{
			"name":                      *name,
			"task_template":             string(tplJSON),
			"assignee_agent_profile_id": *assignee,
			"concurrency_policy":        *concurrency,
		})
	if err != nil || status >= 300 {
		return handleResponse(body, status, err)
	}

	if *cron == "" {
		cliOutput(body)
		return 0
	}
	// Extract the routine id so the trigger call knows where to attach.
	var created struct {
		ID      string `json:"id"`
		Routine struct {
			ID string `json:"id"`
		} `json:"routine"`
	}
	_ = json.Unmarshal(body, &created)
	routineID := created.ID
	if routineID == "" {
		routineID = created.Routine.ID
	}
	if routineID == "" {
		cliError("created routine but response missing id (cannot attach cron trigger): %s", string(body))
		return 1
	}

	trBody, trStatus, trErr := client.do(http.MethodPost,
		fmt.Sprintf("/api/v1/office/routines/%s/triggers", routineID),
		map[string]any{
			"kind":            "cron",
			"cron_expression": *cron,
			"timezone":        *timezone,
		})
	return handleResponse(trBody, trStatus, trErr)
}

func routinesSetStatus(args []string, status string) int {
	fs := flag.NewFlagSet("routines status", flag.ContinueOnError)
	id := fs.String("id", "", "Routine ID (required)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
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
	body, code, err := client.do(http.MethodPatch,
		fmt.Sprintf("/api/v1/office/routines/%s", *id),
		map[string]any{"status": status})
	return handleResponse(body, code, err)
}

func routinesDelete(args []string) int {
	fs := flag.NewFlagSet("routines delete", flag.ContinueOnError)
	id := fs.String("id", "", "Routine ID (required)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
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
	body, code, err := client.do(http.MethodDelete,
		fmt.Sprintf("/api/v1/office/routines/%s", *id), nil)
	return handleResponse(body, code, err)
}
