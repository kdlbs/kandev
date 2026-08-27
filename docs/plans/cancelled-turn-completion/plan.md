---
spec: docs/specs/tasks/requirements/workflow-cancelled-turn-completion.md
created: 2026-08-02
status: implemented
---

# Implementation Plan: Cancelled Turn Completion

## Overview

Add the persisted workflow-step policy and template default first, expose it through every existing workflow configuration contract, then route only explicit user cancellation through the established completion pipeline. Wire the setting into the shared workflow editor with localized, touch-safe presentation, document the public contract, and finish with desktop and mobile Playwright coverage. Existing workflow rows stay unchanged.

---

## Backend

### Persisted step contract and template defaults

Files:

- `apps/backend/internal/workflow/models/models.go`
- `apps/backend/internal/workflow/models/export.go`
- `apps/backend/config/workflows/loader.go`
- `apps/backend/config/workflows/kanban.yml`
- `apps/backend/internal/workflow/repository/sqlite.go`
- `apps/backend/internal/task/repository/sqlite/workspace_bootstrap.go`
- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/workflow/service/sync_apply.go`

Changes:

- Add `CancelTriggersTurnComplete bool` to `WorkflowStep`, `StepDefinition`, and `StepPortable`, serialized as `cancel_triggers_turn_complete`.
- Add a replay-safe `workflow_steps.cancel_triggers_turn_complete` migration with database default `false`, and thread the field through create, update, select, scan, template instantiation, default-step seeding, workspace bootstrap, portable import/export, and workflow-sync equality/application.
- Extend the embedded template loader so the YAML field is decoded and copied into step definitions.
- Set the field to `true` on the `simple` template's `Backlog` and `In Progress` steps only. Do not backfill existing rows and do not change custom-step defaults.
- Verify fresh/replayed SQLite schema behavior, the env-gated Postgres bootstrap/round trip, repository round trips, template instantiation, default workspace bootstrap, portable import/export, and sync preservation.

### HTTP, WebSocket, and config MCP surfaces

Files:

- `apps/backend/internal/workflow/controller/controller.go`
- `apps/backend/internal/workflow/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/config_workflow_handlers.go`
- `apps/backend/internal/mcp/server/config_handlers.go`
- `apps/backend/config/prompts/config-context.md`

Changes:

- Extend create/update request structs with optional `cancel_triggers_turn_complete` pointers so omitted create means `false` and omitted update preserves the stored value.
- Return the effective boolean in workflow-step responses and WebSocket step mutation events.
- Add the field to config-mode MCP create/update schemas, payload forwarding, list output, and config-agent prompt documentation.
- Keep authorization, synced-workflow immutability, and the existing cancel WebSocket action unchanged.

### Explicit user-cancel routing

Files:

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/orchestrator/event_handlers_step_completion_test.go`

Changes:

- Introduce a typed completion cause or equivalent private helper input so ordinary agent completion continues to require the pending signal when configured, while a configured explicit user cancellation can bypass that agent-owned gate.
- In `Service.CancelAgent`, preserve authorization, deduplication, runtime cancellation, the visible cancellation message, turn completion, parked queue behavior, and session reconciliation. Reconciliation must return an authoritative result: confirm `WAITING_FOR_INPUT`, close every active turn, and verify no turn remains before evaluating `on_turn_complete` exactly once only when the current non-Office, non-ephemeral, non-archived Kanban step enables the policy. Any state or turn persistence failure fails closed and leaves the workflow transition untouched.
- Retain the clarification barrier, pending/stale-event guards, transition failure behavior, `on_exit`, terminal-state handling, and asynchronous destination `on_enter` processing.
- Avoid a duplicate or transient `REVIEW` state write when a workflow transition owns the final task state (especially terminal transitions); reconcile `REVIEW` after a confirmed no-transition or successful nonterminal transition, while terminal transitions remain direct.
- Keep `cancelAgentSilent`, peer interruption, parent stop, archive, provider error, `AgentStopped`, and other runtime teardown paths outside this policy.

---

## Frontend

### Workflow-step contract and draft persistence

Files:

- `apps/web/lib/types/http.ts`
- `apps/web/app/actions/workspaces.ts`
- `apps/web/components/settings/workflow-card-actions.ts`

Changes:

- Add `cancel_triggers_turn_complete` to backend normalization, create/update payloads, workflow-step types, draft equality, temporary-step remapping, and save payloads.
- Preserve omitted/false semantics so new custom steps remain off and template-derived steps retain their server-provided value.

### Workflow settings UI

Files:

- `apps/web/components/settings/workflow-pipeline-editor-step-actions.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-step-actions.test.tsx` (new)
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`

Changes:

- Under a configured `On Turn Complete` transition, add a localized checkbox labeled **Run completion actions when a turn is cancelled**.
- Explain that the option applies only to a user cancel and that destination actions may immediately start another agent.
- Reuse the existing step-editor update/dirty-state path and disable the control for read-only synced workflows.
- Add stable test IDs and component coverage for default-off, persisted-on, dirty-state, update, and read-only behavior.

### Mobile design contract

- **Desktop outcome and mobile entry point:** both use Settings → Workflows → workflow card → selected step → On Turn Complete. No capability is hidden on phones.
- **Nearest shipped exemplar:** the adjacent `ExplicitCompletionToggle` provides the inline checkbox/help pattern; `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts` provides the narrow-viewport settings flow and geometry checks.
- **Hierarchy and primary action:** the cancellation checkbox remains subordinate to the step's completion transition; its associated semantic label is the actual touch target and spans at least 44px on phones.
- **Presentation:** inline inside the existing focused step editor. A drawer would add unnecessary navigation for one boolean setting.
- **Scroll and geometry:** preserve the settings page's existing document scroll owner, use a touch-sized row/label target, and introduce no fixed controls, nested scrolling, or horizontal overflow.
- **Shared logic:** desktop and mobile use the same `WorkflowStep` draft and update callback; only responsive spacing/touch sizing may differ.
- **Mobile proof:** a `mobile-*.spec.ts` flow enables and saves the setting by touch, reloads it, cancels a live turn, and observes the same workflow transition without document overflow.

---

## Public Documentation

Files:

- `docs/public/tasks-and-workflows.md`
- `docs/public/workflow-tips.md`
- `docs/public/workflow-import-export.md`
- `docs/public/workflow-sync.md`

Changes:

- Document pause-in-place versus complete-and-advance behavior, the standard Kanban template default for newly created workflows, and the absence of an upgrade backfill.
- Add `cancel_triggers_turn_complete` to the workflow import/export reference and synchronized workflow examples.
- Explain the interaction with `auto_advance_requires_signal`, clarification barriers, destination auto-start actions, and explicit user cancellation scope.
- Keep `workflow-import-export.md` and `workflow-sync.md` as reference pages; keep `tasks-and-workflows.md` and `workflow-tips.md` task-oriented how-to/explanation pages.

---

## Tests

- **What:** schema default/replay, repository round-trip, loader/template copy, default Kanban values, portable import/export, workspace bootstrap, and workflow sync preserve `cancel_triggers_turn_complete`.
  **Files:** `apps/backend/config/workflows/loader_test.go`, `apps/backend/internal/workflow/repository/sqlite_test.go`, `apps/backend/internal/workflow/models/export_test.go`, `apps/backend/internal/workflow/service/service_test.go`, `apps/backend/internal/workflow/service/sync_apply_test.go`, `apps/backend/internal/task/repository/sqlite/builtin_workflow_test.go`, `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`.
  **How:** table-driven model/loader tests plus real SQLite repository and service tests; include same-database migration replay and an env-gated Postgres bootstrap/round trip.

- **What:** HTTP/config MCP create, update, list, schema, and event payloads preserve omitted-versus-false semantics.
  **Files:** controller/handler tests beside the changed packages.
  **How:** request/response tests that create `true`, explicitly update to `false`, and omit on update.

- **What:** enabled explicit user cancellation runs completion exactly once for acknowledged, escalated, and missing-execution reconciliation; disabled steps remain in place.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`.
  **How:** real repository service tests with action-bearing steps and deterministic mock cancellation results, plus injected session-state and turn-close failures that prove no transition runs before reconciliation settles.

- **What:** signal-gated cancellation bypasses only the agent signal, pending clarification still blocks, archived/Office/ephemeral tasks do not move, queued messages remain parked, terminal targets complete, destination auto-start remains active, and stale ready/completed events cannot double-transition.
  **Files:** `apps/backend/internal/orchestrator/task_operations_test.go`, `apps/backend/internal/orchestrator/event_handlers_step_completion_test.go`.
  **How:** focused regression cases reusing the existing cancel guard, workflow engine, clarification, and queue harnesses.

- **What:** frontend normalization/draft comparison and the settings toggle preserve the field and emit one update with localized, read-only, dirty-state, and touch-target behavior.
  **Files:** `apps/web/components/settings/workflow-card-actions.test.ts`, `apps/web/components/settings/workflow-pipeline-editor-step-actions.test.tsx`, `apps/web/lib/api/domains/workflow-api.test.ts`.
  **How:** pure draft tests plus Testing Library interaction tests.

---

## E2E Tests

- **Scenario:** a desktop user enables cancellation completion, saves, starts a delayed turn, cancels it, and sees the task move to the configured destination exactly once while the cancellation message remains visible.
  **File:** `apps/web/e2e/tests/workflow/workflow-cancel-completion.spec.ts`.
  **What to verify:** persisted setting after reload, active-turn cancel through the UI, destination step/board position, session input readiness, and no duplicate destination auto-start.

- **Scenario:** a step leaves the option disabled.
  **File:** `apps/web/e2e/tests/workflow/workflow-cancel-completion.spec.ts`.
  **What to verify:** cancelling returns the same session to input-ready state without moving the workflow step.

- **Scenario:** a phone user enables and saves the setting by touch, reloads it, cancels a delayed turn, and observes the same destination step.
  **File:** `apps/web/e2e/tests/workflow/mobile-workflow-cancel-completion.spec.ts`.
  **What to verify:** the label's 44px touch target, touch-reachable setting and cancel controls, persisted checked state, workflow transition, no document horizontal overflow, and the existing single scroll owner.

---

## Verification Results

- Backend persistence/configuration packages passed their focused `go test -tags fts5` suites; the cancellation-focused orchestrator suite passed 30 tests, including injected session-state/turn-close failures and terminal-state ordering.
- The broad `make -C apps/backend test` run reached all packages but stopped on the pre-existing `internal/gateway/websocket/TestTaskEventBroadcaster_NoDuplicateSubscriptions` expectation (62 subscriptions vs the test's 61); no gateway files are part of this change, and the feature's relevant backend packages remain green.
- Frontend focused tests passed (29 workflow API/settings action/component tests, including create-step forwarding), web typecheck and lint passed, and both i18n ratchet/check commands passed.
- Public documentation validation passed (58 tests and 41 published pages).
- Managed Playwright coverage passed: desktop enabled/disabled scenarios 2/2 and a clean follow-up mobile run 1/1; the mobile assertion measures the associated label's 44px target.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-step-contract-and-template](task-01-step-contract-and-template.md)

Wave 2 (parallel candidates after Task 01; user authorization required):

- [x] [task-02-configuration-surfaces](task-02-configuration-surfaces.md)
- [x] [task-03-cancellation-routing](task-03-cancellation-routing.md)

Wave 3 (parallel candidates after their dependencies; user authorization required):

- [x] [task-04-settings-ui](task-04-settings-ui.md)
- [x] [task-05-public-docs](task-05-public-docs.md)

Wave 4:

- [x] [task-06-e2e-coverage](task-06-e2e-coverage.md)

The default execution order is sequential in the primary conversation. Wave labels do not authorize subagents.

---

## Risks

- The cancel path and late `agent.ready` handling share a per-session guard. Completion evaluation must happen after cancellation owns the turn while preserving the stale-event rejection that prevents double transitions.
- `on_turn_complete` can include side effects and a destination can auto-start. The setting must be described as running completion actions, not merely moving a card.
- Template steps are instantiated through several paths: new workspace bootstrap, default empty-workflow seeding, backend template service, frontend draft creation, import, and sync. Missing one path would make the advertised standard Kanban default inconsistent.
- `auto_advance_requires_signal` and the clarification hard barrier are separate gates. Only the former is bypassed for configured user cancellation.
- The standard template change affects new E2E seed workflows and may change the workflow step reached by existing cancel/recovery tests even though their same-session and parked-queue assertions remain valid.

## Open Questions

None. The user-approved direction is the per-step boolean, enabled in the standard Kanban template for new workflows without backfilling existing rows.
