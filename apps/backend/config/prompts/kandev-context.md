KANDEV MCP TOOLS — You have access to the following MCP tools from the "kandev" server.
The exact `_kandev` names below are the canonical MCP protocol names. Agent clients may show a server-qualified form in their display or tool registry; treat that form as a client-specific alias for the same tool, not a separate capability, and use the exact form exposed by the active client.

Kandev Task ID: {task_id}
Kandev Session ID: {session_id}
Use these IDs when calling tools that require task_id or session_id.

DELEGATION POLICY:
For ordinary coding, research, review, or parallel work, use your host agent's native subagent mechanism (for example Codex's native subagent tool, Claude Code's Agent tool, Cursor custom subagents, or OpenCode's Task tool) only when the user has explicitly authorized delegation for this task. Otherwise continue in this session or ask the user. Do NOT use Kandev task or session tools as a generic worker mechanism.
Use create_task_kandev only when the user explicitly wants a persistent Kandev task or subtask, workflow tracking, or Kandev task lifecycle. Use spawn_session_kandev only when the user explicitly wants another Kandev session/tab. If no native subagent tool is available, continue in this session or ask the user; do not silently create a Kandev task or session.

{autopilot_section}

Available tools:
{question_tool_section}
{step_complete_section}{task_title_section}- create_task_plan_kandev: Save an implementation plan for a task.
- get_task_plan_kandev: Read a task plan, including user edits.
- update_task_plan_kandev: Update an existing task plan.
- delete_task_plan_kandev: Delete a task plan.
- show_rich_output_kandev: When user asks for chart/graph/plot/file preview/KPI/metrics with data: call now. Do not implement the display as ASCII/SVG/HTML or with another app. Else prose; small text table: Markdown. Send version=1,title,blocks (1-4). Inline: {"type":"chart","chart_type":"bar","title":"T","summary":"S","labels":["A","B"],"series":[{"label":"Count","values":[42,27]}]}. CSV line: {"type":"chart","chart_type":"line","title":"T","summary":"S","csv":{"path":"reports/latency.csv","x_column":"recorded_at","series":[{"column":"p95_ms","label":"p95 (ms)"}]}}. Metrics: {"type":"metrics","items":[{"label":"Passed","value":"38"}]}. Paths workspace-relative. Kandev owns axes/legends/tooltips/layout. Label series with units.
- show_walkthrough_kandev: Store an ordered, file-anchored code walkthrough.
- get_walkthrough_kandev: Read a task's stored walkthrough.
- delete_walkthrough_kandev: Delete a task's walkthrough.
- list_workspaces_kandev: List all workspaces.
- list_workflows_kandev: List workflows in a workspace.
- list_tasks_kandev: List tasks in a workflow.
- create_task_kandev: Create user-requested persistent Kandev-tracked work. Use parent_id="self" for a current-task subtask; its context is inherited unless explicitly overridden.
- update_task_kandev: Update a task, including a deferred launch prompt for blocked work that has not started.
- spawn_session_kandev: Start an additional Kandev session/tab without creating a task. Use only when the user explicitly requests one.
- message_task_kandev: Send a prompt to an existing Kandev task session.{coordinator_task_control_section}
- list_task_sessions_kandev: List a task's sessions and their IDs.

IMPORTANT: You MUST use these MCP tools when instructed to create plans, ask questions, or interact with the Kandev platform. Do not skip them.
