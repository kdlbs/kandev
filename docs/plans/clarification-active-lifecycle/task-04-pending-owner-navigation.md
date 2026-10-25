---
id: "04-pending-owner-navigation"
title: "Pending-owner navigation"
status: completed
wave: 4
depends_on: ["03-summary-pending-convergence"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/clarification-active-lifecycle.md"
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
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- lib/utils/pending-clarification.test.ts components/task/task-select-helpers.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
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

Sequential. Desktop and phone must share one resolver and the server projection established by Task 03.

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

- Clarification discovery now follows the newest durable turn, keeps earlier active bundles visible
  when a newer bundle is terminal, remains deletion-proof, and gates unavailable turn history on the
  compact session pending action while retaining legacy no-turn behavior.
- PR review follow-up keeps superseded pending-metadata rows as inert transcript history when turns
  are unhydrated and the server reports no active clarification, while hiding only the authoritative
  latest bundle when clarification remains active. The explicit no-turn legacy overlay path is pinned.
- Mixed-status recovery counts terminal siblings for arrival completeness but renders only pending
  questions, so already-answered cards never request replacement answers.
- The all-unloaded boot state hides pending overlays until turn or compact session authority arrives.
- Desktop navigation re-reads the active source session after asynchronous loading, and mobile item
  projection reuses the shared task pending-action authority helper.
- Closing the mobile task sheet invalidates any asynchronous selection before a later reopen, preventing
  an old session load from navigating or closing the new sheet instance.
- Added one shared pending-owner resolver. Desktop waits for session loading when a task advertises
  input, including non-task routes; phone selection waits, navigates to the owning session, then
  closes the drawer, with safe load-failure fallback.
- Final review remediation forces a no-cache session request for pending selections and rejects a
  failed forced refresh instead of using a stale owner. Desktop and phone capture the click-time
  summary revision/action and discard delayed session results after either value changes.
- Later review remediation generation-guards every per-task session request that reaches the network,
  preventing an older forced response from overwriting a newer snapshot. Sessionless pending fallback
  also releases the outgoing layout before clearing the active session on desktop; the shared loader
  preserves the same freshness rule for phone selection.
- Focused races cover a newer summary arriving before the delayed HTTP continuation on both desktop
  and phone; shared session-loader tests cover cached success and failed forced refresh.
- Rejected forced loads now perform the same click-time summary revision/action revalidation as
  successful loads. Primary-session and sessionless race regressions passed in the 14-test focused suite.
- Bot-review remediation keeps the stale owner-session result discarded but opens the requested task
  through the task-only fallback. Desktop releases the outgoing layout; mobile activates the task,
  navigates, and closes the existing inset drawer. Three focused suites passed 55 tests with zero lint
  warnings, typecheck, and the i18n ratchet.
- Final Codex review leaves selection inert when the task projection itself disappears during an
  authoritative load, while retaining the task-only fallback when a still-present task changes pending
  owner. Desktop and mobile race regressions passed in the 57-test focused run, followed by zero-warning
  focused lint, typecheck, and the i18n ratchet.
- Claude review confirms the durable-turn resolver's existing load-state, start-time, ID, and nanosecond
  ordering coverage; a focused created-time tie-break case closes the remaining comparison branch.
- Final Codex review reconciles the durable wording with implemented behavior: a changed owner opens the
  task-only fallback, while a disappeared task projection alone leaves selection inert.
- Latest Codex review ignores `AbortError` from a superseded shared forced load on desktop and phone,
  preventing the losing continuation from clearing the winning session. Desktop now captures whether a
  legacy pending task had a store projection at click time and remains inert if that projection is
  deleted. This is shared state-only behavior with no layout, navigation surface, touch, or breakpoint
  change, so focused desktop/mobile unit regressions satisfy mobile parity without new Playwright scope.
- Four focused task-selection suites passed 61 tests; full web lint, typecheck, and the i18n ratchet
  passed.
- Six shared desktop/mobile selection and removal suites passed 75 tests; web typecheck, zero-warning
  full lint, and the i18n ratchet passed.
- Final Claude review replaces the module-global phone selection sequence with a controller owned by
  each mounted task sheet. Independent-controller and lifecycle regressions passed in the 42-test
  focused selection run; this remains state-only mobile parity work with no layout or touch change.
- Latest Codex review corrects the last stale-summary scenario to match the implemented phone behavior:
  discard the obsolete owner, open the selected task-only route, and close the sheet.
- Follow-up Codex review invalidates each mounted sheet's controller during unmount, preventing a
  deferred phone load from selecting a task or closing the replacement tablet sheet. The 48-test
  focused selection run, typecheck, zero-warning full lint, and i18n ratchet passed.
- `cd apps && pnpm install --frozen-lockfile` passed.
- The exact three-file Vitest command passed: 3 files, 68 tests.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm run i18n:check` passed; existing real-locale parity warnings remain advisory.
- Follow-up Vitest passed 79 tests across processed-message and pending-clarification suites; web lint,
  typecheck, i18n check, and i18n ratchet passed.
- Delayed Claude review documents the three-state `currentTurnId` scope contract and explicitly calls
  out that an empty scope disables detection, preventing future callers from silently misusing it.
