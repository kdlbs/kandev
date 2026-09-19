package mcp

import (
	"context"
	"encoding/json"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
)

const createAutomationSchema = `{
 "type":"object",
 "additionalProperties":false,
 "required":["workspace_id","name"],
 "properties":{
  "workspace_id":{"type":"string","minLength":1,"description":"Authorized workspace ID, from workspace discovery."},
  "name":{"type":"string","minLength":1},
  "description":{"type":"string"},
  "workflow_id":{"type":"string","description":"Required for normal_task; optional for automation_run."},
  "workflow_step_id":{"type":"string","description":"Optional starting step belonging to the selected workflow."},
  "agent_profile_id":{"type":"string"},
  "executor_profile_id":{"type":"string"},
  "repositories":{"type":"array","description":"Ordered repository/base-branch selections. Empty means no repositories.","items":{"type":"object","required":["repository_id"],"additionalProperties":false,"properties":{"repository_id":{"type":"string"},"base_branch":{"type":"string"}}}},
  "repository_ids":{"type":"array","items":{"type":"string"},"description":"Legacy repository selection. Prefer repositories; do not send conflicting selections."},
  "prompt":{"type":"string"},
  "task_title_template":{"type":"string"},
  "max_concurrent_runs":{"type":"integer","description":"Defaults to 1 when omitted or non-positive."},
  "continuation_policy":{"type":"string","enum":["new_task","reuse_thread"],"description":"Defaults to new_task. reuse_thread requires one concurrent run."},
  "task_mode":{"type":"string","enum":["automation_run","normal_task"],"description":"Defaults to hidden automation_run."},
  "repository_mode":{"type":"string","enum":["none","selected"],"description":"Normally omitted and derived from repository selections."},
  "triggers":{"type":"array","items":{"type":"object","required":["type","config"],"additionalProperties":false,"properties":{
   "type":{"type":"string","enum":["scheduled","github_pr","github_pr_merged","github_push","github_ci","webhook","plugin_event"]},
   "enabled":{"type":"boolean","description":"Set true to activate this trigger. Omission leaves it disabled."},
   "config":{"type":"object","description":"Trigger-specific configuration. scheduled: cron_expression and optional timezone. github_pr: events, repos, branches, authors, labels, exclude_draft. github_pr_merged: all_repos, repos, base_branches. github_push: repos, branches. github_ci: repos, conclusions, check_names, branches. webhook: dedup_key, filters, repository. plugin_event: plugin_id, condition_key, config_version, settings from the installed plugin. GitHub repos entries use owner and name."}
  }}}
 }
}`

func (s *Server) registerConfigAutomationTools() {
	tool := mcp.NewToolWithRawSchema("create_automation_kandev",
		"Create an enabled workspace automation and optional initial triggers. Discover resource IDs first. Each trigger is disabled unless enabled is true. Creation does not manually run it, but enabled triggers can fire normally. Returns the saved automation and triggers, plus the webhook secret once. Non-idempotent: inspect saved automations before retrying an uncertain result.",
		json.RawMessage(createAutomationSchema))
	mcp.WithReadOnlyHintAnnotation(false)(&tool)
	mcp.WithDestructiveHintAnnotation(false)(&tool)
	mcp.WithIdempotentHintAnnotation(false)(&tool)
	s.mcpServer.AddTool(tool, s.wrapHandler(tool.Name, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.forwardToBackend(ctx, ws.ActionMCPCreateAutomation, req.GetArguments())
	}))
}
