package main

import (
	"flag"
	"fmt"
	"os"
)

func runAgentsCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev agents <list> [flags]")
		return 1
	}
	switch args[0] {
	case "list":
		return agentsList(args[1:])
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
	path := fmt.Sprintf("/api/v1/orchestrate/workspaces/%s/agents", wsID)
	return getWithParams(path, "KANDEV_WORKSPACE_ID", wsID, map[string]string{
		"role":   *roleFlag,
		"status": *statusFlag,
	})
}
