---
id: "02-context-workspaces"
title: "Context and task workspaces"
status: done
wave: 2
depends_on:
  - "01-project-identity"
plan: "plan.md"
requirements:
  - REQ-PROJECTS-AGENT-PROJECTS-002
  - REQ-PROJECTS-AGENT-PROJECTS-003
acceptance_criteria:
  - AC-PROJECTS-AGENT-PROJECTS-002.2
  - AC-PROJECTS-AGENT-PROJECTS-003.1
  - AC-PROJECTS-AGENT-PROJECTS-003.2
  - AC-PROJECTS-AGENT-PROJECTS-003.3
  - AC-PROJECTS-AGENT-PROJECTS-003.4
  - AC-PROJECTS-AGENT-PROJECTS-003.5
  - AC-PROJECTS-AGENT-PROJECTS-003.6
system_design:
  - ../../specs/projects/system-design/agent-projects.md
---

# Task 02: Context and task workspaces

## Summary

Provide a canonical project context directory with safe task-local links.
Prepare independent repository worktrees for the coordinator and workers,
and expose authorized context and virtual-root file operations.

## In scope

- UUID-based context storage, initial `notes.md`, safe link creation/repair,
  canonical containment, content-hash checked UI/API Notes writes, and
  no-follow removal of task-local links during cleanup.
- Primary repo cwd and selected repository worktrees using existing
  worktree ownership rules.
- A typed launch contract that names canonical context and all task repo
  worktrees as required writable roots for supported agent adapters.
- Project context/virtual Files API; Git scan exclusion and guarded cleanup.

## Out of scope

- Remote executor sync, frontend Files rendering, coordinator MCP policy.

## Acceptance

- Coordinator and worker observe the same context edits; deleting a worker or
  restarting a session preserves context.
- Wrong symlinks, path traversal, cross-project paths, and unsupported
  executors reject before launch or file mutation.
- Each supported agent family receives writable context and sibling repo
  roots; an ineligible profile fails before agent startup.
- Project task worktrees remain independent; context edits never enter Git
  status or Changes events. Worktree cleanup tolerates the context link and
  removes only that link, never the canonical context.

## Verification

```bash
(cd apps/backend && go test ./internal/projects/... ./internal/worktree/... ./internal/agentctl/server/api/... ./internal/task/service/...)
```

## Files likely touched

- `apps/backend/internal/projects/`
- `apps/backend/internal/worktree/`
- `apps/backend/internal/agentctl/server/api/`
- `apps/backend/internal/task/service/`

## Dependencies

Task 01 supplies project/task identity and the flag boundary.

## Risks

- The file API currently roots itself at a session worktree; virtual context
  operations must preserve its path checks and authorization.
- `tryRemoveEmptyTaskDir` leaves any root containing a `context` symlink;
  teardown must unlink it without following it, then rerun empty-root cleanup.
- Direct shell/editor writes bypass UI/API content-hash checks and remain
  last-writer-wins; tests must state that limit.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/projects/requirements/agent-projects.md)
- [System design](../../specs/projects/system-design/agent-projects.md)

## Results

Passed:

- `go test ./internal/projects/... ./internal/worktree/... ./internal/agentctl/server/api/... ./internal/agent/runtime/lifecycle/...`
- Project context path, link safety, workspace reconciliation, and cleanup
  package coverage.
