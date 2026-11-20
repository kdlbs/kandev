---
id: "04-auto-sort-merge-controls"
title: "Auto-sort-merge controls"
status: cancelled
wave: 2
depends_on:
  - "01-persist-auto-sort-merge-policy"
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.1
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.2
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.4
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.5
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.12
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.14
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.15
  - AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.16
system_design:
  - ../../specs/ui/system-design/message-queue-auto-sort-merge.md
---

# Task 04: Auto-sort-merge controls

## Summary

Cancelled: superseded by [Task 06](task-06-fold-into-auto-merge.md), which folds
the sorted behaviour into Auto-merge and removes the separate setting.

Add the dependent global settings switch and the third queue-header pill,
backed by the Task 01 contract. The frontend orders the policy tuple on its
own, so delayed or unavailable statuses cannot regress either Auto-merge or
Auto-sort-merge state.

## In scope

- Settings types and API, plus `AutoSortMergeToggleFields` disabled and
  rendered OFF while the Auto-merge draft is OFF. Turning the Auto-merge draft
  OFF resets the sort draft to the saved value and clears its dirty flag; the
  PATCH omits `auto_sort_merge_enabled` while the Auto-merge draft is OFF; an
  older save response cannot clobber a newer edit.
- Reword the existing `messageQueueAutoMergeNotice` ("This applies only to new
  admissions") and its consumers (settings help, session Auto-merge pill
  tooltip): global changes affect later messages only; a session Auto-merge
  OFF-to-ON change with Auto-sort-merge ON compacts the queue immediately.
- The queue wire types, `QueueMeta` tuple, per-policy ordering in
  `agent-session.ts` and `session-slice.ts`, and preservation in every
  metadata rebuild.
- `queueStatusMeta` in `hooks/domains/session/use-queue-refetch.ts` projects
  the four sort fields only when present; this is the initial-load, reload,
  reconnect, and mutation-recovery path.
- `setQueueAutoSortMerge`, the `useQueue` action using the shared operation
  token, and the refetch plus localized error on failure.
- The `queue-auto-sort-merge` pill (checked = merge && sort; disabled when
  Auto-merge is OFF or either policy is unavailable), its accessible
  description, and coarse-pointer sizing.
- Locale entries in every shipped locale, with no em dashes.

## Out of scope

- Backend behaviour and E2E (Task 05).

## Acceptance

- Unit tests show that the pill is disabled and unchecked when Auto-merge is
  OFF, even with the preference ON, and enabled and reflecting the preference
  when Auto-merge is ON.
- The settings test shows that the switch disables with the Auto-merge draft
  and covers: saved sort OFF, edited to ON, then Auto-merge turned OFF and
  Save (sort is not sent and stays OFF); saved sort ON with Auto-merge turned
  OFF and back ON before Save (shows ON, nothing sent); shared Save/Discard;
  and a deferred save response that must not clobber a newer edit. With
  Auto-merge ON, the PATCH includes `auto_sort_merge_enabled` only when dirty.
- The header and settings tests assert the reworded Auto-merge notice.
- Slice and handler tests show that a lower-revision or `global`-after-`session`
  sort tuple is ignored without touching the Auto-merge tuple, and the reverse.

## ASCII UI preview

From [plan UI-01/UI-02/UI-03](plan.md#ascii-ui-preview):

```text
( Auto-run [on] ) ( Auto-merge [on] ) ( Auto-sort-merge [on] )
( Auto-run [on] ) ( Auto-merge [off] ) ( Auto-sort-merge [off] )  <- disabled
```

## Verification

```bash
pnpm --dir apps install --frozen-lockfile
(cd apps/web && pnpm exec vitest run components/settings/system/message-queue-settings.test.tsx components/task/chat/queued-ghost-panel-header.test.tsx components/task/chat/queued-ghost-list.test.tsx hooks/domains/session/use-queue.test.ts hooks/domains/session/use-queue-refetch.test.ts lib/api/domains/queue-api.test.ts lib/ws/handlers/agent-session.test.ts lib/state/slices/session)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/lib/types/system.ts`, `lib/api/domains/settings-api.ts`
- `apps/web/components/settings/system/message-queue-settings.tsx` and test
- `apps/web/lib/types/session-events.ts`, `lib/state/slices/session/{types,session-slice}.ts`
- `apps/web/lib/ws/handlers/agent-session.ts` and test
- `apps/web/hooks/domains/session/use-queue-refetch.ts` and new `use-queue-refetch.test.ts`
- `apps/web/lib/api/domains/queue-api.ts` and test, `hooks/domains/session/use-queue.ts` and test
- `apps/web/components/task/chat/{queued-ghost-panel-header,queued-ghost-list}.tsx` and tests
- `apps/web/src/locales/*/` (system, settings, and the chat pill namespace)

## Dependencies

Task 01.

## Risks

- The shared preserve helpers currently assume one policy, so they may need to
  become per-tuple helpers.
- A 4-pill header row on narrow desktop widths must wrap as groups, not
  truncate.

## Parallelism

`parallel-safe` with Tasks 02 and 03 (disjoint files, backend contract fixed
by Task 01).

## Inputs

- System design: Status contract, Frontend state, Queue header and settings UI.
- Patterns: the Auto-merge pill and `AutoMergeToggleFields`.

## Results

- Reused the successful frozen workspace install from Task 01.
- RED: independent store ordering failed on global-after-session and lower sort
  revisions; the other policy continued updating. Generalized tuple handling
  for HTTP/refetch, live events, and optimistic metadata rebuilds.
- RED: after rendering the new control, dependent-settings tests failed on
  disabled-OFF rendering, omitted PATCH data, and deferred-save dirty state.
  Header tests failed on unchecked/disabled dependency behavior before gating.
- The exact targeted Vitest command from this work order passed: 48 files,
  675 tests. Includes real-store live-event and refetch recovery tests, shared
  mutation-token exclusion, ignored mutation tuples, and failed-write recovery.
- `pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet`: passed
  after updating complete settings/status fixtures and save-return typing.
- `pnpm run i18n:zh-hant && pnpm run i18n:pseudo`: passed. All shipped locales
  contain the new copy; no UI em dashes. Node 24.22.2 was provisioned with
  `pnpm dlx node@24 --version`. Final repository gates pin that runtime explicitly.
- Extracted settings draft ownership and automation/mutation components to
  keep the changed functions and new files within the existing size limits.
  Existing exact-copy assertions were removed rather than re-pinned; behavior
  and accessibility linkage remain covered.
- The Traditional Chinese generator also changed unrelated automation-plugin
  translations. Those generator-only changes were restored to their original
  values; only message-queue locale changes are included.
