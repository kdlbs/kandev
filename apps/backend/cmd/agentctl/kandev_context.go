package main

import (
	"flag"
	"net/url"
)

func runContextCmd(args []string) int {
	fs := flag.NewFlagSet("context", flag.ContinueOnError)
	objective := fs.String("objective", "", "Objective ID")
	profile := fs.String("profile", "", "Selected execution profile ID")
	project := fs.String("project", "", "Workspace repository ID")
	environment := fs.String("environment", "", "Task environment ID")
	task := fs.String("task", "", "Existing task ID")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *objective == "" || *profile == "" {
		cliError("--objective and --profile are required")
		return 1
	}
	query := url.Values{}
	for key, value := range map[string]string{"profile_id": *profile, "project_id": *project, "environment_id": *environment, kandevTaskIDKey: *task} {
		if value != "" {
			query.Set(key, value)
		}
	}
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	body, status, err := client.do("GET", orchestrationAPIPrefix+"/runtime/context/"+url.PathEscape(*objective)+"?"+query.Encode(), nil)
	return handleResponse(body, status, err)
}
