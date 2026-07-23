package main

import (
	"flag"
	"net/http"
	"strings"
)

type repositoryFlags []string

func (v *repositoryFlags) String() string {
	return strings.Join(*v, ",")
}

func (v *repositoryFlags) Set(value string) error {
	*v = append(*v, value)
	return nil
}

func runProjectsCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: agentctl kandev projects <list|create> [flags]")
		return 1
	}
	switch args[0] {
	case subcmdList:
		return projectsList(args[1:])
	case subcmdCreate:
		return projectsCreate(args[1:])
	default:
		cliError("unknown projects subcommand: %s", args[0])
		return 1
	}
}

func projectsCreate(args []string) int {
	fs := flag.NewFlagSet("projects create", flag.ContinueOnError)
	name := fs.String("name", "", "Project name (required)")
	description := fs.String("description", "", "Project description")
	leadAgentProfileID := fs.String("lead-agent-profile-id", "", "Lead agent profile ID")
	color := fs.String("color", "", "Project color")
	budgetCents := fs.Int("budget-cents", 0, "Project budget in cents (0 = unlimited)")
	executorConfig := fs.String("executor-config", "", "Executor configuration JSON")
	repositories := repositoryFlags{}
	fs.Var(&repositories, "repository", "Repository URL or path (repeatable)")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	if strings.TrimSpace(*name) == "" {
		cliError("--name is required")
		return 1
	}

	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	payload := map[string]any{
		"name":         *name,
		"repositories": []string(repositories),
	}
	if *description != "" {
		payload["description"] = *description
	}
	if *leadAgentProfileID != "" {
		payload["lead_agent_profile_id"] = *leadAgentProfileID
	}
	if *color != "" {
		payload["color"] = *color
	}
	if *budgetCents > 0 {
		payload["budget_cents"] = *budgetCents
	}
	if *executorConfig != "" {
		payload["executor_config"] = *executorConfig
	}
	body, status, err := client.do(http.MethodPost, "/api/v1/office/runtime/projects", payload)
	return handleResponse(body, status, err)
}

func projectsList(args []string) int {
	fs := flag.NewFlagSet("projects list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	body, status, err := client.do(http.MethodGet, "/api/v1/office/runtime/projects", nil)
	return handleResponse(body, status, err)
}
