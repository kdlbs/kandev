---
id: "01-hierarchy-admission"
title: "Admit canonical hierarchy mutations against current database state"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001
acceptance_criteria:
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.3
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.4
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.6
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.7
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.8
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.9
  - AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.10
system_design:
  - ../../specs/tasks/system-design/subtask-reparenting-drag-drop.md
---

# Task 01: Admit canonical hierarchy mutations against current database state

## Summary

Make every authoritative canonical parent/depth decision on current transaction-bound data under
shared hierarchy admission, then commit without releasing admission. Preserve parent request
presence and current hierarchy state in other full-row writers. This single order includes the
necessary dialect, service, store, Office participation and real registered transport evidence.

## In scope

- Refactor canonical parent helpers to a transaction-bound hierarchy reader with existing
  projections and active-child filtering. Use visited IDs plus the existing 1000-step bound.
- Add narrow repository admission/preflight capabilities and one shared domain `db` lock helper.
  No SQL connection/transaction escapes to service code. Preflight reserves admission before its
  first graph read and releases before provider preparation; final commit reserves, rereads,
  validates, applies explicit parent intent and commits through existing full-row machinery.
- All three insert variants need final current-parent/depth admission, including capacity and
  feeder placement. Use a task-specific creation admission capability or the existing internal
  insert helper, passing the current creation validator explicitly; do not use a context-carried
  generic transaction or silently strengthen trusted import/fixture policies. The service's early
  depth check must use the same locked reader. Preserve parent-based Office exemption and existing
  cross-workspace/archive creation behavior.
- Four ordinary full-row variants, exact update, all common workflow-admission variants,
  `MarkDeferredMoveAppliedForSession` (workspace/current step before existing queue/task guards), and two
  standalone capacity/promotion SQL writers preserve current parent and normalized workspace mode
  unless a canonical admitted request explicitly changes it. Cover parent ABA (A → B → A) with
  stale ordinary/exact/workflow snapshots: current shared_group normalization and materialized
  identity/group must survive despite equal parent IDs. Derive preservation from current
  hierarchy-owned workspace state, while unrelated metadata replacement/deletion remains.
  Keep unrelated full-row fields,
  metadata omission/deletion, position and per-branch deferred-launch semantics.
- Reserve admission before reads in detach, both direct-child reparent methods, guarded parent
  restoration, archive/unarchive methods and ordinary deletion, as enumerated by the owner design.
  Workspace deletion reuses its row-lock boundary with SQLite writer-first inventory.
- Ordinary final deletion refuses all remaining structural children with a typed conflict (HTTP
  409 and existing WS error envelope). No-cascade promotion stays workspace-scoped and clear-to-root;
  compensation validates restored edges under admission before writing. Existing cleanup and
  retry remain outside short transactions. Do not publish failed deletion/restoration success.
- Office scalar parent updates share the helper, sorted subject/target workspace locks and
  transaction-bound existence recheck. Preserve deliberate deeper-cycle/depth permissiveness,
  dashboard direct-self rejection and clear-parent routing to canonical detach.
- Track all changed callers/test doubles necessary to compile the affected packages. Narrow real
  store regressions are the evidence; mocks only adapt interfaces. Update scoped backend guidance
  if the parent-preservation repository contract needs a concise clarification.

## Out of scope

No runner gate, launch policy, project classification, F19/prior23 association changes, physical
workspace reprovisioning, global patch/transaction framework, schema/migration, UI/copy/layout,
browser/app/build/full E2E, unrelated CI expansion, or specialized ephemeral deletion redesign.
Keep every residual in the paired design explicit. Do not claim raw/Office storage is universally
cycle-free. Canonical empty `parent_id` remains its update path, while dedicated/Office clear-parent
detach retains its existing ownership-transfer semantics; do not silently redirect one to the other.

## Acceptance

1. Real overlapping canonical two/three-node and child-create/move submissions cannot jointly
   bypass their existing cycle/depth rules. SQLite independent handles reserve a writer before
   graph reads; PG independent connections demonstrate observed workspace-lock waits and read the
   released holder's committed graph. No process-only or different-subject-row-lock solution.
2. Omitted parent and ordinary/exact/workflow snapshots preserve concurrent parent/normalization;
   explicit empty/same/effective-change behavior, errors, other fields, groups/materialized
   workspace and sessions remain correct. Invalid/losing canonical mutations leave exact rows and
   associations unchanged and emit no success; compensation cannot restore an invalid edge.
3. Registered REST PATCH and WS dispatcher tests prove both position branches, request presence,
   committed valid mutations and existing rejection envelopes/events on real DB data. Office
   compatibility proves its deeper-cycle policy remains permissive while canonical Office-shaped
   updates still reject cycles. Ordinary late-child deletion retains the parent, preserves the
   cascade choice, and returns a conflict that can be retried.

## Test traceability and targeted names

The permanent test names below are implemented selectors. Results distinguish selected passing
cases from skipped or unmatched cases; neither a zero-test match nor a skipped PG case is evidence.

| Criteria | Permanent real-data tests / controls |
| --- | --- |
| .3, .4, .6, .10 | `service/service_hierarchy_admission_test.go`: `TestTaskHierarchyAdmissionConcurrentMoves` (two/three nodes, Kanban and canonical Office-shaped), `TestTaskHierarchyAdmissionCreationDepth`, existing `TestService_UpdateTask_*` (self/missing/archive/workspace/cycle/depth, ordinary nest, sequential reversal, mixed Office depth), `TestTaskHierarchyAdmissionMalformedAncestry` (unrelated cycle, bounded visited-node walk, positive missing ancestor and real malformed ancestor decode failure). |
| .6, .8, .9 | `repository/sqlite/task_hierarchy_admission_test.go` and `task_hierarchy_admission_postgres_test.go`: `TestTaskHierarchyAdmissionSQLiteIndependentHandles`, `TestTaskHierarchyAdmissionSQLiteWriterBeforeRead`, `TestTaskHierarchyAdmissionPostgresWaits`, `TestTaskHierarchyAdmissionPostgresCancellation`, `TestTaskHierarchyAdmissionCreateMove`, `TestTaskHierarchyAdmissionTargetLifecycle`, `TestTaskHierarchyAdmissionDeletionCompensation`, `TestTaskHierarchyAdmissionDeferredMarker`, `TestTaskHierarchyAdmissionPromotionReadFailure`, `TestTaskHierarchyAdmissionSnapshotEncodingFailure`, `TestTaskHierarchyAdmissionPromotionEncodingFailure`, `TestTaskHierarchyAdmissionLegacyMetadata`, `TestTaskHierarchyAdmissionLegacyMetadataPostgres`. Observe own PID wait before releasing PG holder; join both workers; verify rows after commit and cancellation. |
| .7, .8 | Store/service `TestTaskHierarchyAdmissionSnapshotPreservation`, registered REST/WS request-presence controls, `TestDetachTask*`, `TestExactTask*`, `TestUpdateTaskIfWorkflowStepHasCapacity*`, `TestPromoteQueuedTaskIfWorkflowStepHasCapacity*`: stale rename/metadata/workflow/capacity/promotion snapshots, nil/empty/same parent, explicit position, complete field/association/group/session controls, exact version conflict/receipt replay, normalization and failure rollback. |
| .6 through .10 | `handlers/task_hierarchy_admission_test.go`: `TestTaskHierarchyAdmissionRegisteredREST`, `TestTaskHierarchyAdmissionRegisteredWS`; use `RegisterTaskRoutes` and actual dispatcher invocation, real service/store/event bus. Concurrent reversal plus sequential validation/request-presence/position controls, no loser success event. `TestTaskHierarchyAdmissionDeleteConflictMapping` complements registered mutation coverage with an actual structural-child store error wrapped through the Gin 409 mapper. No direct private-handler-only substitute for registered parent mutation coverage. |
| .10 | `office/dashboard/service_hierarchy_admission_test.go` / `office/repository/sqlite/task_hierarchy_admission_test.go`: `TestTaskHierarchyAdmissionOfficePolicy` and `TestTaskHierarchyAdmissionOfficeSerialization`. Deeper cycle still accepted by dashboard/scalar policy, direct-self/missing rejected, same-parent normalization unchanged, shared lock waits and existence recheck; do not replace policy checks with a mock answer. |

Preserve and run existing `TestService_UpdateTask_*`, `TestDetachTask*`, `TestExactTask*`,
`TestTaskRepositoryReplacement*`, `TestUpdateTaskParentID*`, `TestReparentDirectChildren*`,
`TestListChildren*`, `TestListSiblings*`, the exact ordinary-delete selectors below, `TestArchiveTask*`,
`TestUnarchiveTask*` and scoped source-step/workspace-cascade PG lock-order controls. If touching
another permanent test suite, include its exact affected selectors here before completion.

Coordination is bounded and installed before service construction/workers start. Barriers delegate
all real reads/writes, or hold an actual SQL transaction/trigger; no replacement business
predicates, manufactured outcomes, mutable service-field races, sleep-only scheduling or assertion
weakening. Capture the meaningful failing assertions on the old code, then implement the minimal
solution and run the GREEN matrix once. ROOT proof files remain read-only.

## Verification

Design turn runs only catalog/spec/reference/whitespace checks. The following product commands
run only after explicit implementation release. Run **one heavy command at a time** from repo
root, actually joining every returned session ID before starting another. Do not repeat passing
checks without a new change/failure/unresolved concern.

The first RED is the focused permanent service regression, followed by required store/transport
RED cases as added; these commands fail on expected assertions, not compile errors:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -timeout=5m ./internal/task/service -run '^TestTaskHierarchyAdmissionConcurrentMoves$' -count=1 -v)
```

After implementation, with PostgreSQL fixture DSN exported as `KANDEV_TEST_POSTGRES_DSN`, execute
the affected matrix once. All commands have a five-minute Go deadline and resource caps:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -timeout=5m ./internal/task/service ./internal/task/handlers -run '^(TestTaskHierarchyAdmission.*|TestService_UpdateTask_.*|TestDetachTask.*|TestExactTask.*|TestTaskRepositoryReplacement.*|TestCreateChildTask.*|TestCreateTask_Subtask.*|TestDeleteTaskTree_NoCascade.*|TestDeleteTaskTreeRestoresReparentedChildrenAfterRootDeleteFailure|TestDeleteTaskTree_IncludesArchivedDescendants|TestHTTPUpdateTask.*|TestWSUpdateTask.*)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -timeout=5m ./internal/task/repository/sqlite -run '^(TestTaskHierarchyAdmission.*|TestTaskRepositoryReplacement.*|TestReparentDirectChildren.*|TestListChildren.*|TestListSiblings.*|TestDeleteTask(NotifiesQueuePurgeAfterCommit|ClearsPromptSequenceBeforeSessionIDReuse)|TestArchiveTask.*|TestUnarchiveTask.*|TestUpdateTaskIfWorkflowStepHasCapacity.*|TestPromoteQueuedTaskIfWorkflowStepHasCapacity.*|TestPostgresRepository_DeleteTask_.*|TestPostgresCrossStepMoveLocksSourceStepAgainstConcurrentReorder|TestPostgresAdmissionRechecksStaleSourceBeforeMoving|TestPostgresWorkspaceCascadeDeleteLocksStepAgainstConcurrentReorder|TestUpdateTaskWithWorkflowStepAdmissionForDeferredMove.*)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -timeout=5m ./internal/office/dashboard ./internal/office/repository/sqlite -run '^(TestTaskHierarchyAdmission.*|TestUpdateTaskParentID.*)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags=fts5 -race -p=1 -timeout=5m ./internal/persistence/storeconformance -run '^TestStoreConformance$/(sqlite3|pgx)/(task|office)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go run -trimpath -tags=fts5 ./cmd/sqlguard ./internal)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Inventory actual existing deletion/creation/capacity/archive test names before implementation
and refine prefixes if they do not select an affected suite. If five-minute package grouping
needs a causal split, document it and run serial exact selectors; a timeout is failure, never a
passing zero-issues result. Do not broaden to all backend suites or rerun to hide a resource failure.

Changed lint uses the exact PR comparison base, concurrency 2 and allow-serial-runners, CLI 5m,
GNU timeout 6m/kill10s, GOMAXPROCS=2/GOMEMLIMIT=1GiB. At execution record the actual immutable
comparison base as `KANDEV_HIERARCHY_PR_BASE` and expand only the concrete changed packages:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev="$KANDEV_HIERARCHY_PR_BASE" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Backend PR fixups require the full `./...` changed-revision gate before pushing, in addition to
ordinary changed-package hooks. This filters diagnostics to the immutable PR base; no unrestricted
lint-all-issues run or unaffected product suite is implied. Keep active ordinary hooks. If `apps/node_modules` is missing, perform one pinned pnpm 9.15.9
frozen install from `apps/` before hooks; no install is authorized for this design turn.
No automatic resource retry, cache wipe or foreign process kill.

PR documentation reference preflight calls local `.github/scripts/pr-docs.cjs` `validateCoverage`
on changed paths and referenced file contents, with no GitHub publication. Public-doc validators
run only if the small public-guide paragraph is changed. Record each required command, exit code,
selected tests and real PG results here; skips are not PG completion evidence.

## PostgreSQL fixture ownership and resource gates

No fixture is started in the design turn. After release, create one owned fixture named
`kandev-child24-hierarchy-pg` using the repo-pinned Postgres 16 image from
`.github/workflows/ci-base-image.yml`. Before running tests record its exact container ID, image
digest, task/session ownership labels, host binding and **all** mounts/volumes in the task plan.
Prefer tmpfs mounted at the image data directory (including PGDATA), or a private owned bind; the
image's declared data volume must not silently allocate an unrecorded anonymous volume. Bind the
chosen port to loopback only. If the name is occupied by a foreign container, leave it alone and
choose a recorded unique child24 name.

Use isolated schemas via the repository PG fixtures; keep worker/observer handles independent,
record worker PIDs, and actually observe their lock wait before release. Every worker/holder and
test process must be joined. After checks, remove only the exact owned container and any explicitly
owned volumes/private data, verify container/volume/data absence and schema cleanup, and record
the result. The old unproved child20 dangling volume and ROOT proof are untouched.

## Files likely touched

- `apps/backend/internal/db/taskhierarchy.go` (new narrow helper) and `taskhierarchy_test.go`;
  current `tasklock.go` only if its documented scope needs clarification.
- `apps/backend/internal/task/repository/interface.go`, the narrow `hierarchy.go` aliases and
  `hierarchy/` reader/admission/policy leaf (avoids the provider import cycle), `repoerrors/errors.go`,
  `sqlite/task.go`, `sqlite/task_cleanup_barrier.go`,
  `sqlite/task_hierarchy_admission.go` (new), `sqlite/workspace.go` and corresponding focused tests.
- `apps/backend/internal/task/service/service_tasks.go`, `service_child_task.go` only for required
  routing, `service_reparent_test.go`, new `service_hierarchy_admission_test.go`, and affected
  snapshot/detach/cascade/exact tests. `service_exact.go` only if its preserving caller needs change.
- `apps/backend/internal/task/handlers/task_handlers.go`, `task_http_handlers.go`,
  `task_ws_handlers.go`, `errors.go`, new registered real-data `task_hierarchy_admission_test.go` and
  necessary interface mock adapters.
- `apps/backend/internal/office/repository/sqlite/tasks.go`, new focused hierarchy tests, and
  `office/dashboard/service_tasks.go`/tests only for necessary interface/error/event wiring.
- The owning requirement/design, this manifest/order, and at most a concise scoped `AGENTS.md`
  or existing public task-guide clarification warranted by implementation.

## Dependencies and inputs

No prior work order. Implementation release is mandatory. Read the owner design's admission,
participation, request-presence and residual sections; retain current active/current spec statuses.
Use existing `service_reparent_test.go`, `task_purge_race_postgres_test.go` independent connections,
`workspace_cascade_lock_postgres_test.go`, `RegisterTaskRoutes`, dispatcher registration and
`office/dashboard/service_detachment_test.go` as patterns. Read backend/Office scoped guidance.
Prior23 repository-set finalizer, runner F19 and exact-retirement boundaries remain untouched.

## Risks

Workspace-first lock order must hold at every participating transaction entry. A callback that
reads the ordinary pool or performs nested writer preparation will invalidate the solution or
deadlock SQLite. Snapshot hierarchy protection must not become blanket metadata preservation.
Office policy and specialized legacy deletion residuals limit the safety claim. Late-child
ordinary deletion conflicts preserve the parent but may follow partial existing cleanup; retain
and prove compensation/retry behavior without broadening deletion scope.

## Parallelism

`sequential`. Same primary Sol6.1 session; no workers/tasks/sessions/tabs/model switch.

## Later delivery and completion gates

After task-defined checks, ordinary hooks, committed/pushed branch and ready PR are already
authorized by the standing loop; use the repository delivery skills then. Freeze SHA absent an
actual correction. Publish actual corrections promptly without waiting for obsolete-head CI;
stop/join only owned monitors if replacement is necessary. One practical 90-minute allterminal
`scripts/pr-await` must be actually joined, with no manual timer polling or duplicate reviewer
requests. Authenticate configured CodeRabbit App 347564 full current-head/all-file substantive
coverage and disposition every inline/grouped finding. No optional second review or ACK substitute.
Notify ROOT before unrelated CI expansion/retry.

Task completion requires normal expected-head squash and independent actual merged SHA, parent,
tree, all owned blob and remote-inclusion receipts plus joined exact owned cleanup. Retain the
clean managed worktree/dependencies for ROOT archive. Child callbacks to ROOT are queued; ROOT's
later implementation release is an interrupt. Persist phase, IDs/system marker, routing, handles,
results, recovery and next action in the existing task plan without replacing user edits.

## Results

Design checkpoint (2026-10-04): catalog validation passed (347 decisions, 1332 specifications);
all-spec lint passed; local Markdown link-target check passed (10 links); documentation coverage
reference preflight passed with the planned canonical service path as prospective input (covered,
one accepted order/design/requirement). This is reference validation, not a production diff or
product test. The initial Node command was unavailable on PATH; the existing NVM Node 24.18.0
runtime ran the preflight without installation. Whitespace and exact-four-artifact checks passed.

Implementation released by ROOT after independent review of all four artifacts. Task01 is
in_progress in the same primary session; ABA amendment applied before permanent tests/production
edits. Design-checkpoint receipts above remain historical. At design checkpoint: no permanent tests or production edits, product checks, fixture, install,
commit, push or PR. Await later explicit reviewed implementation release.


Implementation verification (same primary, all listed handles actually joined): permanent RED37053
exit 1 exposed two/three-node cycles and all four stale ABA variants; RED1674 exit 1 exposed
creation-depth and late-child deletion races. Initial GREEN42047 was lost on capacity interruption,
recorded LOST/INTERRUPTED with no verdict after checking no live owned descendants; one exact
continuation59868 passed. Final affected service/registered REST+WS19684 exit 0 (3.875s/1.671s),
store75351 exit 0 (12.504s), cancellation95813 exit 0 (1.576s), Office dashboard51122 passed and
corrected affected Office store49517 exit 0 (1.525s). Existing Office fixtures now create referenced
parents; a follow-up real RED22207 required preserving missing-subject error precedence.

Real PostgreSQL workspace waits were observed for move/lifecycle/create workers217–244,
cancellation269 and Office312. SQLite independent-handle tests prove writer reservation before
reads. Cancellation fixture DSNs carry the private search_path across pgx connection replacement.
Task/Office conformance39260 passed SQLite (5.587s); the initial selector missed engine label pgx,
so only the missing PG portion6492 ran and passed (8.262s, all 14 task/Office scenarios).
Scoped SQLguard exited 1 because its full exemption registry rejects an unused out-of-scan entry;
the documented complete static scan30530 exited 0. The Verification commands above now use actual
engine labels and the documented guard scope. No broad product suite was added.

Catalog/spec/reference/harness and whitespace checks are separate from product evidence.
Publication, full current-head review, terminal CI, actual merge and exact owned cleanup remain
pending; this order stays in_progress until those gates are met.


Final source audit found the remaining common full-row entrance,
`MarkDeferredMoveAppliedForSession`. Its workspace/current-step reservation precedes unchanged
queue/session guards and task reads. Real RED71657 (joined exit 1) demonstrated no PG workspace wait
and promotion accepting a real failed hierarchy metadata read. Those paths now reserve admission
and propagate read failure; no deferred-move identity, queue or F19 policy changed. Lint80309
(joined exit 1) reported three local style/staticcheck issues without timeout; corrections replace
the test condition chain, flatten malformed-ancestry assertions and remove stale encoded metadata
from the common writer's arguments. The common writer encodes after current hierarchy preservation.


GREEN51843 joined exit 0 (2.742s), actual PG waits348/351; source/identity controls passed.
Affected service/REST/WS7866 joined exit 0 (1.494s/1.623s); final changed lint25483 joined exit 0
with 0 issues. Public-doc source validation passed (47 pages), all 62 validator tests passed.
Necessary PG positive-write controls28276 exposed an existing Office SQLite-only scalar query
(`IS NOT` syntax42601); corrected canonical positive control moves a leaf, preserving depth policy.
Continuation96409 passed canonical PG current-read rejection and valid commit, but Office exposed
missing `json_valid` function42883. Office now selects a native PostgreSQL16 normalization query
using null-safe comparison and guarded JSON operations, preserving the SQLite query and its
same-parent/invalid-metadata behavior. These are actual failing dialect receipts, not retries for
resources or passing unaffected suites. Ordinary hooks must lint the final changed scope.


Native Office PG query10084 joined exit 0 (1.521s), actual wait397; additional same-parent and
invalid-metadata controls51568 joined exit 0 (1.506s), wait401. Final SQLguard46292 joined exit 0.
No schema/repair or Office deeper-cycle/depth policy change. Single conditional frozen pnpm9.15.9
install21264 joined exit 0 (pnpm9.15.9, 1.9s, frozen lockfile, 923 cached packages, no downloads); dependencies are retained for ordinary active hooks. The final hook lint must cover all
changed Go packages at the immutable comparison base d5142d9db89fa7afc42a3570eeafb4eb681ca0f0
with concurrency2/allowserial/CLI5m/GNU6m kill10s/GOMAX2/GOMEM1GiB.


Review remediation at published head1b360b5f96a8b8ad5b0c2dfc60f39eaae5e09c97: actual inline
4176285850 raised the new SQL/JSON availability dependency; the proposed null/empty guard would
break the permanent malformed-metadata control. Use Go json.Valid on PostgreSQL subject metadata
read under FOR UPDATE after workspace admission, binding the result into the native UPDATE. This
keeps the existing malformed/same-parent policy without a new database minimum. Inline4176286046
identified snapshot marshal fallback data loss; an encoding failure must abort, preserve all fields
and sessions, and emit no success rather than erase current hierarchy-owned workspace state.
The owned CI waiter30295 was interrupted and joined130 before remediation (no verdict, no hosted
cancellations); only its process group3679446 was stopped and confirmed absent.


Review correction RED95792 actually joined exit 1 (store3.540s): valid snapshots captured before raw
legacy injection isolate NULL/blank metadata and NULL parent failures in the new preservation read,
including SQLite ordinary/promotion and PG ordinary writes. Preserve nullable/blank legacy values
with NullString and root COALESCE, while malformed nonempty JSON remains an error. Previous59830
joined exit 1 had service encoding and actual-store 409 controls passing, but NULL fixtures stopped
at the pre-existing public getter; no global getter compatibility expansion was made. The native
PG16 IS JSON guard described above is superseded by locked current-row Go json.Valid, preserving
malformed bytes and same-parent behavior without changing the supported PostgreSQL floor.

CodeRabbit grouped cascade finding: the final store guard deliberately retains parents referenced
by excluded ephemeral/automation children. Existing cascade inventory ownership remains filtered;
refresh alone need not clear the conflict. The owning child lifecycle must remove that relation.
No-cascade promotion and ordinary cascade selection/partial cleanup/compensation/retry semantics
remain unchanged. Broadening deletion ownership is outside the reviewed package.


Correction GREEN39255 joined exit 0: service2.515s (four ABA variants and encoding-failure exact
row/session/event preservation), store5.197s (all fifteen legacy SQLite/PG ordinary/promotion
shapes, malformed-read and encoding rollback), handlers1.423s (actual store conflict wrapped to
HTTP409). The Office selector was unmatched, so only missing exact OfficeSerialization93680 ran
and joined exit 0, 1.516s, actual workspace wait567 plus current normalization/same-parent/malformed
controls. SQLguard9611 joined exit 0. Catalog347 decisions/1332 specs, all-spec lint and whitespace
passed. Ordinary corrective hooks and exact-head publication/review/terminal CI/merge remain gates.


Hosted run37181836703 supplied real failing receipts for three fixtures. Preserve production
admission and final structural-child conflict. `TestUpdateTaskIfWorkflowStepHasCapacity_ReturnsTypedWIPError`
now persists the real candidate in other-step before its actual update and checks unchanged candidate
and full-target occupant rows. The two existing borrowed-environment positives move into
`service_hierarchy_admission_delete_test.go` so the oversized stop-test file does not grow.
`TestDeleteTask_TransfersBorrowedEnvironmentBeforeDeletingOwner` retains actual Service.DeleteTask
transfer after real workspace-scoped no-cascade structural promotion; its ungrouped environment
fixture needs the original service borrower-transfer path, rather than pretending a nil-group
Handoff transfer would preserve it. `TestCleanupTaskResources_TransfersBorrowedEnvironmentBeforeCascadeDelete`
retains actual cleanup(true), then models valid structural promotion before final raw ordinary
delete. This is a component cleanup stage, not full cascade promotion. Both retain original post-owner
delete environment/child-owner assertions and add absent-parent/live-root child, unchanged running
session/environment link and complete environment identity/resource preservation controls.

Exact new affected selectors, run serial with existing race/fts5/trimpath/p1/caps: service
`^(TestDeleteTask_TransfersBorrowedEnvironmentBeforeDeletingOwner|TestCleanupTaskResources_TransfersBorrowedEnvironmentBeforeCascadeDelete)$`;
repository `^TestUpdateTaskIfWorkflowStepHasCapacity_ReturnsTypedWIPError$`. No production missing-task
precedence or lifecycle change. Required full lint99037 joined4 FAILED_TIMEOUT remains failed
forever (0issues then Timeout exceeded); ROOT allows one recovery only after validated corrected
code, same full ./... scope/base/resources, with no automatic second recovery.


Bounded hosted-fixture correction40552 ACTUALLY JOINED0 service1.363s, exact two borrowed-resource
positives with all strengthened post-delete controls. Exact typed-WIP fixture89928 ACTUALLY JOINED0
repository1.205s, candidate/occupant complete rows unchanged. No production deletion/WIP policy
change. One recovery full lint is authorized but has not run yet; actual completed current0ed
CodeRabbit FULL5404607777 has one additional valid direct-target lookup error finding, scoped
real-data correction pending before spending the single recovery gate on a validated candidate.


ROOT explicitly includes two further causal cases before the same unused recovery gate.
Direct-target `GetTask` translates only typed missing to existing invalid-parent; real decode/
storage/context failures preserve identity, subject missing is unchanged. Permanent real-service
`^TestTaskHierarchyAdmissionMalformedAncestry$/^target_read_failure$` and actual admitted SQLite
reader `^TestTaskHierarchyAdmissionDirectParentCancellation$` provide RED before correction.
The cancellation decorator only cancels before delegating the actual transaction reader; it does
not replace any business predicate or read result. Affected GREEN also selects existing positive
`^TestService_UpdateTask_RejectsMissingParent$`. PG compatibility selector is exactly
`^TestPostgresUpdateTaskPreservingDeferredLaunchWithNilMetadataStaysAnObject$`: seed only its
owning ws-task-nil-metadata-pg through the real repository, retain shared raw task helper and all
object/deferred-launch/description assertions. Use the existing proven-owned PG fixture, no skip
as completion. Prior40552/89928 passing unaffected fixture controls are retained without replay.


Direct-target RED23061 ACTUALLY JOINED1 reproduced decode/cancellation masking. First GREEN82828
joined1 because the decode oracle incorrectly required json.SyntaxError; the existing getter owns
models.ErrMalformedWorkflowAgentOverrides. Corrected typed-identity oracle without getter changes.
GREEN96953 ACTUALLY JOINED0 service1.686s covers direct-target malformed record, exact actual reader
cancellation identity, unchanged subject/events, and positive missing-parent classification. Exact
real PG nil-metadata compatibility24797 ACTUALLY JOINED0 store1.346s: owning workspace seeded,
original object/deferred-launch/description assertions retained. Old watcher65017 joined1 terminal
(56pass,13skip,4failed) on historical0ed; no active watcher. The single authorized full changed-lint
recovery now runs on this combined validated candidate; previous99037 remains failed timeout.


Single full changed-lint recovery99256 ACTUALLY JOINED4 FAILED_TIMEOUT on the combined validated
candidate: `0 issues` followed by `Timeout exceeded`, not a pass. Exact immutable base d5142d9,
original binary/full ./..., GOMAX2/GOMEM1GiB/concurrency2/allowserial/CLI5m/GNU6m/kill10s.
No further heavy operations, hook/commit/push or automatic retry; published0ed remains unchanged.
ROOT must provide bounded recovery direction. Both99037 and99256 remain failed receipts.


ROOT explicitly authorizes one adjusted-duration full changed-lint recovery on this validated
candidate: same original binary/full ./.../immutable d5142d9 base/GOMAX2/GOMEM1GiB/concurrency2/
allowserial, CLI timeout10m and GNU hard cap11m with kill-after10s only. This duration exception
does not change the standard check limits or infer timeout cause. Both99037/99256 stay failed.
No test replay, cache deletion, memory increase, foreign kill or automatic additional recovery.
Join before hooks/publication; concrete diagnostics require minimal correction and checkpoint
before another full gate allowance.


Explicit duration-exception full changed lint64185 ACTUALLY JOINED exit0, `0 issues`. Full ./...
exact-base gate passed on the combined validated candidate at the authorized10m/11m bound.
No timeout cause inferred and no prior failure reclassified. Normal active-hook commit and
corrective publication can now proceed; exact new-head full review/terminal CI/merge remain gates.


ROOT explicitly releases the valid grouped child-list error finding from current FULL5404824789.
Exact new selector `^TestTaskHierarchyAdmissionChild(ReadCancellation|DecodeAndDepthErrors)$`
uses malformed persisted-child overrides through real Service.UpdateTask and cancellation before
delegating actual transaction ListChildren; check original typed error identity, complete subject
row/event preservation, and normal ErrInvalidParent+ErrSubtaskDepthExceeded controls. Meaningful
RED precedes minimal unchanged-error return; no reader/query/lock/getter/create/Office/lifecycle
changes. Other returns in the same policy.go reviewed read-only: direct typed missing retains
invalid-parent, genuine target/ancestor failures propagate, creation wraps with %w as before.
After affected GREEN, ONE new-code full CHANGED exact-base gate at explicitly authorized10m/11m
duration only, same resources; old64185 pass is historical,99037/99256 stay failed.
Capacity recovery lost watcher97850: handle unavailable and recordedPIDPGID3861033 absent, stdout
empty; LOST/INTERRUPTED NO VERDICT, not terminal/pass. No duplicate watcher; one replacement
after actual corrective publication.


Child-read RED88410 ACTUALLY JOINED1 service0.346s: real persisted child decode and actual admitted
transaction ListChildren cancellation both masked as invalid-parent; normal depth control passed.
Minimal genuine-error return GREEN43646 ACTUALLY JOINED0 service1.333s verifies typed decode and
cancellation identity, complete subject/event preservation, and both normal depth error identities.
Catalog/all-spec/whitespace checks passed. One authorized corrected-code full gate now runs with
explicit10m/11m duration bound and otherwise unchanged exact base/resources.


Corrected child-read full CHANGED lint13074 ACTUALLY JOINED0, zero issues; full ./... at immutable
base d5142d9 with explicitly authorized10m/11m duration and unchanged resources. Normal active
hooks/publication and new-head review/CI/delivery remain; earlier head/failure receipts unchanged.
