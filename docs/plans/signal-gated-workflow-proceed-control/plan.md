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
by the task UI. Then refine the shared next-step derivation so only an ungated
`on_turn_complete` move suppresses the composer action. Standard chat and
passthrough composers retain their existing busy-state gate and manual task-move
behavior.

## Scope

### In scope

- Carry `auto_advance_requires_signal` from HTTP and WebSocket workflow-step
  payloads into active and cached Kanban state.
- Show the existing next-step composer action after an idle signal-gated turn.
- Continue suppressing the action for ungated turn-complete moves.
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
  only when `auto_advance_requires_signal !== true`.
- Leave `ChatStatusBar` and `PassthroughToolbar` unchanged; both already hide
  the shared next-step action while the agent is busy.
- Leave `proceed()` unchanged so the action continues to use the normal task
  move API and existing error handling.

## Tests

- Hook tests prove a signal-gated turn-complete move exposes the next step and
  an ungated move remains suppressed.
- Mapper tests prove the field survives initial hydration, multi-workflow
  refresh, mobile workspace switching, workflow-step WebSocket updates, and
  live Kanban updates.
- Existing composer and passthrough tests continue to cover the busy-state
  display gate because their input contract does not change.

## E2E test

Extend `apps/web/e2e/tests/workflow/workflow-step-proceed.spec.ts` with a workflow
whose first step has an `on_turn_complete` move and
`auto_advance_requires_signal=true`. Let the mock agent finish without a signal,
assert that the task remains on the step, assert the next-step composer action is
visible, select it, and assert the normal move succeeds. Extend the E2E API
client update type to seed the existing workflow-step field.

The existing mobile task-drawer move test remains the parity check for the
phone-specific path. No new responsive markup is introduced.

## Work orders

- [completed] [Task 01: Preserve signal-gated proceed visibility](task-01-preserve-signal-gated-proceed-visibility.md)

## Verification

Run focused tests from `apps/web`:

```bash
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
  a configured move; the derivation must still require an applicable
  `on_turn_complete` move and a next step.
- Removing the busy-state guard could race a manual move against an active turn;
  the presentation components keep that guard.
- Conflating this action with ADR 0015's signal-writing fallback would silently
  broaden backend semantics and telemetry. This plan deliberately reuses the
  existing manual move only.
