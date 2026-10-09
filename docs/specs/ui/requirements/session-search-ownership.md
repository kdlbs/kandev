---
status: active
system: ui
created: 2026-10-06
owners:
  - kandev
---

# Session Search Ownership Requirements

## Overview

People searching a chat transcript must see results belonging to their current
query and selected session. UI owns this client reply-ownership contract; task
storage, transcript pagination, and backend search semantics remain with their
existing owners. The contract applies equally to desktop and phone chat.

## Terminology

- **Current search:** The mounted search instance, selected session, current
  query, and open search lifetime that can accept results.
- **Retired search:** Work superseded by a query edit, clear, close, committed
  session change, unavailable session, or removal of the instance.

## Requirements

### REQ-UI-SESSION-SEARCH-OWNERSHIP-001: Current search results

**Intent:** Delayed search work shall not restore results or change loading
state after the user moves to another search context.

#### Acceptance criteria

- **AC-UI-SESSION-SEARCH-OWNERSHIP-001.1:** When a query is edited or cleared,
  results and selection from the previous query shall be cleared immediately.
  A retired success or failure shall not publish results, clear current results,
  or change current loading state, including before the replacement query starts.
- **AC-UI-SESSION-SEARCH-OWNERSHIP-001.2:** Closing search shall clear its query,
  results, selection, and loading state. Reopening shall remain usable, with a
  blank query and no retired results. Retired queued work shall not start a new
  search after close.
- **AC-UI-SESSION-SEARCH-OWNERSHIP-001.3:** A committed session change or loss
  shall clear the former search context. Work and callbacks retained from that
  context or a removed instance shall neither start a request nor update a later
  context, even when the same session is selected again. Independent mounted
  search instances shall not retire one another.
- **AC-UI-SESSION-SEARCH-OWNERSHIP-001.4:** A current nonblank search shall still
  debounce for 180 milliseconds, search the selected session with trimmed query
  and limit 50, and display its returned hits. A current failure shall clear hits
  and finish loading with the existing diagnostic behavior. A newer request shall
  retain priority over an older request. Hit navigation and existing bounded
  history backfill shall remain usable.

## Out of scope

- Transcript loaded-data identity, pagination or backfill policy changes.
- Search index, server data, protocol, socket transport, or cancellation changes.
- Composer Ctrl+R history search, new controls, copy, layout, touch behavior,
  scrolling policy, navigation policy, or breakpoint behavior.

## Implementation Plans

- [Current chat search results](../../../plans/session-search-ownership/plan.md)
