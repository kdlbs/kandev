---
created: 2026-10-04
status: in_progress
requirements:
  - REQ-TASKS-FIELD-UPDATES-001
system_design:
  - ../../specs/tasks/system-design/task-field-updates.md
legacy_specs: []
---

# Implementation plan: Preserve concurrent task metadata merges

## Overview and checkpoint

Apply explicit metadata merge intent to canonical current metadata and write only metadata and
its timestamp. One coherent sequential work order owns the required interface, real storage,
service, registered REST proof and compatibility/error/effect controls. It first demonstrates
behavioral RED through existing APIs, then adds the canonical operation and validates both SQL
dialects. ROOT reviewed this package and explicitly released IMPLEMENTATION to this same primary
on 2026-10-04 after the completed DESIGN turn ended WAITING. Implementation and local acceptance
checks are underway; actual merge, review and joined cleanup remain delivery gates.

Tasks owns persisted task intent. Extend the existing
[task field update requirement](../../specs/tasks/requirements/task-field-updates.md) and
[design](../../specs/tasks/system-design/task-field-updates.md), with criteria .8 through
.12. Criteria .1 through .7 stay the prior ordinary-patch contract. The amended pair is active/current after the real SQLite/PG contract
checks; delivery remains in progress. Do not reopen the historical
[ordinary-field delivery package](../concurrent-task-field-edits/plan.md) or rewrite its results.

## Confirmed cause and retained evidence

Authoritative base and local HEAD are `93c80f75454f11a60c3e8458d70b3148194d7dac`.
`Service.UpdateTaskMetadata` merges supplied keys into an early task and calls
`UpdateTaskPreservingDeferredLaunch`, which writes stale scalar and ordinary metadata state.
DB serialization of that snapshot does not preserve intent. ROOT's accepted proof used two
independently constructed Services and SQLite connections, gated real initial `GetTask`
observations before ordered API commits, and joined every worker/cleanup.

Read only, never replay or delete: `/tmp/kandev-metadata-merge-repro_test.go`, SHA256
`bd1386f6930e080ed7cf65d420c6ff75666d3ba7e32982ac11b3c227179755ef`, and
`/tmp/kandev-root26-metadata-merge-proof-receipt.json`. Actual handle 53707 joined exit 1;
`TestRootConcurrentMetadataMergeProof` .29s/package .354s. Both metadata commit orders lose the
first disjoint intent; title-first then stale merge reverts title to Original. Reverse title
order, sequential positive controls, unrelated fields/key, successful APIs and publication
controls pass. No race or compilation failure. Temporary repository reproduction removed.
Permanent meaningful RED after release is a separate test, not a replay of that archive.

## Scope and technical approach

The design inventories interfaces/dependencies, all production callers and participating/excluded
writer families before choosing the seam. Add required `TaskRepository.MergeTaskMetadata`; its
concrete operation reserves SQLite's writer before reading and uses the backend-required PG
task-scoped advisory plus physical task-row lock. Read current raw metadata, protect existing
owners/workspace identity, apply all supplied ordinary keys and timestamp in one transaction.
Reuse current pending-title dialect semantics for supplied values. Publish a postcommit reread.
Do not call full-row or workflow-effect writers for a metadata-only request.

The only direct production entry point is registered REST port preference. GitHub's similarly
named store adapter uses ordinary replacement intentionally; unlink deletes by omission. Leave
that adapter, wire DTOs, WS registrations, scalar/CAS writers and intentional full snapshots
under their existing contracts. No general per-key framework, schema/revision/event order,
runner/Office/cascade/launch-policy/harness/runtime redesign. A newly found causal boundary
requires ROOT amendment; optional polish does not expand scope.

## Tests and acceptance mapping

The test matrix below is implemented. Actual outcomes and remaining gates are recorded below.
Full scenario requirements and exact commands are
in [Task 01](task-01-canonical-metadata-merge.md). Tests assert actual stored data, return/DTO
and events/effects; helper calls, mocked predicates, elapsed time and compile failure are not
behavioral evidence.

| Criteria | Required proof |
| --- | --- |
| .8 | `TestTaskMetadataMergeConcurrentSQLite`: two real Services/independent handles, real snapshot gates, both orders, same-key and sequential controls |
| .9 | `TestTaskMetadataMergeOrdinaryFieldInteraction` and `TestTaskMetadataMergeReplacementControls`: title/ordinary omission/native scalar interplay, both title orders, explicit replacement and full-snapshot exclusions |
| .10 | `TestTaskMetadataMergeOwnersAndValues`, `TestTaskMetadataMergeSQLiteValues`: null/empty/nested/pending dialect matrix, caller maps, large numbers/literal keys, sanctioned owner CAS and materialized workspace identity |
| .11 | `TestTaskMetadataMergeEffectsAndFailures`, `TestTaskMetadataMergeCurrentTransitionAndAssociations`, `TestTaskMetadataMergeSQLiteAtomicity`: unchanged scalar/associated/runner/ledger/entry data, real encoding/storage/cancellation failures and typed errors |
| .12 | `TestTaskMetadataMergeRegisteredPortForwarding`, `TestTaskMetadataMergePostcommitObservation`: actual registered route, ordinary PATCH interaction, DB/DTO/event payloads and read-after-commit failure controls |
| .8 to .11, PG boundary | `TestTaskMetadataMergePostgresPhysicalWait`, `TestTaskMetadataMergePostgresValues`: actual independent physical row wait/current merge, row-only holder, all-keys/owner/value/cancellation controls |

All criteria above are `AC-TASKS-FIELD-UPDATES-001`. Existing ordinary field, hierarchy/parent
ABA, atomic associations/F19, title/deferred/handoff/carrier and protocol controls are retained
in the narrow commands. They provide regression assurance for unchanged contracts, not proof
of the new merge seam. PG SKIP is recorded as SKIP and never as physical-lock proof.

## Mobile and public documentation audit

`mobile-parity`: backend state/data-only correction; no UI/copy/layout/touch/scroll/navigation,
breakpoint, frontend state or mobile composition change. Existing desktop/phone DTO/events remain
identical. Actual registered protocol integration provides causal end-to-end proof; no browser,
web build or E2E planned absent a new causal finding and ROOT amendment.

`docs-maintainer`: reviewed `tasks-and-workflows.md`, `websocket-api.md`, root README and screenshot
catalog. The API reference already distinguishes ordinary metadata replacement from partial field
updates but lacks this REST merge endpoint clarification. At implementation add one short reference
paragraph next to partial updates covering the existing port preference, current merge and retained
replacement/full-snapshot exclusions. No new page, screenshots, navigation or UI strings. The implementation adds that paragraph without new public surfaces. `/record` audit: existing native persistence ownership plus the
explicit contract/matrix preserve rationale; no independent ADR warranted.

## Work orders

- [ ] [Task 01: Apply metadata merge intent atomically](task-01-canonical-metadata-merge.md)

Dependencies: existing reviewed main/base only; one sequential work order, no delegation.

## Verification results

DESIGN gates passed on 2026-10-04:

- `python3 scripts/list-docs.py validate`: exit 0, 347 decisions and 1334 specifications.
- `python3 scripts/lint-spec-files.py --all`: exit 0, all specification files passed.
- `python3 scripts/lint-spec-files.test.py`: exit 0, 36 tests passed.
- Filesystem local-link check: exit 0, 18 links across all four package files.
- `.github/scripts/pr-docs.cjs` exported `validateCoverage`: exit 0, covered, no errors,
  reading these unstaged documents with an explicitly planned runtime trigger. This is delivery
  reference preflight, not current-head CI or implementation evidence.
- `git diff --check`: exit 0; status shows only the owning pair and untracked new plan directory.

IMPLEMENTATION results so far on 2026-10-04:

- Separate permanent existing-API SQLite RED: both metadata orders and title-first fail on stored
  data; reverse-title and same-key sequential controls pass. The accepted ROOT proof was not replayed.
- Service selection in Task 01 passed with race detection (package 2.456s). Later focused current
  transition/association check passed (1.320s): actual committed completed state, workflow step,
  ledger/entry and populated Git/folder attachments survive a delayed merge.
- Registered REST concurrency/ordinary PATCH, DB/DTO/event checks and prior selected handler
  regressions passed in the 3.486s run; only new auth fixture setup failed. Constructor-owned
  workspace creation correction passed the affected auth/error check (1.266s). It retains
  viewer 403, foreign/missing 404, bad-body 400 and no mutation/publication.
- Actual PG16 independent-connection matrix passed (17.007s), including both row-only metadata
  orders, title/owner/cancel physical waits observed by pg_locks and pg_stat_activity/blocking PIDs,
  twelve value/owner/rollback cases and all eight previous ordinary-field physical waits. No SKIP.
- SQLite values/owners/atomicity and held-writer cancellation passed; a test observation of SQLNULL
  initially used the legacy scanner before normalization. Moving the scalar baseline observation
  before deliberate raw NULL assignment fixed that fixture and the focused case passed (1.205s).
  The driver lock-error after cancellation is normalized to the context cause by the new operation.
- Required SQLite regression passed (8.838s); SQL guard passed. Public documentation validator
  passed (47 pages) and its 62 tests passed. Catalog/spec/whitespace gates passed.

Required PostgreSQL-enabled storeconformance passed (101.547s). Executor fake compilation
passed (1.060s, no tests). All DB clients joined and the exact owned fixture/credentials were
removed; the pinned pnpm9.15.9 frozen install passed without lockfile changes. Original full
changed-backend lint over `./...` passed with zero issues at immutable base
`93c80f75454f11a60c3e8458d70b3148194d7dac`, concurrency 2, 10-minute CLI/11-minute outer bound,
GOMAXPROCS 2 and GOMEMLIMIT 1GiB. The first run reported only staticcheck QF1008 on an embedded
selector in the new auth fixture; that narrow correction passed the affected auth test (1.229s)
and the required lint rerun (17.378s). No resource retry or passing broad-suite replay.
Normal active hooks, publication/current-head full review/terminal policy gates and actual merge
remain pending. The external Kandev task plan owns exact handles, failed-fixture diagnostics,
private PG ownership/credential receipts, current-operation and delivery evidence. All criteria
are mapped to data/effects rather than helper counts. No UI/browser/build/E2E causal scope.

## Risks

- Pending SQLite recursive null/nested behavior differs from PG shallow behavior; no uniform
  semantics or nested-key concurrency guarantee is advertised.
- New required method must forward through real wrappers and adapt compile-time fakes without
  accidentally intercepting the real merge regressions.
- PG row locking must read the committed row after waiting, without taking workflow/workspace
  locks afterward; SQLite must reserve before reads across independent handles.
- Later ordinary replacement/full snapshots may intentionally replace merged keys. Postcommit
  responses/events may observe a later commit; postcommit read errors do not undo the mutation.
- Resource failure, hosted leaf failure or new causal scope leaves the task WAITING for bounded
  ROOT direction, never an automatic retry or redesign.
