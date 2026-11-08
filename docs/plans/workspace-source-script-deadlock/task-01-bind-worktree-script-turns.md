---
id: "01-bind-worktree-script-turns"
title: "Bind worktree script messages without opening a turn"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6
system_design:
  - ../../specs/tasks/system-design/attach-workspace-sources.md
  - ../../specs/tasks/system-design/workspace-source-script-turns.md
---

# Task 01: Bind worktree script messages without opening a turn

## Summary

Correct the trusted worktree message adapter while preserving task mutation serialization.
Add regressions through the real materializers, task service, SQLite repository, and script handler.

## In scope

- Apply the [turn-binding policy](../../specs/tasks/system-design/workspace-source-script-turns.md#turn-binding) in `taskServiceAdapter.CreateMessage`.
- Preserve supplied turn IDs without a lookup. Preserve observed active IDs even after that turn completes.
- Use the internal completed lifecycle path only after a successful lookup returns no active turn.
- Propagate lookup errors through existing script warning/cleanup behavior.
- Cover idle attachment, idle/active legacy add-branch, retries, nonzero setup exits, and compensation.

## Out of scope

Task mutex changes, asynchronous setup, DTO/schema changes, public request parameters, boot adapter changes, and browser implementation.

## Acceptance

1. The failing idle-attachment regression passes with real script output and final status. The next user turn starts without a phantom active turn.
2. Active and supplied turn IDs remain exact. Idle script turns are completed and lifecycle-only. Lookup errors create no fallback turn or message.
3. Legacy paths, setup warnings, compensation, exact retries, prelaunch deferral, cancellation boundaries, and the existing mutation-gate regression retain their contracts.

## Implementation sequence

1. Read the linked requirement, both designs, the [plan evidence](plan.md#reproduction-and-feasibility-evidence), and scoped backend guidance.
2. Set this work order to `in_progress`.
3. Add `TestWorkspaceSourceScripts_AttachIdle` before the production correction.
4. Run the red command below and retain the stack showing `createTurn` waiting under setup materialization.
5. Apply the adapter correction.
6. Add the remaining adapter and integration cases from the plan.
7. Run the green checks and record each result.
8. Update this work order and the plan after all acceptance conditions pass.

## Regression construction

Use the existing `setupMaterializerScenario`, `newMaterializerRepo`, and `seedMaterializerTask` fixtures from `branch_materializer_test.go`.
Wire `worktree.NewRepositoryAdapter`, `DefaultScriptMessageHandler`, and the real `taskServiceAdapter`.
Do not stub script-message creation.

Add a second local repository with a setup script and a ready task environment.
Use a rescan/rebind recorder for runtime effects without starting a real agent.
Assert script output, exited/failed status, source rows, materialized paths, turn metadata, and subsequent turn admission.
The active legacy case must keep its original active turn and unchanged CWD.

For compensation, inject a materializer failure after setup and configure a cleanup script.
Verify that attachment rollback and any cleanup message finish without another turn-creation attempt.
The existing warning policy still treats a nonzero setup exit as nonfatal worktree setup.

Use a subprocess-isolated regression if a deadlock cannot be drained safely inside the test process.
The red run must fail because of the lock chain, rather than a fixture error or script timeout alone.

## Verification

Commands start from the repository root. There is no narrow Make target for these package selections.
The red command runs before the production correction and must fail for the expected deadlock.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^TestWorkspaceSourceScripts_AttachIdle$' -count=1 -timeout=30s)
```

After the correction, run these commands sequentially:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp -run 'TestWorktreeScriptMessageAdapter|TestWorkspaceSourceScripts|TestBootMessageAdapter' -count=1 -timeout=180s)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/service -run 'TestAttachWorkspaceSources|TestAddBranchToTask|TestCreateMessageWithCompletedTurn|TestCurrentTurnAuthoritySurvivesLifecycleShadow|TestCreateCompletedTurn' -count=1 -timeout=180s)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/worktree -run 'Test.*Script|Test.*Setup' -count=1 -timeout=180s)
bash scripts/lint-go-changed apps/backend/internal/backendapp/worktree.go apps/backend/internal/backendapp/worktree_script_turns_test.go apps/backend/internal/backendapp/workspace_source_script_turns_test.go
git diff --check
```

## Files likely touched

- `apps/backend/internal/backendapp/worktree.go`
- `apps/backend/internal/backendapp/worktree_script_turns_test.go` (new)
- `apps/backend/internal/backendapp/workspace_source_script_turns_test.go` (new)
- This work order and `plan.md` for execution results.

Read-only context includes `task/service/service_messages.go`, `service_turns.go`, `service_workspace_sources.go`, and `service_branches.go`.
The worktree script handler and both host materializers also provide context.
No production edit outside the adapter is expected.

## Dependencies

None.

## Risks

- A lifecycle turn must never acquire the task mutation gate or publish a conversational start event.
- Binding to a successor after an observed turn completes can misattribute setup output.
- Shared test databases and script subprocesses require immediate cleanup registration and bounded completion.
- The existing setup/cleanup adapter serves initial launch as well as live attachment.

## Parallelism

`sequential`

## Inputs

- [Attachment requirement](../../specs/tasks/requirements/attach-workspace-sources.md), REQ-001 and the exact AC IDs in frontmatter.
- [Attachment design](../../specs/tasks/system-design/attach-workspace-sources.md).
- [Script-turn design](../../specs/tasks/system-design/workspace-source-script-turns.md).
- `apps/backend/internal/backendapp/worktree_test.go` for the existing boot-message lifecycle pattern.
- `apps/backend/internal/task/service/service_workspace_sources_test.go` for mutation-gate coverage.
- `apps/backend/internal/task/service/service_turns_lifecycle_shadow_test.go` for conversation-authority coverage.

## Results

Completed 2026-11-08.

`taskServiceAdapter.CreateMessage` now applies the turn-binding policy. A
supplied `TurnID` is preserved without a lookup. Otherwise the non-creating
`Service.GetActiveTurn` runs; an observed active ID is passed explicitly, a
confirmed absence sets the internal `CompletedTurn` field, and a lookup error
returns without creating a turn or message. No task-service production change
was necessary.

Regression coverage added:

- `worktree_script_turns_test.go`: idle lifecycle binding, supplied completed
  turn ID, observed active turn ID, observed-turn completion with successor
  admission before message persistence, and active-turn lookup failure without
  a fallback message.
- `workspace_source_script_turns_test.go`: idle batch attachment, exact retry,
  nonzero setup exit, idle/active legacy add-branch, and post-setup
  compensation with a cleanup message. The integration fixture uses real SQLite,
  local Git worktrees, the production adapter, the default script handler, and a
  script subprocess.

Red evidence before the production correction:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^TestWorkspaceSourceScripts_AttachIdle$' -count=1 -timeout=12s)
```

The run stopped at the task mutex with the expected chain:
`AttachWorkspaceSources` -> `runWorktreeSetupScript` ->
`taskServiceAdapter.CreateMessage` -> `Service.CreateMessage` -> `createTurn` ->
`lockWorkspaceSources`. With the guard active, the corrected test completed in
about two seconds.

Green commands:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp -run 'TestWorktreeScriptMessageAdapter|TestWorkspaceSourceScripts|TestBootMessageAdapter' -count=1 -timeout=180s)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/service -run 'TestAttachWorkspaceSources|TestAddBranchToTask|TestCreateMessageWithCompletedTurn|TestCurrentTurnAuthoritySurvivesLifecycleShadow|TestCreateCompletedTurn' -count=1 -timeout=180s)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/worktree -run 'Test.*Script|Test.*Setup' -count=1 -timeout=180s)
bash scripts/lint-go-changed apps/backend/internal/backendapp/worktree.go apps/backend/internal/backendapp/worktree_script_turns_test.go apps/backend/internal/backendapp/workspace_source_script_turns_test.go
git diff --check
```

All three test commands passed. The changed-file lint reported `0 issues`, and
`git diff --check` passed.

The review remediation command

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp -run '^TestWorktreeScriptMessageAdapter_' -count=1 -timeout=180s)
```

also passed and exercises the new observed-turn-completion-with-successor case
through the real service and SQLite repository.
