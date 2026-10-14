---
status: active
system: workspaces
created: 2026-10-02
owners:
  - kandev
---

# Repository Branch Reads Requirements

## Overview

Branch selectors share repository branch information. Opening another selector
or refreshing an existing selector must not let an older response replace the
latest requested information or end its loading state early. The workspace
system owns this contract because it owns repository identity and branch data;
task creation, launch recovery, watchers, and settings consume that data.

## Terminology

- **Source:** A saved repository, or a folder path within a workspace.
- **Newest request:** The request started most recently for the same source in
  the same shared client state, regardless of completion order or consumer.
- **Accepted list:** The last successful branch list accepted for that source,
  including an explicitly empty list.

## Requirements

### REQ-WORKSPACES-BRANCH-READS-001: Shared branch read freshness

**Intent:** Users see branch information from their newest request and can trust
its loading state across selectors.

#### Acceptance criteria

- **AC-WORKSPACES-BRANCH-READS-001.1:** When initial reads and refreshes overlap
  for one source, only the newest request shall replace the accepted list.
  This shall hold across separate consumers of the same shared client state.
- **AC-WORKSPACES-BRANCH-READS-001.2:** While the newest request is pending,
  loading shall remain true despite an older request succeeding or failing.
  When the newest request settles, loading shall become false without waiting
  for older requests.
- **AC-WORKSPACES-BRANCH-READS-001.3:** A failed request shall preserve the
  accepted list and its loaded status. If no list has been accepted, failure
  shall leave the source unloaded. An older success arriving after the newest
  request fails shall not replace the accepted list or mark an unloaded source
  loaded. A subsequent requested retry shall remain possible.
- **AC-WORKSPACES-BRANCH-READS-001.4:** A successful newest request shall mark
  the source loaded and accept its list, including an empty list. A later
  obsolete response shall not restore branches removed by that success.
- **AC-WORKSPACES-BRANCH-READS-001.5:** Reads for different saved repositories,
  folder paths, workspace/path combinations, or independent client stores
  shall not supersede each other or share loading ownership. Saved repository
  identity shall retain its existing scope.
- **AC-WORKSPACES-BRANCH-READS-001.6:** Initial reads shall remain demand driven
  and reuse accepted cached lists. Null or disabled sources shall not trigger
  an initial read. Existing bounded transient retry behavior and the distinct
  saved-repository and folder refresh operations shall remain available.
- **AC-WORKSPACES-BRANCH-READS-001.7:** A consumer leaving or changing source
  shall not clear another consumer's loading state or abandon a valid shared
  read. A completed read shall remain usable by later consumers of that source.
- **AC-WORKSPACES-BRANCH-READS-001.8:** Desktop and phone selectors shall
  observe the same accepted list and loading ownership. Reading or refreshing
  branches shall not change a chosen branch, filtering, policy selection, or
  selector composition.

## Out of scope

- Branch selection, branch policies, Git commands, backend behavior, and APIs.
- New caching persistence, polling, retry schedules, authentication or active
  workspace reset semantics, and environment selection.
- Layout, touch interaction, scrolling, navigation, breakpoints, and new copy.

## Traceability

- [System design](../system-design/repository-branch-reads.md)
- [Implementation plan](../../../plans/repository-branch-read-ordering/plan.md)
