---
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
created: 2026-08-20
status: implemented
---

# Implementation Plan: Compact host-probe warning

## Overview

Replace the always-visible host-probe mismatch sentence in the shared agent
profile option renderer with a single warning icon. The profile remains
selectable, and the existing localized sentence remains available through a
focusable and hoverable tooltip on fine pointers or a drawer on coarse
pointers. The change is frontend-only and covers every consumer of
`useAgentProfileOptions`, including task creation, new sessions, subtasks,
Quick Chat, and Office setup.

## Frontend

### Shared profile option renderer

- `apps/web/components/task-create-dialog-options.tsx`
  - Keep `modelProbeNote` derived from the existing host catalog comparison.
  - Render it as one amber `IconAlertTriangle` tooltip/drawer trigger instead
    of a persistent text row.
  - Give the trigger a stable test id, accessible label, focus behavior, and a
    touch-usable hit area. Preserve the existing capability warning and
    terminal-mode indicators.
  - Keep the interactive trigger inside dropdown options, and render a
    noninteractive warning indicator in the selected combobox trigger so the
    shared option renderer never nests a button inside the combobox button.

- `apps/web/components/combobox.tsx`
  - Allow an option to provide a selected-trigger renderer distinct from its
    dropdown renderer.

## Mobile contract

- Desktop and mobile keep the existing `AgentSelector` combobox and option
  list. The warning is supplementary and never blocks profile selection.
- The warning icon is a focusable/hoverable disclosure in the option row on
  fine pointers, and opens the shared coarse-pointer drawer on touch devices.
  The option list remains the single scroll owner and keeps its existing
  viewport containment.
- Reuse the existing Radix tooltip and `useTouchDrawer`/Drawer patterns from
  the shared web UI.

## Tests

### Unit test

- **What:** a host-mismatched profile remains enabled, renders exactly one
  warning trigger, and does not render the advisory sentence as persistent
  option content.
- **File:** `apps/web/components/task-create-dialog-options.test.tsx`
- **How:** extend the existing `useAgentProfileOptions` render probe; assert
  the warning trigger and absence of the always-visible note while retaining
  the current selectability assertions.

### Desktop E2E

- **Scenario:** a mismatched profile in the create-task picker remains enabled;
  the option shows the warning icon; hovering it reveals the localized host
  probe message.
- **File:** `apps/web/e2e/tests/settings/no-silent-model-fallback.spec.ts`
- **What to verify:** the note is not persistent option text, the tooltip is
  visible after hover, and the profile remains enabled.

### Mobile E2E

- **Scenario:** the same mismatched profile is reachable in the mobile create
  task picker; tapping the warning icon opens the advisory drawer.
- **File:** `apps/web/e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts`
- **What to verify:** the icon is reachable by touch, the drawer is visible,
  the profile remains enabled, and the document has no horizontal overflow.

## Verification Results

- `cd apps && pnpm install --frozen-lockfile` — passed.
- `cd apps/web && pnpm vitest run components/task-create-dialog-options.test.tsx` — passed, 17 tests.
- `cd apps/web && pnpm exec eslint components/combobox.tsx components/task-create-dialog-options.tsx components/task-create-dialog-options.test.tsx e2e/tests/settings/no-silent-model-fallback.spec.ts e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts` — passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm run i18n:ratchet` — passed, zero new or modified violations.
- `cd apps/web && pnpm e2e:run --project chromium tests/settings/no-silent-model-fallback.spec.ts` — passed, 1 test.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-no-silent-model-fallback.spec.ts` — passed, 1 test.
- `git diff --check` — passed.
- Fresh synthetic desktop and mobile screenshots were captured, inspected, and
  compressed for the PR description.
- Review remediation: the selected trigger now uses a noninteractive
  indicator, and coarse-pointer inspection uses a Drawer instead of a tooltip.

## Implementation Waves And Parallel Candidates

One sequential task: the shared renderer, unit test, and existing desktop/mobile
E2E assertions must stay in sync.

- [x] [task-01-shared-warning-tooltip](task-01-shared-warning-tooltip.md)

## Open Questions

None. The existing host-probe message and selectability contract remain
unchanged; only its presentation changes.
