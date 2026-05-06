package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
)

// Shared subcommand literals so goconst doesn't flag duplicates across
// the per-noun handler files. `subcmdList` was added with the first
// CLI rollout; `subcmdCreate` was added when the office mutation
// surfaces (agents / routines / docs / task) landed; `subcmdGet` was
// added when the office costs CLI joined memory + task as `get`-style
// readers.
const (
	subcmdList   = "list"
	subcmdCreate = "create"
	subcmdGet    = "get"
)

// runKandevCLI dispatches the kandev subcommand to the appropriate handler.
// Returns an exit code (0 = success, non-zero = error).
func runKandevCLI(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	switch args[0] {
	case "task":
		// Singular `task` group (get/update/create) stays for back
		// compat with skills authored before the plural rollout.
		// New skills should prefer `tasks` which covers list/move/
		// archive/message/conversation as well.
		return runTaskCmd(args[1:])
	case "tasks":
		return runTasksCmd(args[1:])
	case "comment":
		return runCommentCmd(args[1:])
	case "agents":
		return runAgentsCmd(args[1:])
	case "memory":
		return runMemoryCmd(args[1:])
	case "checkout":
		return runCheckoutCmd(args[1:])
	case "label":
		return runLabelCmd(args[1:])
	case "doc":
		return runDocCmd(args[1:])
	case "routines":
		return runRoutinesCmd(args[1:])
	case "approvals":
		return runApprovalsCmd(args[1:])
	case "budget":
		return runBudgetCmd(args[1:])
	default:
		cliError("unknown command: %s", args[0])
		return 1
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: agentctl kandev <command> [flags]")
	fmt.Fprintln(os.Stderr,
		"Commands: task, tasks, comment, agents, memory, checkout, label, doc, routines, approvals, budget")
}

// cliError writes a JSON error object to stderr.
func cliError(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	data, _ := json.Marshal(map[string]string{"error": msg})
	fmt.Fprintln(os.Stderr, string(data))
}

// cliOutput writes JSON data to stdout.
func cliOutput(data []byte) {
	_, _ = os.Stdout.Write(data)
	// Ensure trailing newline for shell friendliness.
	if len(data) == 0 || data[len(data)-1] != '\n' {
		_, _ = os.Stdout.Write([]byte("\n"))
	}
}

// handleResponse checks the HTTP status and writes output or error accordingly.
// Returns 0 on 2xx status, 1 otherwise.
func handleResponse(body []byte, status int, err error) int {
	if err != nil {
		cliError("%v", err)
		return 1
	}
	if status >= 200 && status < 300 {
		cliOutput(body)
		return 0
	}
	// Non-2xx: write body to stderr as the error.
	fmt.Fprintln(os.Stderr, string(body))
	return 1
}

// getWithParams performs a GET request with query parameters. It handles
// client creation, required env var check, path building, and response output.
func getWithParams(basePath, requiredEnvName, requiredEnvVal string, params map[string]string) int {
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	if requiredEnvVal == "" {
		cliError("%s must be set", requiredEnvName)
		return 1
	}
	values := url.Values{}
	for k, v := range params {
		if v == "" {
			continue
		}
		values.Set(k, v)
	}
	q := ""
	if encoded := values.Encode(); encoded != "" {
		q = "?" + encoded
	}
	body, status, doErr := client.do("GET", basePath+q, nil)
	return handleResponse(body, status, doErr)
}
