---
status: active
system: ui
created: 2026-10-07
owners:
  - kandev
---

# Hidden Running Backfill Requirements

## Overview

While a session is running, each mounted transcript re-reads its latest message
window every few seconds to recover missed WebSocket frames. Dockview keeps
inactive session panels mounted, so a window with many running sessions does
this work for every one of them. When the document is hidden, nobody can see
the result. Message freshness after return stays owned by
[Transcript history freshness](transcript-history-freshness.md).

## Terminology

- **Running backfill:** The periodic latest-message read that
  `useRunningMessageBackfill` schedules for a connected, running session with
  an active turn.
- **Hidden document:** `document.visibilityState` is `"hidden"`.

## Requirements

### REQ-UI-HIDDEN-RUNNING-BACKFILL-001: Skip running backfill while hidden

**Intent:** Stop periodic transcript reads that no one can see, without losing
messages when the window becomes visible again.

#### Acceptance criteria

- **AC-UI-HIDDEN-RUNNING-BACKFILL-001.1:** While the document is hidden, a
  running backfill tick shall not request messages.
- **AC-UI-HIDDEN-RUNNING-BACKFILL-001.2:** While the document is visible, running
  backfill shall keep its current initial delay and interval.
- **AC-UI-HIDDEN-RUNNING-BACKFILL-001.3:** The foreground refresh that runs when
  the document becomes visible shall not change, so messages missed while
  hidden are still recovered.

## Out of scope

- Inactive but visible panels, and windows that are unfocused but not hidden.
- Other timers and polls that ignore visibility.

## Implementation plans

- [Hidden running backfill](../../../plans/hidden-running-backfill/plan.md)
