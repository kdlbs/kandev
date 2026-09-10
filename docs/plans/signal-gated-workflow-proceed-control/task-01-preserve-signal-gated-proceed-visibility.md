---
id: "01-preserve-signal-gated-proceed-visibility"
title: "Preserve signal-gated proceed visibility"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.1
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.2
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.3
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.4
system_design:
  - ../../specs/tasks/system-design/workflow-signal-gated-manual-move-visibility.md
---

# Task 01: Preserve Signal-Gated Proceed Visibility

## Summary

Carry the existing signal-gated workflow-step flag into task UI state and use it
to distinguish a truly automatic `move_to_next` transition from a signal-gated
one. Prove the adjacent-step composer action stays available after the gated
agent becomes idle while unsupported gated destinations remain hidden.

## Inputs

- `docs/specs/tasks/requirements/workflow-explicit-completion-signal.md`
- `docs/specs/tasks/system-design/workflow-signal-gated-manual-move-visibility.md`
- `docs/decisions/0015-explicit-completion-signal-for-auto-advance.md`
- `apps/web/hooks/domains/kanban/use-plan-actions.ts`
- `apps/web/lib/state/slices/kanban/types.ts`
- Existing HTTP, snapshot, workspace-switch, and WebSocket workflow-step
  mappers.

## TDD sequence

1. **RED:** Add hook cases showing that a gated `on_turn_complete` `move_to_next`
   action should expose `proceedStepName` while the equivalent ungated move,
   gated `move_to_previous`, and gated `move_to_step` stay hidden. Add coverage
   that an omitted flag keeps the legacy ungated behavior.
2. **RED:** Add mapping assertions for initial hydration, multi-workflow
   snapshot refresh, mobile workspace switching, and workflow-step WebSocket
   updates. Confirm they fail because the flag is absent from `KanbanState` or
   dropped by each mapper.
3. **RED:** Add the focused Playwright scenario and confirm the next-step
   locator remains hidden after the mock agent returns idle.
4. **GREEN:** Add the optional step field, preserve it through each mapper and
   the Go boot-state mapper, and narrow the automatic-transition suppression
   condition in `useNextWorkflowStep`.
5. **REFACTOR:** Keep the policy in the shared hook. Do not duplicate it in
   standard chat, passthrough, or mobile components.
6. Run the focused unit, type, lint, desktop E2E, and mobile parity commands.

## Acceptance

- An idle signal-gated step with a configured `move_to_next` action exposes the
  existing adjacent-next-step composer action.
- The equivalent ungated step does not expose a redundant action.
- Signal-gated `move_to_previous` and `move_to_step` actions remain hidden
  because the existing composer action submits the adjacent next step.
- A busy agent still hides the action until it becomes idle.
- Initial load, cached snapshot refresh, mobile workspace switching, and live
  workflow-step updates preserve identical behavior.
- Selecting the action uses the existing manual task-move endpoint and advances
  to the configured next step.
- No `step_complete_kandev` signal, `manual_fallback` source, new metric, or new
  user-facing copy is introduced.

## Files likely touched

- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/lib/ssr/mapper.ts`
- `apps/web/lib/ssr/mapper.test.ts`
- `apps/web/lib/ws/handlers/workflows.ts`
- `apps/web/lib/ws/handlers/workflows.test.ts`
- `apps/web/lib/ws/handlers/kanban.ts`
- `apps/web/lib/ws/handlers/kanban.test.ts`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.signal-gated.test.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-helpers.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-helpers.test.ts`
- `apps/web/hooks/domains/kanban/use-plan-actions.ts`
- `apps/web/hooks/domains/kanban/use-plan-actions.test.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/workflow/workflow-step-proceed.spec.ts`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_routes_test.go`

## Verification

```bash
cd apps/backend
go test ./internal/backendapp
cd ../web
pnpm exec vitest run hooks/domains/kanban/use-plan-actions.test.ts lib/ssr/mapper.test.ts lib/ws/handlers/workflows.test.ts lib/ws/handlers/kanban.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-all-workflow-snapshots.signal-gated.test.ts components/task/mobile/session-task-switcher-sheet-helpers.test.ts
pnpm run typecheck
pnpm exec eslint hooks/domains/kanban/use-plan-actions.ts hooks/domains/kanban/use-plan-actions.test.ts lib/state/slices/kanban/types.ts lib/ssr/mapper.ts lib/ssr/mapper.test.ts lib/ws/handlers/workflows.ts lib/ws/handlers/workflows.test.ts lib/ws/handlers/kanban.ts lib/ws/handlers/kanban.test.ts hooks/domains/kanban/use-all-workflow-snapshots.ts hooks/domains/kanban/use-all-workflow-snapshots.signal-gated.test.ts components/task/mobile/session-task-switcher-sheet-helpers.ts components/task/mobile/session-task-switcher-sheet-helpers.test.ts e2e/helpers/api-client.ts e2e/tests/workflow/workflow-step-proceed.spec.ts
pnpm e2e:run tests/workflow/workflow-step-proceed.spec.ts -- --grep "shows next step action for an idle signal-gated transition" --retries=0
pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts -- --grep "moves a task to another step from the mobile task drawer" --retries=0
cd ../..
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Dependencies

None.

## Risks

- Missing one state projection creates load-order-dependent visibility.
- A loose boolean default could turn older payloads into gated steps.
- Changing the display components could accidentally weaken the active-turn
  guard; they should remain consumers of the shared derived value.

## Parallelism

`sequential`

## Output contract

Record the RED failures, changed mapper paths, focused verification results,
and the final desktop and mobile E2E counts. Update this file to `done` and the
plan work-order checkbox to `completed` only after all acceptance criteria pass.

## Implementation evidence

- RED unit run: six focused files failed only on the missing signal-gated field
  and the resulting hidden proceed action. The RED desktop browser run failed
  because `proceed-next-step` was absent after the mock agent became idle.
- Changed projections: SSR hydration, multi-workflow snapshot refresh, mobile
  workspace switching, workflow-step WebSocket updates, and live Kanban update
  payloads now preserve `auto_advance_requires_signal`.
- GREEN unit run: 7 files, 84 tests passed.
- Typecheck passed.
- Focused ESLint passed with no errors; it reported four existing complexity or
  file-size warnings.
- Desktop browser regression passed: 1 test.
- Mobile task-drawer parity regression passed: 1 test.
- Specification tests passed: 30 tests. Full specification lint passed. Diff
  check passed.
- PR fixup remediation: the shared policy now exposes only gated
  `move_to_next`; gated `move_to_previous` and `move_to_step` remain hidden,
  and omitted flags remain ungated. The Go boot-state mapper now preserves the
  field for direct task-page hydration. The browser regression waits for the
  task API to report the target step before asserting the UI.
