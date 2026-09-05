---
id: "03-policy-and-mcp"
title: "Enforce policy and expose the closed MCP request"
status: done
wave: 2
depends_on: ["01-persistence", "02-provider"]
plan: "plan.md"
spec: "../../specs/integrations/scoped-coordinator-ci-runs.md"
---

# Task 03: Enforce policy and expose the closed MCP request

Bind caller, grant, workspace, task, workflow/step, task repository, linked PR,
exact head, source run/attempt, and evidence policy. Add authenticated grant
management and the task-mode MCP tool with injected task/session identity and a
strict no-extra-properties schema.
