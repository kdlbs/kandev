---
status: active
system: ui
created: 2026-10-18
owners:
  - kandev
---

# Provider Watcher Task Types

## Overview

Users selecting watcher task types in Sidebar or Threads saved views need the
same categories regardless of which supported integration created the task.
UI owns this reusable saved-view classification contract. Tasks retains task
metadata and lifecycle ownership; Integrations retains watcher creation and
provider synchronization ownership.

This contract clarifies the shared classification beneath
[Threads task-type filters](threads-saved-views.md) (AC-UI-THREADS-SAVED-VIEWS-002.8)
and [Sidebar view evaluation](../system-design/sidebar-archived-filter.md).
It does not replace those surfaces' independent query or saved-view lifecycles.

## Terminology

- **Issue-watch task:** A task carrying an issue/work-item watcher identifier
  from GitHub, GitLab, Jira, Linear, Sentry, or Azure DevOps.
- **Review-watch task:** A task carrying a pull/merge-request review watcher
  identifier from GitHub, GitLab, or Azure DevOps.
- **Usable watch identifier:** A nonempty string other than the exact legacy
  sentinel `undefined`. Identifiers are opaque; classification does not trim
  strings or impose provider-specific identifier syntax.

## Requirements

### REQ-UI-PROVIDER-WATCHER-TYPES-001: Shared watcher task-type recognition

**Intent:** Existing watcher categories include all supported first-party
watcher tasks without treating ordinary linked tasks as watcher-created work.

#### Acceptance criteria

- **AC-UI-PROVIDER-WATCHER-TYPES-001.1:** A task with at least one usable
  issue/work-item watcher identifier from the issue-watch providers above shall
  match Sidebar's issue-watch filter and Threads' issue-watch task type when
  it has no usable review-watch identifier and meets that surface's other
  eligibility and filter conditions. Existing GitHub tasks shall remain included.
- **AC-UI-PROVIDER-WATCHER-TYPES-001.2:** A task with at least one usable
  review-watch identifier from the review-watch providers above shall match
  Sidebar's review-watch filter and Threads' pull-request-review task type,
  subject to the surface's existing eligibility and other filters. Existing
  GitHub tasks shall remain included.
- **AC-UI-PROVIDER-WATCHER-TYPES-001.3:** A manually linked issue or pull/merge
  request without a recognized usable watcher identifier shall remain ordinary
  work. Unrelated metadata, including unrecognized keys that resemble watcher
  identifiers, shall not confer either watcher category.
- **AC-UI-PROVIDER-WATCHER-TYPES-001.4:** An absent, null, empty, non-string, or
  exact `undefined` watcher value shall not confer a category. An unusable
  value shall not mask another usable identifier in the same category. A task
  carrying both categories shall match both Sidebar booleans; Threads shall
  retain its existing review-before-issue task-type precedence.
- **AC-UI-PROVIDER-WATCHER-TYPES-001.5:** Given the same task metadata and
  eligibility, persisted Sidebar results and complete locally evaluated Sidebar
  results shall agree on watcher-filter membership and matching task totals.
  Initial and live task projections shall give Threads the same task type.
  Negative watcher filters shall exclude the corresponding recognized category.
- **AC-UI-PROVIDER-WATCHER-TYPES-001.6:** Existing saved filter values and view
  definitions shall continue to work without resaving or rewriting task metadata.
  Classification shall not change archive/workspace eligibility, query order,
  grouping, paging, or Threads admission limits.
- **AC-UI-PROVIDER-WATCHER-TYPES-001.7:** Desktop and phone users shall receive
  the same classification and filtering outcomes through their existing saved
  view controls. Classification shall use available task data without requiring
  a provider connection or fetching issue/review details.

## Out of scope

- New providers, plugin watcher registration, or arbitrary key-suffix matching.
- Watcher scheduling, intake, dispatch, deduplication, or task creation changes.
- Metadata migration, mutation, validation on write, or link/unlink behavior.
- New endpoints, authorization, cache or freshness policies, layouts, or UI copy.
- Changing other metadata fields' legacy interpretation.

## Implementation plans

- [Provider watcher task types](../../../plans/provider-watcher-task-types/plan.md)
