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

Save the plan using the create_task_plan_kandev or update_task_plan_kandev MCP tool.
After saving, STOP and wait for user review. The user may edit the plan before approving it.
Do not create any other files during this phase — only use the MCP tools to save the plan.
