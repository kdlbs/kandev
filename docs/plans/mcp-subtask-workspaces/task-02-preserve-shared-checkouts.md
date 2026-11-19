---
id: "02-preserve-shared-checkouts"
title: "Preserve shared checkout identity"
status: completed
wave: 2
depends_on:
  - "01-isolate-mcp-children"
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
acceptance_criteria:
  - AC-TASKS-MCP-WORKSPACE-MODE-004.5
  - AC-TASKS-MCP-WORKSPACE-MODE-004.7
  - AC-TASKS-MCP-WORKSPACE-MODE-004.8
  - AC-TASKS-MCP-WORKSPACE-MODE-004.9
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
---

# Task 02. Preserve shared checkout identity

## Summary

Preserve each implicit repository slot for explicit `inherit_parent`.
Validate the selected checkouts before inserting the child and bind its
session to the parent's existing worktrees.

## In scope

- Add `TestMCPSubtaskSharedWorkspaceRepositorySelection` with a ready parent
  carrying a generated primary checkout and a named, nested adopted checkout
  on the same base. Assert both slots and their positions. Record the
  baseline duplicate failure before implementing.
- Preserve the ordered checkout and relative path in server-derived
  `TaskRepositoryInput` data. Write the placement and reuse provenance
  through `buildResolvedTaskRepositoryRow` using existing task repository
  fields and metadata. Keep explicit repository and branch selection on
  its existing admission path.
- Extend `ValidateInheritedWorkspaceRepositorySelection` for these inputs.
  Call it during `prepareTaskForCreation` so existing external-ID lookup
  precedes validation. Use the existing branch planner for identity and
  match the relative path. Cover live, missing, ambiguous, and mixed live
  and inactive rows in the same handler test. Use the existing validation
  response and include the requested checkout.
- Keep the existing launch matcher. Derive branch identity before clearing
  a reuse-only name from fresh checkout and refresh requests. Apply this to
  first launch as well as resume, including a fresh launch after an executor
  mismatch. Preserve explicitly selected checkout pins. Cover this at the
  executor boundary for an eligible fresh transition.
- Match inherited Worktree inventory using the launch canonical identity and
  relative path first. Only use exact Git branch-name matching when no
  canonical slot exists. Preserve distinct-row selection, launch-compatible
  legacy unscoped inventory, and ambiguity rejection when no canonical slot
  exists.
- Add the case `MCP children use fresh or shared worktrees` to
  `agent-worktree-adoption.spec.ts`. From one parent fixture, launch two
  children with omission and one with explicit sharing. Assert separate
  environment IDs, paths, and branches for the first two. Assert the share
  uses the same environment and worktree IDs as the parent. Include an
  uncommitted marker in the adopted checkout. Poll for each child primary
  session ID before waiting for its session to settle.

## Out of scope

- New Git worktree allocation or adoption behavior.
- Broader repository selection syntax, automatic workspace switching on a
  missing slot, and historical backfills.

## Acceptance

- A compatible explicit sharing launch reuses both exact checkouts, with
  their existing uncommitted files. An eligible fresh executor transition
  does not use copied checkout names as Git instructions.
- A missing, inactive-only, or ambiguous checkout fails with the repository
  and checkout name. No child, session, or workspace group membership is
  added. A live match and an unrelated inactive row allow creation.
- A duplicate external-ID call returns the existing authorized child even
  if inventory changed after the first creation. An unmaterialized parent
  remains on the existing launch-time validation path.

## Verification

```bash
(cd apps/backend && go test ./internal/mcp/handlers -run 'TestMCPSubtask.*WorkspaceRepositorySelection|TestHandleCreateTask_InheritParentRejects|TestHandleCreateTask_ExternalID' -count=1)
(cd apps/backend && go test ./internal/task/service -run '^TestValidateInheritedWorkspaceRepositorySelection' -count=1)
(cd apps/backend && go test ./internal/orchestrator/executor -run 'TestInheritedCheckoutSelectorsStayOutOfMaterialization|Test.*CanonicalSlot|TestValidateReuseEnvironmentInventory|TestLaunchPreparedSession_Inherited' -count=1)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/agent-worktree-adoption.spec.ts -- --grep "MCP children use fresh or shared worktrees")
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/create_task_subtask_workspaces_test.go`
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/inherited_workspace_admission.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go` and
  `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_reuse_inventory_test.go`
- `apps/web/e2e/tests/task/agent-worktree-adoption.spec.ts`

## Dependencies

[Task 01](task-01-isolate-mcp-children.md) resolves the mode before repository
mapping.

## Risks

A sanitized slug alone cannot identify an adopted checkout. Validate its
relative placement and canonical entry. ID and inventory checks must also
survive changes between creation and launch. Cover the executor transition
to ensure a parent-only checkout name cannot become a fresh Git pin.

## Parallelism

`sequential`

## Inputs

- [Shared checkout design](../../specs/tasks/system-design/mcp-workspace-mode.md#inherited-repository-compatibility)
- [Executor transition design](../../specs/tasks/system-design/additional-session-workspace-reuse.md#executor-transition)
- `adoptCheckoutBranchCanonicalSlots` in
  `executor_environment_reuse.go` and `uniqueAdoptionBranchSlug` in
  `service_worktree_adoption_workspace.go`.
- `apps/web/e2e/helpers/agent-worktree-adoption.ts` has the linked worktree
  fixture. Use `waitForSessionDone` and the session and environment API
  responses to verify the actual MCP calls.

## Results

Implemented exact inherited checkout identity and create-time inventory
validation for shared Worktree selections. Missing, inactive-only, and
ambiguous matches fail before task/session writes; external-ID replay remains
ahead of inventory validation. Reuse-only checkout metadata preserves the
canonical launch match while preventing a copied checkout name from becoming
a Git instruction on a fresh executor transition.

The fresh/shared MCP child E2E passed in Chromium, including distinct fresh
checkout paths and branches, parent worktree reuse for sharing, and the
uncommitted adopted-checkout marker. Focused task-service and executor tests,
the full MCP handlers and server suites, changed-Go lint, the backend build,
and `git diff --check` passed. Documentation and specification validation and
lint passed.

Review regressions pass: first launch after an executor mismatch clears a
copied secondary checkout selector while preserving its branch identity and
explicit pins; canonical slots win over colliding branch slugs; one legacy
unscoped Worktree row remains launch-compatible; and exact branch-name
fallback rejects ambiguous named holders without a canonical slot. Inactive
scoped rows do not suppress a live legacy fallback, while a live scoped row at
another placement prevents that fallback for the repository.
