package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
)

func runMemoryCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev memory <get|set|summary> [flags]")
		return 1
	}
	switch args[0] {
	case subcmdGet:
		return memoryGet(args[1:])
	case "set":
		return memorySet(args[1:])
	case "summary":
		return memorySummary(args[1:])
	default:
		cliError("unknown memory subcommand: %s", args[0])
		return 1
	}
}

// memoryGet retrieves memory entries for the current agent, with optional
// layer and key filters.
func memoryGet(args []string) int {
	fs := flag.NewFlagSet("memory get", flag.ContinueOnError)
	layerFlag := fs.String("layer", "", "Filter by layer")
	keyFlag := fs.String("key", "", "Filter by key")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	agentID := os.Getenv("KANDEV_AGENT_ID")
	path := fmt.Sprintf("/api/v1/office/agents/%s/memory", agentID)
	return getWithParams(path, "KANDEV_AGENT_ID", agentID, map[string]string{
		"layer": *layerFlag,
		"key":   *keyFlag,
	})
}

// memorySet upserts a memory entry for the current agent.
func memorySet(args []string) int {
	fs := flag.NewFlagSet("memory set", flag.ContinueOnError)
	layerFlag := fs.String("layer", "", "Memory layer (required)")
	keyFlag := fs.String("key", "", "Memory key (required)")
	contentFlag := fs.String("content", "", "Memory content (required)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	if *layerFlag == "" || *keyFlag == "" || *contentFlag == "" {
		cliError("--layer, --key, and --content are all required")
		return 1
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	if client.agentID == "" {
		cliError("KANDEV_AGENT_ID must be set")
		return 1
	}

	payload := map[string]any{
		"entries": []map[string]string{
			{
				"layer":   *layerFlag,
				"key":     *keyFlag,
				"content": *contentFlag,
			},
		},
	}

	path := fmt.Sprintf("/api/v1/office/agents/%s/memory", client.agentID)
	body, status, doErr := client.do(http.MethodPut, path, payload)
	return handleResponse(body, status, doErr)
}

// memorySummary retrieves a summary of the agent's memory entries.
func memorySummary(args []string) int {
	fs := flag.NewFlagSet("memory summary", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}

	if client.agentID == "" {
		cliError("KANDEV_AGENT_ID must be set")
		return 1
	}

	path := fmt.Sprintf("/api/v1/office/agents/%s/memory/summary", client.agentID)
	body, status, doErr := client.do(http.MethodGet, path, nil)
	return handleResponse(body, status, doErr)
}
