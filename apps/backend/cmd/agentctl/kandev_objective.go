package main

import (
	"encoding/json"
	"flag"
	"net/url"
)

const (
	kandevAssigneeKey  = "assignee"
	kandevStatusKey    = "status"
	kandevTaskIDKey    = "task_id"
	kandevTitleKey     = "title"
	kandevUpdateAction = "update"
)

func runObjectiveCmd(args []string) int {
	if len(args) == 0 {
		cliError("usage: objective <list|create|update> [flags]")
		return 1
	}
	if args[0] == "list" {
		return objectiveList(args[1:])
	}
	if args[0] != "create" && args[0] != kandevUpdateAction {
		cliError("unsupported objective action")
		return 1
	}
	fs := flag.NewFlagSet("objective "+args[0], flag.ContinueOnError)
	op := registerAssistantMutation(fs)
	id := fs.String("id", "", "Objective ID for update")
	title := fs.String(kandevTitleKey, "", "Objective title")
	mode := fs.String("mode", "", "answer, inspect, execute or design")
	source := fs.String("source-comment", "", "Owner's source comment ID")
	status := fs.String(kandevStatusKey, "", "Objective status")
	revision := fs.Int64("revision", 0, "Expected objective revision")
	acceptance := fs.String("acceptance", "", "JSON array of {id,description} acceptance criteria")
	evidence := fs.String("evidence", "", "JSON array of typed evidence references")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	payload := map[string]any{}
	for key, value := range map[string]string{kandevTitleKey: *title, "mode": *mode, "source_comment_id": *source, kandevStatusKey: *status} {
		if value != "" {
			payload[key] = value
		}
	}
	if !addObjectiveArrays(payload, map[string]string{"acceptance": *acceptance, "evidence": *evidence}) {
		return 1
	}
	op.add(payload)
	method, path := "POST", orchestrationAPIPrefix+"/runtime/objectives"
	if args[0] == kandevUpdateAction {
		if *id == "" || *revision < 1 {
			cliError("--id and --revision are required for update")
			return 1
		}
		method, path = "PATCH", path+"/"+url.PathEscape(*id)
		payload["expected_revision"] = *revision
	}
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	body, code, err := client.do(method, path, payload)
	return handleResponse(body, code, err)
}

func objectiveList(args []string) int {
	fs := flag.NewFlagSet("objective list", flag.ContinueOnError)
	after := fs.String("after", "", "Continuation cursor")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	body, code, err := client.do("GET", orchestrationAPIPrefix+"/runtime/objectives?after="+url.QueryEscape(*after), nil)
	return handleResponse(body, code, err)
}

func addObjectiveArrays(payload map[string]any, arrays map[string]string) bool {
	for key, raw := range arrays {
		if raw == "" {
			continue
		}
		var rows []map[string]any
		if len(raw) > 24000 || json.Unmarshal([]byte(raw), &rows) != nil {
			cliError("%s must be a bounded JSON array", key)
			return false
		}
		payload[key] = rows
	}
	return true
}
