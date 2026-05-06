package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
)

func runAgentsCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev agents <list|create|update|delete> [flags]")
		return 1
	}
	switch args[0] {
	case subcmdList:
		return agentsList(args[1:])
	case subcmdCreate:
		return agentsCreate(args[1:])
	case "update":
		return agentsUpdate(args[1:])
	case "delete":
		return agentsDelete(args[1:])
	default:
		cliError("unknown agents subcommand: %s", args[0])
		return 1
	}
}

// agentsList lists agents in the workspace with optional role and status filters.
func agentsList(args []string) int {
	fs := flag.NewFlagSet("agents list", flag.ContinueOnError)
	roleFlag := fs.String("role", "", "Filter by role")
	statusFlag := fs.String("status", "", "Filter by status")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	wsID := os.Getenv("KANDEV_WORKSPACE_ID")
	path := fmt.Sprintf("/api/v1/office/workspaces/%s/agents", wsID)
	return getWithParams(path, "KANDEV_WORKSPACE_ID", wsID, map[string]string{
		"role":   *roleFlag,
		"status": *statusFlag,
	})
}

// agentsCreate hires a new agent for the current workspace. The
// office governance layer decides whether the agent is created
// immediately or queued behind a hire_agent approval — `agents
// list` after the call surfaces the actual state.
func agentsCreate(args []string) int {
	fs := flag.NewFlagSet("agents create", flag.ContinueOnError)
	name := fs.String("name", "", "Agent name (required)")
	role := fs.String("role", "", "Agent role (required): ceo | worker | specialist | assistant | reviewer")
	icon := fs.String("icon", "", "Optional avatar icon slug")
	reportsTo := fs.String("reports-to", "", "Agent ID this new agent reports to")
	budgetCents := fs.Int("budget-monthly-cents", 0, "Monthly budget in cents (0 = unlimited)")
	maxSessions := fs.Int("max-concurrent-sessions", 0, "Maximum concurrent sessions (0 = default)")
	reason := fs.String("reason", "", "Justification surfaced on the hire_agent approval, when one is required")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	if *name == "" || *role == "" {
		cliError("--name and --role are required")
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

	payload := map[string]any{
		"name": *name,
		"role": *role,
	}
	if *icon != "" {
		payload["icon"] = *icon
	}
	if *reportsTo != "" {
		payload["reports_to"] = *reportsTo
	}
	if *budgetCents > 0 {
		payload["budget_monthly_cents"] = *budgetCents
	}
	if *maxSessions > 0 {
		payload["max_concurrent_sessions"] = *maxSessions
	}
	if *reason != "" {
		payload["reason"] = *reason
	}

	body, status, err := client.do(http.MethodPost,
		fmt.Sprintf("/api/v1/office/workspaces/%s/agents", wsID), payload)
	return handleResponse(body, status, err)
}

// agentsUpdate edits an existing agent profile. Mutually optional
// flags — at least one must be set; the backend's PATCH semantics
// only apply fields that are present in the payload.
func agentsUpdate(args []string) int {
	fs := flag.NewFlagSet("agents update", flag.ContinueOnError)
	id := fs.String("id", "", "Agent ID (required)")
	name := fs.String("name", "", "New name")
	icon := fs.String("icon", "", "New icon slug")
	budgetCents := fs.Int("budget-monthly-cents", -1, "Monthly budget in cents (-1 = unchanged)")
	maxSessions := fs.Int("max-concurrent-sessions", -1, "Max concurrent sessions (-1 = unchanged)")
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

	payload := map[string]any{}
	if *name != "" {
		payload["name"] = *name
	}
	if *icon != "" {
		payload["icon"] = *icon
	}
	if *budgetCents >= 0 {
		payload["budget_monthly_cents"] = *budgetCents
	}
	if *maxSessions >= 0 {
		payload["max_concurrent_sessions"] = *maxSessions
	}
	if len(payload) == 0 {
		cliError("at least one update flag required (--name, --icon, --budget-monthly-cents, --max-concurrent-sessions)")
		return 1
	}

	body, status, err := client.do(http.MethodPatch,
		fmt.Sprintf("/api/v1/office/agents/%s", *id), payload)
	return handleResponse(body, status, err)
}

// agentsDelete removes an agent from the workspace. Fails server-
// side when the caller lacks `can_delete_agents` or when the agent
// is mid-session.
func agentsDelete(args []string) int {
	fs := flag.NewFlagSet("agents delete", flag.ContinueOnError)
	id := fs.String("id", "", "Agent ID (required)")
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

	body, status, err := client.do(http.MethodDelete,
		fmt.Sprintf("/api/v1/office/agents/%s", *id), nil)
	return handleResponse(body, status, err)
}
