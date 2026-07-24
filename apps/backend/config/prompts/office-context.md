KANDEV OFFICE MCP TOOLS — This Office session has exactly the tools listed below from the "kandev" server.
Always use the exact tool names shown below (they include the _kandev suffix).

Kandev Task ID: {task_id}
Kandev Session ID: {session_id}
Use these IDs when a tool requires task_id or session_id.

Available tools:
- ask_user_question_kandev: Ask the user one or more clarifying questions in a single call. Required: questions (1-4 items, each with prompt and 2-6 labeled options). Optional: context.
- create_task_plan_kandev: Save an implementation plan for the current task. Required: task_id, content. Optional: title.
- get_task_plan_kandev: Retrieve the current task plan, including user edits. Required: task_id.
- update_task_plan_kandev: Update the current task plan. Required: task_id, content. Optional: title.
- delete_task_plan_kandev: Delete the current task plan. Required: task_id.
- list_related_tasks_kandev: List parent, child, sibling, blocker, and blocked tasks. Optional: task_id (defaults to the current task).
- list_task_documents_kandev: List documents on an accessible related task. Required: task_id.
- get_task_document_kandev: Read one document on an accessible related task. Required: task_id, document_key.
- write_task_document_kandev: Create or replace a document on the current task or an ancestor. Required: task_id, document_key, content. Optional: title, type.

Office state changes are performed through `$KANDEV_CLI kandev ...`, subject to this agent's runtime permissions. Use the injected Office skills for exact commands and do not search for additional Kandev MCP tools. Workspace administration is outside this run.

IMPORTANT: You MUST use these MCP tools when instructed to create plans, ask questions, or exchange task documents. Use `$KANDEV_CLI` for authorized Office mutations.
