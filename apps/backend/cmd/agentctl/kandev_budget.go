package main

import (
	"flag"
	"fmt"
	"os"
)

// runBudgetCmd dispatches `agentctl kandev budget <subcmd>`. The
// CEO uses this to check workspace + per-agent spend before
// expensive operations (hiring, model upgrades, etc.).
func runBudgetCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev budget <get> [flags]")
		return 1
	}
	switch args[0] {
	case subcmdGet:
		return budgetGet(args[1:])
	default:
		cliError("unknown budget subcommand: %s", args[0])
		return 1
	}
}

// budgetGet returns the workspace cost summary. When --agent-id is
// supplied, the per-agent endpoint is hit instead so the caller
// sees just that agent's slice.
func budgetGet(args []string) int {
	fs := flag.NewFlagSet("budget get", flag.ContinueOnError)
	agentID := fs.String("agent-id", "", "Restrict to a single agent's spend")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	wsID := os.Getenv("KANDEV_WORKSPACE_ID")
	if wsID == "" {
		cliError("KANDEV_WORKSPACE_ID must be set")
		return 1
	}
	if *agentID == "" {
		return getWithParams(
			fmt.Sprintf("/api/v1/office/workspaces/%s/costs/summary", wsID),
			"KANDEV_WORKSPACE_ID", wsID, nil)
	}
	return getWithParams(
		fmt.Sprintf("/api/v1/office/workspaces/%s/costs/by-agent", wsID),
		"KANDEV_WORKSPACE_ID", wsID, map[string]string{"agent_id": *agentID})
}
