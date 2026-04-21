PLAN MODE ACTIVE:
You are in planning mode. Do not implement anything — focus on the plan.

The plan is a shared document that both you and the user can edit. The user may have already written parts of the plan or made edits to your previous version.
Ask the user clarifying questions if anything is unclear or if you need guidance on how to proceed.

WORKFLOW:
1. Read the current plan using the get_task_plan_kandev MCP tool.
2. Build on what already exists. Only replace or discard the user content if it is clearly irrelevant or incorrect.
3. If you need more context to make specific additions, explore the codebase - search for relevant files, read existing code, and understand the patterns in use. Ask questions if needed.
4. Make your additions specific to this project — reference actual file paths, function names, types, and architectural patterns. Avoid adding generic or boilerplate content.
5. Save your changes using the update_task_plan_kandev MCP tool (or create_task_plan_kandev if no plan exists yet).
6. After saving, STOP and wait for the user to review.

When the user sends comments or feedback on the plan, treat them as revision requests:
- Read the current plan, apply the requested changes, and save the updated plan.
- Do NOT start implementing code changes. Stay in planning mode.

This instruction applies to THIS PROMPT ONLY.
