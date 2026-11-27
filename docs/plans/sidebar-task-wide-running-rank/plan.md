---
created: 2026-10-07
status: completed
requirements:
  - REQ-UI-SIDEBAR-RUNNING-ACTIVITY-001
system_design:
  - ../../specs/ui/system-design/sidebar-running-first-activity-sort.md
legacy_specs: []
---

# Implementation Plan: Task-wide running rank in the sidebar

## Overview

Running first will recognize any running session on a task.
Task 01 delivered the summary, server ranking, and local consumers as one slice.
Task 02 proved desktop and phone behavior and updated the public reference.
Both sequential work orders are complete under the explicit implementation request.

UI owns the personal sorting contract. Tasks owns session state and summary
publication. The existing UI requirement/design pair covers this complete outcome.

## Confirmed diagnosis

Read-only evidence captured on 2026-10-07 reproduced the user's sidebar order.
The query used Running descending, Red descending, Orange descending, then
Last activity descending, with no grouping.

| Task | Observed sessions | Summary | Rank before the fix |
| --- | --- | --- | --- |
| Investigate slow task workspace resume | Both `WAITING_FOR_INPUT` | Primary waiting, no foreground activity | Non-running, orange preferred |
| Design Kandev Projects mode | One `RUNNING`, two waiting, all non-primary | No primary, foreground `generating` | Non-running despite its spinner |

Resume task ID: `fe56e867-8fd0-4b8b-8cf2-701c563ae019`.
Projects task ID: `301ce89a-129c-4697-ae50-861291d0628a`.
Its running session ID was `6f5c6e7b-6152-40bb-9fc3-cc08fb7856db`.
These IDs document the observation. Tests use disposable seeded records.

`sidebarStateFields` reads `primary_session.state`.
`sidebarRunningCTEs` aggregates that primary state through the tree.
`resolveTaskTreeRunning` uses the same primary-only check locally.
`deriveSessionFields` aggregates foreground activity across sessions instead.
The row spinner therefore uses broader evidence than the sort.

The current implementation follows its previous primary-only specification.
This package amends that product definition through AC .14-.16.
It does not treat the old specification as an implementation regression.

## Scope

### In scope

- Add one revision-owned running-session flag to task summaries.
- Derive the flag from complete session observations and repair requested legacy rows.
- Apply it consistently to server, covered local, desktop, and phone sort paths.
- Preserve filters, subtree promotion, chain precedence, pins, and manual child order.
- Cover mixed sessions, lifecycle updates, upgrade reads, stale responses, and paging.
- Update the existing public sidebar sorting reference during implementation.

### Out of scope

- Primary-session election or repair of the user's task data.
- Changes to session lifecycle, workflow placement, or active conversation selection.
- Changes to Status sorting or icon precedence for permission, preparing, and background work.
- New settings, endpoints, feature flags, schema migrations, or startup-wide scans.
- Other listing modes, refactoring, delegation, publication, or broad verification.

## Technical approach

### Bounded runtime projection

Extend `TaskStatusSummary` with optional `HasRunningSession *bool`, wire name
`has_running_session`. Include it in `SemanticJSON` and cloning.
`BuildFromAuthoritative` and live projection use complete per-task session
observations. The predicate ignores primary designation and foreground activity.
False requires complete evidence, including a known empty collection.

Extend `ReconcileTaskStatusSummaries` and its existing CAS repair path to persist
the new flag for legacy requested rows. Preserve revisions, unrelated fields,
semantic activity, cancellation handling, and winner reload after a lost write.
Use existing session restore and `events.SessionRemoved` in the same projector.
Add the removal source to `Projector.Start` and remove the observed session idempotently.

### Query and local parity

Compute a separate running scalar in `sidebarStateFields` and join its keyed
aggregate into the ancestor result using existing source keys, avoiding another
recursive-walk column.
Keep primary session state for existing Status bucketing.
Aggregate the scalar before page selection. Legacy rows use indexed session
`EXISTS`. Page enrichment alone cannot fix ordering before paging.

Preserve explicit false in TypeScript and all `TaskSwitcherItem` projections.
Update `resolveTaskTreeRunning`, not the chain comparator's precedence.
Covered local eligibility checks the new member when Running occurs anywhere
in the chain. A missing flag uses server evaluation.
Generic legacy items retain only their known primary runtime evidence.

Existing summary delivery and cache invalidation carry rank updates.
Cover a secondary start outside the displayed page and a stale HTTP response
after a newer workspace summary event. Do not sort only the returned page.

### Compatibility matrix

| Source / shape | Expected behavior | Evidence |
| --- | --- | --- |
| SQLite / known boolean | Global tree rank from summary before paging | Repository tests and shared fixture |
| PostgreSQL / known boolean | Same rank and bool semantics | Env-gated real database tests |
| Server / legacy or absent summary member | Scoped session `EXISTS` before paging | Legacy first-read repository test |
| Covered local / complete booleans | Same order as server | Shared local/SQL fixtures |
| Covered local / absent member | Server fallback | Coverage/source tests |
| Generic legacy item | Known primary runtime fallback, otherwise unknown/non-running | Utility test |
| Runtime source error | Existing retry/query error, no invented running state | Projector/service error tests |

## ASCII UI preview

### UI-01: Desktop sidebar, Running > Red > Orange > Last activity

Entry point: the existing Tasks sidebar. State: Projects has a running secondary
without a primary. Resume is idle and orange. Neither task is pinned.

```text
Before                           After
TASKS                            TASKS
[running] Other running task     [running] Other running task
[idle]    Resume (orange)        [running] Projects
[running] Projects               [idle]    Resume (orange)
[idle]    Other idle task        [idle]    Other idle task
```

Labels abbreviate the reported titles. Bracketed status labels describe existing
icons rather than new product copy. Running order before color is required.
Spacing is illustrative. The header and existing sidebar scroll owner remain.
The selected task and conversation do not change during rank updates.
Maps to AC .2, .6, .7, .14-.16.

### UI-02: Phone task picker, same saved chain

Entry point: the current task-title picker trigger.
The existing inset Tasks drawer is the mobile exemplar.
Its fixed header, safe areas, scroll body, and row touch actions remain.

```text
Task title [v]
  +-----------------------------------+
  | Tasks                  All tasks v | fixed header
  |-----------------------------------|
  | [running] Projects                | scroll body
  | [idle]    Resume (orange)          |
  | [idle]    Other idle task          |
  +-----------------------------------+
```

Before the fix, Resume precedes Projects in this same drawer.
After the fix, both surfaces use the same task-wide runtime evidence.
The primary row action remains task navigation. The repair adds no controls.
Maps to AC .6, .8, .14-.16. Phone proof opens this real picker with touch input.

## Tests

All named new tests below are proposed implementation deliverables.

| Criteria | Proposed regression and location |
| --- | --- |
| .14 | `TestRunningSessionSummaryMixedSessions` in `statussummary/running_session_test.go`: no primary, idle primary, failed sibling, multiple running sessions, known empty collection |
| .15 | `TestRunningSessionSummaryLifecycleAndRestart` in the same file: secondary start, final stop, deletion, rehydration, unrelated events |
| .7, .15, .16 | `TestReconcileTaskStatusSummariesRunningSessions` in `service/service_status_summary_running_test.go`: legacy repair, stale CAS, partial input, read errors |
| .2, .3, .4, .6, .9, .11-.12, .14, .16 | `TestSidebarTaskWideRunningRank` in `repository/sqlite/sidebar_task_query_running_test.go`: colors, both directions, secondary criterion, filters, collapse, pins, manual children, pre-page legacy rank |
| .6, .14 | Extend `repository/testdata/sidebar-local-conformance.json` and Go/TS harnesses with mixed-session aggregate cases |
| .3, .6, .14 | Extend `task-tree-running.test.ts`, `sidebar-local-view.test.ts`, and `apply-view.test.ts` |
| .6, .9, .16 | Extend `task-overview-coverage.test.ts` and `sidebar-task-source.test.ts` for missing member versus explicit false |
| .7, .15 | Extend `task-status-summary.test.ts` and `hooks/domains/kanban/use-sidebar-task-page.test.tsx` for accepted revisions and off-page invalidation |
| .6, .8, .14 | Extend desktop and phone item-projection tests to preserve true, false, and absence |

The first RED regression seeds a running secondary without a primary and an
idle orange peer. Running-first ordering must put the secondary-running task
first. Current production code puts the orange peer first.

## E2E tests

Extend the existing desktop and phone sort-chain specs and their shared helper.
Keep their existing editor, persistence, grouping, and navigation assertions.
Add the reported no-primary scenario plus a waiting-primary/running-secondary
scenario. Use durable seeded session records rather than fabricated spinner data.

- `tests/task/sidebar-running-first-activity-sort.spec.ts`, project `chromium`.
- `tests/task/mobile-sidebar-running-first-activity-sort.spec.ts`, project `mobile-chrome`.

Map both scenarios to AC .2, .6-.8, .14-.16. Prove initial order, reload,
secondary start/final stop, and preserved active conversation.
Desktop scopes to the real sidebar. Phone scopes to the open Tasks drawer.
Restore saved views and colors in `finally`. Managed runs rebuild all assets.

## Work orders

- [x] [Task 01: Deliver task-wide running rank](task-01-task-wide-running-rank.md) (`completed`)
- [x] [Task 02: Prove desktop and phone order](task-02-browser-proof-and-docs.md) (`completed`)

Order: 01 -> 02. Sequential execution. No delegation is authorized.
Exact commands belong to each work order. Fresh worktrees need
`(cd apps && pnpm install --frozen-lockfile)` before their first pnpm command.

## Verification results

Product implementation, browser proof, builds, and documentation checks passed
on 2026-10-07. The requirement is `active` and its system design is `current`.

- Task 01 backend projection, reconciliation, query, memory, and conformance tests passed.
- Task 01 frontend regression suite passed 15 files and 314 tests; typecheck and
  targeted ESLint passed.
- Task 01 sidebar repository/handler tests passed on SQLite and PostgreSQL.
  PostgreSQL used an isolated disposable PostgreSQL 16 container, stopped after
  the check. Both dual-dialect packages passed.
- Task 02 managed E2E: `chromium` 2/2 and `mobile-chrome` 2/2 passed. Each
  managed run rebuilt the backend and production Vite assets.
- Public documentation tests passed 62/62; all 47 published pages validated.
- Specification catalog, spec-linter tests (36/36), and full spec lint passed.
- `git diff --check` passed.

Review remediation passed on 2026-10-07. HTTP enrichment keeps its initial
batched session and pending-action reads ahead of the optional summary read so
cancellation still stops later reads. Before repairing a summary, it takes a
fresh batched session and pending-action observation after loading that summary
revision. A deterministic handler regression covers both a secondary session
starting and stopping between the initial batch and summary read, and verifies
that the newer running flag is retained. The SQLite running/activity rollup
uses the indexed task-scoped legacy `EXISTS` fallback and keeps explicit
summary booleans authoritative. The original 64 MiB native-memory gate is
restored and passes: the highest running/activity query peak was 64,991,352
bytes in both the empty and 101-task fixtures, with no retained growth.

Design-package validation passed on 2026-10-07 before implementation:

- Catalog validation: 361 decisions and 1425 specifications.
- Specification-linter tests: all 36 passed.
- Full specification lint passed.
- Local `validateCoverage` preflight covered both work orders without errors.
  A proposed runtime source path exercised the implementation coverage gate.
- All package links and exact web test/lint input paths resolve.
- All changes remain unstaged and uncommitted.

## Risks

- A partial restored session map can incorrectly clear a true aggregate.
- A false value omitted by serialization becomes indistinguishable from legacy data.
- Repair after paging alone leaves old secondary-running tasks on the wrong page.
- SQLite and PostgreSQL represent JSON booleans differently.
- A change to primary state projection can alter unrelated Status sorting.
- Combined running/activity sorts can affect native query memory. Retain bounded memory tests.
- PostgreSQL proof requires an isolated database and `KANDEV_TEST_POSTGRES_DSN`.

## Related delivery record

The [original sort-chain package](../sidebar-running-first-activity-sort/plan.md)
records the completed primary-only behavior and its historical test results.
This package supersedes that predicate. It does not rerun or replace that
package's completed work orders. Task 02 extends its existing browser scenarios.

- [Amended requirements](../../specs/ui/requirements/sidebar-running-first-activity-sort.md)
- [Amended design](../../specs/ui/system-design/sidebar-running-first-activity-sort.md#task-wide-running-projection)
- [Decision](../../decisions/2026-10-07-task-wide-running-sidebar-rank.md)
