---
id: "01-atomic-policy-patches"
title: "Preserve atomic branch-policy patches and task snapshots"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-001
  - REQ-WORKSPACES-BRANCH-POLICIES-004
acceptance_criteria:
  - AC-WORKSPACES-BRANCH-POLICIES-001.4
  - AC-WORKSPACES-BRANCH-POLICIES-001.5
  - AC-WORKSPACES-BRANCH-POLICIES-001.7
  - AC-WORKSPACES-BRANCH-POLICIES-001.8
  - AC-WORKSPACES-BRANCH-POLICIES-001.9
  - AC-WORKSPACES-BRANCH-POLICIES-001.10
  - AC-WORKSPACES-BRANCH-POLICIES-004.1
  - AC-WORKSPACES-BRANCH-POLICIES-004.2
  - AC-WORKSPACES-BRANCH-POLICIES-004.7
system_design:
  - ../../specs/workspaces/system-design/branch-policies.md
---

# Task 01: Preserve atomic branch-policy patches and task snapshots

## Summary and phase barrier

Apply optional branch-policy edits to the current committed row atomically,
return/publish its normalized result, and prove actual new-task snapshots retain
all successful workflow changes. One sequential vertical slice owns storage,
service, registered transports, persistent task coverage and minimal public docs.

<!-- kandev-system: task=20478f62-1f6c-4af8-9057-cbedc56991a4 session=647b0064-81a0-4c55-9ff6-27a8de401152 parent=14825981-b175-411d-999a-31ddc2aa5fc3 -->

Root reviewed the four artifacts after the ended design turn and explicitly
released implementation in this SAME primary session. Local implementation and
checks are complete; delivery remains authorized through actual merge.
No delegation, other agents/tasks/tabs, model switch or operator approval prompt.
Preserve task title, marker, IDs, user edits, parent question barriers and
completion gates from [the plan](plan.md). A parent question is a final action.

## In scope

- Proposed typed pointer patch and policy-specific repository transaction. Retain
  initial service authorization, repository scope and read-only checks; current
  row read, merge, validation/defaults, uniqueness, write and returned row occur
  atomically. Existing service normalizer supplies the pure policy validator.
- PostgreSQL namespace+ID advisory transaction lock before canonical read/update,
  plus row lock for legacy writes/deletes; SQLite scoped writer no-op update
  before read. No pool-only or service-mutex-only synchronization.
- Whole-effective-policy validation and existing errors; conflict mapping for
  uniqueness precheck and DB constraint. Affected rows reject missing policy or
  deleted repository. Failed mutations roll back timestamp and all fields and
  emit no success event. Retain intentional full-set store update compatibility.
- Deterministic real-service/DB lost-update RED and bounded disjoint/same-field,
  defaults/validation/deletion/authorization controls; actual registered REST
  PATCH and WS dispatch response/event/presence; independent real DB tests.
- Actual CreateTask/persistent TaskRepository evidence for complete post-edit
  tuple and unchanged pre-existing snapshot after edits and deletion, including
  actual task event projection.
- Minimal `docs/public/git-operations.md` partial-update/default clarification,
  dependency-free document validation and normal authorized later delivery.

## Out of scope

Create/Delete/Gitflow redesign, initializer concurrency, repository-set work,
general concurrency audit, schema/history/manifests/ETags/global revisions,
new endpoints/ADR/flag, UI full-draft conflicts/dirty tracking/CAS,
frontend/runtime/prompt/worktree/PR changes without a proven causal defect,
layout/copy/browser/build/E2E, broad audits/suites, accepted proof replay,
passed-check repetition, moving-main rebase or synthetic compatibility tests.
Mobile skill does not require rendered coverage: this is backend state/data.

## Acceptance

1. A permanent real-service/DB regression fails on the original stale-write
   behavior before production edits; after correction independent services/stores
   preserve disjoint metadata and workflow edits in both orders and return the
   committed normalized row. Omitted target survives base edits; explicit blank
   resets to the current effective base. Validation/conflict/failure rollback,
   same-field last commit and legacy full writes retain existing semantics.
2. Real SQLite and env-gated PostgreSQL tests prove locking/current reads and
   atomicity across independent connections. Registered REST/WS each prove one
   missing-field request with truthful full response/event and no failed-write
   publication. Actual task creation persists the complete current tuple while
   pre-existing task rows and events retain immutable snapshots, even after
   policy deletion. An absent PG DSN is reported as a skip; hosted new behavioral
   tests must actually execute before merge.
3. Exact task checks and later normal PR gates pass with retained terminal
   receipts; internal contracts and minimal public documentation match final
   behavior. Completion requires actual merge verification and joined owned
   cleanup, not a ready PR or local green.

## Implementation order

1. Load `/tdd` and backend test guidance after the later explicit release. Mark
   this work order in_progress and update platform/durable plan without losing
   user edits. Use accepted proof only as source evidence; write
   `TestBranchPolicyPatchConcurrentEdits` on real SQLite service/store. Gate the
   storage boundary after the service's authorized initial read and before the
   actual write, using interface embedding that forwards real behavior. Seed
   different tuples; metadata-after-workflow must fail for the confirmed reason.
   Use context-bound channels and cleanup that releases/cancels and joins on all
   paths. No business mocks or new production test hooks.
2. Add proposed `models.RepositoryBranchPolicyPatch`, five string pointers.
   `PatchRepositoryBranchPolicy(ctx, id, repositoryID, patch, normalize)` returns
   `*models.RepositoryBranchPolicy, error`; the policy-specific normalizer has
   the existing `func(*models.RepositoryBranchPolicy) (*models.RepositoryBranchPolicy, error)`
   shape. Keep it pure and limited to this domain; its update-mode target
   defaulting honors presence (omitted invalid legacy blank rejects unchanged,
   supplied blank resets, create defaults unchanged). Store acquires locks before
   canonical read, checks scope, overlays present fields, invokes service
   normalizer, checks name uniqueness with transaction query and DB constraint,
   writes locked candidate, and returns captured row only after commit. Keep
   full-set Update method for intentional callers; move only partial service to
   Patch. Use transaction queries rather than reader pool after admission.
3. Finish bounded service/store controls and real PG connection/lock tests. Reuse
   isolated helpers, record distinct PG backend PIDs, observe a wait on the
   dedicated advisory key before current-row resolution, and demonstrate a
   base edit committed before blank reset is reflected. Cover row-lock interaction
   with a retained full writer and delete so advisory-only coordination cannot
   pass. SQLite uses independent writer handles on the same private fixture.
4. Exercise `RegisterRepositoryBranchPolicyRoutes` through Gin and dispatcher
   with real database/service, retaining events. Each path has one omitted-field
   integration, explicit blank or null boundary assertions as needed, returned
   full DB tuple and failure/no-event assertion. Do not repeat the concurrency
   matrix in both transports. Adjust test fixture wiring locally if needed;
   handler behavior remains the shared service contract.
5. Reuse `seedWorkspaceAndWorkflowForCreate`, real CreateTask and persisted rows.
   Create the old task before edits, finish both successful patches, create the
   new task, assert all branch_policy_* fields and effective base in persisted
   rows and actual task events. Delete the policy and reload both existing tasks;
   old snapshot remains original, new snapshot remains corrected. Resolver-only
   assertions and synthetic event objects do not satisfy this outcome.
6. Update the existing git-operations section, then exact scoped checks below.
   Keep original broad pair draft and historical plan completion unchanged;
   record only this work order's results. Later delivery follows plan gates.

## Verification

All commands run from repo root via independent subshells, except explicitly
labelled otherwise. Each heavy command is a SEPARATE tool call. Retain session_id,
join to terminal, record command/scope/handle/exit/next action before another
heavy command. No parallel heavy commands and no duplicate run to infer status.
Planned new test names must exist and match exact patterns; an empty match is
not coverage. After meaningful failures/corrections rerun only affected tests.

### RED (before production changes, after later release only)

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^TestBranchPolicyPatchConcurrentEdits$' -count=1)
```

Record expected lost-workflow assertion. This is permanent regression coverage,
not a replay of `TestRootBranchPolicyPatchProof`.

### GREEN service and persistent task boundary

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^(TestBranchPolicyPatchConcurrentEdits|TestBranchPolicyPatchTargetDefaults|TestBranchPolicyPatchValidationRollback|TestBranchPolicyPatchControls|TestBranchPolicyPatchTaskSnapshots|TestRepositoryBranchPolicyServiceNormalizesAndRejectsInvalidUpdates|TestNormalizeRepositoryBranchPolicyDefaultsPullRequestTarget|TestRepositoryBranchPolicyServiceRejectsImproveWorkspaceMutations|TestReplaceTaskRepositoriesPreservesFreshBranchWithPolicySnapshot|TestTaskEventsSerializeBranchPolicySnapshot)$' -count=1)
```

### Registered transport boundary

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/handlers -run '^(TestBranchPolicyPatchHTTP|TestBranchPolicyPatchWS|TestRepositoryBranchPolicyHTTPHandlersCoverCRUDAndStatusMappings|TestRepositoryBranchPolicyWSHandlersCoverCRUDAndGitflow|TestRepositoryBranchPolicyHandlersRejectImproveWorkspaceMutations)$' -count=1)
```

### SQLite and PostgreSQL behavior

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestBranchPolicyPatchSQLite|TestPostgresBranchPolicyPatchBehavior|TestPostgresBranchPolicyPatchConcurrency|TestRepositoryBranchPolicyCRUDAndCaseInsensitiveUniqueness|TestRepositoryDeletePrunesBranchPolicies)$' -count=1 -v)
```

`KANDEV_TEST_POSTGRES_DSN` is inherited only when available. No DSN means the two
PG tests must skip explicitly; never record boot or skip as behavior passing.
Prefer one successful combined run with a proven-owned PG fixture if practical.
If first run skipped, a later exact PG-only invocation with a newly available
DSN is new evidence, not repetition of passed SQLite tests:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestPostgresBranchPolicyPatchBehavior|TestPostgresBranchPolicyPatchConcurrency)$' -count=1 -v)
```

Hosted PostgreSQL jobs already execute this package; obtain leaf-log/artifact
proof of these new test names running with real connections before merge.
Do not change CI merely to obtain a boot claim.

### Required persistence guard and conformance (separate joined calls)

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go run -trimpath -tags fts5 -p=1 ./cmd/sqlguard ./internal)
```

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/persistence/storeconformance -count=1)
```

Use available PG DSN for conformance too. No schema/history/manifest edit is
expected or justified by this patch.

### Initial affected-package lint (one bounded command)

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./internal/task/models/... ./internal/task/repository/... ./internal/task/service/... ./internal/task/handlers/... --new-from-rev=b0dc2bef512eda8def545ba5cf2edc67bbedf73d --concurrency=2 --allow-serial-runners --timeout=5m)
```

If a valid PR finding requires backend correction, mandatory full changed-code
lint replaces scope before push. Obtain the one exact live PR base SHA, record
it, and export task-specific `BRANCH_POLICY_PR_BASE_SHA` to that SHA:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev="${BRANCH_POLICY_PR_BASE_SHA:?record exact PR base SHA first}" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Nonzero/zero issues, timeout, crash and lost handles are NOT passing receipts.
Retain evidence and seek bounded parent recovery; no automatic retry, cache
wipe, foreign process kill or weakening. Read current lint config/tool before
execution without installing speculative tooling.

### Lightweight artifact and public checks

```bash
python3 scripts/list-docs.py validate
python3 scripts/list-docs.py specs --system workspaces --text branch --format paths
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/workspaces docs/plans/branch-policy-patch docs/public/git-operations.md
git status --short -- docs/plans/branch-policy-patch
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Use the existing `.github/scripts/pr-docs.cjs` `validateCoverage` API with
ACTUAL changed paths (tracked diff and untracked docs) and contents of the four
artifacts. Validate REQ/AC/design/plan references, owned path existence and
exact test-name coverage before completion; catalog/spec lint alone is not
traceability proof. No new validator scripts needed. A doc-only classification
is exempt in this API; report that exemption and separately use its exported
frontmatter parser to check the same four files' REQ/AC/design/plan chain and
file existence. Do not invent triggering paths to claim coverage.

### Dependency and resource ownership

After release only, if apps/node_modules is missing, use Node 24.21 PATH,
bash login=false and one `pnpm@9.15.9 install --frozen-lockfile` from apps before
normal hooks. No lockfile or cache changes. No frontend build/typecheck/E2E.
At most one practical owned PG instance: private PGdata directory OR capture
exact container ID, label, creation and all mount/anonymous volume IDs upfront.
Register cleanup immediately; join DB handles and close connections before
removal. Remove only proven-owned objects; never old unproven `2c48...` volume,
foreign DB/daemon data or prune. Truthful local skip with hosted actual PG
behavioral evidence is acceptable; infrastructure marathon is not required.

## Files likely touched

Existing production:

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/repository_branch_policy.go`
- `apps/backend/internal/task/service/service_repository_branch_policies.go`

Proposed focused files:

- `apps/backend/internal/task/models/repository_branch_policy_patch.go`
- `apps/backend/internal/task/repository/sqlite/repository_branch_policy_patch.go`
- `apps/backend/internal/task/repository/sqlite/repository_branch_policy_patch_test.go`
- `apps/backend/internal/task/repository/sqlite/repository_branch_policy_patch_postgres_test.go`
- `apps/backend/internal/task/service/service_repository_branch_policy_patch_test.go`
- `apps/backend/internal/task/service/service_branch_policy_patch_snapshot_test.go`
- `apps/backend/internal/task/handlers/repository_branch_policy_patch_test.go`

Existing fixture/helper files only if necessary:

- `apps/backend/internal/task/handlers/repository_branch_policy_handlers_test.go`
- `apps/backend/internal/task/repository/repoerrors/errors.go` (verify exact file
  before any proposed new conflict sentinel; reuse existing errors when possible)

Documentation:

- `docs/public/git-operations.md`
- Owning Workspaces pair and this plan/work order, only final conformance/results.

Read-only consumers: `service_task_branch_policy_snapshot.go`,
`service_events_branch_policy_test.go`, provider wiring, web workspace API/hooks,
store/boot events, runtime/prompt/worktree/title/PR consumers and original
`docs/plans/branch-policies/*`. Do not edit without an actual causal defect.

## Dependencies and inputs

None. Accepted proof/receipt and current owning pair are sufficient design inputs.
Read [atomic partial policy updates](../../specs/workspaces/system-design/branch-policies.md#atomic-partial-policy-updates),
[requirements](../../specs/workspaces/requirements/branch-policies.md),
[historical plan](../branch-policies/plan.md), the proposed snapshot ADR and
`apps/backend/AGENTS.md`. Patterns: `repository/sqlite/message_agent_plan.go`,
`repository/sqlite/repository_set.go`, PG isolated/lock observation helpers,
`createTestService`, `seedWorkspaceAndWorkflowForCreate`, real CreateTask fixture,
registered handler fixture and existing service normalizer. No new architecture
boundary or ADR is requested.

## Risks and parallelism

`sequential`. Cross-field defaults require the current locked base; validating a
stale row or post-commit rereading cannot prove the contract. Per-policy advisory
locks do not cover other-name collisions or legacy writers; keep constraint and
row locks. SQLite independent pools must acquire writer before first read.
Service normalizer must stay pure and not create storage import cycles. PG skips
need hosted behavioral execution receipts. No global response/event order is
promised. No production test hooks or generic callbacks framework.

## Delivery and recovery

Follow [plan delivery gates](plan.md#delivery-and-completion-gates-later-release-only):
normal hooks, ready PR, one retained/joined pr-await, head freeze except valid
remediation, authenticated App 347564 full changed-path CURRENTHEAD review,
actual finding disposition, unrelated CI leaf evidence to parent before expansion,
no blind retries or bypass, expected-head normal squash and independent actual
merge/tree/blob/remote-main verification. Leave clean managed worktree/deps/caches.
Only design handoff, concrete blocker/recovery request or actual merge/cleanup
receipt goes queued to parent. PR creation/local green do not complete the task.

For interruptions, read platform plan preserving user edits and retained handle
log; join any active command before replacement. Record command, handle, exit,
owned cleanup and next action at each boundary. Parent-question tool ends turn.
No claim of hardware crash immunity.

## Results

Root reviewed all four design artifacts and explicitly released implementation
in this same session. Task 01 is in progress; normal delivery through actual
merge remains authorized. The broad owning specification pair remains draft.

- Permanent real SQLite service regression: handle 67951 joined exit 1 before
  production edits, with the expected stale workflow and metadata failures in
  both orders. No race; accepted root archive was not replayed.
- Implemented typed field presence, locked current-row mutation, pure service
  normalization, transaction uniqueness and truthful persisted return values.
  PostgreSQL timestamp precision required reading the written row before commit.
- Service fixture corrections: event uses `task_id`; foreign-workspace ownership
  is seeded with CreateWorkspace. Controls handle 69120 joined exit 0; actual
  persisted task snapshot assertions passed in handle 48568. Final combined
  service evidence follows the production precision correction.
- Registered REST/WS checks: handle 32389 joined exit 0. Final transport evidence
  follows the production precision correction.
- Store handle 5431 joined exit 0, race enabled, with SQLite behavior plus actual
  PostgreSQL behavior/concurrency tests (no skips). Independent physical backend
  IDs and observed advisory/row-lock waits are retained in
  `/tmp/kandev-child21-store-green.log`. Legacy CRUD/delete controls passed in
  handle 37502; that run exposed the corrected PostgreSQL timestamp mismatch.
- SQLguard handle 24552 joined exit 0. Store conformance handle 90805 joined exit
  0 with actual PostgreSQL enabled (113.910s). No schema or manifest changes.
- Initial lint handle 40369 joined exit 1 with one nestif issue. Extracted the
  SQLite writer admission helper; all new files staged for revision coverage.
  Final bounded scoped lint handle 1721 joined exit 0, zero issues, with all new
  files included in the revision scope.
- Owned PostgreSQL startup handle 44146 joined exit 0. Exact container, named
  volume, mounts, labels and creation receipts are retained at
  `/tmp/kandev-child21-pg-owner.json`; cleanup follows final joined DB checks.
- Actual changed-path PR-docs preflight returned covered; catalog, specification,
  public source and whitespace checks passed. Final results and normal hook
  evidence will replace remaining pending delivery details before publication.

Final post-correction receipts (exact Verification commands above, race enabled):

- Store handle 65969 joined exit 0 (14.060s): all new SQLite and actual PG behavior
  and concurrency tests, including the added actual uniqueness-error mapping.
- Service handle 82659 joined exit 0 (2.199s): complete focused new/existing matrix,
  actual CreateTask persisted tuple/events and old/new immutable snapshots.
- Handler handle 56756 joined exit 0 (1.782s): registered REST/WS plus existing
  CRUD, workspace scope and read-only controls.
- Final logs: `/tmp/kandev-child21-store-final.log`,
  `/tmp/kandev-child21-service-final.log`, `/tmp/kandev-child21-handlers-final.log`,
  `/tmp/kandev-child21-lint-final.log`. Earlier SQLguard/conformance logs are
  `/tmp/kandev-child21-sqlguard.log` and `/tmp/kandev-child21-conformance.log`.
- Owned PostgreSQL container and named volume removed only after every DB handle
  joined. Rechecked exact IDs, labels and sole mount against upfront receipt;
  absence independently verified and cleanup timestamp retained there.

Checkpoint: local runtime checks complete, no DB resource or test/lint handle
live. Pinned frozen install handle 62949 joined exit 0 (Node 24.21.0 / pnpm 9.15.9),
no lockfile changes. Catalog/spec lint/public source (47 pages), 62 public-validator
tests and whitespace checks passed. Normal hooks/publication, hosted real-PG
execution, authenticated full current-head
CodeRabbit review and actual merge remain pending. Keep order in progress;
external final receipts belong in the platform plan to preserve published head.
No completion or hardware crash immunity claimed.
