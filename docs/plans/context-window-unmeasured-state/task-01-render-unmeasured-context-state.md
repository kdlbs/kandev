---
id: "01-render-unmeasured-context-state"
title: "Render the unmeasured context state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/context-window-unmeasured-state.md"
---

# Task 01: Render the unmeasured context state

## Intent

Update the existing context-window hover to distinguish a reliable window with used === 0 from
a measured non-zero usage sample, using translated pending copy and preserving the current
interaction and measured-data behavior.

## Acceptance

- A reliable zero-usage entry keeps the context trigger visible, announces usage not measured,
  renders em-dash usage values, and never presents numeric 0% or 0 of ... usage.
- The pending tooltip retains the current source and compaction rows plus the existing pinnable
  hover, keyboard, touch, outside-dismissal, and Escape behavior.
- A positive or exactly-full sample still renders the current numeric display, and an impossible
  used > size report remains hidden.
- All new visible and accessible copy is translated in the English and pseudo task catalogs.

## TDD sequence

1. Add the pending component test and run the focused Vitest command to establish the expected RED
   failure against the current numeric-zero rendering.
2. Add the English/pseudo translation keys and implement the smallest conditional rendering
   change in TokenUsageDisplay.
3. Rerun the focused test, typecheck, i18n check, and i18n ratchet; keep existing source,
   compaction, and tooltip-dismissal cases green.

## Files likely touched

- apps/web/components/task/chat/token-usage-display.tsx
- apps/web/components/task/chat/token-usage-display.test.tsx
- apps/web/src/locales/en/task.json
- apps/web/src/locales/pseudo/task.json

## Dependencies

None.

## Parallelism

sequential. The component test, translation keys, and pending render branch define the contract
consumed by the browser coverage task.

## Verification

- cd apps && rtk pnpm --filter @kandev/web test -- --run components/task/chat/token-usage-display.test.tsx
- cd apps/web && rtk pnpm run typecheck
- cd apps && rtk pnpm --filter @kandev/web run i18n:check
- cd apps && rtk pnpm --filter @kandev/web run i18n:ratchet

## Inputs

- Spec What, pending-state scenarios, and Out of scope.
- Existing reliability guard, pinnable tooltip, source row, and compaction row in
  apps/web/components/task/chat/token-usage-display.tsx.
- Frontend i18n and mobile rules in apps/web/AGENTS.md.

## Output contract

Report the RED assertion, exact files changed, translation keys, focused test count, typecheck and
i18n results, mobile-contract confirmation, remaining risks, and synchronized task/plan/spec
status.

## Results

Implemented. The RED assertion was `getByRole("button", { name: "Context window: usage not measured" })`
failing against the then-current "Context window: 0% used" trigger.

Files changed:

- `apps/web/components/task/chat/token-usage-display.tsx` — reliable `used === 0` entries now render a
  pending state: translated accessible label on the trigger, em-dash "—%" and "— of {{size}} tokens"
  values, a translated pending explanation row, with the ring, source row, compaction row, and
  pinnable tooltip behavior unchanged. Positive, exact-full, and impossible-report branches are
  behaviorally identical.
- `apps/web/components/task/chat/token-usage-display.test.tsx` — new pending-state case asserting the
  accessible label, em-dash values, pending copy, retained source/compaction rows, and the absence of
  numeric 0%/0-of text; existing positive, source, compaction, and dismissal cases retained.
- `apps/web/src/locales/en/task.json` — keys `contextWindowNotMeasured`,
  `contextWindowUsagePendingPercent`, `contextWindowUsagePendingTokens`,
  `contextWindowUsagePendingExplain`.
- `apps/web/src/locales/pseudo/task.json` — regenerated via `pnpm run i18n:pseudo`.

Verification:

- Focused Vitest (`components/task/chat/token-usage-display.test.tsx`) — 12 tests passed.
- `pnpm --filter @kandev/web run typecheck` — passed.
- `pnpm --filter @kandev/web run i18n:check` — keys OK, pseudo in sync.
- `pnpm --filter @kandev/web run i18n:ratchet` — new-code ratchet clean, guard allowlist intact.
- ESLint on the two changed component files — zero warnings.

Mobile contract: no new drawer, route, breakpoint branch, or scroll owner; the ring remains the
tap-pinnable entry point, verified in task 02's mobile E2E. Remaining risk: a provider reporting a
legitimate zero-token sample displays the pending state until the first positive sample, which is
the intended product trade-off recorded in the plan.

