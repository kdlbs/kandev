---
created: 2026-09-17
status: draft
requirements:
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-001
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-002
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-003
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-004
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-005
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-006
system_design:
  - ../../specs/orchestration/system-design/coordinator-view.md
legacy_specs:
  - ../../specs/workspace-orchestrators/spec.md
---

# Implementation plan: Central workspace Coordinator view

## Overview

Add the grouped workspace task overview beside the existing persistent
coordinator chat. Implement scoped observations first, then the visible page,
then browser/media evidence. This sequence preserves the existing Kanban execution
and conversation ownership contracts while adopting the maintainer's proposed UI.

Baseline: local coordinator commit `b1cd0d2e`, whose direct parent is exact
v0.94.0 (`bf819a02`). The baseline contains implemented coordinator/automation
features and unfinished personal-assistant extensions. This plan only delivers
the new central view; it does not declare the assistant extension finished or
authorize deployment, publishing, additional schedules or a plugin installation.

All work orders are pending. The design turn ends with this complete package.

## Scope

### In scope

- Workspace-wide canonical task observations, filters, explicit paging/coverage,
  status grouping and task/PR links.
- Existing coordinator selection/chat beside tasks; desktop and mobile parity.
- Live refresh, stale-response rejection, scoped empty/error/setup states.
- Targeted tests, updated user-facing documentation and synthetic PR media.

### Out of scope

- A second task engine/scheduler, typed report storage, workflow monitoring policy,
  plugin host APIs, automatic adoption, cross-workspace coordination, assistant
  attention reconciliation/input resolution or enforced read-only provider tools.
- Board/dependency/diff editor duplication or buttons that dispatch builds/sweeps.
- Splitting/publishing the current 476-path snapshot or migrating live data.

The [requirements](../../specs/orchestration/requirements/coordinator-view.md)
own observable behavior. The [system design](../../specs/orchestration/system-design/coordinator-view.md)
owns boundaries and the explicit group-precedence table.

## Technical approach

1. Use `listTasksByWorkspace` and the existing task DTO/status summary, with
   canonical hidden/configuration-task exclusions and workspace authorization.
   Implement a page-scoped observation hook, deterministic grouping, paging
   coverage and monotonic status updates. Reuse the shared task cache/WS path;
   no independent persisted task state or schema migration.
2. Add `/workspaces/:workspaceId/coordinator`, reuse conversation content from
   `app/settings/orchestration`, and wire workspace/sidebar/mobile navigation.
   Existing conversation URLs keep working. Preserve drafts by scoped assignment;
   selecting/filtering/refreshing performs no dispatch.
3. Exercise the feature with synthetic workspaces, pending requests, multiple
   coordinators, task pages and delayed responses; update public docs and produce
   final desktop/mobile screenshots plus a short silent video.

No plugin source is imported. Relevant ideas retained from
[the reviewed plugin pin](https://github.com/yattdev/kandev-plugin-coordinator/tree/6fb1fdd63c1728258c40a67df037116c5ca9bbb8)
are verified workspace/session scope, stable chat identity and explicit
unavailable/setup states. Typed reports and its custom scheduler remain outside
this package.

## Tests

These are planned tests, not existing passing results. Test names should retain
the criterion references in nearby comments if names change during implementation.

| Criteria (prefix `AC-ORCHESTRATION-COORDINATOR-VIEW-`) | Planned evidence                                                                                                                                                                                                                                                                                  |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 001.1, 006.1                                           | `task/handlers/coordinator_listing_test.go`: `TestCoordinatorListingExcludesHiddenTasks`, `TestCoordinatorListingRejectsForeignWorkspace`; include a retained private conversation and feature-off case. Extend an existing listing test file instead if its fixture already covers the boundary. |
| 001.2, 001.4                                           | `use-coordinator-tasks.test.ts`: `loads additional pages without claiming complete counts`, `resets server filters without mutating tasks`; mixed linked/unlinked tasks and more than 100 records.                                                                                                |
| 001.3, 002.1–002.3                                     | `coordinator-task-groups.test.ts`: `pending input outranks running across sessions`, `idle is not stalled`, `merged PR is not task completion`, `missing summaries retain unknown coverage`.                                                                                                      |
| 004.1–004.3                                            | `use-coordinator-tasks.test.ts`: `rejects older HTTP status after WS delta`, `clears old workspace before delayed response`, `reconciles after reconnect`, `read failure retains stale coverage`.                                                                                                 |
| 003.1–003.4                                            | `coordinator-page.test.tsx`: `keeps selected chat and draft across filters`, `switches assignment without transferring content`, `opening does not dispatch`, `invalid profile shows setup`.                                                                                                      |
| 005.1–005.2, 006.2–006.3                               | `coordinator-page.test.tsx` plus browser flow: mobile task/chat parity, keyboard labels/focus, native input links, feature-off route/navigation.                                                                                                                                                  |

## E2E tests

Add `apps/web/e2e/tests/orchestration/coordinator-view.spec.ts`, project `chromium`,
using existing disposable backend fixtures and orchestration cleanup helpers.
Cover all task groups with canonical synthetic signals; include an unlinked task,
hidden conversation, missing PR/diff values and a second workspace. Show the
same chat identity across task navigation, workspace return and mobile tab
switches. Verify selection/observation does not create a delivery task or run.

Run the new spec together with `workspace-orchestrators.spec.ts` and
`automation-orchestrator.spec.ts`, with no retries, to preserve the repaired
shared-fixture isolation. Mobile coverage uses 390-pixel viewport inside the
feature flow and a native mobile browser project if required by the current
`mobile-parity`/`e2e` skills at implementation time.

## Work orders

- [ ] [Task 01: Scoped task observations](task-01-task-observations.md)
- [ ] [Task 02: Central Coordinator page](task-02-coordinator-page.md)
- [ ] [Task 03: Feature evidence](task-03-feature-evidence.md)

Dependency order: 01 → 02 → 03. Work stays in the primary session; waves do not
authorize subagents. Exact commands and likely files are in each work order.

## Verification results

Implementation checks: pending. Existing v0.94.0 coordinator regression results
are baseline evidence only; they do not verify this unbuilt page.

Design-package validation on 2026-09-17: specification linter tests passed
(30 tests), all specification files passed lint, and all six requirements / 19
acceptance criteria resolve through the three work orders. Relative document
links and whitespace checks passed. The separate scope audit's CSV reconciles
all 476 committed prototype paths and both rename destinations with the pinned
diff. Public docs are intentionally unchanged until the page's actual behavior
is delivered. No implementation tests were rerun for this documentation change.

## Risks

- Workspace task lists are paginated, and canonical events may arrive during a
  read. Counters must state coverage; revision guards must prevent stale status.
- Existing caches may contain only the active board. Wire all observed tasks
  into a scoped update/invalidation path without leaking another workspace.
- Chat extraction can break composer drafts, streaming, mobile scrolling or
  legacy routes; verify the original conversation browser spec alongside the new one.
- Existing privacy and assistant context safeguards are intertwined with the
  prototype. The later contribution split needs its own extraction/validation.
- Plugin-required host APIs are absent at v0.94.0. Keep this implementation on
  current core contracts; upstream core/plugin packaging and PR branch are unresolved.
- Synthetic media must be regenerated after the final UI. The existing issue's
  prototype captures cannot be presented as screenshots of this page.

## Wider delivery sequence

This package owns the task overview only. The [delivery plan](../orchestration-delivery/plan.md)
orders private publication, the assistant rollout gate, candidate qualification,
private-data rehearsal, live pilot and upstream export around these work orders.
