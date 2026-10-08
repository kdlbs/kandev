---
id: "01-canonical-metadata-merge"
title: "Apply metadata merge intent atomically"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-FIELD-UPDATES-001
acceptance_criteria:
  - AC-TASKS-FIELD-UPDATES-001.8
  - AC-TASKS-FIELD-UPDATES-001.9
  - AC-TASKS-FIELD-UPDATES-001.10
  - AC-TASKS-FIELD-UPDATES-001.11
  - AC-TASKS-FIELD-UPDATES-001.12
system_design:
  - ../../specs/tasks/system-design/task-field-updates.md
---

# Task 01: Apply metadata merge intent atomically

## Summary and release boundary

Give the existing explicit metadata merge service a required canonical metadata-only repository
operation. Preserve concurrent disjoint keys and current scalar/owner state using native database
serialization, while retaining supported values and truthful postcommit response/event behavior.
This is one sequential slice: interface plus its concrete implementation/service/test integration
must compile and prove behavior together.

ROOT reviewed and explicitly released this package in a later IMPLEMENTATION interrupt on
2026-10-04 after the DESIGN turn ended WAITING. Status is in_progress in this same primary.
Keep accepted ROOT proof read-only; write new permanent behavioral RED separately.

## In scope and ownership

| Owned files/boundary | Work |
| --- | --- |
| `apps/backend/internal/task/repository/interface.go` | Required `MergeTaskMetadata(ctx,id,overlay) error`, no optional/snapshot fallback |
| `apps/backend/internal/task/repository/sqlite/task_metadata_merge.go` (new) | All-keys canonical merge with SQLite writer reservation, PG advisory then row lock, raw current metadata and metadata/timestamp-only update |
| `apps/backend/internal/task/service/service_workflow.go` | Preserve auth/existence observation and postcommit reread/publication; pass intent to the new operation |
| `models/task_metadata_update.go`, current repository helpers | Only necessary domain-specific ownership/workspace reuse, keep ordinary helper semantics and raw unrelated numbers; no global helper refactor |
| New service files `service_task_metadata_merge_test.go`, `service_task_metadata_merge_values_test.go`, `service_task_metadata_merge_effects_test.go` | Concurrent APIs, ordinary interaction/exclusions, owner/value/failure/effect/publication evidence |
| New repository files `task_metadata_merge_test.go`, `task_metadata_merge_postgres_test.go`, `task_metadata_merge_cancellation_test.go` | Real dialect behavior, native physical wait, value/error/rollback controls |
| New handlers file `task_metadata_merge_registered_test.go`; existing `task_port_forwarding_test.go` | Real registered REST plus existing invalid-body/not-found behavior; assertions on actual DB/DTO/event payloads |
| New companion `service_task_metadata_merge_fixture_test.go`, `handlers/task_metadata_merge_fixture_test.go`, `orchestrator/executor/task_metadata_merge_fixture_test.go` | Required method adaptation on existing standalone fake types only; fail closed outside each purpose; real wrappers forward |
| Existing construction helpers in `service_task_field_updates_test.go`, `handlers/task_field_updates_test.go` | Reuse pair/gates; expose constructor-created Service and optional owner on registered fixture without runtime dependency swaps |
| `docs/public/websocket-api.md` | Short reference clarification of existing REST preference merge versus ordinary metadata replacement and snapshot exclusions |
| Owning requirement/design, this plan/work order | Status/results/traceability only after actual conformance/checks |

Read applicable backend AGENTS, `/tdd` and backend test reference after release. The same production
repository serves SQLite and PostgreSQL. Inventory all new required-method implementers at compile
integration; the list above is grounded but not permission to broaden production callers.

## Out of scope

GitHub `TaskIssueStore.UpdateTaskMetadata` adapter/link/unlink remains ordinary replacement. No new
wire fields, WS metadata action, schema/revision/event ordering, generic merge framework, process
mutex, early reread-as-fix, independent per-key commits, arbitrary later snapshot safety, Office,
runner/cascade/launch/completion/hierarchy policy, runtime/queue/harness storage, UI/copy/browser,
build or E2E work. Parent/hierarchy admission and atomic associations/F19 remain separate. A causal
finding outside the package requires ROOT scope amendment; optional polish is deferred.

## Acceptance

1. Existing-API real SQLite RED proves both disjoint merge losses and title-first loss; canonical
   service/registered-route GREEN retains all admitted intents and controls in either order.
2. The required operation applies all supplied keys/timestamp atomically to canonical metadata
   using actual native independent-connection serialization, preserving supported value/owner
   rules and every unrelated row field, table and workflow/runner effect.
3. Real PG wait/value/cancellation tests actually execute; postcommit DB/DTO/event/error controls,
   prior contract regressions and required persistence/lint/doc gates pass with exact receipts.

## Behavioral RED (before any production/interface change)

Create `TestTaskMetadataMergeConcurrentSQLite` and `TestTaskMetadataMergeOrdinaryFieldInteraction`
through existing `Service.UpdateTaskMetadata` and `Service.UpdateTask`. Reuse `taskFieldServicePair`
and `seedFieldTask` from `service_task_field_updates_test.go`. Both construction-time real
`GetTask` gates must signal that actual snapshots exist before either API is released. No business
predicate, DB behavior or canonical merge is mocked, no dependency is swapped after production
cleanup workers start. Release one gate, join that real API, then release/join the second.
Run both metadata orders and both ordinary-title/merge orders. Assert final alpha and port preference,
accepted title, unchanged unrelated fields/key, API returns and `TaskUpdated` contents. The known
RED is three failed data cases (metadata both orders, title-first), not failing compilation or
mock call counts. Keep reverse-title/sequential controls green. Register cleanup before fatal
paths, cancel/release and join every worker and production cleanup on all outcomes.

From repo root, after release only:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^(TestTaskMetadataMergeConcurrentSQLite|TestTaskMetadataMergeOrdinaryFieldInteraction)$' -count=1 -v)
```

## Implementation sequence

After meaningful RED, add the required interface/real operation and minimum fake adaptations in
one compile-coherent change. Follow the paired design's canonical operation exactly: encode a
request-owned clone, reserve/lock before canonical metadata reads, protect current owner/workspace
state, retain current pending dialect rules, update only metadata/timestamp and commit. Service
keeps its early existence read but never passes that task to storage. Preserve `errors.Is` and
postcommit read error behavior. Do not reuse the full-row transaction/effects path or rely only
on a task-specific advisory lock instead of the physical row.

Add remaining meaningful controls sequentially. Keep functions/tests under scoped lint limits;
new tests live in new files rather than growing already oversized historical files. Filter only
established protected namespaces; no general metadata key allowlist or new transport policy.

## Required test matrix and honest coverage

All mappings below refer to `AC-TASKS-FIELD-UPDATES-001`.

| Proposed test | Cases and data/effect assertions | AC |
| --- | --- | --- |
| `TestTaskMetadataMergeConcurrentSQLite` | Both disjoint commit orders through independent real Services/SQLite handles; same ordinary top-level key later intent; sequential controls; persisted row and each observed update payload | .8 |
| `TestTaskMetadataMergeOrdinaryFieldInteraction` | Both title/merge orders plus description/priority/assignment/position omissions through ordinary API. Prior native scalar state/priority/position/title commit survives a delayed merge; current completed/step row never restored to old values. Same human title ownership remains resolved. | .9, .11 |
| `TestTaskMetadataMergeReplacementControls` | Real ordinary explicit metadata replacement after merge may delete ordinary merged key; reverse order retains replacement keys plus overlay. Legacy snapshot-after-merge control documents permitted overwrite without asserting new safety. Ordinary empty replacement still clears ordinary keys. | .9, .10 |
| `TestTaskMetadataMergeOwnersAndValues` | Real deferred CAS writes/removals, handoff CAS/provenance and carrier owners, title claim/set before merge and explicit human title after merge, forged/removed owner keys cannot be resurrected. Materialized shared workspace mode/group including parent ABA unchanged; input/nested maps unchanged. | .10 |
| `TestTaskMetadataMergeSQLiteValues` | Stored NULL/empty/JSON null normalize to object; nil/empty overlay preserves keys/timestamp behavior; pending/nonpending explicit null and nested object/array/empty values; omitted current null; false/zero; punctuation keys; raw large-number preservation | .8, .10 |
| `TestTaskMetadataMergeEffectsAndFailures`, `TestTaskMetadataMergeCurrentTransitionAndAssociations` | Actual service path preserves scalar row, parent/workspace, repositories/folders, current runner/participant records, transition ledger and step-entry data. Supply a real pending-entry context and dispatch observation to detect accidental full-row allocation. Verify no state/move/entry dispatch; ordinary typed state/step control still performs its own effects. Real bad encoding/storage/cancel/missing/auth failure produces no merge mutation/success events; compare persisted row/timestamp/effects and typed causes. | .9, .11 |
| `TestTaskMetadataMergeSQLiteAtomicity` | Multi-key valid input plus unencodable value cannot partly commit; real SQLite trigger abort on metadata UPDATE rolls back all keys/timestamp; malformed/non-object stored metadata fails unchanged; actual native SQLITE_BUSY from independent held writer, cancellation at the real error-return boundary and join; missing task errors.Is | .11 |
| `TestTaskMetadataMergePostcommitObservation` | Construction-installed forwarding gate after real merge commit allows later ordinary title/native owner commit before reread; returned row and event match later observation/timestamp. Construction-installed postcommit read failure returns original error, durable merged row, no success publication. No runtime dependency swap. | .12 |
| `TestTaskMetadataMergeRegisteredPortForwarding` | Real handlers `registerHTTP`, real Services/independent SQLite connections, actual PATCH port preference versus registered ordinary PATCH title and second real merge in ordered stale-read cases. Compare HTTP DTO, DB row and actual events. Unknown/trailing/missing/null/nonboolean/malformed bodies, missing task and viewer/foreign-scope denial preserve row/timestamp and current statuses. Invalid request performs no successful publication. | .8, .9, .11, .12 |
| `TestTaskMetadataMergePostgresPhysicalWait` | Env-gated independent connection pair/private schema, holder locks only physical task row without workspace or merge advisory lock. Observer confirms backend PID Lock wait and pg_blocking_pids before holder commits alpha/port or title/owner changes in both permutations. Waiter calls new real canonical method; current merge/scalars/timestamp must match actual commit. Cancel waiting operation and prove errors.Is plus unchanged row/effects; join before cleanup. | .8 to .11 |
| `TestTaskMetadataMergePostgresValues` | Actual new method in PG: nonpending/pending nested/null matrix, raw omitted large values, protected absent/present owners/workspace, nil/empty input, encoding failure and real trigger-raised UPDATE failure with all-keys/timestamp unchanged | .10, .11 |

Use native statements only to establish known physical blockers or deliberate DB failures, not to
simulate service predicates. Exercise real owner/CAS methods for policy proof. Data row effects
and causal counts/deltas are assertions; method call counts alone are not. Do not claim existing
controls prove the new method, a test SKIP proves PG serialization, or helper compilation proves
behavior. Planned names must exist and each anchored selector must execute tests before GREEN is
recorded. Amend file/name mapping truthfully if narrow implementation grouping changes.

## Verification after release

Run each command separately, one heavy LOCAL operation at a time, retaining **every** returned
handle and actually joining it before starting the next heavy/dependent operation. Log exact
command/base/head/exit/duration and results. The commands are independently rooted at repo root.
The new method must run in both dialects; no broad full-suite replay for extra assurance.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^(TestTaskMetadataMerge.*|TestTaskFieldUpdates.*|TestUpdateTaskMetadata.*|TestUpdateTaskCannotReplaceDeferredLaunchAttribution|TestUpdateTaskTitle_ClearsPendingAgentTitleMetadata|TestUpdateTaskDescriptionOnly_LeavesPendingAgentTitleAndStoredTitleIntact|TestTaskHierarchyAdmission.*|TestTaskRepositoryReplacement.*|TestServiceUpdateTaskWithNewStepRecordsTaskUpdate|TestServiceUpdateTaskWithoutStepChangeRecordsNoLedgerRow|TestUpdateTask_StripsCallerSuppliedOfficeCarrierMetadata|TestUpdateTask_PreservesExistingOfficeCarrierMetadataAcrossAGenericUpdate)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/handlers -run '^(TestTaskMetadataMergeRegisteredPortForwarding.*|TestHTTPUpdateTaskPortForwarding.*|TestTaskFieldUpdatesRegisteredProtocols|TestTaskHierarchyAdmissionRegistered.*|TestRegisteredTaskRepositoryReplacement|TestHTTPUpdateTaskTitleOnlyPreservesRepositories|TestHTTPUpdateTaskExplicitEmptyRepositoriesClears|TestWSUpdateTaskTitleOnlyPreservesRepositories|TestHTTPUpdateTask_IgnoresExternalIDField)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestTaskMetadataMergeSQLite.*|TestTaskFieldUpdatesSQLiteCurrentRow|TestTaskHierarchyAdmissionSQLite.*|TestTaskRepositoryReplacement.*|TestUpdateTaskPreservesWinningTitleAgainstStaleUpdate|TestFullMetadataWrite_PreservesLiveHandoffProvenance|TestFullMetadataWrite_HandoffProvenanceAbsentStaysAbsent)$' -count=1 -v)
```

After exact owned PG16 fixture receipt and private `KANDEV_TEST_POSTGRES_DSN` export, run actual PG
method coverage. Record executed subtests/physical wait receipts; SKIP does not satisfy acceptance.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestTaskMetadataMergePostgresPhysicalWait|TestTaskMetadataMergePostgresValues|TestTaskFieldUpdatesPostgresPhysicalWait)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go run -trimpath -tags fts5 -p=1 ./cmd/sqlguard ./internal)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/persistence/storeconformance -count=1)
```

Required executor standalone-fake compilation, if touched (no AC proof attributed):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/orchestrator/executor -run '^$')
```

PG16 fixture receipt before use includes exact container ID, owner/task labels, pinned image
digest, ALL mount/volume identities and private credential file location (never print credentials).
Use tmpfs for PGDATA and explicitly cover declared volumes, no anonymous volumes or foreign binds.
Reuse private-schema independent-connection patterns in `newHierarchyPostgresRepoPair`,
`hierarchyBackendPID`, `waitHierarchyPostgresLocks`. Observe actual row wait via `pg_stat_activity`
and `pg_blocking_pids`, not sleep timing. Cancel/release/join all waiter and observer clients before
exact owned container/schema cleanup. Never touch unknown foreign volume
`2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`.

Mandatory actual CHANGED backend lint gate: resolve original installed binary and immutable actual
PR base from authoritative remote/API evidence; store in `task26_lint_binary` and `task26_pr_base`.
Run original binary `./...`, not a scoped/hook substitute, with this task-local ROOT duration bound:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 11m "$task26_lint_binary" run ./... --new-from-rev="$task26_pr_base" --concurrency=2 --allow-serial-runners --timeout=10m)
```

Nonzero/timeout even with zero issues is FAILED. Resource failure requires exact receipt/owned
process checkpoint and WAITING for ROOT bounded direction. No auto retry, cache wipe or foreign
kill. Correct concrete diagnostics narrowly and run affected checks; no passing broad replay.

Cheap design and later docs/reference gates:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/concurrent-task-metadata-merges
```

Use `.github/scripts/pr-docs.cjs` exported `validateCoverage` with filesystem-backed requirement,
design, plan and work-order contents for unstaged cross-reference preflight (no GitHub publication).
Verify every local document link and stable AC, each requirement is in its design, and every
work-order design is in the manifest. No install needed for DESIGN. After the public API paragraph
is implemented, run `node --test scripts/validate-public-docs.test.mjs` and
`node scripts/validate-public-docs.mjs` from repo root using the existing Node PATH.

## Dependencies and scope audit

Depends on the reviewed base and existing canonical task storage/auth/title/owner/projection
contracts. No unfinished sibling. Native row serialization does not grant authority to metadata
callers over owner records or scalar fields. Supplied pending values retain their dialect-specific
semantics; omission does not replay null deletion. Explicit ordinary replacement and later full
snapshots remain excluded from symmetric per-key safety. Input maps are borrowed and never mutated,
not protected against simultaneous caller mutation.

Mobile exception: backend state/data-only change with identical DTO/event wire and no viewport
behavior. Registered REST integration is causal proof; no UI/browser/build/E2E unless ROOT amends.
Public impact is one API-reference paragraph after release, no public speculative claim now.

## Risks

Map/null encodings, current pending-title state and protected workspace provenance must be read
inside the lock, not from service observations. PG advisory alone cannot coordinate scalar writers;
physical-row wait is required. No later workspace/step locks prevents lock inversion. Empty/protected
input still succeeds with normal timestamp/event behavior. A postcommit read can fail after durable
mutation and must not emit a misleading success event or pretend rollback.

## Parallelism

`sequential`. One primary session; no delegation, tasks/tabs/sessions or model/profile switch.

## Inputs

- [Owning requirements](../../specs/tasks/requirements/task-field-updates.md), criteria .8 to .12.
- [Owning design](../../specs/tasks/system-design/task-field-updates.md), explicit metadata boundary,
  supported values, writer inventory and exclusions; prior ordinary sections remain authoritative.
- Real `taskFieldServicePair`/gate construction; existing port service and registered handler tests;
  title/deferred/handoff/carrier, hierarchy/association/ledger controls; PG private-schema/row-wait pattern.
- Accepted read-only ROOT proof and identity/checkpoint/resource receipts in the Kandev task plan.

## Delivery constraints after release

Normal hooks/Conventional Commit/push/ready PR, no bypass/amend. One conditional pinned pnpm9.15.9
frozen install from apps in this fresh worktree only if dependencies are absent, existing Node
PATH/bash login=false; no setup/lockfile mutation. Preserve published head absent a corrective
finding; no main-only rebase near terminal checks or synthetic merged tests.

One practical 90m all-terminal `scripts/pr-await` handle retained/actually joined before replacement;
no duplicate GitHub polling. Require configured authenticated CodeRabbit App347564 substantive FULL
review of all actual files with source=covered=CURRENTHEAD/kind=reviewed; inspect auto before at most
one necessary full request per corrected head. ACK/skip/boilerplate are not review evidence. Every
inline/grouped actionable finding gets concrete disposition; optional extra assurance/style deferred.
Hosted leaf failures require exact log/artifact/source and ROOT bounded scope/rerun decision, never
blind rerun or weakened gates. Merge expected current head normally only after actual terminal
policy gates/full current review and no actionable/unresolved/hidden/error. Independently prove
MERGED/mergedAt/mergeSHA/parent/tree/all owned blobs/authoritative remote main and joined owned cleanup.
Do not mark task complete before merge/proof/cleanup. Leave clean managed worktree/dependencies for
ROOT archival. The task plan retains detailed receipts; ROOT plan owns only the loop/current child.

## Results

Local behavior is implemented and acceptance checks have executed after the explicit later release.
See [manifest results](plan.md#verification-results) for passed selections and concrete corrected
fixture failures. Permanent RED precedes implementation and the accepted ROOT archive remains
unreplayed. Native PostgreSQL physical wait/current merge/value/rollback coverage executed, with no
SKIP; SQLite cancellation preserves its typed cause at the observed native busy-error return boundary;
the forwarding probe changes no SQL outcome. The affected corrected check passed with race
detection (6.386s). PG separately proves cancellation during row wait. Required persistence
SQL guard, PostgreSQL-enabled storeconformance and executor interface compilation passed. All
DB clients joined before exact fixture/credential cleanup; pinned frozen hook dependencies are
installed. Original full changed-backend lint passed with zero issues after one narrow test-only
selector correction and its affected auth check. Delivery is still in_progress: normal hooks/publication, full current-head
review/terminal policy and actual merge plus independently joined cleanup remain gates. Exact
command handles and owned-fixture receipts are in the external Kandev task plan.
