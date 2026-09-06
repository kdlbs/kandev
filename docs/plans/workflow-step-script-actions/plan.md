---
created: 2026-09-05
status: done
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-001
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-002
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-003
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-004
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-005
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-006
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-007
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-008
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
legacy_specs: []
---

# Implementation Plan: Workflow Step Scripts and Inline Step Tabs

## Overview

Add `run_script` actions to step entry, agent completion, and step exit. Execute
them in the trigger-owning agent session, stream durable command output into
chat, and apply explicit timeout and failure policies. Improve the existing
workflow card by placing compact Agent, Automation, and Policies tabs inside its
inline selected-step panel. The current step strip, page-level workflow editing,
manual save, and mobile information hierarchy remain intact.

## Scope

### In scope

- Typed, portable, ordered `run_script` actions on `on_enter`,
  `on_turn_complete`, and `on_exit`.
- Destination-session binding for entry; source-session binding for completion
  and exit, including profile reuse, parking, and replacement.
- Durable at-most-once run state, agentctl workspace execution, output
  streaming, failure policy, recovery, logs, and metrics.
- Compact Agent/Automation/Policies tabs inside the existing inline selected-step
  editor, focused action editors, and actionable configuration checks.
- The existing mobile workflow card with a bounded horizontal step strip,
  touch-safe tabs, explicit move controls, and no document-level overflow.
- Existing manual-save, read-only, import/export, sync, inheritance, and
  transition semantics.
- Script execution rendering in normal agent chat, E2E coverage, localization,
  and public documentation.

### Out of scope

- Additional triggers, interactive TTY input, retries, schedules, custom
  environment values, shell selection, or a separate script path field.
- A freeform canvas, arbitrary graph edges or node coordinates, zoom controls,
  or new workflow topology.
- Dedicated workflow-editor routes, a second workflow layout, desktop side
  inspectors, or mobile journey/step/action routes.
- New workflow-level inheritance/default rules for existing step policies.
- Live Test action execution from settings.
- Persisting unsaved drafts across reloads or devices.

## Technical approach

1. Extend workflow action enums and config in
   `apps/backend/internal/workflow/models/models.go`, portable conversion in
   `apps/backend/internal/workflow/models/export.go`, and typed compilation in
   `apps/backend/internal/workflow/engine`. Validate command,
   `timeout_seconds`, and `failure_policy` at every ingestion boundary.
2. Add `WorkflowScriptRun` and a repository interface under
   `internal/task/models` and `internal/task/repository`, with replayable SQLite
   and Postgres migrations. Persist the immutable occurrence claim before
   process admission.
3. Extend the existing agentctl process runner and client with a stable request
   identity. Add `WorkspaceProcessRunner` to `internal/agent/runtime` so callers
   bind an exact session, execution, and `Execution.WorkspacePath` while reusing
   current environment, output, process-group, stop, and status behavior.
4. Implement `workflow_script_runner.go` in the orchestrator and result-bearing
   callbacks in the workflow engine. Route every entry, completion, exit,
   workflow-switch, deferred, and manual transition path through the same
   coordinator and startup reconciler.
5. Add `workflow-action-catalog.ts` and `workflow-editor-view-model.ts` under
   `apps/web/lib/workflows`. Reuse `workflow-dirty-state.ts` and immutable step
   mutations to derive compact summaries, lifecycle groups, transition edges,
   selection repair, and resolvable diagnostics without changing the wire shape.
6. Retain `WorkspaceWorkflowsClient`, `WorkflowCard`,
   `WorkflowPipelineEditor`, and the inline selected-step panel. Integrate the
   shared action catalog and view model there, then add a compact segmented tab
   control without adding workflow-editor routes or route selection state.
7. Keep each workflow card's existing `SettingsSaveProvider` contributor,
   client-only identity remapping, destructive confirmations, and dirty
   navigation ownership. Multiple dirty workflows continue to save together.
8. Extend `ScriptExecutionMessage`, `message-renderer.tsx`, and
   `processed-message-filtering.ts` for workflow metadata and in-place updates.
   Add localized workflow/task catalog entries and targeted Playwright coverage.

## Tests

- `AC-TASKS-WORKFLOW-STEP-SCRIPT-001.1` through `.6`:
  `internal/workflow/models/workflow_test.go`,
  `portable_test.go`, and service/handler tests cover valid, defaulted, ordered,
  duplicated, imported, synchronized, and rejected action configs.
- `AC-TASKS-WORKFLOW-STEP-SCRIPT-002.1` through `.7` and
  `AC-TASKS-WORKFLOW-STEP-SCRIPT-003.1` through `.9`:
  `process_runner_test.go`,
  `workflow_script_runner_test.go`, and
  `event_handlers_workflow_triggers_test.go` cover workspace/session ownership,
  lifecycle order, process groups, and both policies.
- `AC-TASKS-WORKFLOW-STEP-SCRIPT-004.1` through `.7`: SQLite/Postgres
  `workflow_script_run_test.go` and orchestrator
  reconciliation tests cover concurrent claims, replay, restart, and shutdown.
- `AC-TASKS-WORKFLOW-STEP-SCRIPT-005.1` through `.7`:
  `workflow_script_runner_test.go`,
  `script-execution-message.test.tsx`, and
  `processed-message-filtering.test.ts` cover message persistence, coalescing,
  truncation, reconnect, and turn exclusion.
- `AC-TASKS-WORKFLOW-STEP-SCRIPT-006.1` through `.5`:
  `workflow-action-catalog.test.ts`,
  `workflow-step-mutations.test.ts`, and script action component tests cover
  compatible triggers, focused editing, validation, read-only state, and save.
- `AC-TASKS-WORKFLOW-STEP-SCRIPT-007.1` through `.4`: workflow handler
  authorization tests, orchestrator metric/log
  tests, and `validate-public-docs` cover trust and bounded observability.
- `AC-TASKS-WORKFLOW-STEP-SCRIPT-008.1` through `.10`:
  `workflow-editor-view-model.test.ts`, workflow-card and inline step-panel
  component tests, and settings save contributor tests cover selection repair,
  compact tabs, issue targets, multiple dirty workflows, and responsive
  composition.

## E2E tests

- `workflow-settings.spec.ts` (`AC-TASKS-WORKFLOW-STEP-SCRIPT-008.1` through
  `.6`, `.9`): desktop authors a script through compact tabs in the existing
  inline step editor, changes steps without losing the draft, saves once, and
  retains all workflow-level fields.
- `workflow-settings.spec.ts` (`AC-TASKS-WORKFLOW-STEP-SCRIPT-008.4`, `.5`):
  configuration checks select the correct inline step, tab, and invalid action;
  transitions changed in the recipe update the existing step strip immediately.
- `workflow-step-script-profile-switch.spec.ts`
  (`AC-TASKS-WORKFLOW-STEP-SCRIPT-002.2` through `.4`): completion/exit output
  remains in the source
  session and entry output appears in the selected reused/new destination
  session before its prompt.
- `workflow-step-scripts.spec.ts`
  (`AC-TASKS-WORKFLOW-STEP-SCRIPT-003.5` through `.9`,
  `AC-TASKS-WORKFLOW-STEP-SCRIPT-004.2`,
  `AC-TASKS-WORKFLOW-STEP-SCRIPT-005.2`, `.3`): covers non-zero exit, timeout,
  block, continue, reload,
  duplicate delivery, and interrupted recovery.
- `mobile-workflow-cycle-guardrails.spec.ts` and a focused inline-authoring case
  in the `mobile-chrome` project
  (`AC-TASKS-WORKFLOW-STEP-SCRIPT-006.4`,
  `AC-TASKS-WORKFLOW-STEP-SCRIPT-008.7`, `.8`): selects a step and tab in the
  existing workflow card, authors and reorders scripts, saves, inspects output,
  meets 44-pixel targets, and has no document-level horizontal overflow.
- `workflow-settings.spec.ts` (`AC-TASKS-WORKFLOW-STEP-SCRIPT-006.3`,
  `AC-TASKS-WORKFLOW-STEP-SCRIPT-008.9`): synchronized workflows
  retain complete inspection while all
  mutation affordances remain disabled with a reason.

## Work orders

- [x] [Task 01: Define the script action contract](task-01-define-script-action-contract.md)
- [x] [Task 02: Persist workflow script runs](task-02-persist-workflow-script-runs.md)
- [x] [Task 03: Add workspace process execution](task-03-add-workspace-process-execution.md)
- [x] [Task 04: Integrate workflow trigger scripts](task-04-integrate-workflow-triggers.md)
- [x] [Task 05: Build the workflow editor view model](task-05-build-workflow-editor-view-model.md)
- [x] [Task 06: Build the desktop workflow inspector (superseded)](task-06-build-desktop-workflow-inspector.md)
- [x] [Task 07: Build lifecycle action recipes](task-07-build-lifecycle-action-recipes.md)
- [x] [Task 08: Build mobile workflow editing (superseded)](task-08-build-mobile-workflow-editing.md)
- [x] [Task 09: Render workflow scripts in chat](task-09-render-workflow-scripts-in-chat.md)
- [x] [Task 10: Prove the workflow experience (superseded)](task-10-prove-and-document-experience.md)
- [x] [Task 11: Harden script occurrence and lock ownership](task-11-harden-script-occurrence-and-locks.md)
- [x] [Task 12: Restore inline workflow editing with compact tabs](task-12-restore-inline-workflow-tabs.md)
- [x] [Task 13: Prove the revised inline experience](task-13-prove-revised-inline-experience.md)

Tasks 01 through 10 record the first implementation pass. The review and design
revision add Tasks 11 through 13. Tasks 11 and 12 can proceed independently;
Task 13 follows both and owns final integration evidence.

## Verification results

- Backend race-enabled workflow-script/profile-switch tests pass (27 tests),
  and the affected orchestrator, engine, task-model, and SQLite repository
  packages pass (3,825 tests).
- Frontend action-catalog, editor-view-model, automation, and focused editor
  tests pass; frontend typecheck, targeted lint, i18n check, and the new-code
  i18n ratchet pass.
- Desktop inline workflow settings/editor and script/profile-switch E2E pass
  (27 tests); the mobile workflow editor/settings suite passes (7 tests); the
  dedicated script/profile-switch suite passes (7 tests).
- Runtime evidence covers source/destination session binding, non-zero and
  timeout outcomes, block/continue policies, repeated occurrences, reload
  idempotency, and interrupted recovery without rerunning a process.
- Specification lint, public documentation validation, work-order audits, and
  `git diff --check` pass for the revised package.

## Risks

- The entry boundary commits before its session-bound actions, so `block`
  cannot roll back a destination step or earlier repository-owned side effects.
- Ambiguous process admission must prefer an interrupted audit result over a
  duplicate non-idempotent command.
- Refactoring every existing action into one catalog can accidentally change
  serialization or ordering; characterization tests must precede the UI move.
- Reintegrating tabs can duplicate legacy transition controls unless the inline
  panel has one clear owner for each setting.
- An action identity derived from array position changes during reordering;
  selection must be repaired deterministically without changing persisted data.
- Desktop and mobile can drift if they duplicate mutations instead of sharing
  the catalog and view model.
- Fallback lifecycle identities must distinguish repeated transitions through
  the same source, destination, and session without weakening duplicate-event
  suppression.
- Per-run coordination locks must be released only after the last waiter leaves;
  deleting a keyed mutex immediately after one unlock would create a race.
