---
created: 2026-10-27
status: implemented
requirements:
  - REQ-TASKS-WORKSPACE-SEARCH-002
system_design:
  - ../../specs/tasks/system-design/workspace-search.md
legacy_specs: []
---

# Implementation plan: Command-panel eligible tasks

## Outcome and evidence

Select idle command-panel tasks after step visibility is applied. The existing
hook requests the first twenty workspace tasks, then discards hidden steps; a
rendered qualification using the actual hook, command-list content, command
primitives and English catalogs failed when twenty newer hidden rows preceded
one eligible task. An ordinary first-page control passed and its selection
invoked navigation. This establishes the settled-data failure, not a settings
save, response-race, or task-lifecycle failure.

One vertical work order covers the SQL predicate, authorized HTTP path, and
shared command-panel data consumer. The repository's sidebar query filters
steps but has different hierarchy ordering and configuration-task exclusion;
it cannot preserve this list's behavior. The proposed optional workspace-list
filter avoids changing the widely implemented positional repository interface.

## Scope and dependencies

- Apply eligibility before the twenty-row page, retaining the existing
  workspace order then step-position display ordering and five/twenty limits.
- Preserve ordinary listing, text and conversation search, active/archive
  grouping, access, other filters, cancellation, and existing presentation.
- No new layout, copy, state abstraction, schema migration, unbounded fetch,
  settings freshness, or workflow lifecycle work. There are no dependencies
  on another work order. Recheck current open PR ownership before implementation;
  title inventory found no command-panel/task-search overlap. PR #4736 changes
  workflow-step deletion and related files, not the proposed list/hook files;
  this is not an audit of all open PR files.

## Verification and risks

The work order specifies independently authored SQL, HTTP/service and rendered
consumer regressions and exact focused checks. SQLite and a real
environment-gated PostgreSQL test cover dialect behavior; an absent DSN is a
skip, not a PostgreSQL pass. No dependency install or heavy check runs during
design. After explicit implementation authorization and the exclusive heavy
lease, install frozen workspace dependencies once and execute the checks
serially. Record actual red/green evidence and every command result.

The filter relies on the selected workflow identity and its settled step data.
Keep the no-step and temporarily absent-identity fallbacks; do not claim this
fix resolves stale state. Reconcile the opt-in's COUNT/SELECT filters and
authorization with the existing list path and verify the API cannot silently
fall back to an unfiltered response. A data-only shared-result change qualifies
for the mobile-parity exception; existing phone navigation and result action
remain unchanged. The additive workspace-list REST option is internal UI
transport: its GET route is not documented as a supported public API in
`docs/public`, the root README, or the screenshot catalog. Existing public
workflow documentation already describes the setting; no parameter reference
or public copy change is needed.

## Work order

- [x] [Task 01: Select eligible tasks before paging](task-01-select-eligible-tasks.md)

## Results

Implemented and checked under the exclusive heavy lease. The [work-order
results](task-01-select-eligible-tasks.md#results) record the focused red and
green evidence, test limits, PostgreSQL skip, and typecheck allocation. Hosted
CI and review remain pending.
