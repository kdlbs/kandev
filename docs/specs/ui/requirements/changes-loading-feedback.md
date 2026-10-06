---
status: active
system: ui
created: 2026-10-01
owners:
  - kandev
---

# Changes Loading Feedback Requirements

## Overview

Users need loading feedback without extra rows that change the spacing in Changes.
UI owns this presentation contract across Git refresh, file details, and commit expansion.
Git data and request lifecycles remain with their existing owners.

## Requirements

### REQ-UI-CHANGES-LOADING-001: Compact loading feedback

**Intent:** Show passive progress and freshness in one stable toolbar location.

#### Acceptance criteria

- **AC-UI-CHANGES-LOADING-001.1:** While passive requests remain pending, Changes shall show one small toolbar spinner, subject to the warning priority below.
  Passive requests include Git refresh, file detail enrichment, and commit-file reads.
  An unresolved Git refresh failure shall replace that spinner with a warning icon between retry attempts.
  An active Git recovery attempt shall show the spinner in that same position.
  The status shall appear immediately after the complete left action group, before the toolbar spacer.
  When the eye button is the group's final action, the status shall appear directly to its right.
  Without reviewable content, the spinner shall still appear on the toolbar's left side.
- **AC-UI-CHANGES-LOADING-001.2:** Hover or keyboard focus shall reveal a localized tooltip for the current status.
  The spinner tooltip shall say "Loading changes..." in the current locale.
  Assistive technology shall receive a localized loading or unavailable freshness status.
  A warning tooltip shall explain that available changes can be stale and that recovery is automatic.
  Phone users shall see the status without a hover interaction.
- **AC-UI-CHANGES-LOADING-001.3:** Passive loading shall not add a status card or a padded commit-file placeholder to the content.
  File rows shall omit repeated loading text and unknown line counts without an eligible prior display value.
  Existing files and commits shall remain visible during refresh.
- **AC-UI-CHANGES-LOADING-001.4:** The indicator shall remain while passive requests are pending or a Git refresh failure remains unresolved.
  Settling one request shall not hide another request's loading or failure state.
  Switching task, session, or environment shall discard the previous context's loading feedback.
  Git refresh failures shall use the toolbar warning without a body banner, toast, or manual Retry button.
  Inline commit-detail failures shall retain their existing error and Retry behavior.
- **AC-UI-CHANGES-LOADING-001.5:** Desktop, narrow panels, and phone Changes shall retain toolbar height and one content scroller.
  The spinner shall remain outside overflow menus and shall not overlap neighboring actions.
  Existing phone actions shall retain touch targets of at least 44px.
- **AC-UI-CHANGES-LOADING-001.6:** A pending initial refresh shall not show a completed empty state.
  After complete empty membership arrives, Changes shall show its existing empty state and remove the spinner.
- **AC-UI-CHANGES-LOADING-001.7:** Hover or keyboard focus on the Review eye button shall show the localized tooltip "Review".
  The tooltip shall remain available when the button has no visible label.
  The button shall retain its accessible name and existing Review action on desktop and phone.

## Exclusions

Git mutations retain their action feedback and disabled behavior.
Diff content continuity belongs to the linked Platform contract.
Cache limits, repository identity, and workspace recovery remain unchanged.
Automatic Git refresh recovery belongs to the linked Platform contract.

## Related documents

- [Design](../system-design/changes-loading-feedback.md)
- [Panel geometry](panel-toolbars.md)
- [Bounded Changes collections](bounded-changes-rendering.md)
- [Automatic Git refresh recovery](../../platform/requirements/workspace-git-status.md)
- [Implementation plan](../../../plans/changes-loading-feedback/plan.md)
