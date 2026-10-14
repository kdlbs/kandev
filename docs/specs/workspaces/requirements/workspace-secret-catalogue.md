---
status: active
system: workspaces
created: 2026-10-10
owners:
  - kandev
---

# Workspace Secret Catalogue Requirements

## Overview

Users can save a Workspace secret while its initial metadata listing is still
loading. An older successful listing must not erase that accepted work.
Workspaces owns this contract because it owns secret scope and the current
workspace metadata lifetime. Desktop and phone use the same settings surface.

This is the hook-local workspace catalogue contract. The shared Global catalogue
has a separate owner lifetime in the
[repository-secrets requirement](repository-secrets.md). Its independently
reviewed Global-publication extension remains authoritative for Global scope.

## Requirements

### REQ-WORKSPACES-SECRET-CATALOGUE-001: Accepted workspace metadata during initial loading

**Intent:** Keep accepted changes visible when an earlier listing succeeds in
the same current workspace catalogue.

#### Acceptance criteria

- **AC-WORKSPACES-SECRET-CATALOGUE-001.1:** When Workspace secret creation is
  acknowledged while an earlier initial listing is pending, successful
  completion of that listing, including an empty result, shall preserve the
  accepted secret and its rendered settings row on desktop and phone.
- **AC-WORKSPACES-SECRET-CATALOGUE-001.2:** When accepted local creation, update,
  or removal changes the current workspace catalogue during an earlier initial
  listing, successful completion of that listing shall preserve the entire
  current list's membership, order, and metadata values. This applies to every
  current row in a mixed list, including an empty current list after removal
  and a list whose values return to their earlier state. Rows found only in the
  older listing shall not be introduced after those accepted changes.
- **AC-WORKSPACES-SECRET-CATALOGUE-001.3:** Without an intervening accepted local
  change, an initial successful listing shall publish its complete result,
  including an empty list. A current successful listing shall settle loaded
  and loading state even when its contents are superseded by local changes;
  those changes shall not restart or admit another initial listing.
- **AC-WORKSPACES-SECRET-CATALOGUE-001.4:** Preservation shall apply only within
  the unchanged selected scope, workspace, and supplied initial listing.
  Changing workspace or scope shall retain workspace isolation and discard
  obsolete listing results. A replacement supplied initial listing, including
  an explicit empty list, shall replace local metadata without an initial read.
  Unmounting shall abandon the pending listing without publishing it elsewhere.

## Related contracts

Reuse [AC-WORKSPACES-REPOSITORY-SECRETS-001.1, .2, and .14](repository-secrets.md#requirements)
for scope creation, workspace ownership, and independence from unrelated Global
loading. They are compatibility controls, not redefined here. The earlier
[metadata lifetime design](../system-design/repository-secrets.md#metadata-list-lifetimes)
excluded same-list ordering from that correction; this pair explicitly owns
the additional workspace success-publication outcome.

## Out of scope

- Initial-read rejection or catch settlement changes. Loss from rejection is
  not qualified; expansion requires independent causal evidence and owning
  requirement/design reconciliation first.
- Global catalogue rules, shared store ownership, cross-instance caches,
  refresh APIs, navigation lifecycle redesign, or late mutation callbacks from
  an abandoned settings lifetime.
- Secret values, reveal, authorization, encryption, backend/API contracts,
  bindings, transfers, and running execution environments.
- Layout, touch behavior, scrolling, navigation, localized copy, and new user
  operations.
