---
status: current
system: ui
requirements:
  - REQ-UI-HIDDEN-RUNNING-BACKFILL-001
---

# Hidden Running Backfill System Design

## Purpose and boundaries

`useRunningMessageBackfill` in
`apps/web/hooks/domains/session/use-session-messages.ts` owns the running
backfill timers. `useVisibilityBackfill` in the same file, through
`useForegroundRefresh`, owns the refresh on return to the foreground. Only the
first changes.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `AC-UI-HIDDEN-RUNNING-BACKFILL-001.1`, `001.2` | [Tick gate](#tick-gate) |
| `AC-UI-HIDDEN-RUNNING-BACKFILL-001.3` | [Return to visible](#return-to-visible) |

## Tick gate

The `sync` callback returns early when `document.visibilityState` is
`"hidden"`, next to the existing in-flight guard. The timers stay scheduled and
`shouldRunMessageBackfill` is unchanged, so the effect does not re-run on
visibility changes and the next visible tick runs normally.

## Return to visible

`useForegroundRefresh` already calls `fetchAndStoreMessages` when the document
becomes visible. That read covers any messages that arrived while ticks were
skipped.
