---
created: 2026-10-07
status: in_progress
requirements:
  - REQ-TASKS-FIELD-UPDATES-002
system_design:
  - ../../specs/tasks/system-design/task-field-updates.md
legacy_specs: []
---

# Implementation plan: Preserve disjoint workflow edits

## Overview and checkpoint

One sequential work order owns the typed workflow patch, concrete shared SQL, participating
service writers and registered transport proof. Tasks owns persisted workflow definitions and
ordinary presence. Extend the existing [requirement](../../specs/tasks/requirements/task-field-updates.md)
and [design](../../specs/tasks/system-design/task-field-updates.md); preserve existing task and
metadata criteria/lifecycle and historical delivery results. The workflow contract is implemented; local delivery remains in progress. Compact repeated task inventory prose to retain the design's size limit;
the explicit metadata-merge section is preserved byte-for-byte.

The completed DESIGN checkpoint left exactly four unstaged/uncommitted artifacts before
production/permanent tests/install/product checks/commit/PR. ROOT subsequently released the
reviewed package and exclusive GLOBAL LOCAL-HEAVY68. Same primary session; no delegates, tasks,
sessions or model switches. Design completion is not task completion.

## Confirmed cause and evidence

Current/accepted diagnostic baseline: `905fa03c5b8ae90c661dc9fec2e324d355e269aa`.
Service.UpdateWorkflow overlays optional fields onto an early row then calls full-row storage.
Held name after committed prompt erases prompt; held prompt after committed name erases name.
Both calls succeed. SetWorkflowHidden/SetWorkflowSource share this write footprint and participate.
UpdateWorkflowIfUnchanged's atomic expected-timestamp predicate must stay fail closed.

Protected ROOT proof is READ ONLY: `/tmp/kandev-root-workflow-partial-patch-candidate_test.go`,
0400, SHA256 `013b0227bcafaf1f4b7d0fafaaddc1c1749a512bf603fa5b9bd155c5ccb2bd45`.
Never replay/copy/import/mutate/delete. Evidence directory
`/tmp/kandev-root-workflow-patch-discovery-20261007/` contains receipt/native/classification JSON
and proof.log. Native 67429 / 8f1127 -> cbbc26 actually joined exit 1; receipt records wrapper
PID/PGID 1249211, child PID/PGID 1249234, UTC start 2026-10-07T06:52:01.277087Z and terminal
06:53:45.648899Z, joined/group empty/ROOT clean. Qualification 51b9a2 synchronous exit 0/group
gone/ROOT clean is supplied ROOT context, not a new child execution.

Two causal FAIL; sequential and explicit-empty controls PASS. Actual production service, independent
bare services, real SQLite repositories and SQL read plus bounded barrier. No executed registered
REST/WS, events, frontend/browser, PG or independent physical-connection proof. Original overlay-only
experiment left ROOT tracked files clean. Do not broaden these evidence claims.

## Scope and technical approach

Add seven-presence-field models.WorkflowFieldUpdate, required WorkflowRepository.UpdateWorkflowFields,
one fixed-column allowlisted UPDATE RETURNING and three service writers. Preserve exact fenced
full-row path, initial authorization/existence observation and current response/event shapes.
Return/publish the stored row; publication can interleave. No RMW transaction or hierarchy lock.
Cancellation evidence is deterministic before write admission plus actual statement-abort rollback;
no reversal of an already committed mutation or inference of commit from transport failure.

| Boundary | Compatibility | Required evidence |
| --- | --- | --- |
| SQLite | One supplied-column statement, omitted/null preserved, empty/false supplied | Real independent services/connections, actual read barriers and rollback controls |
| PG | Shared Rebind/RETURNING, native row serialization | Private owned fixture, distinct backend PIDs, actual row lock wait and final row |
| REST/WS | Four optional fields, existing errors/DTO/events | Registered router/dispatcher through DB/response/event recorder |
| Exact command | ExpectedUpdatedAt and full-row CAS retained, unavailable fencing fails | Stale conflict after ordinary patch and successful exact control |
| Editor/provider | Intentionally supplies all four fields | Source audit and actual all-field replacement/empty control |
| Hidden/source | Hidden alone or source/path bundle, existing no-op | Both overlap directions, false/source normalization and no-op cases |

Exclude global writer migration, task hierarchy/paused work, schema/API/revision framework,
stale full-editor merge, global ordering/latest snapshot, arbitrary later snapshot safety,
browser/E2E/build replay absent causal need. Desktop/phone use unchanged interfaces; no rendered
UI preview or mobile interaction change. The existing public WebSocket API reference now clarifies workflow partial presence.

## Tests

These tests were independently authored after ROOT release, never copied from ROOT.

| AC | Permanent file and test |
| --- | --- |
| 002.1 | service/service_workflow_field_updates_test.go: TestWorkflowFieldUpdatesConcurrentSQLite, TestWorkflowFieldUpdatesDomainWriters |
| 002.2, 002.5 | repository/sqlite/workflow_field_updates_test.go: TestWorkflowFieldUpdatesPresence; service: TestWorkflowFieldUpdatesFullDraftControl |
| 002.3 | service: TestWorkflowFieldUpdatesExactFence; repository: TestWorkflowFieldUpdatesFailures; registered error controls |
| 002.4 | handlers/workflow_field_updates_integration_test.go: TestRegisteredWorkflowFieldUpdatesHTTP, TestRegisteredWorkflowFieldUpdatesWS |
| 002.1-002.3 | repository/sqlite/workflow_field_updates_postgres_test.go: TestPostgresWorkflowFieldUpdatesPhysicalConcurrency, TestPostgresWorkflowFieldUpdatesCompatibility |

Paths are relative to apps/backend/internal/task. Registered server request-to-DB/response/event
tests supply causal end-to-end evidence; no frontend shape, layout or interaction changes need
Playwright. Existing editor source confirms intentionally full supplied values. Required PG
behavior covers the touched shared dialect method, not a broad schema replay. The work order's
existing conformance selector anchors only TestStoreConformance/(sqlite3|pgx)/task, excluding
harness/other owners and all upgrade fixtures. Its Run validates catalog coverage already, so no
separate catalog product test is added. New causal workflow test names are explicitly planned;
existing regression names are separately anchored to actual source.

## Work orders

- [ ] [Task 01: Preserve workflow patch intent](task-01-preserve-workflow-patches.md), in_progress, sequential.

## Verification results

Product checks NOT RUN (design barrier). Catalog validation passed (357 decisions, 1416 specs),
all 36 specification-linter tests passed, and full specification lint passed. Native document
check handle 23436 actually joined exit 0. Offline validateCoverage reported covered, zero errors
with the proposed runtime path (simulation only) and this actual work order/four local documents.
Diff whitespace passed; status shows exactly two modified specs and two untracked plan files;
index is empty. Historical task ACs and the explicit metadata design were compared to HEAD and
are byte-identical. Protected proof SHA256 and 0400 mode match before/after read-only inspection.
No historical task/metadata result is reused as workflow proof or rewritten.

ROOT's cheap amendment checkpoint passed: catalog, 36/36 spec tests, full spec lint, offline
reference coverage and whitespace/status gates. Original amended-document native handle 99549
actually joined exit 0. Static source audit verified nine existing anchored regression names,
eight explicitly planned workflow names and task-only sqlite3/pgx conformance selection.
Existing metadata design remains byte-identical; amended design is 32692 bytes, within limit.
Four artifacts remain unstaged/uncommitted, HEAD/index unchanged. No product tests or heavy slot
claimed; LOCAL-HEAVY67 remains ROOT's exclusive resource. END amended DESIGN for ROOT seal.


## Implementation checkpoint: ROOT resource decision required

ROOT released reviewed-package implementation and exclusive GLOBAL LOCAL-HEAVY68 in this
same primary session. All four sealed design hashes matched before implementation; the one
order is in_progress. Field-scoped workflow SQL/service correction and independently authored
permanent coverage are present, unstaged/uncommitted. Initial publication has not occurred.

| Original execution | Actual result | Native handle / chunks | Owned child PGID |
| --- | --- | --- | --- |
| Independent SQLite service RED | Two causal lost-field assertions FAIL | 52929 / 10f834 -> b6a94c, joined exit 1 | 1388389 |
| Affected GREEN | Service/store PASS; transport DTO pointer/string test assertions FAIL | 5566 / fe402d -> 2cb972, joined exit 1 | 1416384 |
| Corrected transport + nullable presence | Registered HTTP/WS both directions, empty/read-only, DB/response/events and store presence PASS | 23845 / 78cc49 -> 557fc2, joined exit 0 | 1435652 |
| Owned PG setup | Host namespace permission failure before tests | 79404 / 0e35cc -> 228ce1, joined exit 1 | 1444972 |

Logs/OS/native receipts: `/tmp/kandev-child68-local-20261007/`. Go runs retained reviewed
trimpath/fts5/race/p1, GOMAXPROCS=2/GOMEMLIMIT=512MiB, CLI 5m/GNU 6m kill-after 10s. The
corrected assertions honor existing nullable DTO strings; no product change was needed.
SQLite proof uses distinct actual driver connections, real SQL reads and bounded joined
barriers; service/domain/presence/full-draft/exact-fence/auth/cancellation/statement-abort
checks passed. PG tests were compiled but never executed, and are not validation evidence.

PG container `ad7fa2ecc78dfec72bb9fe791de265615c0673e2f1279b58d8a5faf47a9e0a0c` was newly
created from the reviewed pinned PostgreSQL 16 image, task/session-labelled, private loopback,
512MiB/2CPU, tmpfs data and owned password bind; no existing volume was used. Setup hit
PermissionError reading `/proc/1445029/ns/pid` as uid 1000. Finally stopped/removed that exact
container; namespace cleanup inspection also lacked permission. A fresh independent cleanup
query confirmed container absence, original host PID absence and all four original native
wrapper/child groups empty; password file removed. See `pg-checkpoint-cleanup.json`.
Namespace-wide physical process proof is unavailable; do not infer it or claim PG success.

ROOT's no automatic retry rule applies to this permission/resource qualification failure.
Stop local work at this checkpoint; no successor fixture or retry. PG dialect/physical tests,
task-only conformance, SQLguard, scoped lint, pinned dependency install, active normal hooks,
commit/push/ready PR remain pending. HOSTED/MERGE authority is NONE. Protected ROOT proof
checksum/0400 remains unchanged; managed worktrees/dependencies, caches, foreign resources,
refs/FETCH_HEAD and the unproved old volume remain preserved. No implementation completion.

## Resource and delivery barriers

Later local execution owns ONE GLOBAL LOCAL-HEAVY. Existing Node 24.21/Go 1.26 PATH, Bash
login=false; Go trimpath/fts5/race/p1/GOMAXPROCS=2/GOMEMLIMIT=512MiB. Lint concurrency 2/allow
serial, CLI 5m/GNU 6m kill-after 10s. Initial lint is scoped to affected task service/repository/
SQLite/handlers/model contracts and only inventoried necessary changed fake/adapter packages.
Actual backend fixup still needs ONE full CHANGED ./... at exact
API comparison base, GOMAXPROCS=2/GOMEMLIMIT=1GiB, same bounds. No automatic retry after
timeout/resource/transport/unknown/out-of-scope outcomes: ROOT checkpoint. Retain every original
native handle/PID/PGID/log/UTC/immutable cutoff, actual join and physical group return; no invented
metadata. One pinned pnpm 9.15.9 frozen apps install if absent is allowed only under later grant.
Preserve managed worktrees/deps, foreign resources, caches, refs and FETCH_HEAD. Normal hooks.

Ready PR: frozen head and automation disabled verified. Separate ROOT HOSTED release after heavy
return: ONE original 90m collector/GNU 91m kill-after 10s; no reset/successor/retry without ROOT.
All six required checks plus actual Backend/Frontend/E2E parent SUCCESS/native-platform proof.
CodeRabbit app 347564: substantive FULL exact-head/all-files/source-covered kind reviewed, accept
auto first, one necessary request only for proven gap, no ACK approval/optional polish.
Separate SERIALMERGE ROOT grant: normal expected-head squash/no admin; static live-main
compatibility/owned blobs, recorded PR base historical rather than live equality, no rebase or
synthetic tests. Actual GitHub merge/Git tree/all blobs/remote plus joined cleanup; ROOT independent
verify/archive/checksum-proof release. These are barriers, not current execution permission.

## Risks

Required interfaces may expose standalone fake gaps; adapt minimally without snapshot fallback.
RETURNING/normalization need actual PG evidence; unknown fixture ownership or unavailable resources
are ROOT checkpoints. Full drafts intentionally overwrite supplied fields. Event tests must not
assume cross-request ordering. Prior task/metadata contracts must remain intact while amending
their owning pair.

## Authorized PG recovery and SQLguard checkpoint

ROOT authorized exactly one recovery after the original failed PG setup. Only private fixture
metadata changed: exact Docker ownership/State/Mounts, recorded host process identity and fresh
exact-container absence; no namespace scans/escalation/resource increase. Original failed
setup evidence remains unchanged. Namespace-wide process proof remains unavailable.

Recovery native85826 / 9a3c3c -> 5678ea actually joined exit0 at
2026-10-07T07:58:53.966026Z, wrapper1459967/childgroup1459969 empty. Actual independently
connected PG backend pairs87/88 and90/91 physically waited on workflow-row transaction locks;
both disjoint directions, presence/normalization/hidden/source/CAS/statement rollback passed.
Task-only sqlite3/pgx conformance passed all seven selected task-owner subtests each; no other
owner/harness/upgrade fixture replay. The new exact labelled capped tmpfs container
67ae8eb2aad0c6d9e44f8e31c4ab741186c56cf4d744a3ffd4e72b394e136bb9 was removed and proved
absent; hostPID1460011/startticks24555543 absent, password removed, cleanup_errors empty.
Receipts `pg-recovery*.json`, `pg-tests.json` and `task-conformance.json` retain UTC/cutoffs,
actual stage joins and the unavailable namespace qualification. Passing SQLite/transport
checks were not replayed. Interrupted cheap document native76570/70481f ->64e80d was recovered
and actually joined exit0; catalog,36 spec tests,19 harness tests/full lints and harness hook pass.

Reviewed scoped SQLguard invocation used its default global exemption registry and stopped on
`unused SQL guard exemption internal/task/inventoryrepair/journal.go:verifyBackup:sqlite-catalog`.
Native synchronous243a1e joinedexit1, wrapper1463868/childgroup1463870 empty, UTC
07:59:24.054532 ->07:59:24.642137. CheckFiles requires every supplied exemption to be used
by selected files; the rejected exemption is outside the scoped repository directory.
No SQLguard success verdict, registry change, wider scan or retry. ROOT's out-of-scope rule
requires this new checkpoint. SQLguard/scoped lint/install/hooks/publication remain pending;
HOSTED/MERGE authority NONE. See `sqlguard-checkpoint.json` for fresh group absence.

## Current local delivery result

Implementation and required local validation passed in the same primary session. The required
repository seam is one supplied-column UPDATE RETURNING; ordinary edit, hidden and source/path
writes use it. Exact CAS and intentional full-draft behavior remain. Only two task-handler
standalone fakes needed adaptation; no outside adapter or production package was changed.
Existing task ACs and metadata design remain byte-identical. Public websocket-api.md now
clarifies omission/null/empty/full-draft behavior and observation semantics; scoped backend
guidance names the seam. No UI/browser/E2E/build or hosted success is claimed.

- Actual independently authored SQLite RED lost the other successfully saved field in both
  directions; service/domain/presence/full-draft/fence/auth/cancellation/rollback GREEN passed.
- Corrected registered HTTP/WS overlap and empty/read-only checks passed native23845; my
  first test assertions incorrectly compared nullable DTO pointers with event strings,
  corrected to the existing shape without changing product behavior. New-only null/empty
  request/same-field controls passed native88777/93b58f ->42986c; no passing overlap replay.
- Authorized PG recovery and task-only sqlite3/pgx conformance passed native85826. Original
  setup79404 remains failed/no PG verdict; exact owned recovery cleanup proved, namespace-wide
  qualification remains unavailable as ROOT directed. No foreign/unproved volume used.
- Original directory-only SQLguard243a1e failed because it loaded global exemptions. ROOT
  authorized exactly one canonical `go run -trimpath -tags fts5 -p=1 ./cmd/sqlguard ./internal`,
  matching Makefile/backend CI. Native32978/81e194 ->531ead joined exit0, group1473927 empty.
  No exemption/checker mutation or filtering and no new production scope.
- Affected initial lint passed zero issues native54179/2021ac ->7fe3fb joined exit0,
  wrapper1482462/group1482464 empty, UTC08:05:04.206904 ->08:10:01.360195. Only task service,
  repository/sqlite, handlers, models and repository contract packages; all ten Go files
  staged before lint so new files were included. GOMAX2/GOMEM512/concurrency2/allowserial,
  CLI5m/GNU6mkill10 retained. No initial full ./... lint.
- Final document gates native40988/8a8751 ->c02b82 joined exit0: catalog357 decisions/1416 specs,
  36 spec and 19 harness tests/full lints, 62 public-doc tests/47 published pages, offline
  actual changed-file reference coverage covered/errors0 and whitespace. Design32689 bytes.

Receipts/logs retain original handles, actual OS PID/PGIDs, UTC/immutable cutoffs, joins and
owned group returns in `/tmp/kandev-child68-local-20261007/`. One pinned pnpm9.15.9 frozen apps install
passed native73836/3bee37 ->f7d8ac, joined exit0/group1501257 empty, because commitlint was absent. Normal active pre-commit and commit-msg hooks
are required; a private transparent shim only adds concurrency2/allowserial to the actual
pinned golangci-lint binary for hooks. No bypass/base substitution/global configuration edit.
Commit/push/ready publication are in progress. Separate ROOT HOSTED and MERGE grants remain
absent, so this order remains in_progress and overall delivery is not complete.
