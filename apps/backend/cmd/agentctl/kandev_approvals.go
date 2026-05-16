package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
)

// runApprovalsCmd dispatches `agentctl kandev approvals <subcmd>`.
// Approvals gate sensitive office mutations (hiring, budget grants,
// etc.) and the CEO uses this group to clear the inbox.
func runApprovalsCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev approvals <list|decide> [flags]")
		return 1
	}
	switch args[0] {
	case subcmdList:
		return approvalsList(args[1:])
	case "decide":
		return approvalsDecide(args[1:])
	default:
		cliError("unknown approvals subcommand: %s", args[0])
		return 1
	}
}

func approvalsList(args []string) int {
	fs := flag.NewFlagSet("approvals list", flag.ContinueOnError)
	status := fs.String("status", "", "Filter by status (pending, approved, rejected)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	wsID := os.Getenv("KANDEV_WORKSPACE_ID")
	path := fmt.Sprintf("/api/v1/office/workspaces/%s/approvals", wsID)
	return getWithParams(path, "KANDEV_WORKSPACE_ID", wsID, map[string]string{
		"status": *status,
	})
}

// approvalsDecide flips an approval to approved or rejected. The
// caller is responsible for confirming the policy fit (the backend
// checks RBAC; the CEO is allowed by default).
func approvalsDecide(args []string) int {
	fs := flag.NewFlagSet("approvals decide", flag.ContinueOnError)
	id := fs.String("id", "", "Approval ID (required)")
	decision := fs.String("decision", "", "approve | reject (required)")
	note := fs.String("note", "", "Optional decision note (visible on the approval row)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	if *id == "" || *decision == "" {
		cliError("--id and --decision are required")
		return 1
	}
	status := ""
	switch *decision {
	case "approve", "approved":
		status = "approved"
	case "reject", "rejected":
		status = "rejected"
	default:
		cliError("--decision must be 'approve' or 'reject'")
		return 1
	}
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	body, code, err := client.do(http.MethodPost,
		fmt.Sprintf("/api/v1/office/approvals/%s/decide", *id),
		map[string]any{
			"status":        status,
			"decision_note": *note,
		})
	return handleResponse(body, code, err)
}
