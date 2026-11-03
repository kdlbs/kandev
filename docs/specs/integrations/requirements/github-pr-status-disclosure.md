---
status: active
system: integrations
created: 2026-10-29
owners:
  - kandev
---

# GitHub PR Status Disclosure Requirements

## Overview

Users must be able to open a linked PR while its status loads or refreshes.
The integration system owns this detailed GitHub disclosure and its actions.
The separate [task summary](../../ui/requirements/pr-task-status-summary.md)
owns informational sidebar and card tooltips.

## Requirements

### REQ-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001: Stable status actions

**Intent:** Keep the PR action reachable while asynchronous status updates arrive.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.1:** During one open disclosure,
  data updates shall not move or resize its outer frame or its Open PR action.
  This applies to the desktop topbar and chat status chip, with one or several PRs.
  Viewport changes and movement of the trigger may reposition the frame.
- **AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.2:** The disclosure shall show a
  labelled Open PR link as soon as the selected PR URL is available. The link
  shall remain visible and operable during loading, refresh, empty results,
  and status errors. It shall open the selected PR, including after tab changes.
  Its accessible name shall contain the visible localized Open PR label.
- **AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.3:** First loading shall show a
  localized loading state. Refresh shall retain available results and show an
  updating indicator. Updates shall remain live without closing the disclosure.
- **AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.4:** Long content shall use one
  vertical scroll region below the PR identity and action. All status details
  and existing automation and merge controls shall remain reachable. The frame
  shall fit the available viewport without horizontal page or details-body
  overflow. Phone PR tabs shall fit or wrap without horizontal scrolling.
  An overflowing PR selector shall stay bounded on short phones so that PR
  selection cannot hide the fixed action or collapse the details region.
  The selector height shall remain fixed while the multi-PR disclosure is
  open, including when linked PRs arrive or are removed.
- **AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.5:** Pointer transfer from the
  trigger to the disclosure shall keep it open. Keyboard users shall reach
  the PR link and scroll region. Escape shall dismiss the disclosure and retain
  the existing focus-return behavior. Data updates shall not steal focus.
- **AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.6:** The existing touch drawer
  shall expose the same fixed PR action and independently scrolling details.
  Its height shall remain stable during data updates. Phone and coarse-pointer
  PR actions shall have a target of at least 44 by 44 CSS pixels. Closing,
  safe-area spacing, and all existing PR capabilities shall remain available.

## Out of scope

- Provider refresh timing, cache identity, merge eligibility, and automation behavior.
- Sidebar summary redesign, new click-to-pin behavior, and hover-delay changes.
- GitLab or plugin UI redesign and new settings or runtime flags.

## Design and implementation

- [System design](../system-design/github-pr-status-disclosure.md)
- [Implementation plan](../../../plans/stable-pr-status-disclosure/plan.md)
