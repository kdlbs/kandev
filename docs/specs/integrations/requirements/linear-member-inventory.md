---
status: active
system: integrations
created: 2026-10-14
owners:
  - kandev
---

# Linear Member Inventory Requirements

## Overview

The integration system owns the Linear team member inventory consumed by the
watcher's Creator filter. A user selecting a team must be able to choose any
matching member returned by Linear, including members beyond its first result
page. This is one provider inventory contract with a visible picker outcome.

## Terminology

- **Matching member:** A user returned by Linear for the selected team's existing
  membership scope, under the configured workspace credential.
- **Complete inventory:** All matching members across the provider's finite,
  successfully traversed result pages, without a Kandev member-count cap.

## Requirements

### REQ-INTEGRATIONS-LINEAR-MEMBERS-001: Complete team member inventory

**Intent:** Make every matching team member available to the watcher's Creator
filter without representing an incomplete inventory as successful.

#### Acceptance criteria

- **AC-INTEGRATIONS-LINEAR-MEMBERS-001.1:** When Linear returns matching members
  across multiple pages, the member inventory shall include all of those members,
  including a member appearing only after the first 250 results. The existing
  Creator filter shall receive that member's identity and display information on
  desktop and phone.
- **AC-INTEGRATIONS-LINEAR-MEMBERS-001.2:** Every page shall retain the selected
  team's existing membership scope and the configured workspace credential. An
  empty team selection shall remain invalid; a successful empty inventory and a
  successful single-page inventory shall retain their existing result shapes.
- **AC-INTEGRATIONS-LINEAR-MEMBERS-001.3:** If any page fails or the caller cancels
  or exceeds its deadline during traversal, the inventory operation shall fail
  without returning earlier pages as a successful or partial member list.
- **AC-INTEGRATIONS-LINEAR-MEMBERS-001.4:** If Linear omits the metadata needed to
  determine completion, or advertises another page with an empty or previously
  used continuation cursor, the operation shall terminate with an error and no
  partial member list. A terminal page shall not require a continuation cursor.

## Out of scope

- Assignee selection, which uses Any, me, and unassigned rather than members.
- Changes to membership eligibility, credential ownership, or workspace scope.
- Labels, teams, workflow states, issue-search or watcher-poll pagination.
- Frontend cache freshness, workspace switching, selection preservation races,
  new error UI, picker layout, touch behavior, navigation, or virtualization.
- Mutations, new public response fields, dependencies, configuration, or storage.

## Implementation plans

- [Linear Creator member pagination](../../../plans/linear-creator-member-pagination/plan.md)
