---
id: "01-centered-save-surface"
title: "Centered save surface and reset"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-manual-save.md"
---

# Task 01: Centered save surface and reset

Implement the coordinator-facing Reset action and the responsive centered save
surface. Keep all persistence and navigation semantics in the existing settings
save coordinator; this task changes the presentation and adds no API calls.

## Acceptance

1. A dirty route renders a centered, compact neutral save surface with the
   localized unsaved-state label, Reset, and Save changes; the primary action
   keeps success-green emphasis without turning the whole surface into a large
   green rectangle.
2. Reset invokes all dirty contributor discard callbacks in stable order, makes
   no persistence request, hides the surface after successful reset, and keeps
   the surface dirty/error state when reset fails or is already in flight.
3. The standalone surface is centered in the settings content pane with a
   roughly 20px desktop bottom inset, remains safe-area aware, and lifts above
   the phone Configuration Chat FAB; the hosted surface is centered above the
   open popover without changing navigation guard, partial-save, retry, or
   revision-safe behavior.

## Verification

- If workspace dependencies are absent: `cd apps && pnpm install --frozen-lockfile`.
- `cd apps && pnpm --filter @kandev/web test -- --run components/settings/settings-save-provider.test.tsx components/settings/settings-layout-client.test.tsx`
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint`
- `git diff --check`

## Files likely touched

- `apps/web/components/settings/settings-save-provider.tsx`
- `apps/web/components/settings/settings-save-provider.test.tsx`
- `apps/web/components/settings/settings-floating-save.tsx`
- `apps/web/components/config-chat/config-chat-panel.tsx`
- `apps/web/components/settings/settings-layout-client.tsx`

## Dependencies

None.

## Parallelism

Sequential. The coordinator, surface, and portal host share the same behavior
contract and must be validated together.

## Inputs

- `docs/specs/ui/requirements/settings-manual-save.md` — centered surface, Reset, and state
  semantics.
- `docs/decisions/0046-settings-route-save-coordinator.md` — route-scoped
  contributor ownership and collision-host boundary.
- `docs/plans/settings-save-action-redesign/plan.md` — Frontend, mobile parity,
  and Tests sections.
- `apps/web/components/kanban/mobile-fab.tsx` — safe-area fixed-action pattern.

## Output contract

Report the implementation summary, exact files changed, focused test/typecheck/
lint results, `git diff --check`, any blockers or risks, and synchronized Task
01/plan status in the same conversation. Do not change E2E files in this task.

## Results

- Added a shared coordinator reset path that discards dirty contributors in the
  existing stable order, blocks duplicate reset submissions, preserves dirty
  state on failure, and keeps navigation discard semantics intact.
- Replaced the right-anchored green rectangle with a centered neutral card,
  localized Unsaved changes/Reset copy, a secondary Reset action, and the
  existing green Save changes primary action.
- Centered the portaled surface in the Configuration Chat host, anchored the
  standalone surface to the settings content pane, and reserved shell space
  for the Configuration Chat FAB rather than the old oversized save card.
- `pnpm --filter @kandev/web test -- --run components/settings/settings-save-provider.test.tsx` — 16 passed.
- `pnpm run typecheck` — passed.
- `pnpm run lint` — passed.
- `git diff --check` — passed.
