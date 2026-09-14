---
status: active
system: integrations
created: 2026-09-11
owners:
  - Kandev
---
# GitHub dashboard mobile requirements

## Overview

Phone users need discoverable GitHub queries and usable result actions. Integrations owns provider queries, their scope, and the dashboard outcomes. Shared task-view navigation is independently owned by [UI](../../ui/requirements/mobile-task-view-access.md).

## Requirements

### REQ-INTEGRATIONS-GITHUB-MOBILE-001: GitHub dashboard mobile parity

**Intent:** Preserve desktop dashboard capabilities through focused phone interactions.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-MOBILE-001.1:** Saved queries shall distinguish loading, failure with retry, and an empty collection. A failed save shall retain the entered name and repository and permit retry; successful persistence shall close the form. Workspace changes shall not apply stale outcomes to the new workspace.
- **AC-INTEGRATIONS-GITHUB-MOBILE-001.2:** Below 768px, a labeled current-view control shall open an inset bottom drawer. The trigger shall stay on one line without compressing result count or refresh metadata; its full name shall remain accessible. A separate status row shall label the result count and group updated time with Refresh below the query input. Changing PR/issue kind shall keep the drawer open; selecting a query or Done shall close it. Save shall remain reachable with long collections. Existing per-kind defaults, delete confirmation, and default-action isolation shall continue to work.
- **AC-INTEGRATIONS-GITHUB-MOBILE-001.3:** Phone and coarse-pointer result actions shall have at least 44px hit targets. Long titles and repository names shall remain readable without horizontal overflow. Linked tasks shall expose their workflow step without requiring hover while retaining direct task navigation.
- **AC-INTEGRATIONS-GITHUB-MOBILE-001.4:** Phone pagination shall expose previous, next, current/total page, and direct page selection through an in-app bottom drawer, not an operating-system select menu. Opening the chooser shall reveal and mark the current page. Selecting another page shall navigate once and dismiss; selecting the current page shall dismiss without refetching. Existing search limits and filter-triggered page resets shall remain unchanged, with no horizontal overflow.
- **AC-INTEGRATIONS-GITHUB-MOBILE-001.5:** Drawer and page controls shall support dismissal, focus return, internal scrolling, safe-area clearance, and translated labels. Desktop query state and preferences shall remain unchanged by phone composition.

## Compatibility

The [saved-query defaults contract](../../ui/requirements/github-saved-query-defaults.md) remains authoritative for default selection and scope; the drawer replaces only that document's historical filter-sheet presentation.

## Out of scope

New query storage, renamed/edited saved queries, changed authentication, direct dashboard merge/review actions, and shared task-view data semantics.

## Implementation plans

- [GitHub mobile parity](../../../plans/github-mobile-parity/plan.md)
