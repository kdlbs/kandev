---
created: 2026-10-04
status: in_progress
requirements:
  - REQ-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001
system_design:
  - ../../specs/tasks/system-design/subtask-reparenting-drag-drop.md
legacy_specs: []
---

# Implementation Plan: Serialize Validated Task Re-parenting

## Overview

One sequential work order moves authoritative canonical parent validation into a database mutation
boundary shared by competing hierarchy writers. It also prevents full-row task snapshots from
restoring stale parents. The existing [Tasks requirement](../../specs/tasks/requirements/subtask-reparenting-drag-drop.md)
owns the outcome; this is not a second UI or incident specification.

The design turn delivered four uncommitted artifacts and ended before implementation. ROOT later
reviewed them and explicitly released this single order in the same primary session, including
current-state workspace preservation across parent ABA. Implementation is in progress.

## Evidence and assumption check

Starting HEAD is actual main `d5142d9db89fa7afc42a3570eeafb4eb681ca0f0`. ROOT's read-only proof
`/tmp/kandev-concurrent-reparent-repro_test.go` and receipt
`/tmp/kandev-root24-reparent-proof-receipt.json` are accepted without replay. ROOT actually joined
handle 68719, expected exit 1, test 0.270s/package 0.339s: two real service validations reached the
commit barrier while both tasks were roots, then real SQLite writes persisted A.parent=B and
B.parent=A. Ordinary C-under-D and sequential reverse-cycle rejection controls passed. The wrapper
was installed before `NewService`, delegated actual reads/writes and joined both workers.

Verified cause: `validateTaskUpdateReferences` invokes `resolveParentID` before either commit
transaction; `checkParentCycle`/`validateReparentDepth` therefore decide on stale graphs. Row locks
on distinct subjects cannot prevent a shared cycle. `GetTask` uses the read pool, and common plus
two separate workflow SQL writers can overwrite parent from an earlier snapshot. Creation has the
same pre-insert depth window. The existing workspace-row lock establishes the local PG precedent.

Confirmed scope includes Office policy preservation, current depth/error semantics, workspace
continuity, real transport/store evidence, and no rendered changes. No material question blocks
planning. There is no authorization for workers, new tasks/sessions/tabs, or a model switch.

## Scope

### In scope

- Transaction-bound hierarchy reads and final parent admission, plus a locked early preflight that
  preserves prior23's validation-before-entity-preparation order.
- Optional parent presence through canonical REST/WS, explicit position, ordinary/exact workflow
  snapshots, and current hierarchy-owned workspace normalization.
- Creation-depth/parent-existence insertion admission, detach, direct-child promotion, guarded
  restoration, archive eligibility, and final ordinary parent-deletion conflict.
- Minimal shared-lock participation by Office's scalar parent writer with unchanged policy.
- Real SQLite/PG/service/registered REST/WS regressions, changed-scope lint, SQLguard and task/Office
  store conformance, resource cleanup and durable results.

### Out of scope

UI controls, copy, layout, browser/build/app/full E2E; new launch/runner gates; project/Office policy
changes; F19 or prior23 association redesign; schemas, repair migrations and generic transaction
frameworks; specialized legacy deletion redesign; global event revision/order guarantees. The
design's exact residual-scope section governs Office cycles, arbitrary bulk reparent/direct SQL,
historical shapes, archived-child revival, specialized ephemeral deletion and cross-workspace
Office workspace-cascade links. Do not report universal hierarchy safety.

## Technical approach

Follow the paired design's database boundary, request-presence and participation table. Use narrow
task admission/reader capabilities in the `task/repository/hierarchy` leaf, aliased by the parent
repository package; service parent validation
must use that transaction reader, never `s.tasks.GetTask` on the read pool inside admission.
Preserve unrelated full-row fields and existing receipt/fence/position/deferred-launch branches.
Short early admission is released before provider/find-or-create preparation; final admission
rereads and validates before all task fields commit. Do not hold a transaction over provider I/O.

Reuse PG workspace rows with READ COMMITTED and workspace → sorted steps → task order; use SQLite
writer-before-read across independent handles. Introduce only a domain-specific shared `db` helper,
not new schema or process-only locking. Keep ordinary and Office validation policies separate.

| Store / transport | Intended boundary | Evidence |
| --- | --- | --- |
| SQLite, independent handles | First transactional statement reserves writer; transaction reads current graph | Held writer plus delegated actual operations, two/three-node/depth and stale snapshot tests |
| PG, independent connections | Workspace-first lock; current reads after observed wait | Worker-PID-scoped `pg_locks`/`pg_stat_activity`, release/join and committed graph assertions |
| Registered REST PATCH and WS `task.update` | Preserve optional parent and position, typed rejection, committed result | Real route/dispatcher and DB; no fabricated business decisions |
| Exact ordinary/workflow updates | No parent intent; preserve relationship, retain version/replay/fence | Real operation/version and stale-snapshot controls |
| Office dashboard/scalar store | Shared serialization, existing direct-self/existence policy | Deeper cycle deliberately remains accepted; canonical Office-shaped cycle rejection remains |

Unsupported admission capability fails closed on canonical parent mutation. Historical raw data
and explicit residual writers cannot be cited as covered by transport/store sharing.

## Alternatives and local decision record

A per-Service/process mutex misses independent services/connections; subject-row locks miss
different subjects; final-write-only validation against the read pool misses current-read
isolation. Global serializable transactions would add retry semantics across unrelated work.
A workspace advisory lock for every operation would duplicate the already established workspace
row boundary and require broader lock-order changes. Choose the existing workspace row boundary
with a small empty-workspace fallback and SQLite write reservation. The cost is workspace-wide
PG contention and SQLite's existing database-wide writer contention. This local correction and
its alternatives are fully recorded here and in the owner design; no additional ADR is needed.

Changing Office's deliberate deeper-cycle policy would create a different product contract.
Office participates only in serialization/existence rechecking, so canonical changes are valid
at their boundary while a subsequent permissive Office mutation may still create a cycle.

## Companion packages and documentation

Preserve the statuses/history of `subtask-reparenting-drag-drop`, `fix-nest-under-candidates`,
`subtask-detachment`, detached workspace continuity packages and prior23
`atomic-task-repository-replacement` (the checkout retains its historical in-progress status).
Do not reopen their work orders or change their UI/E2E receipts. Existing durable creation/cascade
and guarded-retirement designs remain linked dependencies, not new implementation scope.

Mobile-parity assessment: backend/shared data only; existing desktop/phone request/rollback paths
remain. No ASCII preview or Playwright/browser/build is appropriate. Public-guide audit read
`docs/public/tasks-and-workflows.md`, `docs/public/websocket-api.md`, README and screenshots. The
design turn changes internal intent only. On implementation add at most one short paragraph in the
existing task guide explaining a rejected concurrent move/delete and refresh/retry if needed.

## Tests

The [work order](task-01-hierarchy-admission.md#verification) owns exact commands and the complete
AC-to-real-test matrix. Permanent RED first exercises canonical overlapping moves, then store
writer ordering and stale snapshots. GREEN covers two/three-node races, child-create depth,
parent archive/delete, bounded malformed ancestry, detach/compensation, omitted/empty/same parent,
position, metadata, groups, sessions, exact operations and Office policy compatibility.

Registered backend REST/WS tests provide end-to-end evidence for the changed data behavior;
mock-only handler tests, pure predicate tests or browser suites cannot replace them.

## Work orders

- [ ] [Task 01: Admit canonical hierarchy mutations against current database state](task-01-hierarchy-admission.md)

One order, wave 1, sequential; no delegation. Keep the store/service/transport integration coherent
before reporting GREEN or changing its status.

## Verification results

Design checks passed on 2026-10-04: catalog (347 decisions/1332 specifications), all-spec lint,
10 local Markdown link targets, prospective package-reference coverage (one accepted order), and
whitespace. Exactly four artifacts are unstaged/uncommitted; existing active/current pair statuses
and historical packages are preserved. These receipts describe the completed design turn. Later implementation receipts are recorded
in the work order; affected service/registered transports, SQLite/PG stores, cancellation, Office
policy and targeted task/Office conformance passed. SQLguard passed. Changed lint, ordinary hooks,
publication/review/CI/actual merge and exact owned cleanup remain delivery gates.

## Risks

- Incorrect first-read ordering or using `r.ro` inside admission leaves the race open.
- Taking graph/workspace after step/task locks risks PG deadlock; begin-before-process-arrival-lock
  risks SQLite single-pool deadlock.
- Blanket metadata preservation could change ordinary replace semantics; preserve only current
  hierarchy-owned mode/group from current shared_group state even across parent ABA; unrelated keys retain replacement/deletion semantics.
- Late-child deletion conflict must retain the parent and surface retry, preserving cascade choice
  and existing partial cleanup/compensation semantics.
- Two-phase preflight may pass before a later competing commit; only final locked validation is
  authority. Prepared shared repository entities follow the existing idempotent contract.
- Office and specialized legacy writers remain explicit policy/residual exceptions.
