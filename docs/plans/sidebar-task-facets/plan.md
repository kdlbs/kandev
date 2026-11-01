---
created: 2026-10-24
status: in_progress
requirements:
  - REQ-PLUGINS-SIDEBAR-TASK-FACETS-001
system_design:
  - ../../specs/plugins/system-design/sidebar-task-facets.md
legacy_specs: []
---

# Implementation Plan: Sidebar Task Facets

## Overview

Deliver existing plugin task-list facets as workspace-scoped sidebar saved-view
Group by choices, with page-local repeated task sections and preserved task
interactions. This delivery record describes the implementation in PR #4755;
it does not claim planning preceded that implementation.

- [Requirements](../../specs/plugins/requirements/sidebar-task-facets.md)
- [System design](../../specs/plugins/system-design/sidebar-task-facets.md)

## Scope and approach

Reuse the existing plugin registration, synchronous value resolver, and
subscriptions. Persist the namespaced option in existing sidebar views and
drafts. Query the ordinary bounded page with built-in grouping, then project
facet sections locally with independent hierarchy and collapse identities.

Keep canonical task IDs for actions and deduplicate bulk targets; use
section-qualified occurrences for range endpoints. Do not allow promoted child
roots to persist an invalid root order. Missing plugins and unknown saved options
must leave the view editor usable without overwriting saved intent.

Sidebar facet sorting/filtering, workspace-wide facet queries, backend schema
changes, SDK changes, and unrelated plugin-repository work are excluded.

## ASCII UI preview

### UI-01: Sidebar Group by and value sections

Entry: sidebar task view, Filters and sort, Group by expanded.
The control order is shared by the desktop popover and phone drawer; phone
composition keeps the existing drawer and its scrolling body.

```text
GROUP BY
[Tag v]  choices: None, Repository, Workflow, Workflow step,
                 Executor type, State, Tag

Sidebar task list:
v blue       2
  Parent
    Child
v urgent     1
  Child
v Unassigned 1
  Other task
```

The repeated Child refers to one canonical task. Its urgent occurrence is a root
because Parent is not in that section. A valid color adds a decorative heading
swatch. Unavailable or unknown grouping displays None without changing the
stored request. Structure maps to AC .1, .6-.10, .13-.15; labels and spacing in
this drawing are illustrative, not hardcoded copy or a pixel specification.

## Work orders and dependency order

- [Task 01: Sidebar facet grouping](task-01-sidebar-facet-grouping.md)
  (`in_progress`: implementation present; integration verification pending).

One vertical work order covers the control, persistence, projection, task
interaction, documentation, and evidence. There is no new infrastructure or
plugin-side prerequisite.

## Verification strategy

Use focused unit/component tests for value membership, source ordering,
per-section hierarchy, occurrence selection, drag safety, persistence,
availability, workspace changes, and unusual saved options. The work order
contains exact commands and the acceptance-to-test map.

User-facing desktop and phone evidence must exercise the sidebar, not merely
`/tasks`. Existing task-list facet browser suites remain useful compatibility
coverage but do not by themselves establish sidebar acceptance. Record only
actually exercised checks in the work-order results.

## Risks

- Grouping before pagination or reordering source tasks changes page semantics.
- A shared child map or canonical-only row anchor merges repeated occurrences.
- Promoted child roots can be written into the wrong sibling scope by root drag.
- Missing plugin fallback can accidentally overwrite saved intent.
- Requirements or public docs alone do not satisfy the deterministic PR delivery
  artifact gate; the changed work order and its linked contracts must ship together.

## Delivery status

Implementation and regression sources are present. Review remediation adds safe
unknown-option fallback, promoted-root drag protection, and this traceable
package. Worker-side checks were intentionally not run; final integration and
browser verification are owned by the integrating agent.
