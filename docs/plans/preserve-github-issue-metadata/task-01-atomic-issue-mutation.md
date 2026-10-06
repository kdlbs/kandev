---
id: "01-atomic-issue-mutation"
title: "Commit issue-only mutations atomically"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001
acceptance_criteria:
  - AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.1
  - AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.2
  - AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.3
  - AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.4
  - AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
system_design:
  - ../../specs/tasks/system-design/link-existing-task-github-issue.md
---

# Task 01: Commit issue-only mutations atomically

## Summary and release barrier

Implement the design's typed issue-only set/remove operation and real protocol/
storage evidence in one coherent sequential pass. Preserve unrelated current raw
metadata, omitted scalars/effects, existing validation and ordinary postcommit
fallback/publication; all five issue keys form one atomic domain.

ROOT reviewed the ended DESIGN package and explicitly released implementation
to this same primary on 2026-10-04, satisfying the implementation release barrier.
Local implementation and acceptance checks are verified; this work order remains
in_progress for actual verified delivery. The `/tdd` backend guidance was applied,
and actual results are retained below. Keep the same primary session/profile;
no delegation/tasks/tabs/model switch.

## In scope and owned boundaries

- Separate meaningful existing-API RED through real `github.Service`, actual
  `githubTaskIssueStoreAdapter`, real task services and independently opened DB
  handles, using construction-time forwarding gates. External GitHub issue
  transport only is mocked. The accepted `/tmp` proof is immutable, not replayed.
- Internal typed link value; required task repository/service operation; actual
  GitHub interface and adapter/caller narrowing; minimal shared lock/raw-reader
  helper reuse with ordinary merge behavior unchanged. Existing fakes only get
  adaptations needed for their existing test purpose/compilation.
- Atomic storage, real registered REST, owner/value/error/rollback/cancellation/
  observation controls and selected existing link/repository/auth/field/merge
  regressions. Minimal existing API reference clarification.
- After local checks, standing-authorized normal hooks/PR/review/terminal checks/
  actual verified squash merge and owned cleanup under the external plan rules.

## Out of scope

No general metadata patch framework, revisions, total event order, new wire/schema/
auth/routing, issue-watch reservation/dedup rewrite, arbitrary full-snapshot
guarantee, runner/Office redesign, UI/copy/layout/browser/E2E/build or broad suite.
No archive replay/deletion, foreign resources/cache/worktrees/process cleanup,
lockfile/harness/runtime changes, automatic retry or hook/merge bypass.

## Acceptance

1. Permanent existing-API delayed link and unlink RED fail on lost accepted
   unrelated stored values before production changes; controlled opposite orders
   and sequential controls demonstrate the causal boundary. Corrected real APIs
   preserve disjoint edits and all-five coherent commit order across connections.
2. SQLite and actual PG physical-wait evidence proves current raw/value/owner
   preservation, omitted scalar/association/effect isolation, cancellation,
   atomic rollback and typed errors. Actual registered routes retain DTO/auth/
   validation/fetch/WithoutCancel and normal postcommit fallback/event behavior.
3. Exact bounded checks and original installed full backend changed-code lint
   pass; actual full current-head review and all-terminal required statuses precede
   normal expected-head squash. Independently verified actual merge plus joined
   owned cleanup is the delivery result. Local done/PR published is not completion.

## Implementation order and test matrix

1. Write new backendapp fixture/concurrency companion tests using existing APIs,
   without the new method/type. Gate real GetTask snapshots at construction and
   gate external GetIssue where useful. Begin two real instances on one actual
   database with independent handles; commit accepted merge before released stale
   link/unlink, then reverse the admitted order and include sequential controls.
   Join every worker even on failure. Require successful API returns, actual row,
   all five keys, `alpha`, port preference, unrelated sentinel and task.updated
   evidence. This must fail behaviorally on current code, not fail to compile.
2. Implement `TaskGitHubIssueLink` and required `UpdateTaskGitHubIssue` methods
   per design. Preserve current authorization/preflight, WithoutCancel admission,
   repository/provider validation and early response title. Store reads raw
   current metadata under locks, applies fixed complete identity/removal, writes
   only metadata/timestamp, scans its candidate in the same transaction after
   normalization, then commits. Service reuses ordinary reload fallback and
   repository-list observation, publishes only normal task.updated.
3. Add remaining matrix in companion files:

| Planned test and companion | Cases and decisive evidence |
| --- | --- |
| `TestGitHubIssueMutationConcurrentSQLite`, backendapp `github_issue_mutation_concurrency_test.go` | Link and unlink versus accepted ordinary merge both commit orders; two service/DB handles; sequential controls; independent tasks and workspaces retain their distinct values. |
| `TestGitHubIssueMutationCoherence`, backendapp `github_issue_mutation_coherence_test.go` | Two complete distinct links, link/unlink in both commit orders, relink and legacy-watch unlink; assert all five identities/removals, preserve watch ID/author/profile fields. |
| `TestGitHubIssueMutationWriterCompatibility`, backendapp `github_issue_mutation_writers_test.go` | Ordinary omission and priority/title/native scalar interaction; human title and generated title claim/set CAS in both directions; server-owned deferred/step-handoff/provenance/carrier/workspace write/removal; same-domain ordinary replacement/full-snapshot permitted later overwrite controls. |
| `TestGitHubIssueMutationRegisteredRoutes`, backendapp `github_issue_mutation_routes_test.go` | Register real GitHub `RegisterHTTPRoutes` and task handlers; actual PUT/DELETE plus PATCH task/preference; actual DB, response and normal task.updated payload, unchanged state/transition events/effects and reload lookup. Scoped caller/provider setup is real; only remote issue transport is mocked. |
| `TestGitHubIssueMutationPostcommitObservation`, backendapp `github_issue_mutation_observation_test.go` | Construction-installed forwarding repository hook commits a later real edit before postcommit reread; ordinary reload may observe it. Inject reread error only at observation, retain committed candidate/fallback and event; repository list failure retains logged fallback behavior. No merge-style new suppression rule or exact receipt claim. |
| `TestGitHubIssueMutationFailures`, backendapp `github_issue_mutation_failures_test.go` | Authorized writer/viewer/foreign workspace/missing task, unavailable store/provider/no personal credentials, fetch rejection, invalid input/mismatch/repository lookup failure; no mutation/publication. Cancel real external fetch before admission; cancel caller after successful validation and observe existing WithoutCancel commit. Task deleted before mutation maps wrapped not-found, with both `errors.Is` identities. |
| `TestTaskGitHubIssueSQLiteValues`, sqlite `task_github_issue_values_test.go` | Both set/remove with pending/nonpending owner, false/zero/empty/arrays/nested/null, large raw number `9007199254740993`, literal keys, SQL NULL/empty/JSON null, normalized workspace group, all protected present/absent owner records; raw DB values plus unchanged full scalar baseline. No DTO precision extension asserted. |
| `TestTaskGitHubIssueSQLiteAtomicity`, sqlite `task_github_issue_atomicity_test.go` | Real rejecting trigger/storage failure, malformed/nonobject raw document, missing task and cancelled transaction; before/after raw document/timestamp identical, no partial domain. Candidate-read failure before commit rolls back. Check errors.Is, scalar/association/ledger/entry/runner isolation. |
| `TestTaskGitHubIssueSQLiteCancellation`, sqlite `task_github_issue_cancellation_test.go` | Actual independent held writer; reuse construction-installed native-driver forwarding busy probe pattern to observe genuine SQLite contention before cancellation, no substituted SQL result. Join waiter/holder and verify row/timestamp rollback with context identity. |
| `TestTaskGitHubIssuePostgresPhysicalWait`, sqlite `task_github_issue_postgres_test.go` | Separate physical connections/private schema, holder locks only task row (no shared advisory/workspace lock); observe pg_locks/pg_stat_activity/blocking PID, commit real merge/scalar/title/owner/current metadata before releasing, assert whole link/remove and current unrelated values. Both operation/order directions, cancellation during actual wait, independent task/workspace isolation and row-only result. |
| `TestTaskGitHubIssuePostgresValues`, sqlite companion values | Same applicable value/owner/legacy/coherence/rollback matrix using actual PG; ordinary pending merge's dialect semantics retained by selected regressions. No SKIP claimed as execution. |

Protect actual populated scalar/session/repository/transition/entry/runner state,
not empty fixtures alone. Assert raw storage for number/null preservation, actual
DTO/event for existing projection, and stored outcome for concurrency. Test names
are planned here, not proof until joined command results exist.

## Verification commands (after release only)

Run one local heavy command at a time from its stated directory; actually join
every returned session/PID handle before dependent mutation or next heavy command.
Use existing Node PATH and bash `login=false`. Keep exact command/output/duration/
exit receipts in the external plan. Timeout/interruption/lost response is no
verdict, never a pass. No passing broad replay.

Permanent behavioral RED first (before production changes):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/backendapp -run '^TestGitHubIssueMutationConcurrentSQLite$' -count=1 -v)
```

After correction, targeted new and directly affected existing behavior:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/backendapp -run '^(TestGitHubIssueMutation.*|TestWrapGitHubTaskIssueStoreError)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestTaskGitHubIssueSQLite.*|TestTaskMetadataMergeSQLite.*|TestTaskFieldUpdate.*|TestClaimTaskTitle.*|TestSetTaskTitle.*|TestUpdateTask.*Deferred.*|TestUpdateTaskPreservesWinningTitleAgainstStaleUpdate|TestUpdateTaskStillDeletesUnrelatedMetadataKeysByOmission)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^(TestTaskMetadataMerge.*|TestTaskFieldUpdate.*|TestUpdateTaskMetadata.*|TestUpdateTask.*Handoff.*|TestUpdateTask.*Office.*|TestSetPendingAgentTitle.*)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/github -run '^(Test(LinkTaskIssue|UnlinkTaskIssue|ParseIssueReference|HttpLinkTaskIssue|HttpUnlinkTaskIssue|HttpListTaskIssues|AssociatePRWithTask|CredentialResolver|TaskCIOptionsResolveAndAuthorizeOwningWorkspace|IssueAndPRWatchOperationsAuthorizeStoredWorkspace|ListWorkspaceTaskIssues).*)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/handlers -run '^TestTaskMetadataMergeRegisteredPortForwarding.*$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/orchestrator/executor -run '^$' -count=1)
```

Before running, confirm exact existing test names against source and narrow/fix
selectors mechanically; a no-tests match is compile evidence only. If standalone
interface fakes elsewhere need changes, add their narrow compile packages and
record why, rather than full-suite replay. New mutation wrappers must forward
the real method; do not turn behavioral regressions into mock business tests.

PG fixture is optional to provision locally but actual PG acceptance is required:
if safely owned fixture cannot be provided, checkpoint WAITING for ROOT. Record
exact ID/label/immutable PG16 image/all mounts+volumes/credentials ownership BEFORE
use, tmpfs/no anonymous volumes/private schema/independent physical connections.
Never touch unknown volume
`2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`.
Set `KANDEV_TEST_POSTGRES_DSN` from owned credentials without logging secrets,
then run sequentially:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestTaskGitHubIssuePostgres.*|TestTaskMetadataMergePostgres.*|TestTaskFieldUpdatesPostgres.*)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go run -trimpath -tags fts5 -p=1 ./cmd/sqlguard ./internal)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/persistence/storeconformance -count=1)
```

Actually join all clients before removing ONLY proven owned container/schema/
credentials. Preserve unrelated processes/worktrees/caches/volumes. No schema
changes expected; do not edit upgrade fixture/manifest unless causally required.

Documentation and reference gates, from repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
git diff --check
git status --short -- docs/plans/preserve-github-issue-metadata
```

Use `.github/scripts/pr-docs.cjs` exported `validateCoverage` with actual package
contents/changed work order and requirements/design/plan, not fragments, before
publication. Validate local document links and REQ/AC mapping. At DESIGN these
docs-only checks are allowed; implementation tests/install/DB are still barred.

Before hooks/package commands after release, exactly one pinned frozen install:

```bash
(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
```

Do not change lockfile or tool/cache/harness setup. Original installed backend
lint binary (record resolved path/hash/version before executing), full ./... gate
independent of scoped lint/hooks; actual immutable PR base is this reviewed base
unless ROOT causally amends it. Default command for that base:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 11m golangci-lint run ./... --new-from-rev=ca1492fafa5bd86bdc02a5fff476b4428c558de8 --timeout=10m --concurrency=2 --allow-serial-runners)
```

Retain exact original binary/command/owned PID/duration/exit. Scoped lint, if
needed for concrete findings, is concurrency2/allowserial bounded. Correct only
concrete causal lint findings and run affected checks/gate; resource failure
requires durable checkpoint+WAITING ROOT bounded direction, never automatic
retry/cache wipe/foreign kill/bypass.

## Files likely touched

- `apps/backend/internal/task/models/task_github_issue.go` (new internal type).
- `apps/backend/internal/task/repository/interface.go` (required method).
- `apps/backend/internal/task/repository/sqlite/task_github_issue.go` and new
  `task_github_issue_*_test.go` companions (canonical storage and evidence).
- `apps/backend/internal/task/repository/sqlite/task_metadata_merge.go` (only
  small shared lock/raw-reader extraction if needed; ordinary semantics intact).
- `apps/backend/internal/task/service/service_task_github_issue.go` (new service).
- `apps/backend/internal/github/service_task_issue.go` (typed dependency/calls).
- `apps/backend/internal/backendapp/adapters.go` (real adapter forwarding).
- `apps/backend/internal/backendapp/github_issue_mutation_*_test.go` companions.
- `apps/backend/internal/github/service_task_issue_test.go` and
  `service_pr_watch_workspace_group_redirect_test.go` (existing fakes only).
- Standalone `TaskRepository` fakes found by compilation, only as required.
- `docs/public/websocket-api.md` (minimal accurate existing-reference paragraph).
- This four-artifact package for status/results/current design promotion.

Production comments state the invariant, with no incident/review/AC narration.
Use companion files when existing files are oversized; no optional cleanup.

## Delivery and stop conditions

Normal active Conventional Commit hooks, no bypass/amend. ROOT's release gives
standing authorization through actual normal merge; no repeated operator prompt.
Read delivery skills at that phase. Ready PR with live body/core readback and
exact local/upstream/remote/PR head. Freeze SHA except causal/required semantic
correction; optional style/logging/docstring assurance gets grounded disposition.
No moving-main rebase; cheap static merge-tree/owned-blob compatibility near merge,
no synthetic merged tests.

ONE retained practical90m `scripts/pr-await --mode all-terminal` JSON wait,
actually joined before replacement, no duplicate/timer pr-state/hosted cancellation.
Inspect completed review during CI and prepare required corrections promptly.
Authenticate configured CodeRabbit App347564 substantive FULL ALL actual files,
source=covered=CURRENT HEAD/kind=reviewed. Automatic full review suffices; inspect
skip/gap before at most ONE necessary new-head request. Claude0s ACK/empty/
boilerplate/skips are not semantic proof; no optional second wait/settings change.
Disposition every real thread/grouped finding with current evidence.

Unrelated hosted failures require exact logs/artifacts/source and WAITING ROOT
bounded direction; no blind second rerun, assertion/time/race/goleak weakening
or broad local UI replay. All-terminal known required statuses/full current review/
no actionable hidden or unresolved/errors before expected-head normal squash,
no admin/bypass. Independently verify actual MERGED/time/SHA/parent/tree/all owned
blobs/authoritative main inclusion. Published head is not merge SHA. Join all
handles and clean only owned resources/managed worktree/deps; retain parent proof
for ROOT archival. ROOT incoming queue is full: save exact checkpoint/resource/
handle/next action in existing external plan and end WAITING, no callback retries.

## Dependencies, risks and parallelism

Dependencies: reviewed base's existing field/merge persistence boundary only.
Parallelism: `sequential`, same primary session. The
[plan risks](plan.md#risks) apply; a newly causal boundary needs ROOT amendment.

## Inputs and results

- [Owning requirement](../../specs/tasks/requirements/link-existing-task-github-issue.md).
- [Issue mutation design](../../specs/tasks/system-design/link-existing-task-github-issue.md).
- [Shared field/merge design](../../specs/tasks/system-design/task-field-updates.md).
- Accepted immutable proof/receipt recorded in [the plan](plan.md).
- Existing backendapp real-service fixture, native SQLite busy-probe and PG
  physical-row-wait tests; actual controller registration and publication path.

Results at local acceptance on 2026-10-04: reviewed implementation release received
in the same primary session. Separate permanent existing-API RED joined exit 1;
corrected production path and full targeted SQLite/registered REST/owner/error/
observation matrix joined exit 0. Actual PG physical/advisory waits, current raw
values, link/unlink coherence, both orders, independent task/workspace writes and
cancellation/rollback passed without claiming SKIP as proof. Selected existing
regressions, SQLguard and actual SQLite/PG storeconformance passed. Executor
interface adaptation compiled only. All clients joined; only owned tmpfs PG and
credential file removed; original ROOT proof retained. See
[verification results](plan.md#verification-results) and the external task plan
for exact commands/receipts. The original full immutable-base lint gate passed
with zero issues after one minimal test binding correction; all documentation
coverage and reference gates passed. Active hooks, ready PR, full current-head
review/all-terminal required statuses and actual verified merge remain gates.
Work order remains in_progress for actual delivery.
