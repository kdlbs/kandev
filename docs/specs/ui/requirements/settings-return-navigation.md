---
status: active
system: ui
created: 2026-10-21
owners:
  - kandev
---

# Settings return navigation

## Overview

Closing Settings returns users to the page that opened Settings. This avoids
losing task context during configuration. UI owns this reusable navigation
interaction. Tasks retain identity, authorization, and lifecycle ownership.

The origin is the last non-Settings location before the current Settings visit.
A visit can include multiple Settings pages.

## Requirements

### REQ-UI-SETTINGS-RETURN-001: Return to the originating page

**Intent:** Users can adjust settings and return to their task in one action.

- **AC-UI-SETTINGS-RETURN-001.1:** After opening Settings from a task, Close settings shall return to that task. It shall preserve the selected session and other URL context without reloading the document.
- **AC-UI-SETTINGS-RETURN-001.2:** Settings redirects and navigation between Settings pages shall retain the original destination. Opening Settings from another application page shall return to that page.
- **AC-UI-SETTINGS-RETURN-001.3:** Close settings shall respect unsaved-change protection. Continue editing shall preserve the Settings page, draft, return destination, and navigation mode. A permitted departure shall return to the origin.
- **AC-UI-SETTINGS-RETURN-001.4:** Without an eligible origin, Close settings shall use the active workspace's existing Home destination. Eligible origins are non-Settings application locations visited in the current document. Direct loads, refreshes, and new tabs shall not inherit another document's origin.
- **AC-UI-SETTINGS-RETURN-001.5:** Each new visit shall use its own origin. Explicit Home navigation shall continue to open Home. A missing or inaccessible destination shall use existing destination error handling without starting or replacing a task session.
- **AC-UI-SETTINGS-RETURN-001.6:** Desktop shall retain its sidebar close action. Phones shall expose an accessible Close settings action on Settings pages, including the index. Its touch target shall measure at least 44px in both dimensions, including narrow fine-pointer windows.

## Out of scope

Restoring unsaved task drafts, editor selections, or scroll positions beyond
existing behavior is excluded. This change adds no cross-refresh history,
task lifecycle operation, or change to Settings save policy.

## Related contracts

- [Task navigation](task-navigation.md)
- [Settings manual save](settings-manual-save.md)
- [Settings menu default](settings-menu-default.md)

## Implementation plans

- [Settings return navigation](../../../plans/settings-return-navigation/plan.md)
