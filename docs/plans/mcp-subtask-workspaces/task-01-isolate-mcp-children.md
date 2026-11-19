---
id: "01-isolate-mcp-children"
title: "Default MCP children to isolated worktrees"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-TASKS-MCP-WORKSPACE-MODE-004.1
  - AC-TASKS-MCP-WORKSPACE-MODE-004.2
  - AC-TASKS-MCP-WORKSPACE-MODE-004.3
  - AC-TASKS-MCP-WORKSPACE-MODE-004.4
  - AC-TASKS-MCP-WORKSPACE-MODE-004.6
  - AC-TASKS-MCP-WORKSPACE-MODE-004.7
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.25
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01. Default MCP children to isolated worktrees

## Summary

Resolve the MCP policy before deriving repository inputs. Treat equivalent
parent repository and base selections as one fresh checkout per child.
Update the contract where the tool and public docs describe the default.

## In scope

- Update `resolveMCPWorkspacePolicy` and call it after parent admission.
  Keep the task-service mode explicit for a resolved parent, including one
  reached through Kanban `self` redirection.
- In `resolveTaskRepositories` and `inheritedRepoInputs`, normalize only
  the repository inputs copied from the parent for `new_workspace`. Resolve
  equivalent known default bases and apply a top-level override before
  grouping. Preserve repository ordering and base intent. Keep explicit
  repository input on the current validation path.
- When a caller supplies top-level `base_branch`, persist explicit selection
  provenance so an unresolved override remains strict and cannot fall back to
  the repository default.
- Add `TestMCPSubtaskFreshWorkspaceRepositorySelection` with the same-base
  reproduction first. Cover multiple distinct bases and repositories, saved
  parent sharing default, a legacy empty base with a known repository
  default, and equivalent bases after an override. Verify the two explicit
  modes and top-level omission at the resolver boundary.
- Set explicit sharing in existing fixtures whose assertions depend on
  parent workspace membership. Keep creator profile, repository source,
  workflow, sequencing, access, and external-ID expectations.
- Update public examples and the tool description together. Acknowledge the
  changed omission behavior and show the explicit mode for automation that
  shares files. Update the queued Kubernetes examples.

## Out of scope

- Shared inventory matching, covered by task 02.
- New MCP enum values, new fields in the tool schema, and cross-surface
  default parity.

## Acceptance

- A task-session call or authorized external call with a resolved parent and
  no mode persists `new_workspace`. Tool and attachment schemas advertise
  the same two optional values with no unconditional schema default.
- Automatic repository mapping collapses same-base duplicates and retains
  distinct bases. Explicit selection, hierarchy, and profile precedence
  pass their existing regressions.
- Two children created from the same multi-checkout parent each persist the
  one fresh selection for the shared base. Task 02 verifies physical paths
  and branches with the existing worktree adoption fixture.

## Verification

Run from the repository root. Run the new handler regression before changing
production code and require the baseline failure.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/server -count=1)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/kubernetes/kubernetes-inherited-workspace.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The Kubernetes run requires the Docker and Kind fixtures. Record its actual
result separately from Chromium and backend evidence.

## Files likely touched

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/create_task_subtask_workspaces_test.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/internal/mcp/handlers/workspace_policy_test.go`
- `apps/backend/internal/mcp/handlers/create_task_self_placement_inheritance_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/create_task_workspace_mode_test.go`
- `apps/web/e2e/tests/kubernetes/kubernetes-inherited-workspace.spec.ts`
- `docs/public/automation-and-mcp.md`, `docs/public/coordination.md`, and
  `docs/public/k8s.md`

## Dependencies

None.

## Risks

Callers that previously omitted the field to share files need to send
`inherit_parent`. A branch override can make two previously distinct bases
equivalent, so the grouping must run after that override.

## Parallelism

`sequential`

## Inputs

- [Materialization policy design](../../specs/tasks/system-design/mcp-workspace-mode.md#subtask-materialization-policy)
- [MCP workspace requirements](../../specs/tasks/requirements/mcp-workspace-mode.md)

## Results

Implemented the omitted-mode default as `new_workspace` and resolved the mode
before deriving repository inputs. Generated fresh selections now group
equivalent repository/base choices after applying known defaults and the
top-level base override. Explicit sharing expectations in existing fixtures
are marked with `inherit_parent`. Updated MCP tool guidance and public MCP,
coordination, and Kubernetes docs.

The override case now persists explicit branch provenance and resolves to the
strict base fallback policy required by `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.25`.

Verification passed: MCP handlers and server suites, Kind-backed Kubernetes
E2E, public-doc validation/tests, changed-Go lint, backend build, specification
catalog validation, and full specification lint.
