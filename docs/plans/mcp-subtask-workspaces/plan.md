---
created: 2026-11-18
status: completed
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
legacy_specs: []
---

# Fix plan for MCP subtask workspaces

## Overview

Give MCP subtasks separate workspaces by default. When a caller explicitly
asks to share, attach the child to the parent's original checkouts, including
an adopted worktree. The first work order delivers isolated creation. The
second delivers shared checkout reuse and verifies its failure paths.

Sources are [issue #5370](https://github.com/kdlbs/kandev/issues/5370),
the [MCP workspace requirements](../../specs/tasks/requirements/mcp-workspace-mode.md),
and the [system design](../../specs/tasks/system-design/mcp-workspace-mode.md).

## Confirmed root cause

`resolveMCPWorkspacePolicy` selects `inherit_parent` on omission.
`resolveTaskRepositories` calls `inheritedRepoInputs` before that choice
affects repository mapping. The mapper copies each parent's repository ID
and base branch and drops its checkout branch. Two valid parent rows on
the same base then become identical. `claimRepositoryBranchSlot` correctly
rejects the duplicate before the task is inserted.

A temporary Go overlay against `a7f4cdd6a75` used the real MCP backend handler
and SQLite task service. It bound a Kanban session to a parent with two rows
for the same repository on `master`. The rows had different checkout
branches. A request with `parent_id="self"` and `start_agent: true` returned
`INTERNAL_ERROR` with the mode omitted, with `inherit_parent`, and with
`new_workspace`. The child count stayed the same each time. The service
reported the duplicate repository on `master`. The fixture used stored
repository rows because the failure occurs before provisioning a worktree.

## Scope

### In scope

- Normal `create_task_kandev` calls from task sessions and authorized external
  clients.
- Mode-specific repository mapping, including ready parent inventory admission
  and launch verification for shared Worktree checkouts.
- Tool description, subtask guidance, and Kubernetes documentation about the
  effect of omitting the field.

### Out of scope

- Default parity across HTTP, Office runtime, and workflow child creation.
  [Issue #4606](https://github.com/kdlbs/kandev/issues/4606).
- Top-level task repository selection, new public arguments, existing task
  migrations, Git worktree adoption redesign, and rendered UI changes.

## Technical approach

1. Resolve the mode after parent admission. For implicit fresh selections,
   combine the parent's rows by repository and effective base branch after
   applying any top-level base override. Keep the first occurrence in parent
   order. The existing Worktree allocator creates the physical branches and
   paths. Keep explicit input on the duplicate and authorization checks it
   uses today.
2. For explicit sharing with no repository or branch selection, copy the
   existing checkout names and relative paths. Extend the current
   create-time inventory check to recognize those slots. The existing
   launch matcher and workspace group attach the child to those worktree
   IDs. A server-authored reuse-only flag prevents a copied name from
   becoming a new checkout instruction on an executor transition.

Changing only the default leaves the duplicate failure. Loosening duplicate
validation would lose checkout identity and affect callers that supply
repository inputs explicitly. Keep normalization scoped to generated choices
for a fresh child and keep identity for every shared checkout.

## Tests

| Criteria | Test evidence |
| --- | --- |
| `004.1` through `004.4` | `TestCreateTaskWorkspaceModeSchema`, `TestCreateTaskWorkspaceModeArguments`, and `TestResolveMCPWorkspacePolicyCompatibility` |
| `004.3`, `004.6` | New `TestMCPSubtaskFreshWorkspaceRepositorySelection` in the MCP handlers package. Omitted and explicit fresh cases, stored parent default, repeated and distinct bases, legacy empty base, and override. |
| `004.5`, `004.8`, `004.9` | New `TestMCPSubtaskSharedWorkspaceRepositorySelection` exercises create-time validation with SQLite. Cover valid mixed inventory, missing and ambiguous slots, and absence of task/session writes in this test. Keep the explicit selection and override regressions. |
| `004.8` and executor transition | New `TestInheritedCheckoutSelectorsStayOutOfMaterialization` covers the lifecycle request. Existing canonical slot and inherited launch tests cover eligibility and binding. |
| `004.8` and first launch after executor mismatch | `TestLaunchPreparedSession_InheritedExecutorMismatchDoesNotPinCopiedSecondaryCheckout` checks that a copied secondary selector is cleared, its slot identity remains, and an explicit checkout pin survives. |
| `004.5`, `004.8`, and `004.9` | MCP handler and task-service cases cover slug-colliding managed checkouts, a legacy unscoped slot, inactive scoped rows, a live scoped row at another placement, and named-holder ambiguity without an exact canonical slot. |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.25` | The fresh selection override regression checks persisted explicit provenance and strict base fallback policy. |

The new MCP handler tests assert successful creation for the reported
same-base fixture. Both fail on the baseline duplicate error.

## E2E tests

- Add one case to `apps/web/e2e/tests/task/agent-worktree-adoption.spec.ts` in
  `chromium` for `004.7` and `004.8`. Reuse the parent fixture with a managed
  checkout and an adopted checkout on the same base. Launch two children with
  omission and one with explicit sharing. Verify their environment bindings,
  checkout paths, and branches. Wait until each child publishes its primary
  session ID before polling that session.
- `apps/web/e2e/tests/kubernetes/kubernetes-inherited-workspace.spec.ts` in
  `containers` keeps its existing scenarios. Set `inherit_parent` on the
  request that tests cross-task reuse refusal.

## Work orders

- [x] [Task 01. Default MCP children to isolated worktrees](task-01-isolate-mcp-children.md) (completed)
- [x] [Task 02. Preserve shared checkout identity](task-02-preserve-shared-checkouts.md) (completed)

## Verification results

- The temporary `TestIssue5370TemporaryReproduction` passed all three baseline
  error assertions. It ran outside the repository using `go test -overlay`.
- The full MCP handler and server suites passed after implementation:
  `go test ./internal/mcp/handlers ./internal/mcp/server -count=1`.
- The MCP handler, server, task-service, and executor suites passed after the
  review fixes: `go test ./internal/mcp/handlers ./internal/mcp/server ./internal/task/service ./internal/orchestrator/executor -count=1`.
- Focused task-service and executor tests passed for inherited selection,
  canonical slot matching, inventory eligibility, and executor transitions.
- Chromium worktree E2E passed for fresh and shared child workspaces. The
  Kind-backed Kubernetes inherited-workspace E2E also passed.
- `make -C apps/backend build` and changed-Go lint passed with zero issues.
- Public-doc validation and tests passed. The specification catalog validation
  and full specification lint passed.
- `git diff --check` passed.

## Risks

- Automations that relied on omitted sharing need to pass
  `workspace_mode: "inherit_parent"` to keep sharing uncommitted files.
- A display slug can collide for two different Git refs. Fresh selection
  grouping uses exact refs. Shared validation uses canonical checkout
  identity plus relative placement.
- A checkout can change between creation and launch. Keep the existing
  runtime inventory and physical ownership checks.
- A shared checkout name can identify a parent-only branch. It must never
  become a checkout instruction when the child materializes separately.
