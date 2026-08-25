---
id: "07-anchor-and-control-shared-runs"
title: "Anchor and control shared runs"
status: completed
wave: 3
depends_on:
  - "01-persist-continuation-policy"
  - "02-dispatch-reusable-turns"
  - "05-maintain-shared-run-lifecycle"
  - "06-add-continuation-control"
plan: "plan.md"
spec: "../../specs/office/automation-runs.md"
---

# Task 07: Anchor and Control Shared Runs

## Acceptance

- Run types and views carry exact turn/thread data and per-run `display_title`; shared-session
  selection focuses the chosen turn and both `triggered`/`task_created` appear as Running.
- Stop current run cancels the selected exact run with correct pending/error cleanup and does not
  affect another scheduled or human turn.
- Desktop rail and mobile drawer/content expose the same outcome; the mobile action is visible,
  at least 44 px, contained, and requires no hover or horizontal scrolling.

## TDD scenarios

1. RED: Render two run IDs sharing a session with distinct turns/titles and prove selection changes
   viewport, title, live state, and summary.
2. RED: Cover `triggered` Running grouping/filtering, legacy rows, deep links, replacement metadata,
   and inert skipped rows.
3. RED: Cover stop success, stale/terminal no-op, backend error, duplicate click, and cancellation of
   one exact run while another turn exists.
4. GREEN: Thread run identity through APIs/components and add one shared cancellation mutation.
5. GREEN: Add localized desktop and mobile actions using existing detail/drawer composition.
6. REFACTOR: Keep transcript hydration session-scoped while viewport/control state is run-scoped.

## Verification

- `cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run components/runs lib/api/domains/automation-runs-api.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:check`
- `cd apps/web && pnpm run i18n:ratchet`
- `git diff --check`

## Files likely touched

- `apps/web/lib/types/automation.ts`
- `apps/web/lib/api/domains/automation-api.ts`
- `apps/web/lib/api/domains/automation-runs-api.test.ts`
- `apps/web/components/runs/automation-detail-page.tsx`
- `apps/web/components/runs/automation-detail-page.test.tsx`
- `apps/web/components/runs/run-transcript.tsx`
- `apps/web/components/runs/run-transcript.test.tsx`
- `apps/web/components/runs/runs-rail.tsx`
- `apps/web/components/runs/runs-drawer.tsx`
- `apps/web/src/locales/en/automations.json`
- `apps/web/src/locales/pt-pt/automations.json`
- `apps/web/src/locales/zh-cn/automations.json`
- `apps/web/src/locales/zh-hk/automations.json`
- `apps/web/src/locales/zh-tw/automations.json`
- `apps/web/src/locales/pseudo/automations.json`

## Dependencies

Tasks 01/02 provide exact run and stop contracts, Task 05 provides projection, and Task 06 owns the
shared frontend type/editor baseline.

## Inputs

- Detail behavior and exact-turn model in the automation runs spec.
- Mobile design contract in `plan.md` and current rail/drawer components.

## Parallelism

Sequential after Tasks 05 and 06 because it consumes their API and shared TypeScript contracts.

## Output contract

Report selection/title/filter assertions, cancellation terminal paths, mobile geometry, locales,
files changed, and exact tests.

## Risks

- Session-only React keys or task-only cancellation can display or stop the wrong shared run.

## Results

Implemented exact run/session/turn API rendering, selected-turn transcript filtering, running-status grouping, and local exact-run stop state shared across desktop and mobile layouts. Desktop and mobile E2E passed 24 and 10 tests.
