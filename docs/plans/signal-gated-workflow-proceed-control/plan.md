---
created: 2026-09-10
status: completed
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002
system_design:
  - ../../specs/tasks/system-design/workflow-signal-gated-manual-move-visibility.md
legacy_specs: []
---

# Implementation Plan: Signal-Gated Workflow Proceed Control

## Overview

Preserve `auto_advance_requires_signal` in every workflow-step projection used
by the task UI. Then refine the shared next-step derivation so a signal-gated
`on_turn_complete` `move_to_next` action remains available while ungated moves
and gated moves to another configured destination suppress the adjacent-step
composer action. Standard chat and passthrough composers retain their existing
busy-state gate and manual task-move behavior, and suppress the action while a
clarification barrier is pending.

## Scope

### In scope

- Carry `auto_advance_requires_signal` from HTTP and WebSocket workflow-step
  payloads into active and cached Kanban state.
- Show the existing next-step composer action after an idle signal-gated
  `move_to_next` turn.
- Continue suppressing the action for ungated turn-complete moves and for
  signal-gated `move_to_previous` or `move_to_step` actions whose destinations
  the adjacent-step control cannot represent.
- Hide the action in both composer surfaces while the current session has a
  pending clarification, including `WAITING_FOR_INPUT`, and reevaluate after
  the barrier clears.
- Preserve the existing busy-state guard, click behavior, error handling, and
  mobile task-drawer path.
- Add focused unit and production-build browser regression coverage.

### Out of scope

- Creating the ADR 0015 `manual_fallback` completion signal.
- Changing backend transition, clarification, cancellation, or
  signal-persistence semantics.
- Adding new copy, controls, layout, card styling, or telemetry.
- Changing which workflow-step action types count as move actions.

## Technical approach

### Preserve the signal-gated field

- Add `auto_advance_requires_signal?: boolean` to the step shape in
  `KanbanState`.
- Copy the value in SSR snapshot mapping, multi-workflow snapshot refresh,
  mobile workspace-switch mapping, workflow-step WebSocket mapping, and the
  live Kanban update projection.
- Add mapper tests so initial load, cached refresh, workspace switch, and live
  update cannot silently discard the field again.

### Refine next-step visibility

- Keep the existing detection of `move_to_next`, `move_to_previous`, and
  `move_to_step` in current-step `on_turn_complete` actions.
- Treat that configuration as an automatic transition for composer suppression
  when the step is ungated, or when the configured action is not `move_to_next`.
  Only a signal-gated `move_to_next` action is an exception.
- Preserve the Go boot-state mapper field so direct task-page hydration has the
  same signal-gated policy input as subsequent client refreshes.
- Use one shared proceed-eligibility policy in `ChatStatusBar` and
  `PassthroughToolbar` so the busy-state and pending-clarification gates cannot
  drift between surfaces.
- Leave `proceed()` unchanged so the action continues to use the normal task
  move API and existing error handling.

## Tests

- Hook tests prove a signal-gated `move_to_next` exposes the next step, an
  ungated move remains suppressed, gated `move_to_previous` and `move_to_step`
  remain suppressed, and an omitted flag keeps the legacy ungated behavior.
- Mapper tests prove the field survives initial hydration, multi-workflow
  refresh, mobile workspace switching, workflow-step WebSocket updates, and
  live Kanban updates. A Go boot-state mapper test covers direct task-page
  hydration.
- Shared composer eligibility tests prove a pending clarification suppresses
  the action while `WAITING_FOR_INPUT`, and passthrough rendering remains hidden
  until that barrier is absent.

## E2E test

Extend `apps/web/e2e/tests/workflow/workflow-step-proceed.spec.ts` with a workflow
whose first step has an `on_turn_complete` `move_to_next` action and
`auto_advance_requires_signal=true`. Let the mock agent finish without a signal,
assert that the task remains on the step, assert the next-step composer action is
visible, select it, wait for the task API to report the target workflow step, and
then assert the stepper reflects the normal move. Extend the E2E API client update
type to seed the existing workflow-step field.

The existing mobile task-drawer move test remains the parity check for the
phone-specific path. No new responsive markup is introduced.

The pending-clarification correction is covered at the shared eligibility and
passthrough rendering boundaries because the clarification overlay lifecycle is
already owned by the session panel state. The desktop browser scenario remains
the end-to-end proof for the eligible signal-gated move path.

## Work orders

- [completed] [Task 01: Preserve signal-gated proceed visibility](task-01-preserve-signal-gated-proceed-visibility.md)

## Verification

Run the focused boot-state mapper test from `apps/backend`:

```bash
go test ./internal/backendapp
```

Run focused tests from `apps/web`:

```bash
pnpm exec vitest run components/task/chat/chat-input-area.test.ts components/task/passthrough-toolbar.test.tsx
pnpm exec vitest run hooks/domains/kanban/use-plan-actions.test.ts lib/ssr/mapper.test.ts lib/ws/handlers/workflows.test.ts lib/ws/handlers/kanban.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-all-workflow-snapshots.signal-gated.test.ts components/task/mobile/session-task-switcher-sheet-helpers.test.ts
pnpm run typecheck
pnpm exec eslint hooks/domains/kanban/use-plan-actions.ts hooks/domains/kanban/use-plan-actions.test.ts lib/state/slices/kanban/types.ts lib/ssr/mapper.ts lib/ssr/mapper.test.ts lib/ws/handlers/workflows.ts lib/ws/handlers/workflows.test.ts lib/ws/handlers/kanban.ts lib/ws/handlers/kanban.test.ts hooks/domains/kanban/use-all-workflow-snapshots.ts hooks/domains/kanban/use-all-workflow-snapshots.signal-gated.test.ts components/task/mobile/session-task-switcher-sheet-helpers.ts components/task/mobile/session-task-switcher-sheet-helpers.test.ts e2e/helpers/api-client.ts e2e/tests/workflow/workflow-step-proceed.spec.ts
pnpm e2e:run tests/workflow/workflow-step-proceed.spec.ts -- --grep "shows next step action for an idle signal-gated transition" --retries=0
pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts -- --grep "moves a task to another step from the mobile task drawer" --retries=0
```

Run specification checks from the repository root:

```bash
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Risks

- Preserving the field in only one hydration path would make the control appear
  or disappear after reload, workflow refresh, or a live settings update.
- Treating every gated step as manually movable would expose the action without
  a configured adjacent destination; the derivation must still require an
  applicable `on_turn_complete` `move_to_next` action and a next step. Gated
  `move_to_previous` and `move_to_step` actions remain suppressed because the
  existing control submits the adjacent next step.
- Removing the busy-state guard could race a manual move against an active turn;
  the presentation components keep that guard.
- Omitting the server boot mapper field would make direct task-page hydration
  disagree with later workflow refreshes; the boot mapper regression test covers
  this path.
- Conflating this action with ADR 0015's signal-writing fallback would silently
  broaden backend semantics and telemetry. This plan deliberately reuses the
  existing manual move only.
