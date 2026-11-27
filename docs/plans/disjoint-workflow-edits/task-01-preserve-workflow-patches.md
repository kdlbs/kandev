---
id: "01-preserve-workflow-patches"
title: "Preserve workflow patch intent"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-FIELD-UPDATES-002
acceptance_criteria:
  - AC-TASKS-FIELD-UPDATES-002.1
  - AC-TASKS-FIELD-UPDATES-002.2
  - AC-TASKS-FIELD-UPDATES-002.3
  - AC-TASKS-FIELD-UPDATES-002.4
  - AC-TASKS-FIELD-UPDATES-002.5
system_design:
  - ../../specs/tasks/system-design/task-field-updates.md
---

# Task 01: Preserve workflow patch intent

## Summary

Preserve supplied workflow fields across independent writers with one typed, touched-column
SQL operation. Deliver real-store concurrency/presence/conflict/rollback and registered REST/WS
evidence in one sequential pass after ROOT explicitly releases implementation and the heavy slot.

## In scope

- Independently author meaningful existing-API SQLite RED: hold the actual initial SQL read,
  commit disjoint second write, release/join first, check successful APIs and lost persisted
  field. Both held-name and held-prompt directions must fail behaviorally before production edits;
  compile/method errors are not RED. Never replay/copy/import the protected ROOT proof.
- Use two independently built bare services, no workers, and independent physical connections
  to one private real database. Retain connection identity evidence; separate service pointers,
  shared handle, delays or mocks alone do not prove independence. Bound every barrier by context/
  timer and release AND join every worker on success/failure/cleanup.
- Add required WorkflowRepository.UpdateWorkflowFields and models.WorkflowFieldUpdate, fixed
  allowlisted supplied-column UPDATE RETURNING. Adapt UpdateWorkflow, SetWorkflowHidden and
  SetWorkflowSource. Preserve exactWorkflowVersionUpdater and UpdateWorkflowIfUnchanged;
  no ordinary snapshot fallback. Adapt only actually necessary fakes/forwarding adapters.
- Test omission/null/empty/mixed/profile trim/no-field timestamp/same-field controls, all-four
  field overwrite/empty control, hidden false/source bundle/normalization/no-op and both ordinary
  name/prompt versus domain-writer overlap directions. Assert unrelated columns unchanged.
- Exercise actual SQLite aborting UPDATE trigger and deterministic pre-write cancellation,
  missing/auth/read-only failure, row/timestamp rollback and no successful events. Do not promise
  reversal of an already committed mutation or infer an unobserved commit from transport failure. Race stale exact command with ordinary patch;
  check typed conflict/no mutation, successful exact and unavailable-fencing controls.
- Register actual HTTP router and WS dispatcher around real service/store/event recorder;
  inspect DB, response and event for supplied presence and existing error mappings. Current
  read-only/auth preflights may make extra reads: gate actual relevant SQL read rather than
  assume call count. Do not assert total event order or latest concurrent snapshot.
- Required shared-method PG evidence: bounded proven-owned disposable fixture/private schema,
  distinct physical backend PIDs, native row blocker, observed actual PG lock wait, committed
  disjoint change before release, joined patch and final row. Cover presence/normalization,
  hidden/source/exact conflict and actual statement rollback. No RMW/advisory/graph transaction.
- Run targeted suites/scoped SQLguard/conformance/lint; assess only a narrow existing API-reference
  clarification and scoped backend guidance if needed. Do not rewrite historical plan results.

## Out of scope

Schema/API/revision/generic transactions, global writer or task hierarchy project, stale full
editor merge, global event order/latest snapshot, browser/E2E/frontend builds or passing replays,
DESIGN product checks/install/permanent tests, delivery operations without later ROOT grants.

## Acceptance

1. Actual independently authored pre-fix causal RED becomes GREEN in both workflow overlap
   directions, including audited domain writers, with presence/full-draft controls preserved.
2. Real rollback/fail-closed CAS/auth/read-only and registered REST/WS DB/response/event evidence
   satisfies 002.2 through 002.5; unrelated fields and existing observation semantics remain.
3. Scoped PG physical concurrency, storage gates and relevant lint pass within owned bounds;
   every original native handle and fixture actually returns before the heavy slot is released.

## Verification

DESIGN must not execute these product commands. After explicit ROOT interrupt implementation
release AND ONE GLOBAL LOCAL-HEAVY grant, load /tdd and its backend-tests.md reference, mark this
work order in progress and run sequentially from repo root using Bash login=false:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/golangci-lint/2.9.0/golangci-lint-2.9.0-linux-amd64:$PATH"
export GOMAXPROCS=2 GOMEMLIMIT=512MiB
# RED PLANNED test: before production edits both causal subtests must lose persisted intent.
(cd apps/backend && timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -timeout 5m -count=1 -run '^TestWorkflowFieldUpdatesConcurrentSQLite$' ./internal/task/service)
# GREEN: TestWorkflowFieldUpdates* and TestRegisteredWorkflowFieldUpdates* names are PLANNED.
# The nine other anchored names exist in current service/handler source. Compile these packages.
(cd apps/backend && timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -timeout 5m -count=1 -run '^(TestWorkflowFieldUpdatesConcurrentSQLite|TestWorkflowFieldUpdatesDomainWriters|TestWorkflowFieldUpdatesPresence|TestWorkflowFieldUpdatesFullDraftControl|TestWorkflowFieldUpdatesExactFence|TestWorkflowFieldUpdatesFailures|TestRegisteredWorkflowFieldUpdatesHTTP|TestRegisteredWorkflowFieldUpdatesWS|TestService_UpdateWorkflow|TestService_UpdateWorkflow_PublishesPromptOnWorkflowEvent|TestService_SetWorkflowHidden_HealsStaleRecord|TestSetWorkflowSourceStampsProvenanceAndPublishes|TestHTTPUpdateWorkflowUpdatesFields|TestHTTPUpdateWorkflowRejectsInvalidBody|TestWSUpdateWorkflowUpdatesFields|TestWSUpdateWorkflowRequiresID|TestWSUpdateWorkflowSurfacesRepositoryFailure)$' ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers)
# PLANNED PG tests. DSN belongs solely to the owned fixture. Require it; SKIP is not acceptance.
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -timeout 5m -count=1 -v -run '^TestPostgresWorkflowFieldUpdates(PhysicalConcurrency|Compatibility)$' ./internal/task/repository/sqlite)
(cd apps/backend && timeout --kill-after=10s 6m go run -trimpath -tags fts5 -p=1 ./cmd/sqlguard ./internal/task/repository/sqlite)
# Existing owner subtests only: TestStoreConformance/sqlite3/task and /pgx/task.
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -timeout 5m -count=1 -v -run '^TestStoreConformance$/^(sqlite3|pgx)$/^task$' ./internal/persistence/storeconformance)
(cd apps/backend && timeout --kill-after=10s 6m golangci-lint run --timeout=5m --concurrency=2 --allow-serial-runners --new-from-rev=905fa03c5b8ae90c661dc9fec2e324d355e269aa ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers ./internal/task/models ./internal/task/repository)
```

Initial lint scope is the three affected service/store/handler packages plus the directly owned
model and repository-interface packages. No initial ./... pass. Service tests use real SQLite;
inventoried handlers/process_handlers_test.go and workflow_handlers_test.go fakes are covered. The separate
orchestrator/executor/executor_mocks_test.go declares UpdateWorkflow; extend lint/compile only to
./internal/orchestrator/executor if this required interface actually forces a changed fake there.
The audited backendapp workflowProviderAdapter intentionally supplies four fields and needs no
planned migration; add ./internal/backendapp only for an inventoried, actually necessary changed
forwarding adapter. No other package expansion without a ROOT scope checkpoint.

Conformance source audit: suite_test.go's TestStoreConformance invokes the helper Run, which
names each subtest engine/descriptor.ID; engine.go declares sqlite3 and pgx. requiredstores
catalog owns task at internal/task/repository/sqlite, distinct from workflow (step/template store).
The anchored selector excludes harness/*, task-share, every other owner and every top-level
TestPreviousStableUpgrade*/TestUpgradeFixtureManifest test. The selected task owner retains its
own fresh/replay/CRUD/capability scenarios, not full upgrade-fixture replay. Run already calls
ValidateAdapters before subtests; no separate catalog-only product command is materially needed
for unchanged descriptors/adapters. Conformance is scoped storage evidence, not causal patch proof.
No product test was run to audit selectors.

Each invocation needs original native session handle, child PID/PGID, private log, actual UTC
start and immutable cutoff captured by the later owned supervisor. Join the same handle and
prove group physical return; no duplicate launch or invented result. These shell blocks define
sequential commands, not an unsupervised batch. Timeout/resource/transport/unknown/out-of-scope
results require ROOT checkpoint, without automatic retry. Compile additional actually touched
standalone-fake packages only if required; report adaptive scope before execution.

PG fixture receipt must identify exact ownership/resource IDs/image digest, all mounts/volumes,
private schema and protected credential file location without printing secrets. Private loopback
and proved-owned ephemeral storage only. Join processes before exact owned-resource cleanup and
verify physical return. Unknown/shared fixture, skipped tests or absent grant require ROOT
checkpoint, not independent provisioning/retry. Reuse existing private-schema PG test patterns.

If later authorized backend CI remediation is necessary, run one full CHANGED ./... lint pass at
the exact API comparison base with GOMAXPROCS=2/GOMEMLIMIT=1GiB, concurrency 2/allow serial,
CLI 5m/GNU 6m kill-after 10s. Historical diagnostic SHA is not an invented live API base.

Cheap document gates are DESIGN-authorized:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks docs/plans/disjoint-workflow-edits
git status --short -- docs/plans/disjoint-workflow-edits
```

Offline reference preflight: .github/scripts/pr-docs.cjs exported validateCoverage with proposed
runtime file plus changed work order and exact local four-document contents. Check every AC/REQ,
design requirement declaration and plan inclusion; no hosted mutation. Record actual commands/
results in this work order and manifest, never overwrite historical companion delivery results.

## Files likely touched

Task-relative paths under apps/backend/internal/task:

- models/workflow_field_update.go; repository/interface.go
- repository/sqlite/workflow_field_updates.go; workflow.go only for necessary reuse
- service/service_resources.go; service/service_workflow_field_updates_test.go
- repository/sqlite/workflow_field_updates_test.go; workflow_field_updates_postgres_test.go
- handlers/workflow_field_updates_integration_test.go and actually affected existing workflow
  handler/service fake files only as needed for compilation and their original test purpose
- Adapter files only for actual required forwarding gaps. Audit full-field backendapp/services.go
  and web workflow-card-actions.ts without silently reinterpreting their request presence
- Four design artifacts, and narrowly needed public websocket-api.md or scoped backend AGENTS.md
  clarification only at implementation if current text requires it

## Dependencies

None between work orders. Later ROOT implementation release and heavy slot are mandatory barriers.
The protected proof and exact current baseline are read-only inputs, not imported test code.

## Risks

Optional/snapshot fallback reintroduces data loss. RETURNING/normalization can differ by dialect.
Interface fake gaps can hide missing coverage. Event interleaving is permitted; no ordering test.
Manifest retains separate HOSTED/SERIALMERGE/normal-hooks/CI/review/ROOT completion gates.

## Parallelism

`sequential`. Same primary; no delegates/new tasks/sessions/model switches.

## Inputs

- REQ-TASKS-FIELD-UPDATES-002 and all five ACs; proposed workflow boundary in owning design.
- Current workflow source/tests and protected diagnostic evidence, read only.
- Kandev task 55d592b3-ba6c-4794-b8de-5b2fcaeb46d2, session
  61b6b8bf-0371-4d81-a499-e3fed018f9ce. Own task plan retains system marker/user edits/barriers.

## Results

Historical design checkpoint: implementation was pending. DESIGN checks passed: catalog (357 decisions, 1416 specs), all 36 spec
linter tests, full specification lint, diff whitespace and offline documentation coverage
(covered, zero errors). Original check handle 23436 actually joined exit 0. Index empty; exactly
four unstaged/uncommitted artifacts. Historical task ACs and metadata design verified unchanged;
protected proof checksum/mode unchanged. No product/permanent-test/install/commit/PR action.
Design does not satisfy implementation/product validation/delivery completion.

ROOT-requested cheap amendment checks passed again: catalog, all 36 spec tests, full spec lint,
offline reference coverage and whitespace/status. Native handle 99549 actually joined exit 0.
Nine existing anchored names verified from source; eight new workflow names explicitly planned.
Conformance selector audited against actual harness/engine/catalog IDs; task SQLite/PG only,
no full-store/harness/upgrade-fixture execution. Cancellation contract now proves deterministic
pre-write cancellation/aborting-statement rollback, not reversal or guessed commit outcomes.
Four artifacts remain unstaged/uncommitted; prior task/metadata scope and all ROOT barriers remain.
No product tests, install, production or heavy slot claimed. END amended DESIGN for ROOT seal.


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
