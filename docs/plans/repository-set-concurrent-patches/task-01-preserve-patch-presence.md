---
id: "01-preserve-patch-presence"
title: "Preserve repository-set patch presence through persistence"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-001
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SETS-001.1
  - AC-WORKSPACES-REPOSITORY-SETS-001.3
  - AC-WORKSPACES-REPOSITORY-SETS-001.4
  - AC-WORKSPACES-REPOSITORY-SETS-001.6
  - AC-WORKSPACES-REPOSITORY-SETS-001.7
  - AC-WORKSPACES-REPOSITORY-SETS-001.8
  - AC-WORKSPACES-REPOSITORY-SETS-001.9
  - AC-WORKSPACES-REPOSITORY-SETS-001.10
  - AC-WORKSPACES-REPOSITORY-SETS-001.11
system_design:
  - ../../specs/workspaces/system-design/repository-sets.md
---

# Task 01: Preserve Repository-Set Patch Presence Through Persistence

## Summary

Replace stale whole-metadata persistence on the service PATCH path with a typed
presence-aware store mutation. Deliver the repaired transaction with real
database regression tests, registered transport evidence and a small public API
clarification in one sequential pass.

## Release barrier

The design turn ended at its handoff. A later explicit parent implementation
interrupt in session `28c36921-68d8-4cba-9997-86cfedbfac3a` released this work order
before production/permanent test changes, dependency installation or heavy checks.
No approval/model-switch question. No agents, extra tasks, sessions or tabs.

## In scope

- `RepositorySetPatch` in a small models file; `PatchRepositorySet` in the
  repository interface and concrete SQLite/PostgreSQL store.
- Service validation/normalization preserving request presence, direct
  touched-column metadata UPDATE and atomic optional member replacement.
- Preserve existing whole-set update semantics through a shared transaction
  helper, including generated item fields and caller timestamp behavior.
- Preserve auth, validation, case-insensitive uniqueness, not-found on a deleted
  parent, complete ordered member/base replacement and failed-write rollback.
- Keep post-commit reread and use its same model for response and event.
- Permanent tests and exact checks below, plus the existing public API paragraph.

## Out of scope

No schema/history/manifest changes, public request/DTO changes, frontend code or
copy, UI conflict handling, revisions/ETags, global event ordering, general
concurrency audit, batch-task atomicity or repository deletion policy changes.
No browser/build/E2E, full service/backend local suite, synthetic compatibility
tests or broad diagnostic loops. Do not replay or remove the parent's accepted
temporary proof archive.

## Acceptance

1. Both metadata directions and membership/metadata overlaps retain every
   disjoint supplied change across independent services/stores. Omitted metadata
   and item rows cannot be rewritten from a stale read; explicit empty/no-op
   and overlapping-field behavior match `.9` through `.11`.
2. Supplied metadata and ordered membership/bases commit or roll back together.
   Invalid/foreign/unauthorized/conflicting/deleted-set cases preserve their
   existing errors and leave no partial mutation or failed-write update event;
   legacy whole-set callers retain their intentional semantics.
3. Registered REST PATCH and WS dispatch return/publish the persisted
   post-change observation using existing DTOs. Real PostgreSQL multi-connection
   behavior and required hosted gates supply dialect evidence; missing DSN is
   recorded as a skip. Scoped checks and actual-path document coverage pass.

## Implementation sequence

1. Read the linked owner pair, plan, backend guidance, `/tdd`, and its backend
   testing reference. Change this work order to `in_progress` after release.
2. Add `TestRepositorySetConcurrentDisjointPatches` against the current service
   and real SQLite store first. Its barrier initially wraps the existing store
   write (embedding the real interface, overriding only that boundary). Run the
   red command below and retain the expected lost-field assertion receipt.
   A compile error, timeout or unrelated failure is not the required red result.
3. Add the internal type/interface method, shared store transaction helper and
   service caller. Move the final test barrier to `PatchRepositorySet` when
   changing the service seam; keep the same persisted-state expectations.
   Avoid a fallback/type assertion to the old write that can silently lose
   presence. Do not add a per-service mutex as the repair.
4. Complete store, service, transport and PG tests below. Reuse fixture/barrier
   helpers only when they remove actual duplication. Keep new tests in small
   files instead of enlarging broad test files or `models.go`.
5. Update only the existing repository-set API paragraph in
   `docs/public/tasks-and-workflows.md`. Suggested wording: "Updates change
   only supplied fields. Omitted name and description values stay unchanged;
   send an empty description to clear it. Concurrent updates to different
   fields preserve both changes. When updates supply the same field, the last
   committed write wins. A supplied member list replaces all members and saved
   bases together." Keep the existing membership compatibility explanation.
   Do not claim this resolves stale complete editor drafts.
6. Run the exact checks serially. Record red/green/PG skips/lint/document
   receipts, actual executed test names/counts and all handles in Results and
   the live task plan. Mark `done` only after every required scoped check is
   terminal and satisfied. Reconcile paired spec lifecycle with actual coverage;
   do not promote unrelated unresolved design sections automatically.
7. Continue the already authorized full delivery through normal hooks,
   commit/push/ready PR, current-head full review and actual verified merge.
   The work order's implementation checks are not the task completion gate.

## Permanent test design

### Service: real SQLite, independent instances

Use `service_repository_sets_patch_test.go` with the existing real fixtures or
a minimal fixture containing real workspace/repository/set stores. For the
concurrency proof, build two `Service` objects over independent repository
objects/SQLite connections to the same owned temporary database. Do not copy a
live service struct or depend on its process-local lock. Construction need only
the dependencies these set operations use; do not start unrelated workers.

- `TestRepositorySetConcurrentDisjointPatches`: channel barrier after the first
  request's real read/validation, immediately before its store write. Complete
  the second operation, then release/join the first. Table subcases cover delayed
  description versus completed name, delayed name versus completed description,
  delayed membership versus completed metadata, and delayed metadata versus
  completed membership. Use distinct initial/new names, descriptions, member
  order and saved bases, including a base on each of two members. Check both
  successful results, final DB values, contiguous explicit replacement order,
  and omitted member IDs/positions/bases/timestamps. A sequential control with
  the same sentinels proves fixture expectations. Inspect captured post-change
  events without requiring globally ordered publication or a revision snapshot.
- `TestRepositorySetPatchPresenceAndValidation`: description clear versus
  absent, name-only, member-only, combined and empty patch. Assert normalization,
  omitted row preservation, successful timestamp/event behavior without sleeps,
  and relevant invalid-name/empty-or-duplicate-members/unsafe-base/both-member-
  fields/unknown-or-foreign-member/name-conflict/unauthorized controls. Rejecting
  any part of a combined update leaves metadata/members/bases/timestamps intact
  and publishes no update. Reuse positive writable/event controls with the same
  fixture so a negative cannot pass through disabled prerequisites. Use existing
  auth fixture patterns for the scoped caller, not fabricated ownership checks.
- `TestRepositorySetPatchDeletedBeforeWrite`: pause the update at the actual
  store seam, delete through an independent real service/store, release/join.
  Require not-found, no resurrection/items and no update event from the failed
  request. The legitimate deletion event remains allowed.
- `TestRepositorySetConcurrentSameFieldPatches`: delay one real validated name
  patch while another commits, then release. Assert the final name equals the
  last commit's supplied value and omitted description/members are preserved.
  Cover membership competition in the PG store test rather than a Cartesian
  product across every field and transport.

Every goroutine, DB and subscription gets cleanup immediately. Gates select
on context cancellation; release is idempotent and cleanup joins goroutines
before closing their DBs. Bound waits generously as deadlock guards, never as
timing assertions. No sleeps to manufacture concurrency; no `t.Fatal` inside
worker goroutines. Assert typed errors and persisted values, not only calls.

### SQLite store controls

Use `repository_set_patch_test.go` in the SQLite package so tests can inspect
raw rows and reuse real `newRepoForSetTests`/`setFixture` patterns.

- `TestPatchRepositorySetPresence`: omitted fields, explicit empty description,
  combined metadata, member-only and empty patch. Compare raw omitted item rows
  including saved bases, IDs and timestamps. Empty patch still updates the
  parent's timestamp and reports a missing parent, as the existing write did.
- `TestPatchRepositorySetRollsBack`: valid metadata plus replacement containing
  one valid and one nonexistent member. The real FK failure occurs after UPDATE,
  delete and a partial insert; assert complete pre-write DB values, including
  parent timestamp and old ordered/base-bearing items, survive. Use a minimal
  failpoint only if an equivalent real constraint cannot target the phase.
- `TestPatchRepositorySetDeleted`: metadata/member/empty patch cannot recreate a
  deleted parent, return success or leave member rows.
- `TestPatchRepositorySetNameConflict`: direct patch colliding case-insensitively
  with another set fails atomically, including when explicit members accompany
  the rename. Service precheck remains separate; no new race-error mapping.

Keep the existing whole-set update tests unchanged unless an actual fixture
dependency needs adaptation. Their commands below pin legacy semantics and
atomicity instead of adding synthetic method-compatibility tests.

### Registered transport boundaries

Use new `repository_set_patch_handlers_test.go`, the real Gin router registered
by `RegisterRepositorySetRoutes`, and its real WS `Dispatcher`. Extend or add
a focused test-only fixture to expose the real DB/service/event bus; avoid
changing production handlers just to add test hooks.

- `TestHTTPRepositorySetPatchPresenceAndEvents`: send actual encoded PATCH
  payloads for omitted metadata, explicit empty description, membership with
  ordered bases, combined fields and empty payload. Assert HTTP status, complete
  response DTO, DB read and captured updated payload share the observation in
  these sequential controls. Add one combined invalid request proving unchanged
  DB state/no update event and stable error category.
- `TestWSRepositorySetPatchPresenceAndEvents`: use `ws.NewRequest` and
  `dispatcher.Dispatch` for `ActionRepositorySetUpdate` with the same small
  presence matrix and error control. Assert response action/request correlation,
  DTO, DB values and actual emitted updated payload. Do not call the unregistered
  handler directly or substitute a mock service/store.

Concurrent service tests prove the race; transport tests prove existing
presence reaches that repaired service. Do not duplicate the entire concurrent
matrix per transport or add gateway ordering promises.

### PostgreSQL: real behavior and actual contention

Use new `repository_set_patch_postgres_test.go` in the SQLite package.
Call `testutil.PostgresDSNFromEnv(t)` and `OpenIsolatedPostgres` for the owned
schema, then reuse `openSecondPostgresConnection` from
`turn_step_stamp_postgres_test.go` to pin independent writer/observer physical
connections to that schema. Do not reuse the helper's single pool for both
writers or add a general multi-connection framework. Initialize schema once,
then use `NewWithInitializedDB` for independent store objects where appropriate.

- `TestPostgresRepositorySetPatchBehavior`: exercise the real patch method for
  omission, clear/no-op, explicit ordered bases, combined-write rollback on
  actual member FK/unique-name failure and deleted-parent rejection. This is
  method behavior, not schema replay.
- `TestPostgresRepositorySetConcurrentPatches`: hold the parent row in an owned
  transaction on one connection, start the actual patch on another, and observe
  its real PostgreSQL lock wait before releasing/joining. The holder can make a
  legitimate bound metadata UPDATE to create the omitted-field sentinel.
  Cover both name/description directions and member/metadata overlap. Also
  gate two independent real membership replacements behind the held parent
  row; final membership must equal one complete supplied order/base list, never
  mixed lists or duplicate positions. Observe both writers waiting, release,
  join and require both successes. Same-field metadata winner follows known
  commit order; do not infer scheduling/commit order from timestamps or sleeps.
  Use existing PostgreSQL wait-observation patterns with bounded contexts.

The existing `postgres-boot` hosted job runs all tests in
`internal/task/repository/sqlite` with a real DSN, including these planned tests.
It is part of the required Backend Aggregator. PostgreSQL 18's focused boot/
upgrade job is supplementary and does not execute these patch test names.
Require the `postgres-boot` receipt proving these tests ran, not just a green
boot/replay test or an absent-DSN local package pass.

## Verification

All commands below run after release, from the repository root unless their
subshell sets `apps/backend`. Execute ONE heavy command per tool call. If it
returns a `session_id`, retain it and use `write_stdin` until actual terminal
exit before any next test/lint/install. Record exact commands, handles, exit
codes and logs; lost handle/crash/timeout/nonzero with zero reported issues is
not a pass. No automatic resource retry, cache wipe or foreign process kill.

Use Bash with `login=false` for Node tooling and hooks; the login Zsh profile
does not expose Node here. Existing Node 24.21.0 is at
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin`. Put that directory first
in the command environment PATH for `node`, `npx`, pnpm and normal Git hooks.
No runtime installation is needed to use it.

First, TDD red only (do not replay the accepted temporary proof):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^TestRepositorySetConcurrentDisjointPatches$' -count=1 -v)
```

Green service command: all new families plus existing affected service controls.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^(TestRepositorySetConcurrentDisjointPatches|TestRepositorySetPatchPresenceAndValidation|TestRepositorySetPatchDeletedBeforeWrite|TestRepositorySetConcurrentSameFieldPatches|TestUpdateRepositorySetReplacesMembershipAndPublishes|TestUpdateRepositorySetLeavesOmittedFieldsAlone|TestUpdateRepositorySetRejectsEmptyMembership|TestUpdateRepositorySetKeepingItsOwnNameIsNotAConflict|TestUpdateAndDeleteMissingRepositorySetReportNotFound|TestUpdateRepositorySetRejectsMemberAndLeavesMetadataAlone|TestRepositorySetUpdateReplacesBasesAtomically|TestRepositorySetRejectsConflictingMemberInputs|TestDeletingARepositoryPublishesTheSetsThatHeldIt|TestDeletingARepositoryInNoSetPublishesNoSetUpdate)$' -count=1 -v)
```

Green SQLite store command: new controls and actual legacy write/base controls.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestPatchRepositorySetPresence|TestPatchRepositorySetRollsBack|TestPatchRepositorySetDeleted|TestPatchRepositorySetNameConflict|TestUpdateRepositorySetRollsBackMetadataWhenMembershipFails|TestUpdateRepositorySetAppliesBothHalvesTogether|TestUpdateRepositorySetOnDeletedSetReportsNotFound|TestUpdateRepositorySetChangesNameAndDescription|TestReplaceRepositorySetItemsRewritesOrderContiguously|TestRepositorySetRoundTripsSavedBaseBranch|TestCreateRepositorySetRejectsNameDifferingOnlyByCase)$' -count=1 -v)
```

Registered HTTP/WS and existing affected/read-only controls:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/handlers -run '^(TestHTTPRepositorySetPatchPresenceAndEvents|TestWSRepositorySetPatchPresenceAndEvents|TestRegisterRepositorySetRoutesWiresHTTPAndWS|TestHTTPUpdateRepositorySetReplacesMembership|TestHTTPUpdateRepositorySetOmittedMembershipIsPreserved|TestHTTPRepositorySetUpdateCarriesMemberBaseBranch|TestWSRepositorySetMutationsRejectReadOnlyWorkspace|TestWSRepositorySetReadsAreAllowedInReadOnlyWorkspace)$' -count=1 -v)
```

PostgreSQL method behavior/concurrency and the existing affected PG store update:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestPostgresRepositorySetPatchBehavior|TestPostgresRepositorySetConcurrentPatches|TestPostgresRepositorySetRoundTrip)$' -count=1 -v)
```

Use at most one owned temporary dedicated PostgreSQL instance if available and
practical under the resource guard, or truthfully record the env skip and rely
on the required hosted real PG gate. Never touch the user's instance/DB or
foreign containers. If a temporary DSN is needed, use the owned fixture's
environment without exposing credentials in logs; cleanup only that fixture.
Do not start PostgreSQL/compile checks in this design turn.

Persistence gates, each separately started and joined:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go run -trimpath -tags fts5 -p=1 ./cmd/sqlguard ./internal)
```

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/persistence/storeconformance -count=1 -v)
```

Bounded focused package lint, using the fixed proofbase for initial work:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./internal/task/models ./internal/task/repository ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers --new-from-rev=220e84bb8b62d860adc05891c835516fbf249f2e --concurrency=2 --allow-serial-runners --timeout=5m)
```

For an actual backend PR fixup, replace the focused invocation with the required
full changed lint from `apps/backend`, once and bounded. Resolve/persist
`repository_set_pr_base` as the exact PR base SHA before starting:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./... --new-from-rev="${repository_set_pr_base:?exact PR base SHA required}" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Do not rerun already passed checks without changed code/finding evidence. Verify
the named test functions actually ran in `-v` output; a no-tests package pass is
not coverage. If tests are renamed/split, update the exact selectors and check
all changed test paths before marking done.

Lightweight document/diff checks, also required at the design handoff:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/specs/workspaces docs/plans/repository-set-concurrent-patches
```

Actual-path delivery coverage preflight, using raw `execFileSync` Git output;
it includes new untracked work orders and reads current working-tree documents:

```bash
node <<'NODE'
const fs = require('node:fs');
const {execFileSync} = require('node:child_process');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const rawPaths = args => execFileSync('git', args, {encoding: 'utf8'}).split('\0').filter(Boolean);
const paths = [...new Set([
  ...rawPaths(['diff', '--name-only', '-z', '220e84bb8b62d860adc05891c835516fbf249f2e']),
  ...rawPaths(['ls-files', '--others', '--exclude-standard', '-z']),
])];
const documents = [
  'docs/specs/workspaces/requirements/repository-sets.md',
  'docs/specs/workspaces/system-design/repository-sets.md',
  'docs/plans/repository-set-concurrent-patches/plan.md',
  'docs/plans/repository-set-concurrent-patches/task-01-preserve-patch-presence.md',
];
const fileContents = Object.fromEntries(documents.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({changedFiles: paths.map(filename => ({filename, status: 'modified'})), fileContents});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

At design time additionally evaluate the three planned production trigger paths
(`interface.go`, `sqlite/repository_set.go`, `service/service_repository_sets.go`)
with these actual documents and label that result projected coverage. The real
implementation preflight above must use actual changed paths only. Inspect the
status list/diff to ensure all actual paths belong to this work order.

After the public paragraph edit, run its normal validators:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

After release, if `apps/node_modules` is absent, install once in `apps/` with
pinned pnpm 9.15.9, frozen lockfile and existing Node 24.21 PATH using Bash
`login=false`, before normal commit hooks. Preserve lockfile/deps/shared caches;
do not install during design or bypass hooks. Commit-hook formatting requires
restaging and a new normal commit, not amend/bypass.

```bash
(cd apps && PATH=/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH npx --yes pnpm@9.15.9 install --frozen-lockfile)
```

## Files likely touched

- `apps/backend/internal/task/models/repository_set_patch.go` (new typed seam).
- `apps/backend/internal/task/repository/interface.go`.
- `apps/backend/internal/task/repository/sqlite/repository_set.go`.
- `apps/backend/internal/task/service/service_repository_sets.go`.
- `apps/backend/internal/task/service/service_repository_sets_patch_test.go` (new).
- `apps/backend/internal/task/repository/sqlite/repository_set_patch_test.go` (new).
- `apps/backend/internal/task/repository/sqlite/repository_set_patch_postgres_test.go` (new).
- `apps/backend/internal/task/handlers/repository_set_patch_handlers_test.go` (new).
- `apps/backend/internal/task/handlers/repository_set_handlers_test.go` only if
  exposing a reusable test fixture is smaller than a dedicated patch fixture.
- Existing service/store repository-set tests only if a real seam/fixture change
  requires adaptation; preserve their behavioral assertions and scoped commands.
- `docs/public/tasks-and-workflows.md` (existing repository-set API paragraph).
- Existing owner requirement/design pair and this plan/work order for lifecycle
  and actual receipts. No frontend production files.

## Dependencies

None. The completed repository-set base-branch package is historical context,
not another execution wave. All work remains in this primary session.

## Risks

- Fixed SQL fragments must not accidentally use stale metadata or interpolate
  request values. `Rebind` and database parameters remain mandatory.
- Membership replacement must follow the parent UPDATE in the transaction;
  moving it first would discard the row-lock serialization boundary on PG.
- Initial service validation is not a concurrency merge or a new repository
  deletion invariant. Do not broaden this repair to redesign those paths.
- Response/event rereads are observations, not globally ordered revisions.
- A missing DSN or skipped PG test is an outstanding hosted evidence dependency.
- New test helper goroutines and connections must drain even on early failure.

## Parallelism

`sequential`. ONE heavy command at a time; no delegation.

## Inputs

- [Requirements](../../specs/workspaces/requirements/repository-sets.md),
  `REQ-WORKSPACES-REPOSITORY-SETS-001` criteria listed in frontmatter.
- [System design](../../specs/workspaces/system-design/repository-sets.md),
  Partial updates and concurrency, Service and transport, Events and boot state.
- [Plan](plan.md), confirmed proof receipt and audited callers/compatibility.
- `apps/backend/AGENTS.md` SQL dialect/RMW rules and persistence checks.
- `.agents/skills/tdd/SKILL.md` and `references/backend-tests.md`.
- Existing repository-set store/service/handler fixtures and PostgreSQL
  independent-connection/lock-wait patterns. Read accepted temporary proof only.

## Delivery and completion gate

Use the live task plan for receipts and recovery checkpoints. After checks,
normal hooks -> commit/push ready PR -> freeze head unless a valid finding
requires code changes. No moving-main rebase or late synthetic compatibility
tests. Keep ONE `scripts/pr-await` all-terminal monitor handle and join it
before replacement. Inspect fresh required hosted gates and policy/thread state.
Require authenticated CodeRabbit App347564 substantive FULL all-changed-paths
review of the CURRENT head (`source = covered = head`, kind reviewed). Inspect
automatic evidence before at most one needed full review request; stale
incremental, skipped or acknowledgement-only evidence is insufficient.
Disposition all actual threads/grouped findings. Optional style/docstring
findings can be deferred without changing the frozen head.

For unrelated CI failures, provide exact failing leaf/log/artifact/source
evidence to the parent before expanding/retrying. No blind repetitions, weakened
assertions/timeouts/race/goleak gates or admin bypass. Squash using normal
expected-head merge only when actual required gates are terminal and clean.
Independently verify merge SHA/time, exact PR tree/owned blobs and authoritative
remote main. Join every handle and clean only owned temporary resources. Leave
the managed worktree clean with dependencies and shared caches available for
the parent's archive. Publication/local green is not task completion.

Parent `14825981-b175-411d-999a-31ddc2aa5fc3` receives queued/omitted delivery
only: concrete design handoff, blocker/recovery needing direction, or verified
merge and joined-cleanup receipt. Do not send ACK/heartbeat-only messages.

## Results

Implementation released by the parent in this same session after reviewing all
four design artifacts. Added `RepositorySetPatch` and `PatchRepositorySet`,
with fixed SQL fragments and bound values for supplied columns. The first
transaction mutation updates the parent row; optional ordered member replacement
follows in that transaction. The legacy whole-set method shares this helper and
retains caller timestamp/item population. No read/merge or service mutex is used.
Service normalization, authorization and post-commit observational reread remain
on the existing path. The public API paragraph now states omission preservation
and concurrent disjoint/same-field behavior.

Local receipts, using the exact selectors and resource flags above:

| Check | Joined handle | Result |
| --- | --- | --- |
| Permanent service RED, original store seam | `99790` | Expected exit 1: all four sequential controls passed; three overlap cases lost an omitted metadata field; omitted membership case passed. No race report. |
| Service GREEN | `1211` | Exit 0, all 14 named families passed; package 1.616s. |
| SQLite store GREEN | `54318` | Exit 0, all 11 named families passed; package 2.322s. |
| Registered HTTP/WS GREEN | `5975` | Exit 0, all eight named families passed, including nine presence/combined/invalid cases for each transport; package 3.822s. |
| Real PostgreSQL 16 | `26543` | Exit 0, all three named families passed; package 12.186s. No DSN skip. |
| SQL guard | `76995` | Exit 0, no findings. |
| Required store conformance, SQLite and PG | `10065` | Exit 0, package 120.463s. Catalog, adapters, fresh/replay/CRUD/behavior and upgrade controls ran. |
| Strengthened final-response assertion | `85171` | Exit 0, `^TestRepositorySetConcurrentDisjointPatches$` with the same flags; all eight cases passed, package 1.398s. Only this changed test family was rerun. |

The real PG behavior family executed presence, rollback, deleted and name-conflict
controls. Its concurrency family executed description-after-name,
name-after-description, members-after-name, same-name-last-commit and competing
member replacements. Independent physical connections and observed PostgreSQL
lock waits establish database contention. Final membership is one complete
ordered candidate with its saved bases, while disjoint metadata survives.

One dedicated cached `postgres:16-alpine` container was capped at 256 MiB/one CPU
and bound to loopback. Only its exact owned ID was removed after PG checks and
conformance joined. The parent proof was inspected read-only and never replayed
or removed. No frontend/browser/build/E2E or broad service/backend suite ran.

Focused lint initially joined `21519`, exit 1 for one new test-helper nesting
finding. Transport-specific request assertions were extracted without changing
coverage. Both affected boundary families reran with the same race flags and
the selector `^(TestHTTPRepositorySetPatchPresenceAndEvents|TestWSRepositorySetPatchPresenceAndEvents)$`:
handle `71270` joined exit 0, all 18 cases passed, package 3.148s. Corrective lint
`14186` joined exit 0 with zero issues under the same limits. No resource retry.

Documentation checks passed: catalog 346 decisions/1327 specifications, all 36
spec-linter tests, full spec lint, all 62 public-doc validator tests and 47
published pages. Actual coverage evaluated all 14 changed paths, accepted the
owning pair and this work order, covered all four production trigger paths and
reported no errors. Tracked whitespace checks passed; final staged check follows.

Normal hooks, publication, terminal hosted gates/full current-head review, actual
merge and joined cleanup remain pending delivery gates.
Exact command/resource/delivery receipts are also retained in the durable Kandev
task plan; publication alone does not complete this work order.
