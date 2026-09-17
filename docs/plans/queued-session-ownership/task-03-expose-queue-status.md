---
id: "03-expose-queue-status"
title: "Expose queue status across task surfaces"
status: in_progress
wave: 3
depends_on:
  - "02-preserve-queued-entry"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-003
acceptance_criteria:
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.1
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.2
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.3
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.4
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.5
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.6
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.7
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
---

# Task 03: Expose queue status across task surfaces

## Summary

Deliver the bounded queue summary and its desktop/phone presentation as one
vertical slice. Users can inspect either conversation while seeing which
session is queued, why it waits, and that retry is automatic.

## In scope

- `launch_queue` summary model, validation, source loading, rebuild, live
  publication, revision/invalidation, and explicit removal.
- Capacity sampling and freshness without semantic task-activity churn.
- Shared queue view model, sidebar label, task-level status region, selected
  session parking note, and suppression of duplicate queued-session launch prompts.
- Full desktop/mobile lifecycle, reload, disconnect, unknown-capacity,
  long-name, prompt-warning, question/error coexistence, and dispatch coverage.
- English, Portuguese, Simplified Chinese, and generated Traditional Chinese
  copy. Public workflow explanation and applicable source/API documentation.

## Out of scope

No global queue page, ETA, ordinal queue position, bypass control, new drawer,
or modification of historical status messages. Do not count launches as queued prompts.

## Acceptance

1. Task list, boot, detail, and WS paths share the revisioned queue projection;
   stale updates cannot resurrect cleared work. Restart restores queued identity,
   missing capacity remains unknown, and refresh does not change activity ordering.
2. UI-01 through UI-03 are realized with localized readable status, retained
   errors/questions, and independent task queue versus selected-session state.
3. Desktop and phone tests prove passive inspection leaves Astra untouched and
   capacity release starts only Luna once, with queue status clearing automatically.

## ASCII UI preview

Excerpt of [the full plan previews](plan.md#ascii-ui-preview), requirement 003:

```text
UI-01 Desktop
Sidebar: [clock] Investigate issue  Queued
Detail:  Queued: Luna. Waiting for session capacity
         5 of 5. Checked just now. Queued since 21:15
         Starts automatically when capacity is available.
         [Astra] [Luna: Queued] [Plan]
         Parked for workflow. Send a message to continue here.

UI-02 Phone
Task drawer row: [clock] Investigate issue  Queued
Task detail:
  Queued: Luna
  Waiting for session capacity
  5 of 5. Checked just now. Queued since 21:15
  Starts automatically.
  [Astra                         v]
  Parked for workflow. Send a message to continue here.
  Conversation ...

UI-03 Shared outcomes
  Disconnected -> Queued, reconnecting, last checked time
  No count     -> Capacity unavailable
  Replay error -> Retry pending plus existing error details
  Dispatched   -> Queue region removed; normal Starting/Running
```

Queue region is outside chat scroll, regardless of selected conversation. Use
the existing mobile drawer/picker and dedicated layout, with one chat scroll
owner and safe-area clearance. Names wrap in detail and truncate in navigation.
No required hover disclosure. New action targets meet 44 px only on touch/mobile.
These structural requirements are fixed; spacing and wording are illustrative.

## Verification

```bash
(cd apps/backend && go test ./internal/task/statussummary ./internal/task/dto -count=1)
(cd apps/backend && go test ./internal/task/service -run 'Test.*StatusSummary|Test.*LaunchQueue' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'TestQueuedSession|TestQueuedEntry|Test.*Ceiling.*Surface|Test.*LaunchQueue' -count=1)
(cd apps/web && pnpm exec vitest run components/task/launch-queue-status.test.tsx lib/tasks/launch-queue-view-model.test.ts lib/ws/handlers/tasks-status-summary.test.ts hooks/domains/session/use-session-resumption.test.ts)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
make -C apps/backend build
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/queued-session-ownership.spec.ts tests/workflow/workflow-session-targeting.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-queued-session-ownership.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Create the named new component/view-model tests during RED. Run any additional
modified test suite explicitly and record its command. The managed E2E runner
builds fresh artifacts and owns cleanup; do not overlap desktop/mobile runs or
use all-worker overrides. Compare rendered phone geometry and labels with UI-02.

## Files likely touched

- `apps/backend/internal/task/statussummary/{model,rebuild,projector,projector_events}.go`
  and new queue projection tests.
- `apps/backend/internal/task/service/service_status_summary_rebuild.go`, task
  DTO enrichment, boot/list wiring, and the existing summary composition provider.
- `apps/backend/internal/orchestrator/ceiling_defer.go`, `ceiling_replay.go`,
  `ceiling_sweep.go`, `ceiling_surface.go`, and tests.
- `apps/web/lib/types/task-status-summary.ts`, `http.ts`, `backend.ts`.
- `apps/web/lib/ws/handlers/task-status-summary.ts`, task hydration/merge paths.
- `apps/web/lib/tasks/launch-queue-view-model.ts` (new) and tests.
- `apps/web/components/task/launch-queue-status.tsx` (new), `task-item.tsx`,
  `task-switcher.tsx`, `task-chat-panel.tsx`, desktop task layout placement.
- `apps/web/components/task/mobile/session-mobile-layout.tsx`,
  `session-task-switcher-sheet.tsx`, and session-picker presentation if needed.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}` catalogs.
- Workflow E2E specs/helper introduced by this package.
- `docs/public/tasks-and-workflows.md`; inspect `agents-and-profiles.md` and
  `configuration.md` for related wording without changing startup defaults.

## Dependencies

Tasks 01 and 02 provide passive eligibility and authoritative queue lifecycle.

## Risks

Summary invalidation must carry clears, and old hydration must not resurrect
queue state. Capacity sampling must not re-sort tasks or expose global session
identities. A mobile status region must not cover the session picker or composer.

## Parallelism

`sequential`

## Inputs

[Design: queue projection and surfaces](../../specs/tasks/system-design/queued-session-ownership.md#queue-projection).
Use existing queued-prompt summary tests as revision/rebuild examples without
merging the two semantics. Read mobile parity, e2e, and docs-maintainer guidance.

## Results

Implementation is present. The revisioned `launch_queue` projection is loaded
from durable task metadata, refreshed with capacity observations, and exposed
through task list, sidebar, detail, desktop chat, and mobile navigation. The
shared view model keeps queue status independent from the selected parked
conversation, and the phone layout has one chat scroll owner.

Passing checks:

- `go test ./internal/task/statussummary ./internal/task/dto -count=1`
- `go test ./internal/task/service -run
  'Test.*StatusSummary|Test.*LaunchQueue' -count=1`
- `go test -race ./internal/orchestrator -run
  'TestQueuedSession|TestQueuedEntry|Test.*Ceiling.*Surface|Test.*LaunchQueue'
  -count=1`
- Focused frontend tests for the queue component, view model, status-summary
  handler, and session recovery passed (93 tests).
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:zh-hant`,
  `pnpm run i18n:check`, and `pnpm run i18n:ratchet`
- `pnpm test` passed (2,140 files, 18,465 tests, 4 skipped).
- `make -C apps/backend build`
- `pnpm e2e:run --project chromium tests/workflow/queued-session-ownership.spec.ts
  tests/workflow/workflow-session-targeting.spec.ts`
- `pnpm e2e:run --project mobile-chrome
  tests/workflow/mobile-queued-session-ownership.spec.ts`
- `node --test scripts/validate-public-docs.test.mjs`,
  `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py
  validate`, `python3 scripts/lint-spec-files.py --all`, and
  `git diff --check`

The rendered desktop and phone scenarios match the preview contract: queued
destination and capacity are visible while the parked source is selected,
queue status is present in task navigation and detail, names wrap on phone,
and releasing the fixture capacity dispatches the destination once. The full
E2E sleep lint audit retains unrelated baseline violations; the scoped preview
and sleep ratchet pass.

The required PostgreSQL projection/rebuild coverage remains outstanding because
`KANDEV_TEST_POSTGRES_DSN` is unset. The work order remains in progress until
the isolated PostgreSQL verification runs. The repository-wide backend test
target also reports unrelated existing environment-sensitive failures; the
task-related backend packages pass.
