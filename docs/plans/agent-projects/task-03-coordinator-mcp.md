---
id: "03-coordinator-mcp"
title: "Coordinator instructions and MCP"
status: done
wave: 3
depends_on:
  - "01-project-identity"
  - "02-context-workspaces"
plan: "plan.md"
requirements:
  - REQ-PROJECTS-AGENT-PROJECTS-002
  - REQ-PROJECTS-AGENT-PROJECTS-003
  - REQ-PROJECTS-AGENT-PROJECTS-005
acceptance_criteria:
  - AC-PROJECTS-AGENT-PROJECTS-002.1
  - AC-PROJECTS-AGENT-PROJECTS-002.2
  - AC-PROJECTS-AGENT-PROJECTS-002.3
  - AC-PROJECTS-AGENT-PROJECTS-002.4
  - AC-PROJECTS-AGENT-PROJECTS-002.5
  - AC-PROJECTS-AGENT-PROJECTS-002.6
  - AC-PROJECTS-AGENT-PROJECTS-003.6
  - AC-PROJECTS-AGENT-PROJECTS-005.3
system_design:
  - ../../specs/projects/system-design/agent-projects.md
---

# Task 03: Coordinator instructions and MCP

## Summary

Launch the main task with server-owned project instructions and a small
coordinator MCP catalog. Create direct workers through server-resolved
economy/frontier tiers and project-scoped identity checks.

## In scope

- Coordinator and worker MCP surfaces, session-bound principal, tool catalog,
  worker create/message/stop/list admission, an eight-active-worker cap,
  result projection, and flag checks.
- Project instruction delivery, profile pinning, context path, and worker
  startup on independent worktrees with adapter-managed writable roots.
- Persisted worker tier/profile, default all-repo selection, validated remote
  base-branch input, and fresh task branch naming.
- Completion/error status projection without automatic coordinator turns.

## Out of scope

- Subscriptions, automatic resume, changing native agent subagent behavior,
  project UI.

## Acceptance

- A coordinator creates economy/frontier workers with stored profile IDs and
  selected repos; direct raw calls cannot substitute a different parent,
  profile, workspace, or project.
- A worker or ordinary task cannot acquire coordinator tools, and flag-off
  calls reject before creating tasks or dispatching runs. Neither project
  surface exposes generic task/session/workflow creation or completion tools.
- Coordinator instructions include the canonical context path and agent/task
  role guidance on every launch, while the agent starts in its primary repo;
  sandboxed writes to context and sibling repos succeed for eligible profiles.

## Verification

```bash
(cd apps/backend && go test ./internal/mcp/profile/... ./internal/mcp/server/... ./internal/mcp/handlers/... ./internal/agent/runtime/lifecycle/... ./internal/projects/...)
```

## Files likely touched

- `apps/backend/internal/mcp/profile/profile.go`
- `apps/backend/internal/mcp/server/`
- `apps/backend/internal/mcp/handlers/`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/projects/`
- `apps/backend/internal/agent/settings/models/models.go`

## Dependencies

Tasks 01 and 02 supply project policy, task identity, and context paths.

## Risks

- A prompt alone is not an authorization boundary. Revalidate all tool
  inputs against the session principal and project record.
- A profile's user-entered CLI flags are not proof that project writable
  roots were granted. Exercise the actual managed launch arguments.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/projects/requirements/agent-projects.md)
- [System design](../../specs/projects/system-design/agent-projects.md)
- Existing `internal/mcp/profile` and `internal/mcp/handlers/create_task_mode.go`.

## Results

Passed:

- `go test ./internal/mcp/profile/... ./internal/mcp/server/... ./internal/mcp/handlers/... ./internal/mcp/scope`
- Coordinator and worker MCP mode, scope, authorization, and mock-agent
  scenario coverage.
