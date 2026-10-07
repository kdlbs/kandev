---
id: "01-preserve-checkout-intent"
title: "Preserve checkout intent through repository saves"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.21
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.23
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01: Preserve checkout intent through repository saves

## Summary

Carry the request's optional branch and refresh choices to the repository
mutation, including secret-binding atomic replacement. Capture both committed
choices for the successful service result and existing event, preserving exact,
legacy and recovery behavior. Deliver one coherent sequential correction with
independent service regressions and actual registered route evidence.

## In scope

- Required internal ordinary intent method and atomic companion, service wiring,
  dialect-safe SQL assignment/RETURNING/commit handling, necessary adapters.
- The causal test matrix below, focused native Windows/physical PostgreSQL CI,
  SQLguard/conformance, scoped conventions and public lifecycle clarification.

## Out of scope

- Other-field patch guarantees, schema/public SDK changes, new transport
  capabilities, UI/runner/defaulting/recovery redesign, revision/event framework.
- Global locks, broad writer audit, broad migrations, physical worktree creation
  claims, arbitrary extra checks, test replay of the supplied candidate.
- Sibling73 PR4305 and its profile-enabled CI stage; no delegates/new tasks,
  sessions/tabs/model switches. No implementation during this DESIGN turn.

## Acceptance

1. Independently authored real service RED/GREEN demonstrates omission
   preservation for both choices, both refresh directions, committed returned
   choices and event projection; supplied blank/false/null behavior remains
   per-surface compatible.
2. Real SQLite and physical PostgreSQL transactions preserve the companion
   atomicity and failure/rollback/deletion/cancellation contracts; intentional
   complete writes, branch CAS, exact Host fences and current admission remain.
3. Actual registered supported routes, guarded compact settings, persisted
   consumer projection, required persistence checks, docs and native Windows
   pass with new-case RUN/PASS evidence. Record every command's actual result;
   no skipped PostgreSQL/native evidence qualifies as completion.

## Implementation sequence

1. After a later explicit ROOT reviewed-package implementation interrupt in the
   same primary, re-read the package and authoritative source/base; mark this
   order in_progress. Acquire ROOT's exclusive GLOBALheavy grant before any
   install/Go/lint/owned PostgreSQL work. Do not infer a grant from silence.
2. Author permanent independent SQLite service tests first. Use the existing
   SQLite template with two independent physical handles and production stores
   and services. Gate a real completed read with bounded channels and cleanup
   joins, then admit the other successful save before releasing the stale one.
   Run the new causal test RED, preserve failing assertions and control passes,
   then implement the bounded correction and run GREEN. Never import, mutate,
   execute or copy the supplied temporary candidate as the permanent test.
3. Add neutral `models.RepositoryCheckoutIntent`, alias it in the repository
   interface package to avoid its existing SQLite provider import cycle, and add
   required `UpdateRepositoryWithCheckoutIntent` on the entity interface. Extend
   the existing optional secret-binding mutator with
   `UpdateRepositoryWithSecretBindingsAndCheckoutIntent`. Adapt necessary
   concrete adapters/test doubles and preserve unavailable-binding errors.
   Ordinary service saves call these methods; exact timestamp saves retain the
   existing full/exact methods and their service precheck plus SQL predicate.
4. Keep other fields' assignments, validation and errors. Omit branch/pull SQL
   assignments independently for nil; bind explicit blank/false. Capture branch,
   pull and timestamp using the UPDATE's RETURNING row inside the transaction,
   close/drain before commit, and project only after successful commit. Extend
   the binding transaction using the same intent-aware helper. Do not reread
   after commit, refresh a stale model before writing, use global event ordering,
   or silently take the full-write fallback for an ordinary request.
5. Complete storage/physical PostgreSQL/routing/admission/exact tests and the
   narrow downstream projection. Use real production mutation and publisher
   paths with independently asserted expected choices. Preserve the ordinary
   error text/sentinels, exact conflict and best-effort publish behavior.
6. Update the public lifecycle paragraph to distinguish repository registration
   defaults from existing saves. Add a concise two-choice intent invariant to
   `apps/backend/AGENTS.md` if implementation adds the documented methods.
   Inventory `.github/workflows/backend-tests.yml` again before adding the
   native Windows step; keep sibling73's profile-enabled stage untouched.
7. Run only the exact required commands below, serially. Update plan/order
   Results with actual RED/GREEN, dialect/route/native evidence and any concrete
   limitation. Finish owned-resource cleanup and join every original handle.
   Explicitly RETURN GLOBALheavy before the standing hosted delivery phase.

## Test matrix

Proposed tests live in new focused files to respect the 800-effective-line
limit. Nearby patterns are `service_workspace_field_updates_test.go`,
`sqlite/workspace_field_updates_postgres_test.go`, registered workspace handlers,
`backendapp/settings_operations_test.go`, and `plugins_workspace_admin_test.go`.
Do not change their historical results or replay their tests solely as evidence
for the new regression.

### Service and real SQLite

`service/service_repository_checkout_defaults_test.go`:

- `TestRepositoryCheckoutDefaultsConcurrentSQLite`: production service instances
  and independent handles on one file; stale rename versus branch, refresh
  enable, refresh disable, both choices; explicit branch with omitted pull and
  explicit pull with omitted branch. Run both causal write orders. The five
  supplied-receipt failure shapes are minimum independently authored RED cases.
  Assert rename plus pair in stored rows, actual returns and service events.
  Include explicit-both/uncontested controls and later same-choice winner.
- `TestRepositoryCheckoutDefaultsPresence`: nil, explicit blank branch, true,
  false, both and empty ordinary request. Verify creation default true is
  unchanged, omitted policy on an existing false row stays false, normal branch
  validation still applies, and empty save still refreshes timestamp/events.
- `TestRepositoryCheckoutDefaultsCompanionAtomicity`: real scoped secret catalog,
  valid references and binding tables, stale rename/binding replacement versus
  both choices. Cover nil/preserve and empty/clear bindings. Force a binding
  insert failure after the settings UPDATE using a real test constraint/trigger;
  assert settings, timestamps and binding rows all roll back and no event.
- `TestRepositoryCheckoutDefaultsOwnMutationProjection`: gate after the real
  persistence method commits but before the first service returns/publishes;
  a second service explicitly changes both choices. First return/event must
  retain its own committed pair; stored final row/second event must reflect the
  second write. This rejects post-commit reread fixes and stale snapshot results.
- `TestRepositoryCheckoutDefaultsFailures`: invalid branch, invalid provider
  pairing, unauthorized workspace, read-only route admission, missing/deleted
  repository and context cancelled before mutation. Assert unchanged sentinels
  and no success events, not merely a non-nil error. Join held workers on all
  assertion failures and preserve foreign rows/bindings.

`sqlite/repository_checkout_defaults_test.go`:

- `TestRepositoryCheckoutDefaultsStorage`: real intent SQL, unchanged other-field
  assignments, bool/timestamp decoding, live-row predicate, deletion between
  read and update and cancelled transaction. No method-existence assertions.
- `TestRepositoryCheckoutDefaultsLegacyAndExact`: deliberate full-model writes
  still replace both values; full companion rollback; correct/stale timestamps
  with and without bindings; a disjoint ordinary write invalidates exact save.
  Narrow branch CAS succeeds only for the observed branch/live row and never
  changes the pull flag. Exercise actual store calls, not predicate string tests.

### Physical PostgreSQL

`sqlite/repository_checkout_defaults_postgres_test.go` (external test package
can construct the real service):

- `TestRepositoryCheckoutDefaultsPhysicalPostgres`: two independently pinned
  backend connections plus observer with distinct recorded `pg_backend_pid()`;
  real service/store stale-snapshot cases in both directions. Also lock the
  repository row with a transaction, start the competing production service
  write, and prove it actually waits on that holder via `pg_blocking_pids` and
  ungranted transaction locks before committing the holder's independent
  branch/pull or rename write. The lock-holder fixture uses real SQL, not a
  claim of a second service transaction. Cover enable, disable, both, branch
  with omitted pull, pull with omitted branch and explicit-both controls.
  Assert committed pair, surviving metadata, return and actual service event.
- `TestRepositoryCheckoutDefaultsPostgresFailures`: real binding insertion
  failure and rollback; holder rollback, delete-then-release, cancelled blocked
  writer, matching/stale exact timestamp with/without bindings and legacy/CAS
  controls. Release/rollback holder, cancel and join all original writers before
  closing handles. Bounded polling is observation, not sleep-as-ordering proof.
  Distinguish commit from rollback outcomes with row/binding/timestamp sentinels.

Environment gating remains usable for normal local runs, but required execution
must provide a proved isolated DSN and show both named test groups RUN/PASS;
SKIP, schema replay and fake adapters do not count.

### Registered routes and exact adapters

- `handlers/repository_checkout_defaults_test.go`:
  `TestRegisteredRepositoryCheckoutDefaultsHTTP` drives registered Gin PATCH
  with actual service/store read gate, both optional fields, null/blank/false,
  stale rename and successful/failed secret companion. Inspect real HTTP DTO,
  database and service event, read-only/scope/invalid errors.
  `TestRegisteredRepositoryCheckoutDefaultsWS` drives the registered action
  dispatcher with its existing branch-only schema, checks branch presence/null,
  preservation of concurrently changed pull, response and actual service event.
  Never pretend WS supports a pull setter.
- `backendapp/repository_checkout_defaults_settings_test.go`:
  `TestRepositoryCheckoutDefaultsGuardedMCP` registers actual MCP handlers with
  the existing guarded dispatcher, real settings registry, production
  `settingsOperations`, scoped caller and actual task service/SQLite. Dispatch
  `ActionMCPUpdateSettings` for a repository target. Verify both choices, stale
  rename, key normalization, non-nullable null rejection, wrong target/caller,
  sensitive-field redaction and returned committed settings. Check the service
  publisher's event only; the compact adapter has no independent publisher.
  `TestRepositoryCheckoutDefaultsExactHost` invokes the actual plugin adapter
  with resource-version match, intervening disjoint save, stale conflict, replay
  no-op and cross-workspace denial. Keep its saved result/version semantics.
- `orchestrator/executor/repository_checkout_defaults_test.go`:
  `TestRepositoryCheckoutDefaultsConsumerProjection` uses real service saves and
  SQLite with a local Git fixture, then the production `resolveTaskRepoInfo` and
  `applyRepositoryConfig`/`buildRepoSpecs` paths. A task without an explicit base
  picks the stored default and both launch shapes retain the stored pull flag;
  an explicit task base keeps precedence. No manually prefilled `repoInfo` or
  refresh-policy redesign. This proves producer wiring, not worktree creation.

## Verification

All commands are planned for implementation, except the final cheap design
block. Run each separately from the stated scope under the exclusive grant,
retaining every handle through its actual exit and freshly checking process
groups are gone. No parallel heavy commands or automatic resource retries.
GNU timeout uses TERM then kill after 10s. Test defaults are trimpath/fts5/race,
one package worker, GOMAXPROCS=2/GOMEMLIMIT=512MiB, Go timeout4m/GNU6m.

RED: run the first command after independently authoring the minimum service
regressions and before production edits. Capture expected STORED/RETURNED/event
failures and passing controls. GREEN: run it after the correction, then the
remaining focused suites once their code is complete. Do not replay passing
checks absent a new change/failure/unresolved concern.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestRepositoryCheckoutDefaults(ConcurrentSQLite|Presence|CompanionAtomicity|OwnMutationProjection|Failures)$' ./internal/task/service)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestRepositoryCheckoutDefaults(Storage|LegacyAndExact)$' ./internal/task/repository/sqlite)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestRegisteredRepositoryCheckoutDefaults(HTTP|WS)$' ./internal/task/handlers)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestRepositoryCheckoutDefaults(GuardedMCP|ExactHost)$' ./internal/backendapp)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestRepositoryCheckoutDefaultsConsumerProjection$' ./internal/orchestrator/executor)
```

These selectors compile each owning package and its test doubles while running
the meaningful new regression groups. Adapt necessary fakes without adding a
second broad suite that replays passing groups. A newly changed adapter's
independent behavior needs its own exact test selector when these groups do
not exercise it.

Before PostgreSQL, inventory resources and prove exact task ownership, label
and literal ID. Use isolated database/schema, loopback-only binding, tmpfs,
no anonymous volumes or host binds; keep the DSN in a mode-0600 file and do not
print credentials. No broad container/volume cleanup. After use, remove only
the literal owned ID and prove fresh absence before GLOBALheavy RETURN.
Use existing environment-gated fixture patterns and the same DSN for:

```bash
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?owned isolated PostgreSQL required}" && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -v -run '^TestRepositoryCheckoutDefaults(PhysicalPostgres|PostgresFailures)$' ./internal/task/repository/sqlite)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestStoreConformance$/^sqlite3$/^task$' ./internal/persistence/storeconformance)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?owned isolated PostgreSQL required}" && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestStoreConformance$/^pgx$/^task$' ./internal/persistence/storeconformance)
```

If new compile adapters require another owning package beyond the five above,
inventory actual changed packages and add its exact package check before marking
done. Do not claim coverage from a missing-method crash or tautological helper.
Run scoped changed-code lint once against the authoritative implementation base
(substitute its exact SHA), concurrency2/allowserial/CLI5m/GNU6m/kill10:

```bash
(cd apps/backend && : "${CHECKOUT_DEFAULTS_BASE_SHA:?authoritative exact base SHA required}" && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m golangci-lint run ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers ./internal/backendapp ./internal/orchestrator/executor --new-from-rev="$CHECKOUT_DEFAULTS_BASE_SHA" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Actual later backend PR corrective findings require one full CHANGED `./...`
lint against the exact live GitHub PR base, GOMAX2/GOMEM1GiB with the same lint
budgets. That conditional remediation command is not an extra DESIGN or initial
implementation audit. If normal hooks need absent JS dependencies, one pinned
pnpm9.15.9 frozen install from apps is allowed only after the implementation
grant; preserve managed dependencies and caches.

### Native Windows and hosted PostgreSQL

Fresh inventory at design found `test-windows`'s `native` suite with independent
workspace-persistence and workflow-selection steps at lines 928 and 939, then
Cursor slug and flag tests. Add a separate `Test Windows repository checkout
defaults` step after workflow start selection and before Cursor slug; recheck
location after sibling73 lands and do not edit its profile-enabled stage.
Use native Windows PowerShell, GOMAXPROCS2/GOMEM512MiB and step6m. Run this
targeted command, checking `$LASTEXITCODE`:

```powershell
go test -trimpath -tags fts5 -race -p=1 -count=1 -json -timeout=4m -run '^Test(RepositoryCheckoutDefaults(ConcurrentSQLite|Presence|CompanionAtomicity|OwnMutationProjection|Failures|Storage|LegacyAndExact)|RegisteredRepositoryCheckoutDefaults(HTTP|WS))$' ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
```

Require actual new-case RUN/PASS under native Windows, not cross-compilation.
Physical PostgreSQL groups must run in the existing PostgreSQL lane: design
inventory found the task SQLite package explicitly listed and no name filter
in its `go test -v -race -timeout 20m` invocation. Both new groups are therefore
selected already; no extra invocation is planned. Reconfirm this after base
changes and only adjust a proved selection gap. Preserve the package inventory
and require actual new-case RUN/PASS rather than merely a green lane. Record
actual native/PG run identities and new test cases from logs in Results.

### Cheap design/document gates

Only this block and prospective document-reference checks are authorized now.
All commands are lightweight and must join before returning.

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/workspaces docs/plans/preserve-repository-checkout-defaults
git status --short -- docs/specs/workspaces docs/plans/preserve-repository-checkout-defaults
```

Use existing `.github/scripts/pr-docs.cjs` `validateCoverage` with the actual
owning requirement/design and plan/work-order contents, the actual anticipated
service runtime path and this changed work order. Confirm covered/no errors,
all AC references exist, each requirement appears in the design frontmatter
and each work-order design appears in the manifest. This is prospective
reference validation, not production/test evidence. Check relative links,
untracked files' whitespace and size budgets. After the planned public edit,
run `node --test scripts/validate-public-docs.test.mjs` and
`node scripts/validate-public-docs.mjs`; no public edit or dependency install
is needed to validate this DESIGN package.

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/repository_entity.go`
- New focused `sqlite/repository_checkout_defaults.go` and test files above
- `apps/backend/internal/task/service/service_resources.go` and new test file
- New handler, backendapp and executor test files above; necessary test doubles
- `.github/workflows/backend-tests.yml` (separate targeted native/PG coverage)
- `apps/backend/AGENTS.md`, `docs/public/git-operations.md`
- Owning requirement/design and this package's statuses/results

## Dependencies

None in the code graph. Execution depends on a later ROOT reviewed-package
implementation interrupt and exclusive GLOBALheavy grant in this same primary.
No automatic continuation from the design handoff. Keep the current task title,
task/session/profile/executor identity and system marker in the task plan.

## Risks

Transaction/RETURNING completion, two dialects' scan behavior and exact error
mapping are the key implementation risks. Preserve existing secret ownership,
reference validation and redaction; tests use disposable references, never
actual credentials. Do not expand the guarantee to other repository fields,
client projection ordering or every complete writer. No new ADR or migration.

## Parallelism

`sequential`

## Inputs

- [Owning requirement, AC .20 through .23](../../specs/workspaces/requirements/worktree-base-refresh.md#repository-checkout-defaults).
- [Owning design](../../specs/workspaces/system-design/worktree-base-refresh.md#repository-checkout-settings-persistence).
- [Manifest and supplied evidence limits](plan.md#evidence-and-assumption-check).
- Scoped backend/GitHub guidance, TDD backend reference, existing patterns named
  above, and standing delivery/resource constraints in the own task plan.

## Results

Design ended before production/permanent tests or heavy commands. ROOT later
reviewed and released this package in the same primary. Implementation is in
progress; exclusive local-heavy ownership, normal hooks and hosted gates remain.

- Independent real-service SQLite RED failed the causal stored/returned pair;
  explicit-both and uncontested controls passed. The intent implementation made
  all ten causal orders GREEN. Presence, real atomic secret bindings/rollback,
  own-commit response/event projection, failure/admission and storage/full/exact/
  CAS controls passed. Authorization fixtures use canonical workspace ownership;
  storage assertions preserve the existing nil/empty binding representation.
- Physical PostgreSQL stale snapshots in both directions and seven initial wait
  cases passed. Initial enable fixtures kept true unchanged, so those passes
  were controls rather than false-to-true causal evidence. A ROOT-reviewed
  fixture correction seeds and asserts false before captured reads/transactions;
  only the two affected enable selectors were rerun and passed: both held
  directions plus physical holder224/writer225/observer226 observed wait. A mistaken launch following
  a rejected edit ran the unchanged fixture; its joined receipt is retained and
  excluded from corrected causal evidence. The original combined command was
  **not** a pass: the disable
  fixture wrote integer 1 instead of 0, and failure fixtures queried the holder
  PID after borrowing its only connection, causing the Go timeout. The original
  process was actually joined with its group freshly absent. ROOT authorized two
  affected selectors after bounded PID capture before transactions and the raw
  integer correction. Disable and every failure/control subcase then RUN/PASS;
  already passing cases were retained without replay. No production change or
  relaxed assertion/timeout followed these fixture failures.
- Registered REST and branch-only WS passed causal saved responses, actual
  service events, secret companion and supported admission/presence controls.
  Guarded compact MCP passed causal normalization, null/caller/target guards and
  existing reference-only secret results. Its corrected affected assertion keeps
  `secret_id` references and proves secret values absent. Actual exact Host match,
  replay, stale/disjoint fence and foreign-target controls passed.
- Real SQLite saves followed by production executor resolution and both launch
  projections passed for refresh enabled/disabled and explicit task-base priority.
  This is producer projection, not physical worktree creation.
- SQLguard and exact SQLite/physical PostgreSQL task-store conformance selectors
  passed, including real boolean/timestamp/conflict/transaction cases, no SKIP.
- Catalog, all 36 specification-linter checks, spec lint, actual-reference docs
  coverage, 19 harness checks, full harness lint and the active targeted harness
  hook passed. The public validator's 62 checks and all 47 pages passed. Separate
  native Windows step placement/YAML/limits were checked after fresh inventory.

Initial scoped lint ended exit 4 with a CLI timeout and three diagnostics.
The unused test assignment was removed and identical new fake methods moved
out of oversized baseline files into focused test files. ROOT authorized one
corrected-code same-package lint with an explicit 1GiB sole-lint exception;
it passed in 23.57s, actually joined with its group freshly absent. No further
resource retry or increased timeout is implied.

Original native handles, command logs and terminal/fresh-group receipts live in
`/tmp/kandev-child74-checkout-defaults-20261007`; the supplied ROOT candidate stays
read-only and untouched. Scoped lint is complete. The single pnpm9.15.9 frozen install succeeded using
935 cached packages; no lockfile change. The exact task-owned tmpfs PostgreSQL
fixture was removed by literal ID and fresh absence verified; private credential
files were removed. Normal-hook commit and ready PR #4308 publication completed.
The first native Windows run exposed two stale exact-version controls that
depended on rapid writes receiving different wall-clock values. Only those
ordinary/binding fixtures now store and read deterministic historical versions
before the real full/exact and disjoint settings writes. Matching writes, stale
conflict assertions and CAS checks remain; production timestamp behavior is
unchanged. Both affected controls passed the bounded race selector locally.
The original hosted observer was stopped by explicit ROOT instruction, actually
joined and its group freshly absent; its partial counts are not a verdict.
The sole corrective full changed-code backend lint used the authoritative PR
base, two-way concurrency, a 1GiB memory setting and five-minute CLI limit. It
emitted no diagnostics and reached the six-minute outer timeout (exit 124).
Its original process was actually joined and its group freshly absent. This
gate failed; no automatic retry, corrective commit or push followed. ROOT
explicitly authorized one recovery with the same limits and retained caches.
That full changed-code lint passed with zero issues in 237.83 seconds, actually
joined with its group freshly absent. The failed receipt remains retained.
Normal-hook corrective delivery follows. Native Windows and hosted PostgreSQL RUN/PASS, all required
checks, substantive current-head full review and ROOT serial merge admission
remain pending external gates. No delegation or new primary was used.
