---
id: "01-preserve-enabled-intent"
title: "Preserve enabled intent through profile saves"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-002
acceptance_criteria:
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.6
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.10
system_design:
  - ../../specs/platform/system-design/agent-settings-parity.md
  - ../../specs/platform/system-design/profile-enabled-omission.md
---

# Task 01: Preserve enabled intent through profile saves

## Summary

Carry enabled presence from the real profile controller to ordinary and atomic
dynamic writes, and return the flag captured by the committing statement. Keep
explicit mixed/only updates, legacy replacement calls, dependencies, working
ownership, model adoption and dynamic rollback compatible.

## In scope

- Implement the required intent-aware repository method and optional atomic
  dynamic companion from the design. Preserve old full method signatures.
- Change only enabled SQL intent and result capture in the shared row writer.
  Keep insert `profileExecer` separate from the new query-capable update seam.
- Independently author permanent real DB/controller and actual transport tests;
  ROOT's protected temporary proof must never be copied or replayed.
- Add environment-gated physical PostgreSQL behavior/wait tests and explicit
  native Windows execution of the new applicable cases in backend CI.
- Adapt existing required-interface fakes and signal wrappers without changing
  their unrelated modeled behavior. Update scoped backend guidance only if its
  existing descriptions become inaccurate.

## Out of scope

The [manifest exclusions](plan.md#scope) apply. No frontend/browser/E2E work,
schema/public API, revision/advisory framework, Office or general writer audit.
No delegates, extra sessions/tabs, model changes, proof cleanup or merge grant.

## Acceptance

1. Both causal enabled directions pass through ordinary and dynamic production
   callers, preserving unrelated name/model edits; explicit controls and own-write
   response/event projection pass over the actual repository.
2. Missing/deleted/error paths and dynamic version/parent/route failures cannot
   produce a false success or partial committed base update. Current policy,
   dependency/force, working owner, model probe and legacy full semantics pass.
3. All listed local gates pass with owned PostgreSQL resources and joined
   handles. Hosted native Windows and physical PostgreSQL new cases actually
   RUN/PASS at the frozen PR head; publication/review/merge follow ROOT barriers.

## Permanent test matrix

The six permanent test files below now exist. Local executions are recorded in
Results; hosted Windows and PostgreSQL gates remain pending. Fixtures use table
subcases and observable barriers.

| Proposed file/test | Subcases and observable assertions |
| --- | --- |
| `controller/profile_enabled_intent_test.go:TestProfileEnabledIntentCaller` | Two real controllers with a real SQLite store. Gate the first actual snapshot, then run a second production enabled-only toggle. Both true-to-false and false-to-true, each rename and model save, assert stored name/model and enabled plus returned enabled. Include uncontested disabled/enabled rename, explicit true/false mixed saves, explicit-only narrow save, unchanged/no-op values and independent profiles. |
| Same file: `TestProfileEnabledIntentOwnCommit` | Gate after actual intent-aware ordinary or dynamic write; commit a later toggle before first DTO delivery. Assert first returned flag is its own write's captured value and final DB has the later toggle. Never substitute a fake precomputed profile. |
| Same file: `TestProfileEnabledIntentDynamicCaller` | Dynamic metadata alone and metadata plus routes with both enabled directions. Explicit enabled mixed route saves retain version progression. Stale version and route failure preserve the pre-attempt base flag/name/model/routes/version, including a toggle committed before the rejected save. |
| `store/sqlite_profile_enabled_intent_test.go:TestProfileEnabledIntentStorage` | Nil, explicit true/false, stale snapshot, missing and soft-deleted rows, canceled/query/scan failures where reachable; same-value writes, create defaults, user-modified/timestamp and independent rows. Legacy `UpdateAgentProfile` and legacy dynamic full method still replace enabled explicitly. A failed dynamic parent/route operation rolls back base fields. Use actual SQL failure/transaction behavior rather than helper predicates. |
| `store/postgres_profile_enabled_intent_test.go:TestProfileEnabledIntentPostgres` | Real isolated schema and distinct physical connections. Ordinary and dynamic intent writes wait on held enabled update; monitor actual lock wait, then commit or roll back holder; both directions preserve correct row result. Dynamic stale-version/parent rollback, explicit mixed and legacy full controls. Emit physical PID/wait/pass evidence; no single-connection masquerade. |
| `handlers/profile_enabled_intent_test.go:TestProfileEnabledIntentHTTP` | Actual REST handler + controller + DB for omitted and JSON null enabled, explicit true/false/mixed, dependency rejection/force and invalid inputs. Actual interleaved toggle; returned DTO and one existing broadcast contain captured enabled; no event on failed save. |
| `mcp/server/profile_enabled_intent_test.go:TestProfileEnabledIntentMCP` | Actual registered compatibility tool schema, guarded dispatcher/controller/DB fixture. Supported name/model updates omit enabled and preserve both interleaved toggle directions; response and one existing MCP event carry captured flag. The published tool supports profile_id/name/model/auto_approve, so no enabled argument or schema expansion is added. Unknown/null controls follow the actual current guard behavior. Explicit enabled controls belong at REST/compact/controller. Mock only transport/publication/provider bounds. |
| `backendapp/settings_profile_enabled_intent_test.go:TestProfileEnabledIntentCompact` | Actual compact operation authorization/decode/controller/store. Omitted enabled preserves both toggles and sanitized saved result. Boolean null schema behavior remains. Do not assert a compact event absent from current source. |

Keep existing `TestUpdateAgentProfileModelIfEmptyIsConditional`,
`TestUpdateAgentProfile_DoesNotOverwriteWorkingOwner`,
`TestAgentProfileEnabled_RoundTrip`, `TestDynamicProfileCreateAndUpdatePersistsCandidates`
and `TestSQLiteRepositoryDynamicProfileCRUDUsesOptimisticVersions` as focused
compatibility controls. Add necessary exact-model/provider/MCP/force controls to
the new table, invoking current validators through the real controller.
Do not enlarge already-long test files. Load `/tdd` and its backend reference
before permanent test edits. Use immediate `t.Cleanup`, bounded contexts,
channel barriers, registered pool cleanup and joined goroutines.

## Execution and resource admission

Nothing below runs during DESIGN. A LATER explicit ROOT implementation INTERRUPT
in primary `8c80c873-e66c-4b23-9108-5514fde85745` releases implementation; obtain
ROOT's GLOBAL heavy admission before the first install/build/test/lint/DB startup.
Only one GLOBAL heavy operation at a time, including siblings. Keep the same
agent/executor, checkout and primary. Reconcile current relevant source if the
base advanced; do not rebase solely for drift.

Record each original native/session handle, process group, command, cwd,
start/cutoff and receipt in the own platform plan. ACTUALLY JOIN its terminal
result and verify its fresh owned group is gone before the next heavy command.
Never discard handles, infer completion from another run, or rerun on timeout
or resource exhaustion. A timeout/transport/unknown mutation checkpoints ROOT;
reconcile unknown state before repeat. Follow normal active hooks, no bypass or
amend. Docs-only correction does not replay backend gates.

Node is `24.21.0` with its verified installation directory explicitly prepended
to PATH; use Bash `login:false` for native commands. If `apps/node_modules` is
absent after release, perform exactly one pinned `pnpm@9.15.9 install
--frozen-lockfile` from `apps/` under admission, record/join the handle, and do
not delete managed/shared dependencies or caches. Resolve the executable path
read-only before starting; never substitute a model/session or override HOME.

### Owned PostgreSQL resource receipt

Before creating resources, persist a receipt naming the task, exact fresh
container name, planned image, port, data mount, schema/DSN and cleanup. Use a
new task-owned container with PostgreSQL data on bounded tmpfs (no reuse of an
old/anonymous volume), memory/CPU caps and loopback ephemeral port. After
creation record the actual container ID, owned labels, mapped port, no-volume
inspection, tmpfs and readiness. Set only the owned DSN for test commands.
Keep the service bounded to this sequential gate; PostgreSQL and the test's
aggregate memory must fit the admitted resource budget. Never print credentials.

Test helpers create their own schema and independent physical connections with
that schema's search path. Record distinct PIDs and a real row lock wait. Close
all pools/monitor handles, drop only that owned schema, and remove only the
receipt-matched container after tests. Verify container disappearance and no
owned residual resources before returning admission. Never prune, delete foreign
containers/volumes, touch an anonymous unproved volume, or treat schema replay as
the behavior result. Failure to start/prove owned PostgreSQL is a ROOT checkpoint.

Use the source-pinned PostgreSQL 16 CI image and this intended fresh name only
after proving that name absent. Persist the intended receipt BEFORE pulling or
starting it. If the image is absent, one admitted bounded pull precedes startup;
an authentication/pull/resource failure checkpoints ROOT rather than selecting
an unreviewed image. Record the actual ID from this command's original output:

```bash
/usr/bin/timeout --signal=TERM --kill-after=10s 6m docker run --detach --pull=never --name kandev-profile-enabled-2d0b6ca1-20261007 --label kandev.task_id=2d0b6ca1-beb7-4af2-bcad-9eedbe9c3abf --memory=256m --cpus=0.5 --shm-size=32m --tmpfs /var/lib/postgresql/data:rw,size=192m -p 127.0.0.1::5432 -e POSTGRES_USER=kandev -e POSTGRES_PASSWORD=kandev -e POSTGRES_DB=kandev_admin --health-cmd='pg_isready -U kandev -d kandev_admin' --health-interval=2s --health-timeout=2s --health-retries=30 ghcr.io/kdlbs/kandev-ci:postgres-16@sha256:fe03a7605299a34ddf5e4f285dff78c3d7190a576b3c6b46f2fcff69f4bffd54
```

After inspecting the actual loopback port and healthy state, construct the owned
`KANDEV_TEST_POSTGRES_DSN` with that port, user/database above and
`sslmode=disable`, then store the receipt privately without emitting credentials.
Before cleanup verify both actual ID and task label against the receipt, then
run `docker rm --force` with ONLY that literal receipt ID. Do not add `--volumes`
or broad cleanup. Reconcile an unknown startup/cleanup response before retrying.

## Verification

Run only after release and admission, from the exact checkout. Each command is
sequential with its own retained/joined handle. GNU `timeout` caps Go at six
minutes with a ten-second kill grace; Go tests cap at four minutes. Explicit
`-trimpath -tags fts5 -race -p 1`, GOMAXPROCS 2 and GOMEMLIMIT 512MiB apply.
Commands below use raw native output; if RTK is injected, use `rtk proxy` and
validate byte-preserving output before parsing. No broad local suite by default.

First RED: create meaningful ordinary production-caller interleaving tests, run
this once and record the causal assertions failing, then implement GREEN:

```bash
GOMAXPROCS=2 GOMEMLIMIT=512MiB /usr/bin/timeout --signal=TERM --kill-after=10s 6m go -C /home/jcfs/.kandev/tasks/preserve-profile-ava_hxg9xkrt/kandev/apps/backend test -trimpath -tags fts5 -race -p 1 -count=1 -timeout=4m ./internal/agent/settings/controller -run '^TestProfileEnabledIntentCaller$'
```

GREEN focused new matrix and the five existing compatibility controls:

```bash
GOMAXPROCS=2 GOMEMLIMIT=512MiB /usr/bin/timeout --signal=TERM --kill-after=10s 6m go -C /home/jcfs/.kandev/tasks/preserve-profile-ava_hxg9xkrt/kandev/apps/backend test -trimpath -tags fts5 -race -p 1 -count=1 -timeout=4m ./internal/agent/settings/store ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/mcp/server ./internal/backendapp -run '^(TestProfileEnabledIntent.*|TestUpdateAgentProfileModelIfEmptyIsConditional|TestUpdateAgentProfile_DoesNotOverwriteWorkingOwner|TestAgentProfileEnabled_RoundTrip|TestDynamicProfileCreateAndUpdatePersistsCandidates|TestSQLiteRepositoryDynamicProfileCRUDUsesOptimisticVersions)$'
```

With the receipt-proved owned `KANDEV_TEST_POSTGRES_DSN` exported, require this
actual physical behavior run and reject SKIP as completion evidence:

```bash
GOMAXPROCS=2 GOMEMLIMIT=512MiB /usr/bin/timeout --signal=TERM --kill-after=10s 6m go -C /home/jcfs/.kandev/tasks/preserve-profile-ava_hxg9xkrt/kandev/apps/backend test -trimpath -tags fts5 -race -p 1 -count=1 -timeout=4m -v ./internal/agent/settings/store -run '^TestProfileEnabledIntentPostgres$'
GOMAXPROCS=2 GOMEMLIMIT=512MiB /usr/bin/timeout --signal=TERM --kill-after=10s 6m go -C /home/jcfs/.kandev/tasks/preserve-profile-ava_hxg9xkrt/kandev/apps/backend test -trimpath -tags fts5 -race -p 1 -count=1 -timeout=4m ./internal/persistence/storeconformance
GOMAXPROCS=2 GOMEMLIMIT=512MiB /usr/bin/timeout --signal=TERM --kill-after=10s 6m go -C /home/jcfs/.kandev/tasks/preserve-profile-ava_hxg9xkrt/kandev/apps/backend run -trimpath -p 1 ./cmd/sqlguard ./internal
```

Scoped lint before publication, independently rooted with a verified executable:

```bash
(cd /home/jcfs/.kandev/tasks/preserve-profile-ava_hxg9xkrt/kandev/apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB /usr/bin/timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./internal/agent/settings/controller ./internal/agent/settings/store ./internal/agent/settings/handlers ./internal/mcp/server ./internal/backendapp ./internal/agent/runtime/lifecycle --new-from-rev=e4f11385ec772d421ab55b90e76c750a92233c02 --concurrency=2 --allow-serial-runners --timeout=5m)
```

Read-only doc/diff gates from repo root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Native Windows: add a `matrix.suite == 'native'` step in
`.github/workflows/backend-tests.yml`, with GOMAXPROCS 2, GOMEMLIMIT 512MiB,
job-step cap six minutes and this exact command from `apps/backend`:

```bash
go test -trimpath -tags fts5 -race -p 1 -count=1 -timeout=4m -v ./internal/agent/settings/store ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/mcp/server ./internal/backendapp -run '^(TestProfileEnabledIntent(Caller|OwnCommit|Controls|DynamicCaller|DynamicRejection|Storage|HTTP|MCP|Compact)|TestUpdateAgentProfileModelIfEmptyIsConditional|TestUpdateAgentProfile_DoesNotOverwriteWorkingOwner|TestAgentProfileEnabled_RoundTrip|TestDynamicProfileCreateAndUpdatePersistsCandidates|TestSQLiteRepositoryDynamicProfileCRUDUsesOptimisticVersions)$'
```

The env-gated PostgreSQL test is excluded. The ordinary/dynamic/transport SQLite cases
must RUN/PASS, not skip or compile only. The existing Backend Postgres fixed
package list already includes settings/store; new Postgres tests execute there
with the service DSN. Fetch actual job logs and case results at the frozen head.

For an actual backend corrective PR finding only, resolve the exact live PR base
SHA into `PROFILE_ENABLED_PR_BASE`, validate a full 40-hex value, and execute
this required full changed-code lint ONCE before corrective push:

```bash
(cd /home/jcfs/.kandev/tasks/preserve-profile-ava_hxg9xkrt/kandev/apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB /usr/bin/timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./... --new-from-rev="$PROFILE_ENABLED_PR_BASE" --concurrency=2 --allow-serial-runners --timeout=5m)
```

No automatic retry on timeout/resource failure. A docs-only finding uses only
doc gates. Resolve queued branch status before an authorized corrective push;
dequeue/push/recheck/restore under the existing merge-queue instructions when
applicable, preserving exact head and explicit remote lease.

## Publication and one observer

Read commit/push/pr/pr-fixup skills when later release reaches those phases.
Before publication freeze the passed SHA, canonical `kdlbs/kandev` repository,
current base, full actual file set and all five operational flags false. Preserve
normal active hooks and live bot updates; no optional cleanup commits or waits.
After publication explicitly RETURN GLOBAL heavy admission to ROOT before this
single observer. Store `PROFILE_ENABLED_PR_NUMBER`, frozen SHA and absolute
start/deadline in the own plan before starting:

```bash
(cd /home/jcfs/.kandev/tasks/preserve-profile-ava_hxg9xkrt/kandev && /usr/bin/timeout --signal=TERM --kill-after=10s 91m scripts/pr-await "$PROFILE_ENABLED_PR_NUMBER" --mode all-terminal --deadline-min 90 --interval-sec 60 --format json)
```

Retain and actually join that handle; communicate while waiting through short
native polls, not duplicate observers or timer `pr-state` polls. Direct own
platform plan/primary observation preserves the fixed start and cutoffs. Follow
the manifest's six-context/parent-job and App 347564 FULL review gates; one
necessary manual request only on proven missing/skip and no live/completed FULL.
Any timeout, transport or unknown mutation checkpoints ROOT without reset/retry.
Actual findings receive grounded disposition and scoped remediation; no optional
polish. Later corrective observer actions require ROOT checkpoint, not a silent
new attempt.

MERGE NONE until a separate serial ROOT grant. Even then require independently
verified expected head, normal squash and actual merged SHA/tree/blobs/remote
inclusion. No admin bypass, synthetic run claims or drift-only rebase. ROOT
archives/releases proof; clean only exact owned resources after all handles join.

## Files likely touched

- `apps/backend/internal/agent/settings/controller/profile_crud.go`.
- `apps/backend/internal/agent/settings/store/store.go`, `sqlite.go`.
- The six new test files named in the matrix; transport production files
  change only if evidence proves required enabled forwarding is missing.
- Interface fakes in `controller/reconciler_test.go`,
  `controller/custom_tui_test.go`, `handlers/profile_duplicate_handlers_test.go`,
  `internal/agent/runtime/lifecycle/profile_resolver_test.go`, and any wrappers
  that currently intercept `UpdateAgentProfile` and must intercept the new call.
- `.github/workflows/backend-tests.yml` for native Windows behavior execution.
- This manifest/work order and the owning requirement/design/supplement for
  accurate lifecycle/results after implementation, with scoped guidance only
  if changed contracts make it inaccurate.

## Dependencies

None between work orders. Execution depends on a later explicit ROOT release
and GLOBAL heavy admission; merge depends on its separate grant.

## Risks

Returned-row scan/commit behavior differs by dialect. Physical waiting and
rollback tests are mandatory. New required interfaces can bypass old test
wrappers unless adapted; caller regressions must prove actual intercepted paths.
Use the existing dynamic transaction, never a non-atomic fallback for production.

## Parallelism

`sequential`

## Inputs

- [Owning requirement](../../specs/platform/requirements/agent-settings-parity.md),
  REQ002 and AC.2/.4/.5/.6/.10.
- [Design supplement](../../specs/platform/system-design/profile-enabled-omission.md).
- Existing enabled/model/working-owner/dynamic tests named above and shared DTO
  conversion, REST/MCP publishers, backendapp compact adapter.
- Domain-owned catalog and utility dependency safety ADRs; repository backend
  guidance; protected ROOT proof receipt, read only.

## Results

Implementation released after ROOT's five-file acceptance receipt at
`/tmp/kandev-root-child73-design-acceptance-20261007.json` (2026-10-07 17:59:41 UTC).
GLOBAL local-heavy admission belongs exclusively to this task for sequential
implementation gates and publication until explicit return. Merge grant: none.
Independent real SQLite production-caller RED failed all four ordinary and four
dynamic interleavings in both enabled directions. GREEN passes the ordinary and
atomic dynamic callers, own-commit return capture, explicit/omitted controls,
current exact-model/MCP/dependency/force behavior, rejected dynamic versions,
real storage errors and rollback, registered REST/MCP/compact paths, plus the
five named compatibility controls. Affected existing provider, model adoption
and profile resolver tests also pass.

Physical PostgreSQL `TestProfileEnabledIntentPostgres` ran and passed without
SKIP: eight waits used distinct writer/holder/monitor PIDs and observed actual
row lock waits before commit/rollback; three dynamic failure rollback controls
and eight explicit/legacy controls passed. Store conformance ran with the owned
DSN and passed; SQL guard passed. The exact owned container and all test schemas
were removed after verification, with no volumes allocated. Receipts retain all
original native handles, terminal joins and fresh process-group disappearance.

Scoped lint passed with concurrency 2 and serial-runner permission on the
actual owned packages; three bounded style corrections passed affected tests
and lint. The single pnpm 9.15.9 frozen install passed, reusing all 935 packages without
lockfile changes. Local implementation gates are complete; normal-hook
publication is in progress, with its receipt retained in the own platform plan. Hosted Windows/PostgreSQL cases, all required checks, substantive FULL
review and separate ROOT merge grant remain pending. DESIGN previously completed
without production/test edits or runtime gates.
