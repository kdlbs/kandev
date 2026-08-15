---
id: "04-pending-owner-navigation"
title: "Pending-owner navigation"
status: completed
wave: 4
depends_on: ["03-summary-pending-convergence"]
plan: "plan.md"
spec: "../../specs/clarification-active-lifecycle/spec.md"
---

# Task 04: Pending-owner navigation

## Acceptance

- Loaded-message clarification discovery uses the newest durable turn from existing turn state,
  survives deletion of all newer-turn messages, and preserves legacy behavior when no turn record
  exists.
- Desktop task activation loads sessions and chooses the newest input-capable session matching the
  task's pending action before remembered/primary preference, including activation from a non-task
  route; clean-task behavior and async selection guards stay unchanged.
- Phone task-drawer activation uses the same resolver, closes only after navigation state is applied,
  and preserves the existing failure fallback and inset-drawer interaction.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- lib/utils/pending-clarification.test.ts components/task/task-select-helpers.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
```

## Files likely touched

- `apps/web/lib/utils/pending-clarification.ts`
- `apps/web/lib/utils/pending-clarification.test.ts`
- `apps/web/components/task/task-select-helpers.ts`
- `apps/web/components/task/task-select-helpers.test.ts`
- `apps/web/components/task/task-session-sidebar.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.test.ts`

## Dependencies

Task 03.

## Parallelism

Sequential. Desktop and phone must share one resolver and the server projection established by Task
03.

## Inputs

- Spec current-turn transcript and pending-owner scenarios.
- Mobile design contract in `plan.md`.
- Existing `resolvePreferredSessionId`, `resolveLoadedSessionId`, selection-token guard, and
  `TaskSession.pending_action` API type.
- Existing phone inset task drawer and `.tap()` E2E conventions.

## Risks

- Summary presence is authoritative: do not fall back to a stale legacy task pending action when a
  present summary explicitly omits it.
- Filter pending owners to input-capable session states and matching action type.
- Preserve newest-first API ordering; do not sort by localized labels or client clocks.
- Do not close the phone drawer before a failed session load has applied its safe task-level fallback.
- No new user-facing copy; if implementation unexpectedly adds any, route it through i18n.

## Output contract

Use TDD: add current-turn and pending-owner cases, observe RED, implement shared logic, then run every
exact command. Report test counts/results, desktop/phone behavior, blockers/risks, actual files, and
update task/plan status.

## Results

- Clarification discovery now follows the newest durable turn, treats the newest bundle's terminal
  state as authoritative, remains deletion-proof, and gates unavailable turn history on the compact
  session pending action while retaining legacy no-turn behavior.
- Added one shared pending-owner resolver. Desktop waits for session loading when a task advertises
  input, including non-task routes; phone selection waits, navigates to the owning session, then
  closes the drawer, with safe load-failure fallback.
- `cd apps && pnpm install --frozen-lockfile` passed.
- The exact three-file Vitest command passed: 3 files, 68 tests.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm run i18n:check` passed; existing real-locale parity warnings remain advisory.
