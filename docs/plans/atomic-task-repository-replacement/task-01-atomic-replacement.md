---
id: "01-atomic-replacement"
title: "Replace task repository associations atomically"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.1
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.7
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.8
system_design:
  - ../../specs/tasks/system-design/attach-workspace-source-replacement.md
---

# Task 01: Replace Task Repository Associations Atomically

## Summary

Repair complete task association replacement as one sequential service/store/transport outcome.
Prepare side-effectful references before the transaction, then finalize against a task-serialized
canonical snapshot and commit delete plus all inserts once. Preserve compatibility and prove real
storage failure/cancellation and independent connection serialization.

## In scope

- Implement the domain-specific TaskRepoRepository replacement seam and pure prepared-input
  finalizer in the [design](../../specs/tasks/system-design/attach-workspace-source-replacement.md).
- Preserve the distinction between direct replacement and UpdateTask checkout semantics; move the
  update's canonical association inheritance into the locked phase with tx-observed environment state.
- Real permanent RED, minimal correction, meaningful SQLite/PG/store/registered transport GREEN,
  existing targeted controls, SQLguard and task-owner conformance, public failure-boundary clarification.

## Out of scope

No runner mutability gate, launch policy, task-wide rollback, Git/repository entity compensation,
CreateTask multi-insert fix, schema, generic transaction engine, new transport contract, UI or browser
work, broad suites, delegations/model/session changes or parent-proof replay/cleanup.

## Acceptance

1. All mapped tests prove exact old rows on failed preparation/validation/storage/precommit
   cancellation and exact ordered full/empty set on success; no partial persisted replacement.
2. Canonical policy and checkout finalization occurs after physical serialization, with observed
   independent SQLite/PG contention and committed predecessor inheritance; F19 and existing error,
   snapshot, metadata, omitted-update and environment-created checkout behavior remain intact.
3. Registered REST/WS real-database results/events and fresh-branch/update field boundary controls
   match the owning contract; all required affected checks pass with retained joined receipts.

## Files owned by this order

Production (only as needed):

- `apps/backend/internal/task/models/task_repository_replacement.go`: domain snapshot (avoids the
  repository/provider/concrete-store import cycle).
- `apps/backend/internal/task/repository/interface.go`: one association replacement operation and
  domain snapshot; exact new method name/signature documented before edits.
- `apps/backend/internal/task/repository/sqlite/task_repository.go`, optional focused
  `task_repository_replacement.go`: tx-bound ordered reader/full-column inserter and replacement.
- `apps/backend/internal/task/service/service_tasks.go`, optional focused
  `service_task_repository_replacement.go`: direct/update routing and prepared/finalized inputs.
- `apps/backend/internal/task/service/service_task_branch_policy_snapshot.go` and
  `repository_checkout_options.go`: factor only required pure rules; keep other callers compatible.
- `apps/backend/internal/task/handlers/task_http_handlers.go` / `task_ws_handlers.go`: only necessary
  compatibility wiring; existing registered actions and payloads retained. No route expansion.

Permanent tests:

- `apps/backend/internal/task/service/service_task_repository_replacement_test.go`: the service
  test anchors in the plan, real file-backed SQLite and SQL failure trigger.
- `apps/backend/internal/task/repository/sqlite/task_repository_replacement_test.go` and
  `task_repository_replacement_postgres_test.go`: store transaction, persistence and observed locks.
- `apps/backend/internal/task/handlers/task_repository_replacement_test.go`: router/dispatcher
  and actual service/database; full public payloads and event recording.
- Existing service snapshot/checkout/F19, handler fixtures, and
  `apps/backend/internal/orchestrator/executor/executor_mocks_test.go` only if their narrow fixture
  interfaces need the new method; never weaken assertions or substitute failure mocks.

Docs:

- The four artifacts in this package; update only selected contract and implementation results.
- `docs/public/tasks-and-workflows.md`: two concise sentences near Multiple repositories/fresh-branch
  recovery explaining failed association replacement retention and separate earlier edits/Git work.
  This remains a how-to guide; no page/media/navigation or screenshot change.

Read-only dependencies: `internal/db/tasklock.go`, `internal/db/sqlite.go`, raw task lock and source-batch
patterns, `internal/db/dialect`, `repository.ProvideContext`, task models/metadata validators,
registered task routes, DTO/error/event projection, existing storeconformance task adapter and
`internal/testutil` independent PG helpers. Do not redesign them without ROOT's direction.

## Dependencies

None. ROOT's later explicit implementation interrupt is required before any production or permanent
test edit. Work owns all mutually dependent changes; do not split by layers or spawn workers.

## Verification

Run from the repository root. One heavy command at a time; retain every session_id and actually join
it before the next command. No arbitrary sleep proves contention; join fixture goroutines/connections.
Every command below is future work unless recorded in Results. Planned test names must exist before
execution; do not accept PASS with no matched tests/subtests.

First add the service regression without changing production code, then run this exact RED anchor.
It must compile and fail for lost original rows in the direct/update resolver/valid second-insert cases:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p 1 ./internal/task/service -run '^TestTaskRepositoryReplacementPreservesOriginalSet$' -count=1 -timeout=5m)
```

After minimal correction, complete the designed regression matrix and run these affected commands
sequentially, preserving exact commands, counts and exit codes:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p 1 ./internal/task/service -run '^TestTaskRepositoryReplacement(PreservesOriginalSet|CompleteSet|Compatibility|Boundary)$' -count=1 -timeout=5m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p 1 ./internal/task/repository/sqlite -run '^TestTaskRepositoryReplacement(StoreRollback|CompleteSet|Cancellation|SQLiteSerialization)$' -count=1 -timeout=5m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p 1 ./internal/task/handlers -run '^TestRegisteredTaskRepositoryReplacement$' -count=1 -timeout=5m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p 1 ./internal/task/service -run '^(TestReplaceTaskRepositoriesSwapsAssociations|TestReplaceTaskRepositoriesPreservesFreshBranchWithPolicySnapshot|TestReplaceTaskRepositoriesBypassesRunnerMutabilityGate|TestRepositoryCheckoutOptionsMetadata|TestRepositoryCheckoutOptionsPreservedOnUpdate|TestRepositoryCheckoutOptionsRejectLocalRepository|TestRepositoryCheckoutOptionsRejectInvalid|TestRepositoryCheckoutCapabilitiesRejectExecutorCredentials|TestRepositoryCheckoutOptionsRejectAmbiguousAttachment|TestRepositoryCheckoutOptionsPreservedWhenBranchChanges)$' -count=1 -timeout=5m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p 1 ./internal/task/repository/sqlite -run '^(TestListTaskRepositoriesOrdersByIDWhenPositionAndCreatedAtTie|TestListTaskRepositoriesByTaskIDsOrdersByIDWhenPositionAndCreatedAtTie)$' -count=1 -timeout=5m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p 1 ./internal/persistence/storeconformance -run '^TestStoreConformance$/^sqlite3$/^task$' -count=1 -timeout=5m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go run -trimpath -tags fts5 ./cmd/sqlguard ./internal)
```

The service `Boundary` test includes ordinary task-field persistence. Put actual fresh-branch
handler/Git failure-boundary coverage under the registered handler test anchor, using a disposable
Git repository and real association storage failure; retain the existing 5xx without promising Git
rollback. No backend process or browser is started.

For PostgreSQL, require a real authorized DSN (do not print secrets). Run on existing dedicated test
service or owned private fixture per the plan; absent local proof, require actual hosted test log
receipts on current published head. Environment-gated skipped cases never satisfy acceptance.

```bash
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?real isolated PostgreSQL DSN required}" && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p 1 ./internal/task/repository/sqlite -run '^TestPostgresTaskRepositoryReplacement(Rollback|Serialization)$' -count=1 -timeout=5m)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?real isolated PostgreSQL DSN required}" && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p 1 ./internal/persistence/storeconformance -run '^TestStoreConformance$/^pgx$/^task$' -count=1 -timeout=5m)
```

Serialization proof: at least three independent physical connections/PIDs, worker genuinely waiting
on the task lock before canonical callback, holder commits distinct changed policy/checkout data,
successor inherits committed data and produces one complete winning set. Also test two full
replacements without interleaved rows. Lock-wait cancellation retains exact originals and joins
workers. SQLite independent handles use real writer-before-read conflict, context cancellation and
observable callback barriers; normal shared pool queueing alone is insufficient proof.

Backend CHANGED lint: pin `TASK_PR_BASE` to the exact PR base SHA before the one run, rather than a
moving origin/main. Do not infer success from zero issues followed by timeout exit 4. GNU timeout
6m/kill-after 10s, CLI timeout 5m, concurrency 2/serial runners, memory 1GiB. No resource retry:
checkpoint and ask ROOT for bounded direction if this fails. Docs-only fixup does not trigger another
backend lint; corrective backend edits justify affected reruns only.

```bash
(cd apps/backend && : "${TASK_PR_BASE:?exact PR base SHA required}" && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev="$TASK_PR_BASE" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Lightweight artifact gates (also allowed during design):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks docs/plans/atomic-task-repository-replacement
git status --short -- docs/specs/tasks docs/plans/atomic-task-repository-replacement
```

Use `.github/scripts/pr-docs.cjs` exported `validateCoverage` against actual package contents and
changed paths, including untracked docs; assert this order's eight AC IDs belong to its requirement,
its design declares the requirement, and the plan includes that design. Once the public guide is
edited, run `node --test scripts/validate-public-docs.test.mjs` and
`node scripts/validate-public-docs.mjs`. Do not install or execute Go/product tests during design.

Later hooks: inspect dependency presence first. If absent, exactly one pinned pnpm 9.15.9
`install --frozen-lockfile` from `apps/` before hooks; keep ordinary hooks active. No test/assertion,
retry/timeout/race/hook weakening, cache wipes or foreign process kills.

## Risks

See the plan's risks and the design's legacy writer limits. Canonical snapshot and error precedence
must be based on the locked set, and complete metadata/policy columns must survive transactional
insertion. A callback using writer/reader/provider APIs violates the pure boundary. Failed
replacement does not compensate earlier field edits or Git work. Postcommit reads do not provide a
global event order or stale-reader guarantee.

## Parallelism

`sequential`. Keep this primary session; no workers or additional task/session/tab/model.

## Inputs

- [Owning requirement](../../specs/tasks/requirements/attach-workspace-sources.md), selected REQ-002.
- [Focused design](../../specs/tasks/system-design/attach-workspace-source-replacement.md).
- [Plan](plan.md), accepted ROOT proof and implementation/delivery resource rules.
- Existing snapshot tests in `service_repository_branch_policies_test.go`, checkout tests in
  `repository_checkout_options_test.go`, F19 in `service_runner_switch_test.go`, ordering tests and
  `runner_switch_postgres_test.go` independent backend-wait pattern; backend test skill reference.

## Results

ROOT released implementation in a later explicit continuation. Task01 remains in progress through
normal hosted gates and independently verified squash merge; external head-specific receipts belong
in the existing Kandev task plan so publishing them does not create another CI cycle.

Permanent real-SQLite RED: exact first command above, handle 29828 joined exit 1, test 0.320s/package
0.388s. Direct/update resolution and valid second-insert trigger failures lost the exact original rows;
the successful update control passed. ROOT's accepted archived proof was not replayed or modified.

Implemented the model-owned domain snapshot (avoiding the repository/provider/concrete-store import
cycle), deep-copied preparation, pure canonical policy/checkout finalizer, writer-before-read task
guards, full-column transaction and safe committed-row response fallback. The only executor-package
edit supplies a fail-closed method to its explicit test aggregate interface; no executor behavior changed.

Joined GREEN receipts so far, using the exact resource-capped/tagged/race commands above:

- Service matrix: handle 73482 exit 0, package 1.541s. Includes canonical interleaving, explicit policy
  precedence, equality after environment creation, redirect matching, contribution metadata, early
  provider/checkout failures, separate task edits and committed-row fallback.
- Existing snapshot/checkout/F19 controls: handle 49158 exit 0, package 1.575s.
- Store matrix: handle 54264 exit 0, package 2.476s, including foreign-owner rejection, real second-insert
  rollback, encoding, cancellation and independent SQLite concurrent replacements.
- Registered REST/WS and fresh-Git boundary: handle 64532 exit 0, package 2.653s. Actual registered
  envelopes/mappings, complete results, explicit empty event array, no failed update event, and
  retained creation evidence plus exact original DB row after Git success/storage failure.
- Task store conformance: SQLite handle 83399 exit 0, package 4.685s; real PostgreSQL handle 69663
  exit 0, package 8.339s. The owned PG fixture has no volumes/binds; full allocation/cleanup receipts
  live in the Kandev plan. PostgreSQL skips are not proof.

Final PostgreSQL session-isolation control: handle 77949 joined exit 0, package 3.593s, with distinct
physical PIDs and observed lock waits despite a RepeatableRead worker session default. Ordering
controls: handle 27521 joined exit 0, package 1.318s. The exact owned PG container/tmpfs and credential
files have been removed and absence verified; no volume/bind resources existed.

Corrected SQLguard: handle 16562 joined exit 0, no violations. The exact pinned full CHANGED lint
handle 57618 joined exit 124 at the GNU 6m bound (CLI 5m configured), with empty diagnostics. This is
FAILED and does not establish lint cleanliness. No automatic resource retry was attempted. All owned
fixture/command handles are terminal. ROOT authorized one identical bounded warm-cache recovery;
handle 44426 joined exit 1 with gocritic dupBranchBody (handler test line 152), ifElseChain
(handler line 169 and service line 328), and nestif complexity 11 (handler line 130). Recovery log:
`/tmp/kandev-root23-lint-recovery.log`. ROOT separately authorized minimal correction of these actual findings in the two owned test
files. The real transport helper and switch assertions preserve all scenario mappings and
original-row/event/response checks. Affected service handle 48475 joined exit 0, six named canonical
scenarios matched (package 1.399s); the parent compatibility body necessarily executes too. Registered
transport handle 28673 joined exit 0, twelve REST/WS cases matched (package 2.584s), fresh-Git excluded.
No store/PG/conformance/ordering/SQLguard or other service matrices were replayed. The single
authorized corrected-code full CHANGED lint handle 85224 joined exit 0 with zero issues at the same
base and bounds; log `/tmp/kandev-root23-lint-corrected.log`. The initial timeout remains failed
evidence with unproved cause. The staged implementation remains reviewable. SQLguard's first run found the SQLite placeholder-boundary omission; it was corrected
with the existing tx.Rebind pattern. No guard suppression was added. Public docs validation passed
62 tests and 47 pages; catalog, spec lint, reference coverage and whitespace passed.

Dependencies were absent: one frozen install from apps using pinned pnpm 9.15.9 joined exit 0, 923
reused packages, zero downloads. Existing pre-commit and commit-msg hooks are active. Owned temporary
wrappers keep the actual hook linter bounded at concurrency 2, CLI 5m/GNU 6m/kill-after 10s and route
pnpm to the verified cached 9.15.9; neither hook nor assertions are bypassed. Primary session only.
