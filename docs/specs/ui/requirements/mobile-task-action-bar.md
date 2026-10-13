---
status: active
system: ui
created: 2026-10-12
owners:
  - Kandev frontend
---

# Mobile task action bar requirements

## Overview

The task composer needs a readable terminal notice and reachable controls on
phones. UI owns this reusable presentation contract across chat and passthrough
composers. Tasks and integrations retain archive, workflow, and PR state authority.

## Requirements

### REQ-UI-MOBILE-TASK-ACTION-BAR-001: Readable phone composer actions

**Intent:** Keep task status and actions usable without taking unnecessary space
from the conversation.

#### Acceptance criteria

- **AC-UI-MOBILE-TASK-ACTION-BAR-001.1:** Below 768px, a terminal PR notice
  shall occupy its own full-width region above utility and workflow controls.
  Its message shall identify the PR or count and distinguish merged from closed
  without merging. Archive and dismiss shall remain visible alongside the message.
- **AC-UI-MOBILE-TASK-ACTION-BAR-001.2:** At phone widths of 320px, 393px,
  640px, and 767px, complete controls shall fit or wrap without clipping,
  overlapping hit areas, or document horizontal overflow. A long step label
  shall remain readable and accessible. The workflow action shall remain at
  the trailing edge of the action region, independently of the terminal notice.
- **AC-UI-MOBILE-TASK-ACTION-BAR-001.3:** Phone controls, including narrow
  fine-pointer viewports, and coarse-pointer controls shall satisfy the
  [control sizing](control-sizing.md) touch minimums. At or above 768px with
  a fine pointer, existing compact composer controls shall retain their 24px
  role; ordinary controls shall use the 28px role.
- **AC-UI-MOBILE-TASK-ACTION-BAR-001.4:** Opening Archive shall retain the
  [archive confirmation preference](../../tasks/requirements/archive-confirmation.md).
  Cancel shall keep the task and notice available. Dismiss shall hide only
  that task's notice and retain existing persistence across reload/task switches.
  A pending archive or workflow move shall retain its established duplicate
  prevention and disabled behavior.
- **AC-UI-MOBILE-TASK-ACTION-BAR-001.5:** Todo, transcript navigation, usage,
  sharing, queue, dependency, and eligible provider controls shall remain
  reachable through their existing actions. Usage shall retain its touch drawer.
  Activating the workflow action shall move to the same task-owned step as desktop.
- **AC-UI-MOBILE-TASK-ACTION-BAR-001.6:** With no terminal notice, the notice
  region shall consume no space. One or multiple terminal PRs shall retain the
  existing all-merged/all-closed visibility rules. Switching viewport shall
  preserve task/session state and saved preferences.
- **AC-UI-MOBILE-TASK-ACTION-BAR-001.7:** New or changed messages and accessible
  labels shall be localized, with correct singular/plural counts. The notice
  shall wrap normally in long translations without hiding its meaning or
  shrinking text to fit. Controls shall retain semantic names and keyboard activation.
- **AC-UI-MOBILE-TASK-ACTION-BAR-001.8:** The action bar shall remain in the
  composer region outside the transcript scroller and above existing bottom
  navigation/safe-area clearance. It shall introduce no additional vertical
  scroller. Opening or dismissing a notice shall leave the composer usable.

## Out of scope

- Task lifecycle, PR eligibility, sharing, workflow options, archive transport,
  archive confirmation design, and persistent preference changes.
- A new mobile navigation bar, settings, provider automation, or overflow menu.
- A repository-wide control sizing or internationalization migration.

## Related documents

- [System design](../system-design/mobile-task-action-bar.md)
- [Implementation plan](../../../plans/mobile-task-action-bar/plan.md)
