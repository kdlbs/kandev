---
id: "01-serialize-starter-admission"
title: "Serialize repository starter admission and transport outcomes"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-002
acceptance_criteria:
  - AC-WORKSPACES-BRANCH-POLICIES-002.1
  - AC-WORKSPACES-BRANCH-POLICIES-002.2
  - AC-WORKSPACES-BRANCH-POLICIES-002.3
  - AC-WORKSPACES-BRANCH-POLICIES-002.4
  - AC-WORKSPACES-BRANCH-POLICIES-002.6
system_design:
  - ../../specs/workspaces/system-design/branch-policies.md
---

# Task 01: Serialize repository starter admission and transport outcomes

## Summary and barrier

Implement one repository-scoped conditional starter operation, sharing admission
with ordinary policy inserts. Deliver typed concurrent loser conflicts and
complete winner responses/rows/four after-commit events through the existing
service and registered transports.

<!-- kandev-system: task=c9711fb9-259e-447e-ae5c-dc92e4c27a6b session=96357481-e114-4ff7-abe4-f140023ee782 parent=14825981-b175-411d-999a-31ddc2aa5fc3 -->

ROOT reviewed all four artifacts after the ended design turn and explicitly
released implementation in this same primary session. This order is in_progress.
Read [plan phase barrier](plan.md#phase-barrier-and-identity) and keep the same
profile/session/title. Normal delivery through actual merge is authorized.
No delegation/new agents/tasks/tabs/model switching/operator approval prompts.
Preserve marker, IDs, user edits and parent-question final-action barrier.

## In scope

- Inventory actual starter/ordinary insert producers, narrow transaction
  admission helper, canonical empty check, four-row commit and single ordinary
  insert semantics, precise existing conflict sentinels.
- Permanent meaningful RED, independent-store SQLite/PG behavior and waits,
  real service/Git validation, registered REST/WS responses/status/events and
  bounded failure/compatibility controls.
- Minimal existing public clarification, exact checks and authorized post-release
  delivery through independently verified actual merge and joined cleanup.

## Out of scope

No parent proof replay, production test hooks, general engine/retry framework,
schema/history/migration, new API/ETags/event revisions, branchPolicyPatch
redesign, legacy edit/delete redesign, UI/draft/CAS/mobile interaction changes,
browser/E2E/build, task snapshot/prompt/runtime/worktree changes, new docs page/
manifest, moving-main rebase, synthetic compatibility or unrelated CI expansion.

## Acceptance

1. Both actual insert writers acquire repository admission before canonical
   predicate/read/write; competing starters produce one winning complete tuple
   and typed existence loser. Both legal ordinary-create orders, repository
   isolation and existing uniqueness/default/error behavior are proven on real
   independent SQLite pools and PG physical connections with observed waits.
2. Actual service/Git and registered REST/WS paths return winner rows/tuples,
   exactly four created events after commit, meaningful loser conflict and no
   loser success events. Bounded validation, scope/read-only, missing/deleted
   parent, cancellation and rollback controls preserve current behavior;
   unrelated errors are not mislabeled already seeded.
3. Exact task checks, public/doc traceability, owned resource cleanup and later
   delivery gates have terminal receipts. Preserve the broader draft pair and
   historical packages; no completion claim before actual verified merge.

## Implementation sequence

1. After explicit release, re-read inputs and reconfirm producer inventory.
   Add new focused permanent tests first. For PG RED, hold the new repository
   advisory key in an independent transaction while empty, start the actual
   worker starter and observe its progress. It must wait at admission; old code
   instead reaches insert/returns success. In the same fixture, holder ordinary
   insertion committed before release must make the worker return the existence
   sentinel and preserve only the holder policy. Assert behavioral rows/error
   as well as the observed wait. Do not replay the archived trigger proof.
2. Implement small transaction-bound admission in both store insert paths.
   PostgreSQL advisory namespace is `repository-branch-policy-admission:` plus
   repository ID, before COUNT or insert. SQLite scoped no-op UPDATE obtains
   writer before count even for zero matching rows. Ordinary create has no
   empty predicate. Starter returns existing existence sentinel only from
   positive canonical count and commits all four or rolls back all. No reader
   pool, service mutex or general callback/cancel/retry framework.
3. Reuse exact named-index uniqueness classifier with a typed unique-constraint
   guard for ordinary name conflicts;
   primary/FK/other SQL/cancel/commit failures keep truthful failure semantics.
   Ensure generated IDs/timestamps and winning tuples match committed rows
   at current serialization precision. Preserve per-policy patch namespace,
   row locking/validation and full update/delete behavior. No nested lock
   adoption for those paths without causal evidence and parent direction.
4. Finish controlled starter/starter, starter/ordinary both orders and PG
   different-repository tests. SQLite tests use truly separate writer pools on
   one private file. A fixture-held uncommitted ordinary insert plus test-only
   delegating SQL observation can prove writer-before-predicate without sleep
   scheduling; do not replace business logic. PG asserts distinct holder/worker/
   observer PIDs and actual `pg_stat_activity` lock wait before count. Include
   waiting cancellation, failed later insert rollback and subsequent lock reuse.
5. Service validation uses isolated actual Git fixtures (`initRealGitRepo`,
   isolated Git env) or the existing faithful branch transport seam. Registered
   handler tests use `RegisterRepositoryBranchPolicyRoutes`, Gin and real
   dispatcher, separate services/stores and distinct production/development
   pairs. Reuse `initBranchPolicyGitRepository` and add alternate real refs.
   Transport-only barriers coordinate the real store, never fake conflict
   errors. Register events before requests, use a race-safe collector and
   record current persisted rows within success event callbacks to prove commit
   preceded publication. Assert created response total/IDs/tuples/timestamps,
   loser REST409/WS conflict envelope and no extra/loser events. No Cartesian
   repetition; distribute scope/error controls through the affected layers.
6. Clarify the existing public paragraph, then run only exact GREEN/checks
   below. Retain actual evidence in order/plan; original plan statuses unchanged.
   This outcome needs no rendered/mobile/browser tests.

## Verification

Each heavyweight command is a SEPARATE tool call. One heavy command at a time;
retain every session_id and ACTUALLY JOIN its terminal exit before the next.
Record command, scope, handle, terminal exit, cleanup and next action in the
platform plan. No duplicate run to infer status. New test names below are
planned and must exist; no-tests-to-run/skip is not behavioral passing evidence.
Only rerun for new changes/failures/previously skipped coverage. Commands are
independently rooted subshells from repo root, no chained dependent cwd.

### RED after release, before production edits

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^TestPostgresGitflowAdmissionBeforeRead$' -count=1 -v)
```

Inherit `KANDEV_TEST_POSTGRES_DSN` only for a proven-owned fixture. Capture the
expected pre-admission/stale decision failure and actual joined exit. An absent
DSN is a skip, not RED: if one practical owned fixture cannot execute this test,
use the permanent deterministic SQLite admission case before production edits:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^TestGitflowAdmissionSQLite$/^ordinary_first$' -count=1 -v)
```

The SQLite alternative must show stale predicate/lock failure instead of typed
existence with an uncommitted ordinary holder, not merely a sequential already-
exists check. If neither yields behavioral RED, report bounded evidence to ROOT
before proceeding; do not claim a skipped/miswired test as RED.

### GREEN independent SQLite/PG store boundary

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestGitflowAdmissionSQLite|TestPostgresGitflowAdmissionBeforeRead|TestPostgresGitflowAdmissionConcurrency|TestPostgresGitflowAdmissionBehavior|TestRepositoryBranchPolicyCRUDAndCaseInsensitiveUniqueness|TestRepositoryDeletePrunesBranchPolicies|TestBranchPolicyPatchSQLite|TestPostgresBranchPolicyPatchBehavior|TestPostgresBranchPolicyPatchConcurrency)$' -count=1 -v)
```

Use one combined real PG run when practical. Missing DSN must visibly skip PG;
later newly available actual PG evidence uses only the skipped selector:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestPostgresGitflowAdmissionBeforeRead|TestPostgresGitflowAdmissionConcurrency|TestPostgresGitflowAdmissionBehavior|TestPostgresBranchPolicyPatchBehavior|TestPostgresBranchPolicyPatchConcurrency)$' -count=1 -v)
```

The existing hosted PG job executes this package. Retain actual new test-name
execution/connection/wait receipts before merge; database boot or skip is not
coverage. No CI rewrite merely for a boot claim.

### GREEN service and real Git boundary

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^(TestGitflowAdmissionService|TestGitflowAdmissionControls|TestRepositoryBranchPolicyServiceGitflowStarterIsAtomicAndOneTime|TestRepositoryBranchPolicyServiceNormalizesAndRejectsInvalidUpdates|TestNormalizeRepositoryBranchPolicyDefaultsPullRequestTarget|TestRepositoryBranchPolicyServiceRejectsImproveWorkspaceMutations)$' -count=1)
```

### GREEN registered transports and events

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/handlers -run '^(TestGitflowAdmissionHTTP|TestGitflowAdmissionWS|TestRepositoryBranchPolicyHTTPGitflowMapsConflicts|TestRepositoryBranchPolicyWSHandlersCoverCRUDAndGitflow|TestRepositoryBranchPolicyHandlersRejectImproveWorkspaceMutations)$' -count=1)
```

### Required persistence checks, separate joined calls

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go run -trimpath -tags fts5 -p=1 ./cmd/sqlguard ./internal)
```

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/persistence/storeconformance -count=1)
```

Use the same available owned DSN for conformance. No schema/manifest edit is
expected. These persistence checks remain separate from focused store tests.

### Initial scoped lint, one bounded command

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./internal/task/repository/... ./internal/task/service/... ./internal/task/handlers/... --new-from-rev=9b6328210ff5c6098727d344ac7e7754495f516b --concurrency=2 --allow-serial-runners --timeout=5m)
```

Ensure new files are included in revision scope before recording zero issues.
For actual backend PR fixup, obtain and record exact LIVE PR base SHA once and
export task-specific `GITFLOW_ADMISSION_PR_BASE_SHA`. Full CHANGED lint is
mandatory before fixup push, once under the same bounds:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev="${GITFLOW_ADMISSION_PR_BASE_SHA:?record exact live PR base first}" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Nonzero with zero issues, timeout, crash or lost handle is NOT PASS. Retain
receipt and obtain bounded ROOT recovery direction before retry/expansion;
no automatic retry, cache wipes, foreign process kills or race/assertion weakening.

### Lightweight documentation gates

```bash
python3 scripts/list-docs.py validate
python3 scripts/list-docs.py specs --system workspaces --kind requirement --format paths
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/workspaces docs/plans/gitflow-starter-admission docs/public/git-operations.md
git status --short -- docs/plans/gitflow-starter-admission
```

After the actual public paragraph edit only, using explicit existing Node PATH
and bash login=false:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Use `.github/scripts/pr-docs.cjs` exported `validateCoverage` with ACTUAL tracked
and untracked changed paths/contents; no fictional production paths to make a
documentation-only diff trigger. Its design-only result is exempt, so separately
use exported `parseFrontmatter` to check REQ/AC presence, system design declaration,
resolved plan/design paths, plan requirements/design inclusion and exactly one
pending work order. After implementation the actual full diff must be covered.
Confirm planned selectors match real new test functions before marking done.

## Resource and dependency execution rules

No fixture/install now. After release, at most ONE practical proven-owned PG
instance (2 CPU/512MiB), private bind data preferred. Capture exact container
ID, task labels, all mounts and volume IDs before testing; register cleanup
immediately. Use existing isolated DB helpers. Strict deadlines and cancellation
plus join for every goroutine/connection precede proven-owned cleanup. Never
remove foreign DB/daemon data, root removed fixture or old unproved child20
volume. No infrastructure marathon; truthful skip plus hosted actual behavior
receipts is acceptable, but skip cannot supply required initial RED.

If fresh apps/node_modules is absent after release, perform one pinned pnpm
9.15.9 `install --frozen-lockfile` from apps before hooks with Node 24.21 explicit
PATH and bash login=false. Do not repurpose HOME/CODEX_HOME, wipe caches or
mutate lockfiles. No installation for this design turn.

## Files likely touched

Production ownership:

- `apps/backend/internal/task/repository/sqlite/repository_branch_policy.go`
- Proposed narrow `apps/backend/internal/task/repository/sqlite/repository_branch_policy_admission.go`
- `apps/backend/internal/task/service/service_repository_branch_policies.go` only if existing typed mapping/committed result needs a causal adjustment.

Proposed focused permanent tests:

- `apps/backend/internal/task/repository/sqlite/repository_branch_policy_admission_test.go`
- `apps/backend/internal/task/repository/sqlite/repository_branch_policy_admission_postgres_test.go`
- `apps/backend/internal/task/service/service_repository_branch_policy_admission_test.go`
- `apps/backend/internal/task/handlers/repository_branch_policy_admission_test.go`

Existing fixtures/helpers only for narrowly necessary wiring:

- `apps/backend/internal/task/handlers/repository_branch_policy_handlers_test.go`
- `apps/backend/internal/task/repository/sqlite/repository_branch_policy_patch_postgres_test.go` (reuse physical PID/wait helpers; preserve patch assertions).

Documentation:

- Existing owning pair, this plan/order results, `docs/public/git-operations.md` paragraph after release.

Read-only controls/consumers: `repository/interface.go`, `repository/provider.go`,
`repository_branch_policy_patch.go`,
handler registration/status mapping, service normalizer/Git branch discovery,
legacy updates/deletes, task/runtime/web consumers, original and patch plans.
Do not edit unrelated APIs or guidance unless actual causal change warrants it.

## Dependencies and inputs

None. Base/evidence/resource receipts in [plan](plan.md#accepted-evidence);
[owning requirement](../../specs/workspaces/requirements/branch-policies.md#req-workspaces-branch-policies-002-guided-gitflow-starter);
[atomic admission design](../../specs/workspaces/system-design/branch-policies.md#atomic-starter-admission);
proposed snapshot ADR, historical `docs/plans/branch-policies` and just-merged
`docs/plans/branch-policy-patch`; `apps/backend/AGENTS.md`.
Patterns: `message_agent_plan.go` transaction admission, current patch lock/
name-conflict classifier, `newPGPolicyPatchFixture`, `openPostgresRepo`,
`openSecondPostgresConnection`, `waitForPostgresLock`, independent SQLite pools,
real Git service fixtures and registered handler fixture.

## Risks and parallelism

`sequential`. Predicate before lock is the original failure. Count must see the
committed holder after admission, not a stale transaction snapshot. SQLite
no-op write must acquire writer for an empty predicate. Advisory locks apply
only to participating insert paths; no pool/service serialization falseproof.
Do not confuse an ordinary fifth policy with corruption, or name-conflict with
already-seeded. No global event ordering/durable delivery guarantee. Keep test
observers race-safe, context-bound and joined even on assertion failure.

## Delivery and recovery

Follow [plan delivery gates](plan.md#delivery-and-completion-gates-after-later-release).
After release normal hooks/push/ready PR and actual merge are authorized. Freeze
published head except valid findings; full changed-code bounded fixup lint.
Retain one all-terminal pr-await monitor, join before replacement, authenticated
full CodeRabbit current-head coverage, all actual thread/grouped-finding
dispositions, bounded ROOT direction for unrelated CI and no bypass/retry churn.
Only expected-head normal squash plus independently fetched merged SHA/time/
tree/all owned blobs/remote-main inclusion and all joined/owned cleanup complete
delivery. Leave clean managed worktree/deps/caches and parent archive intact.

Interrupt recovery reads platform plan/user edits and joins any retained command
before replacement. Preserve terminal receipts and next action. Messages only
queued concrete design handoff, actionable blocker/recovery or actual merge plus
cleanup. Parent-question is final action. No operator reauthorization prompt.

## Results

ROOT explicitly released the reviewed package in the same primary session.
Both actual create writers now share transaction-bound repository admission;
ordinary inserts retain their normal semantics. Starter existence comes only
from the canonical positive count. Stored rows are captured inside the owning
transaction, and insert name conflicts require the actual named unique
constraint and typed SQLite/PostgreSQL error. Existing patch/legacy writes and
service/handler mappings remain unchanged.

Meaningful permanent PostgreSQL RED (86327, exit 1) proved premature success and
five rows before the held admission lock, instead of typed existence and one
holder row. SQLite constraint-message RED (21121, exit 1) proved overbroad name
classification; the narrow insert guard fixed it. Neither replayed the parent
archive. Final new independent-store checks passed (97082, exit 0), as did final
new real-Git service checks (99029, exit 0). Existing patch/CRUD, service and
registered transport compatibility controls passed in their scoped runs.
SQLguard and SQLite/PG storeconformance passed as separate joined checks.
See [plan results](plan.md#verification-results) and the durable platform ledger
for exact receipts, fixture corrections and command ownership.

The existing public starter paragraph was clarified; public validation and
actual-path PR-docs coverage passed. Final registered transport tests passed (39896, exit 0, 1.432s). Publication,
review and merge receipts remain pending; this order remains in_progress through delivery.

Owned PostgreSQL fixture cleanup completed after all test handles joined: exact
container `d5d87c9f105bc60dbd49c39e46b05f544d4ebe631f2876bc74d31a9323e559ae`
and its single private bind data directory were removed, zero anonymous volumes.
Receipt: `/tmp/kandev-child22-pg-owner.json`. One frozen dependency install
with Node 24.21.0 and pnpm 9.15.9 joined handle 57016, exit 0; lockfile unchanged.

Initial scoped lint joined handle 19446, exit 0, zero issues, with concurrency 2,
allow-serial-runners, CLI 5m and GNU hard 6m/kill-after 10s under GOMAXPROCS 2
and GOMEMLIMIT 1GiB. It included all staged new files and compared the actual
accepted base `9b6328210ff5c6098727d344ac7e7754495f516b`. Final catalog/spec
lint, whitespace and actual eleven-path documentation coverage also passed.
Normal publication and hosted review/check gates remain external pending work.
