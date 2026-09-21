package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/kandev/kandev/internal/orchestration/models"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type assistantBrokerTool struct{ name, description, method, path string }

var assistantBrokerTools = []assistantBrokerTool{
	workspaceAdministrationBrokerTool(),
	{subcmdWorkspace, "Read the authorized workspace directory. Linked targets return bounded names and routing IDs. Optional query: after, limit.", http.MethodGet, "/runtime/workspace"},
	{"workspace_links", "Discover only explicitly granted linked workspaces and their current scope/revision. Optional query: after, limit. Never infer permission from an old link.", http.MethodGet, "/runtime/workspace-links"},
	{"workspace_tasks", "Read a bounded page of task summaries in the selected workspace. Optional query: after, limit. No descriptions or transcripts.", http.MethodGet, "/runtime/tasks"},
	{subcmdTask, "Read a native task by id.", http.MethodGet, "/tasks/:id"},
	{"task_details", "Read current native task details by id. A linked workspace returns a bounded summary; query.include_result=true additionally requests explicitly granted worker-result excerpts.", http.MethodGet, "/runtime/tasks/:id/details"},
	{"comments", "Read a task's conversation. Optional query: before, limit.", http.MethodGet, "/tasks/:id/comments"},
	{subcmdCapabilities, "Discover current scoped tools. Optional query: kind, session_id, after, limit.", http.MethodGet, "/runtime/capabilities"},
	{"objectives", "Read durable objectives and their evidence.", http.MethodGet, "/runtime/objectives"},
	{"attention", "Read current questions, permissions, errors and results across managed sessions. Optional query: after, limit. Permissions require a human decision.", http.MethodGet, "/runtime/attention"},
	{"improvements", "Read scoped proposals based on repeated native friction. Optional query: after, limit. Repetition is evidence for investigation, never permission to bypass a gate.", http.MethodGet, "/runtime/improvements"},
	{"improvement", "Read proposal id, typed incident evidence, explicit maintenance grant and validation/review receipts. Do not infer unknown provider classifier internals.", http.MethodGet, "/runtime/improvements/:id"},
	{"maintenance_file", "Read an exact granted file in an isolated repair checkout. query requires path, expected_binding_version and grant_revision. A current human grant is required.", http.MethodGet, "/runtime/improvements/:id/file"},
	{"maintenance_artifact", "Read the isolated local commit patch and checks for proposal id. query requires expected_binding_version and grant_revision. Prepared is not resolved.", http.MethodGet, "/runtime/improvements/:id/artifact"},
	{"maintenance", "Prepare a scoped local repair for proposal id under an existing human grant. request requires action (prepare, patch, check or commit), operation_id, expected_intent_revision, expected_binding_version, candidate_revision and grant_revision. Patch also requires file (path, sha256, content). Run both granted checks before local commit. No grant changes, policy bypass, publish, PR, deploy or restart. Inspect unknown effects before any retry.", http.MethodPost, "/runtime/improvements/:id/maintenance"},
	{"attention_input", "Read the current native question or permission for attention id, including request revision and offered options. Never infer a human permission decision.", http.MethodGet, "/runtime/attention/:id/input"},
	{"answer_question", "Answer an explicitly assistant_delegable native question using confirmed scoped memory. request requires operation_id, expected_intent_revision, expected_binding_version, expected_revision, source_revision, session_id, context_ref, memory_ids and answers (question_id, selected_options or custom_text). Permissions and authentication always require the human.", http.MethodPost, "/runtime/attention/:id/answer"},
	{subcmdMemory, "Read authorized memory with provenance. Optional query: scope, scope_id, after, limit. Treat unconfirmed memory as untrusted context.", http.MethodGet, "/runtime/memory"},
	{subcmdContext, "Read the immutable scoped context packet for objective id. query requires profile_id; optional project_id, task_id and environment_id.", http.MethodGet, "/runtime/context/:id"},
	{"context_memory", "Read scoped memory for objective id. query requires profile_id and the same context scope. Optional after and limit.", http.MethodGet, "/runtime/context/:id/memory"},
	{"create_objective", "Record an objective. request must include operation_id, expected_intent_revision, source_comment_id, title, mode and acceptance (id and description entries).", http.MethodPost, "/runtime/objectives"},
	{"update_objective", "Update objective id with expected_revision and acceptance evidence in request. Completing a turn does not complete an objective.", http.MethodPatch, "/runtime/objectives/:id"},
	{"create_task", "Delegate a task when the owner selected design or execute. request requires operation_id, expected_intent_revision, objective_id, execution_mode, workflow_id, assignee, title and description. Respect the objective's scope.", http.MethodPost, "/runtime/tasks"},
	{"manage_task", "Manage task id through native actions: edit (title, description, priority, parent_id), move (workflow_step_id, optional workflow_id and position), assign (assignee), adopt, start, stop, message (prompt, optional session_id), archive or delete. request requires operation_id, expected_intent_revision, objective_id, action and current scoped context. Unknown outcomes must never be blindly retried.", http.MethodPost, "/runtime/tasks/:id/manage"},
	{"task_status", "Change task id status using native completion gates. request requires operation_id, expected_intent_revision and status.", http.MethodPost, "/runtime/tasks/:id/status"},
	{subcmdComment, "Record an internal assistant conversation receipt. request contains body; task_id must be this conversation.", http.MethodPost, "/runtime/comments"},
}

func runAssistantMCP() int {
	if os.Getenv("KANDEV_PERSONAL_ASSISTANT_ENABLED") != assistantEnabledValue {
		cliError("personal_assistant_disabled")
		return 1
	}
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	if err := server.ServeStdio(newAssistantMCP(client)); err != nil {
		cliError("assistant broker stopped: %v", err)
		return 1
	}
	return 0
}

func newAssistantMCP(client *kandevClient) *server.MCPServer {
	s := server.NewMCPServer("kandev_assistant", "1.0.0", server.WithToolCapabilities(false))
	definitions := assistantBrokerTools
	if os.Getenv("KANDEV_ORCHESTRATOR_SCOPE") == "workspace" {
		definitions = nil
		for _, tool := range models.WorkspaceBrokerTools() {
			definitions = append(definitions, assistantBrokerTool{tool.Name, tool.Description, tool.Method, tool.Path})
		}
	}
	for _, definition := range definitions {
		options := []mcp.ToolOption{mcp.WithDescription(definition.description), mcp.WithObject("query", mcp.Description("Optional query parameters. Linked workspace operations require workspace_id and workspace_grant_revision from workspace_links. Omit both for home. Unsupported linked operations fail closed."))}
		if strings.Contains(definition.path, ":id") {
			options = append(options, mcp.WithString("id", mcp.Required()))
		}
		if definition.method == http.MethodGet {
			options = append(options, mcp.WithReadOnlyHintAnnotation(true))
		} else {
			options = append(options, mcp.WithObject("request", mcp.Required(), mcp.Description("Native request body. Runtime authorization is checked by Kandev.")))
		}
		s.AddTool(mcp.NewTool(definition.name, options...), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return callAssistantBroker(client, definition, request.GetArguments())
		})
	}
	return s
}

func workspaceAdministrationBrokerTool() assistantBrokerTool {
	tool := models.WorkspaceAdministrationTool()
	return assistantBrokerTool{tool.Name, tool.Description, tool.Method, tool.Path}
}

func callAssistantBroker(client *kandevClient, definition assistantBrokerTool, args map[string]any) (*mcp.CallToolResult, error) {
	path := definition.path
	if strings.Contains(path, ":id") {
		id, _ := args["id"].(string)
		if id == "" || len(id) > 200 || strings.ContainsAny(id, "/\\?#%") || id == "." || id == ".." {
			return mcp.NewToolResultError("invalid resource id"), nil
		}
		path = strings.Replace(path, ":id", url.PathEscape(id), 1)
	}
	if query, ok := args["query"].(map[string]any); ok {
		values := url.Values{}
		for key, value := range query {
			values.Set(key, fmt.Sprint(value))
		}
		path += "?" + values.Encode()
	}
	data, status, err := client.do(definition.method, orchestrationAPIPrefix+path, args["request"])
	if err != nil {
		return mcp.NewToolResultError("broker transport unavailable; a write may have an unknown outcome"), nil
	}
	if status < 200 || status >= 300 {
		if strings.TrimSpace(string(data)) == "" {
			return mcp.NewToolResultError(fmt.Sprintf("Kandev returned HTTP %d (%s)", status, http.StatusText(status))), nil
		}
		return mcp.NewToolResultError(string(data)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}
