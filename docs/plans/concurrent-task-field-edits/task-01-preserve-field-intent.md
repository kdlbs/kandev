---
id: "01-preserve-field-intent"
title: "Preserve request intent through the task-row write"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-FIELD-UPDATES-001
acceptance_criteria:
  - AC-TASKS-FIELD-UPDATES-001.1
  - AC-TASKS-FIELD-UPDATES-001.2
  - AC-TASKS-FIELD-UPDATES-001.3
  - AC-TASKS-FIELD-UPDATES-001.4
  - AC-TASKS-FIELD-UPDATES-001.5
  - AC-TASKS-FIELD-UPDATES-001.6
  - AC-TASKS-FIELD-UPDATES-001.7
system_design:
  - ../../specs/tasks/system-design/task-field-updates.md
---

# Task 01: Preserve request intent through the task-row write

## Summary and implementation barrier

Make ordinary partial updates preserve omitted current task fields through the actual storage
boundary, with current-row hierarchy/completion validation and existing ownership rules. Keep
the service/association/transport/event behavior in one coherent sequential slice. ROOT reviewed this package and sent the later explicit IMPLEMENTATION release to this sole
primary on 2026-10-04. This order and its plan are now `in_progress`.

## Interfaces, dependencies, and likely files

Read the paired design's complete dependency and writer inventories before editing.
Existing symbols keep their contracts; implemented new files are marked new below.

| Owned file or boundary | Responsibility |
| --- | --- |
| `apps/backend/internal/task/models/task_field_update.go` (new) | `TaskFieldUpdate`, `TaskFieldUpdateResult`; typed request presence and locked prior state/step/parent-change result |
| `apps/backend/internal/task/models/task_metadata_update.go` (new) | Relocated existing protection computation, `ProtectedTaskMetadataUpdate`; no added namespaces/merge policy |
| `apps/backend/internal/task/repository/interface.go` | Required `UpdateTaskFieldsWithParentAdmission` using existing validator alias |
| `apps/backend/internal/task/repository/sqlite/task_field_updates.go` (new) | SQLite/PostgreSQL typed patch transaction; final current `FOR UPDATE` read after step locks; apply presence, reuse hierarchy and `updateTaskTx`, postcommit dispatch |
| `apps/backend/internal/task/repository/sqlite/task_hierarchy_admission.go`, `task.go` | Reuse current-row reader/normalization/step locks; private omitted-metadata context presence passes through `updateTaskTx` to `buildTaskUpdateQuery` so a locked document is not treated as a supplied merge patch. Scope private presence to mutation only, never postcommit dispatch; legacy entry-point policy remains |
| `apps/backend/internal/task/service/service_tasks.go`, `service_task_metadata.go` | Required patch call, normalized assignee presence, fix priority-only guard, locked prior state/step for bookkeeping, service wrapper for relocated metadata helper |
| `apps/backend/internal/task/service/service_requests.go` | Preserve current request contract; avoid extra fields or changed wire shape |
| Service/repository/handler tests named below | Actual data and concurrency assertions; limited helper construction cleanup and interface adaptation |
| `apps/backend/internal/task/handlers/task_http_handlers.go`, `task_ws_handlers.go`, `task_handlers.go` | Read-only existing registration/normalization/mapping dependency. Only causal mapper correction, if proven, with registered regression before publication; no new `WorkflowStepID` mapping |
| `apps/backend/AGENTS.md` | Brief factual ordinary-patch versus full-snapshot boundary update if implementation makes current guidance stale |
| `docs/public/websocket-api.md`; optionally `tasks-and-workflows.md` | Short field-omission/concurrency clarification with explicit metadata/full-snapshot limits; no new page or UI copy |

Standalone `TaskRepository` test implementations exist in service `agent_conversations_test.go`,
`agent_conversations_pagination_test.go`, `handoff_workspace_test.go`, and `handoff_cascade_test.go`;
inventory exact compile fallout cheaply after adding the interface method. Adapt only required
fakes/wrappers, retaining their original purposes. Embedding `TaskRepository` forwards the required
method, so legacy recording overrides are not evidence that the new path was intercepted. Install
test wrappers during service construction, before cleanup worker start; never swap dependencies
after `NewService`.

Dependencies: parent24 hierarchy admission and parent23 atomic repository replacement are already
merged in base `5db79135cb67cb21a0747b2e96444a555343d972`. Reuse current graph policy/normalization,
F19 snapshots, protected metadata/title CAS, completion and transition helpers. No new package,
schema, global revision, callback framework, UI, runner, launch policy, or resource cascade.

## Acceptance

1. Presence reaches the required canonical method, which creates its candidate from the locked
   current task. Both disjoint orders retain both fields across independent Services/connections;
   scalar/CAS prior commits, mixed priority/assignee and explicit-empty/same-field controls work.
2. Metadata replacement/pending-title behavior, current protected owners, hierarchy/ABA/cycle,
   completion/step/position and association/F19 contracts remain. Failures roll back actual rows,
   ledgers and entry effects, preserve typed causes and suppress success publication.
3. Registered REST/WS and actually executed independent PG physical-wait cases prove the real
   boundary; exact affected race and lint gates pass with joined receipts, then authorized normal
   delivery reaches independently verified merge and exact owned cleanup.

## TDD and meaningful permanent tests

The nine named tests below now exist. Initial existing-API RED was recorded before production edits. First add the existing-API regression
in `service/service_task_field_updates_test.go`. Use two independently constructed Services with
independent repository handles on the same private file-backed SQLite DB, schema initialized
once. Inject delegating `GetTask` wrappers during construction: each caller's first target read
actually fetches the row, signals snapshot arrival, then blocks on its own context-bound release.
Do not intercept or fake business predicates. Release first caller, await its actual successful
API completion, release second and join; run both orders. This test compiles and fails for lost
fields on unmodified production code. Keep sequential and unrelated fields controls. The old
parent-owned proof is read-only and never copied into the repository wholesale or replayed.

| Exact permanent test | Required assertions and criteria |
| --- | --- |
| `TestTaskFieldUpdatesConcurrentSQLite` | Both title/description orders; two actual snapshots before either mutation; APIs succeed, canonical DB retains both edits, hierarchy/workspace/priority controls and ordinary events; `.1`, `.2` |
| `TestTaskFieldUpdatesRequestPresence` | Scalar omission/null-equivalent service inputs, title/description empty, unassign/detach, position zero, repositories nil/empty, empty/nonempty metadata; same-field later-write behavior and mixed fields. Do not promise null clears a pointer. `.2`, `.3`, `.7` |
| `TestTaskFieldUpdatesPriorityAndAssignee` | Actual service plus DB mixed priority and valid human assignee/clear, genuine priority-only scalar control, invalid assignee retains row and no success event; no call-count-only test. `.3` |
| `TestTaskFieldUpdatesProtectedOwners` | Actual generated-title claim and set between early read and patch; current omitted title/owner retained; human title resolves ownership and late agent setter loses. Real deferred-launch CAS and handoff/carrier/carry mutations, explicit metadata replacement/deletion/null controls, current protections with and without explicit position; omitted pending metadata preserves null values while supplied pending maps retain existing null deletion; no helper-only decision evidence. `.4`, `.5` |
| `TestTaskFieldUpdatesCurrentRowFailure` | Real intervening hierarchy change invalidates mixed field+parent request; current completion guard rejects supplied state; malformed persisted metadata/current-row decode and unencodable requested metadata roll back, real store trigger rejects update, cancelled context propagates. Compare actual before/after row, timestamps, metadata, ledger and entry records; typed causes via `errors.Is`; no success events/dispatch. Include a succeeding control. `.3`, `.6` |
| `TestTaskFieldUpdatesCoupledWorkflow` | Intervening real scalar state and existing workflow move before omitted-field patch survive; omitted step/state creates no transition/event; explicit service-level step uses current source and records the actual ledger. The repository current-row test verifies entry allocation, committed dispatch identities and runner projection under unchanged rules. Explicit state event old-state is the locked current state, with no new postcommit ordering assertion. `.3`, `.5`, `.6` |
| `TestTaskFieldUpdatesRegisteredProtocols` in `handlers/task_field_updates_test.go` | Register actual `RegisterTaskRoutes` PATCH and `ws.ActionTaskUpdate` dispatch with real Service/SQLite. Both transport pairings and commit directions exercise disjoint updates with construction-time initial-read gates; assert canonical DB, returned DTO and actual bus events. Omitted/null/empty metadata/title/description/repositories/parent/assignee, existing trim/clear markers, invalid mixed request and existing registered association-failure controls complement these cases; `.1` to `.7` |
| `TestTaskFieldUpdatesSQLiteCurrentRow` in `repository/sqlite/task_field_updates_test.go` | New method plus independent real handles; locked current scalar fields, mixed explicit parent, encode rollback, literal position; real committed ledger/entry/runner dispatch and late runner-trigger rollback; actual dispatched legacy writer retains its pending merge behavior. Current-row decode/store/cancel controls are in the Service test. No helper predicates; `.2` to `.6` |
| `TestTaskFieldUpdatesPostgresPhysicalWait` in `repository/sqlite/task_field_updates_postgres_test.go` | Env-gated new method, `newHierarchyPostgresRepoPair`, distinct backend PIDs/private shared schema/observer. Workspace blocker proves graph reservation; task-row-only blocker proves final read waits even without workspace lock. Observe actual `pg_stat_activity`/`pg_blocking_pids` wait, mutate disjoint scalar field in blocker and commit, then join waiter and inspect row. Both fields/orders, mixed metadata/title, cancellation/no mutation, valid-parent and rejected-parent controls; `.1` to `.6` |

Tests use channels as causal synchronization, deadlines as failure bounds, no timing thresholds
or sleeps as a correctness claim. Join every worker/cleanup worker/DB connection/observer before
fixture teardown even on fatal assertion. Actual current-row predicates run; fault-inject only
real transport/storage barriers or errors, not a mocked policy answer. Inspect actual event
payloads, not just counts. If protocol mapping changes, add its actual registered case before
publication and rerun it. Do not claim a mapper unit test is registered protocol evidence.

## Verification commands after implementation release

Run from repository root. Each command is separately joined before any next heavy operation.
No giant package suite or broad local test audit. Before RED/GREEN, use `rg -n '^func Test...'`
to prove every selected test exists; during execution retain `-v` output to prove no empty
selection. New test names intentionally cannot be found during DESIGN; existing control names
were found in source at design handoff. Revise selectors only if the final causal test inventory
changes, and record the exact final command/results in this order and plan.

Meaningful initial RED, before proposed API/production edits:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^TestTaskFieldUpdatesConcurrentSQLite$' -count=1 -v)
```

Affected GREEN commands (service, protocols, storage; independently rooted):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^(TestTaskFieldUpdatesConcurrentSQLite|TestTaskFieldUpdatesRequestPresence|TestTaskFieldUpdatesPriorityAndAssignee|TestTaskFieldUpdatesProtectedOwners|TestTaskFieldUpdatesCurrentRowFailure|TestTaskFieldUpdatesCoupledWorkflow|TestTaskHierarchyAdmissionConcurrentMoves|TestTaskHierarchyAdmissionSnapshotPreservation|TestTaskHierarchyAdmissionChildReadCancellation|TestTaskHierarchyAdmissionChildDecodeAndDepthErrors|TestTaskHierarchyAdmissionDirectParentCancellation|TestTaskRepositoryReplacementCompatibility|TestTaskRepositoryReplacementBoundary|TestTaskRepositoryReplacementRejectsTaskReferencesBeforeEntityCreation|TestUpdateTaskTitle_ClearsPendingAgentTitleMetadata|TestUpdateTaskDescriptionOnly_LeavesPendingAgentTitleAndStoredTitleIntact|TestServiceUpdateTaskWithNewStepRecordsTaskUpdate|TestServiceUpdateTaskWithoutStepChangeRecordsNoLedgerRow)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/handlers -run '^(TestTaskFieldUpdatesRegisteredProtocols|TestTaskHierarchyAdmissionRegisteredREST|TestTaskHierarchyAdmissionRegisteredWS|TestRegisteredTaskRepositoryReplacement|TestHTTPUpdateTaskTitleOnlyPreservesRepositories|TestHTTPUpdateTaskExplicitEmptyRepositoriesClears|TestWSUpdateTaskTitleOnlyPreservesRepositories|TestHTTPUpdateTask_IgnoresExternalIDField)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestTaskFieldUpdatesSQLiteCurrentRow|TestTaskHierarchyAdmissionSQLiteIndependentHandles|TestTaskHierarchyAdmissionSQLiteWriterBeforeRead|TestUpdateTaskPreservesWinningTitleAgainstStaleUpdate|TestUpdateTaskWithExplicitPositionWritesLiteralValue|TestTaskRepositoryReplacementStoreRollback|TestTaskRepositoryReplacementCompleteSet)$' -count=1 -v)
```

Actual PostgreSQL run, after exact owned fixture receipt and exported private
`KANDEV_TEST_POSTGRES_DSN` (never print credentials). Env-gated SKIP is not proof:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestTaskFieldUpdatesPostgresPhysicalWait|TestTaskHierarchyAdmissionPostgresWaits|TestTaskHierarchyAdmissionPostgresCancellation)$' -count=1 -v)
```

PG fixture: prefer immutable/pinned PostgreSQL 16 image, tmpfs for PGDATA, no anonymous volumes or
host binds. Before use record exact container ID, task label, image digest, **all** mounts and
volumes and private credential location; initialize only private schemas, independent clients.
If image declares a volume, explicitly cover it with tmpfs so Docker creates no anonymous data
volume. Join tests/observers before deleting only this owned container/schema. Never touch old
unknown child20 volume `2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`.

Scoped lint: resolve and record original installed binary and immutable base first. Base remains
the actual PR base from GitHub at publication, not stale remote-tracking state. With shell
variables `task25_lint_binary` and `task25_pr_base` set from those receipts:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m "$task25_lint_binary" run ./internal/task/models ./internal/task/repository ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers --new-from-rev="$task25_pr_base" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Mandatory BACKEND PR-FIXUP full changed-revision gate before push, also against actual immutable
PR base with the **original installed binary**, separately from hooks/scoped lint. ROOT supplies
this task-specific duration exception for a corrected candidate, not a repo policy replacement:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 11m "$task25_lint_binary" run ./... --new-from-rev="$task25_pr_base" --concurrency=2 --allow-serial-runners --timeout=10m)
```

Capture exit, duration, exact binary/base/head and retained handle receipt. Zero issues plus
nonzero/timeout is FAILED. Resource timeout/transport failure means stop new heavy operations,
reconcile exact owned process, retain receipts and end WAITING for ROOT bounded direction; no
automatic retry/cache deletion/foreign kill. Concrete lint findings get the minimum correction
and exact affected check; checkpoint before any further full allowance required by ROOT release.

Cheap documentation checks (no Node setup in design turn):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/concurrent-task-field-edits
```

If public docs change after release, run both `node --test scripts/validate-public-docs.test.mjs`
and `node scripts/validate-public-docs.mjs` with existing runtime PATH. No frontend/Vitest/build/
E2E command is required for this backend data-only slice. All new logic must be exercised through
the meaningful real-path tests above; moving unchanged metadata logic does not require a duplicate
helper-only assurance suite.

## Runtime and hook preparation after release only

At design handoff PATH lacked Node/pnpm and `apps/node_modules` was absent. Existing runtimes were found at
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin` (Node/corepack/pnpm shim) and
`/home/jcfs/.nvm/versions/node/v24.18.0/bin/node`. Prefer the existing mise directory in the local
command PATH, verify actual versions cheaply, and use its `corepack pnpm@9.15.9`; `apps/package.json`
pins that version. Only if deps remain absent, perform a SINGLE `pnpm install --frozen-lockfile`
from `apps/` with pinned 9.15.9 after ROOT release, retain/join its handle. No shell-profile,
harness, lockfile or system installation mutation. Keep deps for parent archive and normal hooks.
The authorized single pinned frozen install actually joined exit 0 (1.8s); dependencies are now
present and retained. No bypass, disabled hooks, amend or moving-main rebase.

## Delivery, resource ownership, and recovery

After joined TDD/affected checks/full lint and normal hook commit, push and ready PR under standing
post-release authorization. Corrections publish promptly; do not wait obsolete CI before pushing.
One practical 90-minute `scripts/pr-await <PR> --mode all-terminal --format json` monitor;
retain/join it before replacement. Current substantive authenticated CodeRabbit App347564 full
report must have source=covered=CURRENTHEAD/kind=reviewed for every changed file. Inspect actual
auto-full first; at most one necessary full request. Incremental-disabled SKIP, Claude 0s ACK,
empty self-review, OpenCode-disabled and boilerplate are not semantic coverage. All inline and
grouped findings get concrete fixed/invalid/optional disposition; do not invent threads or churn
SHA for optional style/logging/extra assurance. No optional second-review wait.

Current-head hosted leaf failure: capture exact logs/artifacts/source classification, then ROOT
scope amendment or ONE explicitly authorized same-head specific-job rerun after terminal workflow.
No blind/second retry, synthetic merge testing or moving-main rebase. Cheap current-main
compatibility/owned-blob checks are allowed. Merge only normal expected-head squash when required
policy/gates are terminal clean, full current review covers all files, and no actionable unresolved,
hidden or error state remains. Independently prove actual GitHub MERGED/mergedAt/merge SHA/parent/
tree/all owned blobs and authoritative remote main; published head is not the merge SHA.

Every handle must actually join, including tests, fixture commands, lint, monitor, merge,
verification and cleanup; lost/interrupted handles give NO VERDICT, reconcile exact owned process
before dependent work. Delete only exact owned fixtures; keep clean managed worktree/deps and
ROOT proof untouched. Completion is actual verified merge plus joined cleanup. Parent owns archive,
proof deletion and next discovery.

Keep task plan version-safe and preserve user edits, system marker, task/session IDs, title owner,
question barrier, completion gates and crash next action. ROOT's queued buffer is full and it
checks this same primary directly; no queue_full message/question retry or runtime/storage
changes. Critical missing direction gets durable exact checkpoint and concise final WAITING,
then end turn before any new heavy action. A successful critical parent-question call ends its
turn immediately; retain actual ID, never fabricate or reask.

## Parallelism and risks

Sequential; no delegation, other persistent task/tab/session/model/profile or operator question.
The last current-row read must follow step locks and hold the native task lock. Do not confuse
postcommit observation with exact receipts or internal snapshot compatibility with patch safety.
Any causal UI/backend-policy expansion needs a concrete ROOT amendment before implementation.

## Inputs and results

Inputs: paired owning requirement/design, parent24 hierarchy and parent23 association specifications,
root/scoped AGENTS, `/fix`, `/spec`, `/plan`, `/planner-orchestration`, `/tdd` backend test reference,
retained ROOT receipts. Mobile audit is backend data-only; public docs are reference clarification.

Results: existing-API independent SQLite RED reproduced both lost-field orders and mixed
priority/assignee dropping assignment before production edits. Additional actual data REDs
identified pending-title omission/null deletion and postcommit presence-context leakage into
a legacy write; minimum corrections preserve those existing boundaries. Latest joined affected
checks: 18 Service names (2.054s), followed by the two directly affected names after final context
scoping (1.406s); eight handler names including actual registered protocols (3.567s); seven SQLite
storage names (2.441s); three actually executed PostgreSQL names (13.863s), including eight new
observed independent physical-wait subcases. No env-gated skip is claimed as execution.
Owned private schemas/connections drained, and both exact tmpfs containers/credential files
were removed. Older mapper fakes were adapted only for their original title/association purpose.
The first scoped lint found two concrete test-code issues, now minimally corrected. Final cheap
documentation checks and scoped original-binary lint passed. Mandatory full changed `./...` lint
passed with zero issues, exit 0, elapsed 340.28s at immutable base
`5db79135cb67cb21a0747b2e96444a555343d972`, within the task-local CLI10m/GNU11m bound. Normal
hooks, publication, current-head full review and independently verified actual merge remain
delivery steps. This order stays in progress; detailed handles, receipts and crash next action
remain in the external task plan.
