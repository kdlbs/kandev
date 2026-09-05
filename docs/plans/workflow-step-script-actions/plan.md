---
created: 2026-09-05
status: implemented
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

# Implementation Plan: Workflow Step Scripts and Focused Editor

## Overview

Add `run_script` actions to step entry, agent completion, and step exit. Execute
them in the trigger-owning agent session, stream durable command output into
chat, and apply explicit timeout and failure policies. In the same package,
replace the dense inline workflow form with a dedicated constrained pipeline,
focused step inspector, lifecycle action recipes, and native mobile navigation.
The backend contract lands before orchestration, and the shared editor view
model lands before either viewport composition so each layer builds on a tested
source of truth.

## Scope

### In scope

- Typed, portable, ordered `run_script` actions on `on_enter`,
  `on_turn_complete`, and `on_exit`.
- Destination-session binding for entry; source-session binding for completion
  and exit, including profile reuse, parking, and replacement.
- Durable at-most-once run state, agentctl workspace execution, output
  streaming, failure policy, recovery, logs, and metrics.
- A dedicated workflow editor route with compact pipeline summaries, a desktop
  Agent/Automation/Policies inspector, focused action editors, and actionable
  configuration checks.
- A mobile vertical journey with full-height step/action editors, a temporary
  action-choice drawer, explicit move controls, safe areas, and touch parity.
- Existing manual-save, read-only, import/export, sync, inheritance, and
  transition semantics.
- Script execution rendering in normal agent chat, E2E coverage, localization,
  and public documentation.

### Out of scope

- Additional triggers, interactive TTY input, retries, schedules, custom
  environment values, shell selection, or a separate script path field.
- A freeform canvas, arbitrary graph edges or node coordinates, zoom controls,
  or new workflow topology.
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
6. Add the dedicated route in the SPA settings route table and
   `apps/web/src/settings-routes.workspace-data.tsx`. Refactor `WorkflowCard`,
   `WorkflowPipelineEditor`, and `StepConfigPanel` into a
   route-level draft shell, desktop pipeline/inspector, lifecycle action
   editors, and mobile journey/step/action compositions. Add a sibling `new`
   route for client-only creation and replace its URL after first Save.
7. Register the route-level contributor with `SettingsSaveProvider`. Keep
   client-only identity remapping in `useWorkflowDraftContributor`, existing
   immediate destructive confirmations, and dirty navigation ownership.
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
  `workflow-editor-view-model.test.ts`, desktop/mobile editor
  component tests, and settings save contributor tests cover route selection,
  summaries, issue targets, dirty navigation, and responsive composition.

## E2E tests

- `workflow-editor.spec.ts` (`AC-TASKS-WORKFLOW-STEP-SCRIPT-008.1` through
  `.6`, `.9`): desktop
  authors a script through the lifecycle recipe, changes steps without
  losing the draft, saves once, and observes streaming and terminal chat state.
- `workflow-editor.spec.ts` (`AC-TASKS-WORKFLOW-STEP-SCRIPT-008.4`, `.5`):
  configuration checks
  navigate directly to an invalid action field;
  transitions changed in the recipe update the pipeline immediately.
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
- `mobile-workflow-editor.spec.ts` in the `mobile-chrome` project
  (`AC-TASKS-WORKFLOW-STEP-SCRIPT-006.4`,
  `AC-TASKS-WORKFLOW-STEP-SCRIPT-008.7`, `.8`): navigates journey to step to action and back,
  authors/reorders scripts,
  saves, inspects output, meets 44-pixel targets, respects safe areas, and has no
  document-level horizontal overflow.
- `workflow-editor.spec.ts` (`AC-TASKS-WORKFLOW-STEP-SCRIPT-006.3`,
  `AC-TASKS-WORKFLOW-STEP-SCRIPT-008.9`): synchronized workflows
  retain complete inspection while all
  mutation affordances remain disabled with a reason.

## Work orders

- [x] [Task 01: Define the script action contract](task-01-define-script-action-contract.md)
- [x] [Task 02: Persist workflow script runs](task-02-persist-workflow-script-runs.md)
- [x] [Task 03: Add workspace process execution](task-03-add-workspace-process-execution.md)
- [x] [Task 04: Integrate workflow trigger scripts](task-04-integrate-workflow-triggers.md)
- [x] [Task 05: Build the workflow editor view model](task-05-build-workflow-editor-view-model.md)
- [x] [Task 06: Build the desktop workflow inspector](task-06-build-desktop-workflow-inspector.md)
- [x] [Task 07: Build lifecycle action recipes](task-07-build-lifecycle-action-recipes.md)
- [x] [Task 08: Build mobile workflow editing](task-08-build-mobile-workflow-editing.md)
- [x] [Task 09: Render workflow scripts in chat](task-09-render-workflow-scripts-in-chat.md)
- [x] [Task 10: Prove the workflow experience](task-10-prove-and-document-experience.md)

Tasks 02 and 03 are parallel-safe after Task 01. Tasks 08 and 09 are
parallel-safe after their dependencies because they own separate mobile editor
and chat transcript surfaces. All other work is sequential at its dependency
boundary.

## Verification results

- Focused frontend Vitest coverage passes for the action catalog, editor view
  model, step mutations, workflow draft creation, focused editor, chat
  rendering, and processed-message filtering.
- Frontend typecheck, lint, i18n check, and new-code i18n ratchet pass.
- Desktop focused editor E2E passes the persisted draft flow and the
  client-only new-workflow flow. Mobile `mobile-chrome` editor E2E passes the
  journey, action drawer, reorder, save, touch-target, and overflow checks.
- Affected backend package tests pass, including workflow script persistence,
  process lifecycle, orchestration, and engine packages. The broad backend
  suite has seven host-home config discovery failures because
  `/root/.kandev/config.yaml` is selected by those tests.
- Specification lint and public documentation validation pass. Workflow docs
  cover executor permissions, persisted chat output, import/export, and sync.

## Risks

- The entry boundary commits before its session-bound actions, so `block`
  cannot roll back a destination step or earlier repository-owned side effects.
- Ambiguous process admission must prefer an interrupted audit result over a
  duplicate non-idempotent command.
- Refactoring every existing action into one catalog can accidentally change
  serialization or ordering; characterization tests must precede the UI move.
- Dedicated route navigation can discard drafts if the save contributor is
  scoped below the route shell.
- An action identity derived from array position changes during reordering;
  selection must be repaired deterministically without changing persisted data.
- Desktop and mobile can drift if they duplicate mutations instead of sharing
  the catalog and view model.
