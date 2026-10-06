[PLANNING PHASE]
Analyze this task and create a detailed implementation plan.

Before creating the plan, ask the user clarifying questions if anything is unclear.
Use the ask_user_question_kandev MCP tool to get answers before proceeding.

First check if a plan already exists using the get_task_plan_kandev MCP tool.
If the user has already started writing the plan, build on their content — do not replace it.

IMPORTANT: Before writing the plan, explore the codebase thoroughly. Read relevant files, search for existing patterns, and understand the project's architecture. Your plan must reference actual file paths, function names, types, and patterns from this project — not generic advice.

The plan should include:
1. Understanding of the requirements
2. Specific files that need to be modified or created (with actual paths from the codebase)
3. Step-by-step implementation approach grounded in existing code patterns
4. Potential risks or considerations

When including diagrams (architecture, sequence, flowcharts), always use mermaid syntax in code blocks.

For focused revisions, get_task_plan_kandev(offset, limit) reads a bounded range when the discovered schema supports it. Ranges count Unicode code points. Continue with next_offset and the first page's version as expected_version; reconcile conflicts. Omit offset and limit for a full read when the whole plan is needed.
Save a new plan with create_task_plan_kandev, change one exact unique fragment with edit_task_plan_kandev, or add a section with update_task_plan_kandev(mode="append"). Reuse the current version from a read or successful write. Never submit a fragment as a replacement; whole-document replacement requires complete current content and expected_version. Restart pagination after writes because offsets belong to the earlier version. Append is not idempotent; inspect current state after a lost response before retrying.
After saving, STOP and wait for user review. The user may edit the plan before approving it.
Do not create any other files during this phase — only use the MCP tools to save the plan.
