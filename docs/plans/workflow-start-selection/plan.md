---
created: 2026-10-07
status: in_progress
requirements:
  - REQ-TASKS-WORKFLOW-START-SELECTION-001
system_design:
  - ../../specs/tasks/system-design/workflow-start-selection.md
legacy_specs: []
---

# Implementation plan: Preserve workflow start selection

## Overview

A name-only step update can restore its observed start flag after another
caller changes selection. Deliver one sequential vertical correction: fresh
caller regression, explicit-presence service/repository seam, transaction-owned
flag result, then targeted transport, storage, and compatibility verification.
The [requirement](../../specs/tasks/requirements/workflow-start-selection.md)
is the accepted behavior. ROOT accepted the revised design package and released
implementation through the same primary session.

ROOT's accepted proof uses the production controller, two Services, and one
actual SQLite repository at source base
`a458174b023b33fe16c755bfed762025bf3706a5`. Both omission directions failed on
stored and returned flags; ordinary name-only and explicit controls passed.
The protected proof is read-only and will not be copied or replayed. Its exact
receipts and evidence limitations are in the paired design.

## Scope

### In scope

- Carry presence of `is_start_step` for ordinary controller updates only.
- Preserve nil intent in SQL and return the flag captured by the same transaction.
- Keep explicit promotion/clear, demotion reporting, rollback, and access guards.
- Fresh permanent tests for real callers, projections, independent stores, and
  changed PostgreSQL behavior; exact/full-model controls for the shared helper.
- Bounded native CI execution for the affected regressions, coordinated via ROOT.

### Out of scope

- Other step fields, full writer migration, global admission or event ordering.
- Schema, API, public-documentation, frontend, mobile, browser, or E2E changes.
- Provider, gateway, or task-service production edits; new gateway hub and
  task-creation fixtures.
- Go checks, database fixtures, install, commits, PRs, or release in DESIGN.
- Implementation until a later reviewed-package ROOT interrupt in this primary.

## Technical approach

Follow [Presence seam](../../specs/tasks/system-design/workflow-start-selection.md#presence-seam)
and [Transaction and projections](../../specs/tasks/system-design/workflow-start-selection.md#transaction-and-projections).
Add the two proposed internal entry points and extend the existing private
update helper; keep all existing public full-model/exact signatures. Audit
compile adapters first. No migration, dependency, retry, or new lock framework.

| Surface | Intent / identity | Behavior | Planned evidence | Unsupported input |
| --- | --- | --- | --- | --- |
| REST PUT | Step ID + optional boolean, context authorization | Nil preserves current flag | Registered router test | Existing binding/404/read-only errors |
| Registered WS/MCP update | `step_id`, `*bool`, guarded MCP dispatch | Same controller seam | Registered dispatch and event assertions | Existing guard/validation classification |
| Settings domain patch | Existing `settingsWorkflowController` | Inherits seam | Compile dependency audit | Existing decode/authorization failures |
| Legacy service/repository | Full model | Flag remains explicit | Full-model control | Existing validation/storage errors |
| Exact Host | Full config, workflow + step versions | Preserve both fences | Current/stale/failed-write cases | Existing version conflict |
| SQLite | Actual factory, independent stores | Tx preserves flag before own result | File-backed two-store causal tests | Native error, no retry |
| PostgreSQL | Rebound integer-boolean SQL, READ COMMITTED | Preserve current row after lock wait | Env-gated real multi-connection test | Unconfigured environment is unverified, not PASS |

## Tests

All proposed new tests are grouped under the anchored family
`^TestWorkflowStartSelection...$`. Exact names, files, cases, and commands are
in [Task 01](task-01-preserve-selection.md). There is one regression outcome
per causal scenario, rather than duplicating unchanged suites.

| Acceptance | Required evidence |
| --- | --- |
| `.1` | Controller real omission in both observed directions, independent SQLite stores, PG row-lock wait |
| `.2` | Explicit true/false, latest explicit intent, ordinary nonoverlap controls |
| `.3` | Registered REST and guarded WS/MCP response + exact payload/count assertions through actual `stepevents.Publisher`; unchanged gateway mapper source audit |
| `.4` | Target write failure after demotion, flag-capture failure where practical, access/missing/read-only cases with no events |
| `.5` | Real `ResolveStartStep` selected IDs in both omission directions and actual no-start fallback ID; unchanged task-creation resolver wiring source audit |
| `.6` | Full-model explicit behavior and current/stale exact version fences with rollback |

The full prefix is `AC-TASKS-WORKFLOW-START-SELECTION-001`.

## E2E tests

Use registered backend request-to-persistence-to-publisher integration tests
for this data contract. No new gateway notification or task-creation fixture
executes; their unchanged mapping/routing is supported by the design's source
consumer audit and real resolver selected/fallback ID controls. No Playwright
file/project is added: there is no changed rendered composition or
viewport-dependent behavior. Frontend response transforms and WS consumers are
inventoried in the design; their APIs and layouts stay unchanged.

## Work orders

- [ ] [Task 01: Preserve selection through ordinary step edits](task-01-preserve-selection.md)

## Verification results

Fresh permanent caller omission tests reproduced both causal failures before
production edits. The narrow intent seam then passed those controller tests.
Focused local regressions and exact Host controls passed; fixture corrections
and original terminal results are recorded in Task 01. PostgreSQL 16 intent and
row-lock cases, SQLite/PostgreSQL store conformance, sqlguard and scoped lint
passed. Hosted native Windows and PostgreSQL CI evidence remain pending and
are tracked in the external task plan. The design checkpoint remains historical
evidence.

## Risks

- Nil intent accidentally becoming explicit through a legacy wrapper recreates
  the bug; test the public caller before adding the new method.
- A post-commit read can return another edit's selection; capture on the Tx.
- PostgreSQL behavior must actually run with a disposable configured DSN before
  acceptance; compile-only or SKIP is insufficient.
- Native CI edits share `.github/workflows/backend-tests.yml` with Child71's
  delivery. Coordinate the hunk through ROOT without reading its worktree.
- Heavy and MERGE authority are absent. Later delivery must follow the live
  task plan's resource/observer gates and ROOT's separate grants.
