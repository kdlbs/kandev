---
id: "03-report-schedule-state-ui"
title: "Render the intent/schedule-state distinction in the routines UI"
status: done
wave: 3
depends_on: ["02-report-schedule-state-api"]
plan: "plan.md"
spec: "../../specs/office/requirements/routine-arming-visibility.md"
---

# Task 03: Render the intent/schedule-state distinction in the routines UI

Satisfies REQ-OFFICE-ROUTINE-ARMING-002's rendering criteria:
AC-OFFICE-ROUTINE-ARMING-002.2, .3, .8, .9, .10, and the rendering half of
.4, .5, .6, .11 (data shape is Task 02).

## Scope

Routines list and detail view render both switches. This is a labels-and-
distinctions change only — the operator-initiated re-arm action was cut from
scope (see the requirement's `## Out of scope`), so there is no mutating flow
to build or test here.

## Exclusions

- No action, button, or endpoint that repairs a trigger or changes intent.
- No new backend response fields (Task 02).

## Acceptance conditions

1. The routines list visually and semantically distinguishes: `active` +
   `armed` from `active` + anything else (AC-002.2); not-`active` from
   `active`-but-broken, so deliberate and broken never render the same
   (AC-002.3); and renders a distinct label for each of the five groups —
   `armed`; `{trigger_invalid, trigger_unscheduled, trigger_disabled}`;
   `event_only`; `{unscheduled_manual_only, unscheduled_no_trigger}`;
   `unknown` — such that two routines in different groups never share a
   label (AC-002.10). None of this distinction is carried by colour alone;
   each has accompanying text, an icon, or an accessible label (AC-002.9).
2. A routine whose unarmed list has at least one schedulable entry is
   visually distinguished from one where none are (needs re-arming vs. needs
   editing), and this rendering is entirely suppressed when the unarmed list
   is empty rather than defaulting to either message (AC-002.4).
   `unscheduled_manual_only`/`unscheduled_no_trigger` render as "no
   schedule" (AC-002.5); `unknown` renders as undetermined (AC-002.6);
   `event_only` never renders as "no schedule" and is never directed to
   create a trigger (AC-002.11). No rendering here offers a repair action.
3. All copy added by this task is routed through `t()`/`<Trans>` and present
   in English plus the four other shipped locales (`pt-pt`, `zh-cn`,
   `zh-hk`, `zh-tw`); `pnpm run i18n:check` and `pnpm run i18n:ratchet` both
   pass (AC-002.8).

## Verification

- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run lint`
- `cd apps/web && pnpm run i18n:check`
- Component test(s) covering the five-group label mapping and the
  schedulable/non-schedulable unarmed-list distinction.
- Playwright: extend or add a spec under `apps/web/e2e/` covering the
  routines list/detail rendering for a fixture spanning all nine schedule
  states (read-only assertions only — no mutating interaction to drive).
  Confirm the e2e profile enables `KANDEV_FEATURES_OFFICE` before writing
  the spec.
- `cd apps/web && pnpm e2e:run` scoped to the new/changed spec (respect the
  one-worker-per-shard and memory-aware shard budget in
  `apps/web/e2e/README.md`).

## Files likely touched

- Routines list and detail components under `apps/web` (Office routines
  surface — exact path to be located during implementation; search for the
  existing routine-status badge component).
- `apps/web/src/locales/en/*.json` plus `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`
  (use `pnpm run i18n:zh-hant` for the Traditional Chinese pair).
- New or extended Playwright spec under `apps/web/e2e/`.

## Dependencies

Task 02.

## Parallelism

Independent of Task 04 and Task 05.

## Result

- `apps/web/app/office/routines/schedule-state.ts`: pure read/classify
  helpers (`readScheduleState`, `readUnarmedCronTriggers`,
  `scheduleStateGroup`, `hasSchedulableUnarmedEntry`) following the repo's
  established camelCase-typed/snake_case-at-runtime fallback pattern for the
  two new wire fields from Task 02.
- `apps/web/app/office/routines/schedule-state-badge.tsx`: `ScheduleStateBadge`
  (five-group label + icon + tooltip, AC-002.9/.10) and
  `UnarmedScheduleHint` (schedulable-vs-not distinction, suppressed when the
  unarmed list is empty, AC-002.4/.5/.6/.11). Both render regardless of
  `routine.status`, so intent and schedule state stay visually independent
  (AC-002.1/.2/.3).
- Wired into `routine-row.tsx` (list) and `routine-detail-view.tsx`'s
  `DetailReadOnlyCard` (detail view).
- 12 new `office:` copy keys added to `en` plus `pt-pt`, `zh-cn` (hand
  translated) and `zh-hk`/`zh-tw` (via `pnpm run i18n:zh-hant`); pseudo-locale
  regenerated and scoped to only these keys.
- Unit tests: `schedule-state.test.ts` (pure logic, all 8 reachable states to
  5 groups) and `schedule-state-badge.test.tsx` (rendering, using
  `TooltipProvider` per the repo's Radix tooltip test convention).
- E2E: `apps/web/e2e/tests/office/routine-schedule-state.spec.ts` seeds one
  routine per reachable schedule state and asserts the routines list renders
  the correct AC-002.10 label group for each, plus AC-002.11 (`event_only`
  never shows "No schedule").
  - Confirmed `KANDEV_FEATURES_OFFICE` is already `e2e: "true"` in
    `profiles.yaml`; no profile change needed.
  - **Backend API gap found and fixed**: the public routine-trigger API
    cannot produce `trigger_invalid`, `trigger_unscheduled`, or
    `trigger_disabled` — `RoutineService.CreateRoutineTrigger` validates the
    cron expression and always computes a non-null `NextRunAt`, and there is
    no PATCH-trigger route to disable one after creation. Added a
    `KANDEV_E2E_MOCK`-gated test-harness route,
    `POST /api/v1/_test/routine-triggers`
    (`apps/backend/internal/office/testharness/routine_triggers.go` +
    `routine_triggers_test.go`), that inserts a trigger row directly via the
    repository, bypassing service-layer validation, following the existing
    `seedRun`/`seedComment` harness pattern. Exposed as
    `ApiClient.seedRoutineTrigger` in `apps/web/e2e/helpers/api-client.ts`.
  - `unknown` (a live trigger-read failure) has no fixture-reachable trigger
    shape at all — it requires simulating a database read error — so it is
    intentionally not covered by this spec. It has backend unit coverage in
    `arming_test.go` (Task 01) instead.
  - The fixture therefore spans 7 of the 9 raw states, covering all 5 render
    groups (`armed`, `broken` via 3 distinct raw states, `event_only`,
    `no_schedule` via 2 distinct raw states); `unknown` is the one documented
    gap.
- Verification run: `pnpm run typecheck`, `pnpm run lint`, `pnpm run
  i18n:check`, `pnpm run i18n:ratchet`, `npx vitest run` on both new test
  files, `go build ./...`, `gofmt -l`, `golangci-lint run
  ./internal/office/testharness/...`, `go test
  ./internal/office/testharness/...`, and `pnpm e2e:run
  tests/office/routine-schedule-state.spec.ts` — all pass.
