---
created: 2026-10-17
status: implemented
requirements:
  - REQ-OFFICE-AGENT-TASK-RELATIONS-001
  - REQ-OFFICE-AGENT-TASK-RELATIONS-003
  - REQ-OFFICE-AGENT-TASK-RELATIONS-004
system_design:
  - ../../specs/office/system-design/agent-task-relations.md
legacy_specs: []
---

# Implementation Plan: Office Agent Task Relations

## Overview

Let Office agents declare `blocked_by` and `workspace_mode` when they create a
task through the signed runtime endpoint (GitHub issue #4547, narrowed by the
author's comment). Blockers are scoped by the shared read relation. An explicit
`workspace_mode: new_workspace` gives an agent-created child its own worktree.
Nothing changes when the fields are omitted. The four fields the runtime still
refuses keep their existing CLI definitions and pre-send refusal; their future
is tracked in #4613.

## Confirmed current behavior

- `runtime.CreateTaskInput.unsupportedField` and `agentctl taskCreate` reject
  all six optional fields before sending.
- `taskservice.CreateChildTask` hardcodes `WorkspacePolicy{Mode:
  inherit_parent}`. This package adds an explicit-mode override and keeps that
  default.
- Omitted-mode resolution already differs by path: the HTTP task API applies a
  parent's `default_child_workspace`; MCP and `CreateChildTask` use
  `inherit_parent`. This package does not change any of them. Reconciling them
  is tracked in #4606.
- Office assignment wake admission (`REQ-OFFICE-SCHEDULER-002`, #4562) already
  holds `task_assigned` while a blocker is pending, on every creation surface.
  This package only creates the edges.
- The creator interfaces carry `causingRunID` (run causation). The new
  relations parameter goes next to it.

## Work orders

| # | Work order | Wave | Depends on | Requirements |
| --- | --- | --- | --- | --- |
| 01 | [Accept relations on runtime task create](task-01-runtime-create-relations.md) | 1 | none | 001, 003, 004.3 |
| 02 | [Expose supported relation flags in the CLI and skills](task-02-cli-and-skills.md) | 2 | 01 | 004 |
| 03 | [Verify agent task relations end to end](task-03-end-to-end-verification.md) | 3 | 01, 02 | 001.1, 001.3, 003.1, 003.5, 004.1 |

Waves mark order only; they do not authorize subagents.

## Dependency order

```text
01 runtime create relations ──> 02 CLI + skills ──> 03 end-to-end
```

## Verification strategy

- Unit and handler tests in `internal/office/runtime` cover validation, scope,
  error mapping, and audit events.
- Task-service tests cover `CreateChildTask` policy resolution and edge
  creation with rollback.
- `cmd/agentctl` tests cover flag parsing, payloads, and the still-refused flags.
- The Playwright `agentctl-cli.spec.ts` suite drives the real CLI binary
  against the backend to provide end-to-end evidence.

Full commands are in each work order. Before the final commit, run
`make -C apps/backend fmt` and then `make -C apps/backend typecheck test lint`
as the repository requires.

## Risks

- **Parent as blocker.** Ancestor blockers are accepted
  (`AC-OFFICE-AGENT-TASK-RELATIONS-001.8`) because sequential pivots use them.
  A child blocked by a parent whose agent waits for `task_children_completed`
  stalls. The skill documentation warns against it; detection is out of scope.

## Exclusions

See the requirement's out-of-scope list: no priority, shared groups, ordering
policy, edge mutation on existing tasks, wake deferral, mixed blocker and
hierarchy stall detection, or UI work.

## Results

All three work orders are done; see each work order's Results.

Repository-wide checks: `make fmt`, `make typecheck`, and `make lint` passed.
`make test` did not pass cleanly on this machine, for reasons outside this
change:

- Backend packages that timed out under full-suite load were rerun with a longer
  timeout. Every package that consumes changed code passed (`backendapp`,
  `task/handlers`, `mcp/handlers`, `orchestrator`, `task/service`, Office
  packages, `cmd/agentctl`). `task/repository/sqlite`, which this change does
  not touch, exceeded a 60-minute timeout under load.
- `TestManagedNPMRuntimeLaunchIgnoresWorkspaceNpmrc` and
  `TestRuntimeUpdateSummaryKeepsRecipientClaimsIsolated` fail identically on an
  unmodified `origin/main`.
- `agentctl/server/process` submodule tests, two `scripts` tests, and three
  `e2e/helpers/seed-repository-origin.test.ts` Vitest cases fail because a local
  git identity-allowlist hook blocks fixture commits and pushes.
- The web Vitest suite otherwise passed: 26,948 passed, 3 failed (above), 4
  skipped.
