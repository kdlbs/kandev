---
id: "01-persist-settings-fields"
title: "Persist workspace settings field intent"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-SETTINGS-UPDATES-001
acceptance_criteria:
  - AC-WORKSPACES-SETTINGS-UPDATES-001.1
  - AC-WORKSPACES-SETTINGS-UPDATES-001.2
  - AC-WORKSPACES-SETTINGS-UPDATES-001.3
  - AC-WORKSPACES-SETTINGS-UPDATES-001.4
  - AC-WORKSPACES-SETTINGS-UPDATES-001.5
  - AC-WORKSPACES-SETTINGS-UPDATES-001.6
  - AC-WORKSPACES-SETTINGS-UPDATES-001.7
  - AC-WORKSPACES-SETTINGS-UPDATES-001.8
system_design:
  - ../../specs/workspaces/system-design/workspace-settings-updates.md
---

# Task 01: Persist workspace settings field intent

## Summary

Deliver the workspace owner's bounded partial-settings persistence seam and
independent real-database regressions. Preserve nullable intent, exact CAS,
admission, complete-write callers, and each mutation's response/event
observation. Execute sequentially in the existing primary only after the later
ROOT reviewed implementation interrupt; authoring and heavy runs have separate
release gates recorded in [the plan](plan.md#phase-and-resource-gates).

## In scope

- Repeat the design's direct-caller, full/CAS-writer, reset, and mock audit at
  the released head. Verify method/route/SQL identities before editing.
- Independently author the regression files below using `/tdd` and its backend
  test reference. Protected ROOT candidate and receipts are read-only context;
  no replay, copy, import, mutation, or deletion.
- Add `WorkspaceFieldUpdate`, required `UpdateWorkspaceFields` on the repository
  interface, and the workspace-local bound allowlisted SQL implementation with
  returned-row scanning and optional exact predicate. Preserve full methods.
- Change the existing service to persist supplied intent and admitted actual
  unit movement, then publish/return the persisted observation.
- Update required mocks and wrappers. Exercise unchanged registered REST/WS
  and exact-plugin consumers, and add only the causal Windows native test step
  to `.github/workflows/backend-tests.yml`.
- Record command receipts, real test outcomes, lifecycle promotion, and all
  standing ROOT gates. No additional work order, native delegate, or session.

## Out of scope

No schema, API/WS shape, UI/store/copy, hierarchy, runtime or runner migration;
no new authentication or reach rules, visibility/task-number changes, revision
framework, global event/current-row promise, or universal writer serialization.
No broad browser suite or product build for this backend data repair. No initial
full backend lint, optional cleanup, or publication without its phase release.

## Acceptance

1. Meaningful independently authored service/SQLite tests expose both stale-read
   overlap failures before the fix and pass afterward, with presence/default
   controls and bounded cleanup; all criteria .1-.6 and .8 have targeted real
   persistence and admission evidence.
2. Registered REST and WS requests through real service/SQLite/bus prove optional
   presence, final disjoint union, own response/event observation, and existing
   error/placement support (.1, .2, .4, .6-.8). No fake writer or direct-handler
   invocation substitutes for these flows.
3. Exact scoped checks pass within released resource bounds. New PostgreSQL and
   applicable native cases actually RUN/PASS on the hosted exact head; no SKIP,
   package-only exit, build-only evidence, stale log, or candidate replay counts.
   All documentation references and four-artifact lifecycle records remain
   accurate. Completion and delivery remain subject to ROOT's separate gates.

## Independent test design

Create `service/service_workspace_field_updates_test.go` with these cases:

- `TestWorkspaceFieldUpdatesConcurrentSQLite`: open the existing template and
  a second independent SQLite handle on the same temporary file; verify distinct
  physical connections. Wrap the actual repository read to hold its completed
  snapshot exactly once. For both name-held and policy-held orders, start the
  held service, wait for read admission, complete the other service's write,
  release/join the held operation, and assert both successful calls plus saved
  name/enabled=true/timeout=45. Repeat a default-versus-other-settings overlap
  and ensure distinguishable preexisting sibling/default values survive. A
  sequential control and explicit-false control must run. Use bounded channels,
  context and owned goroutine cleanup, not timing sleeps or one shared mutex.
- `TestWorkspaceFieldUpdatesPresenceAndDefaults`: default false/120, all four
  ID omissions/clears/trimmed values, empty name/description, mixed fields,
  explicit false without timeout reset, same-field later successful value,
  empty request timestamp refresh, and workspace isolation. Verify stored SQL
  NULL directly where response normalization hides the representation.
- `TestWorkspaceFieldUpdatesExactFenceSQLite`: real matched exact save; stale
  precheck conflict; held initial read followed by an independent disjoint save
  then release to force the SQL fence conflict. Include matched/stale direct
  whole-row CAS controls. No success event or rejected field/timestamp changes.
- `TestWorkspaceFieldUpdatesAdmissionAndFailures`: use existing real org-unit
  service/access fixtures for manage denial, cross-org reach denial, allowed
  same-org actual unit move, scoped caller lacking unit.manage, wrong-org and
  missing destinations/placer, empty/same-unit controls. Hold an unrelated
  scalar read while another admitted service move commits: omitted UnitID must
  preserve the new placement. Reject zero/negative timeout, missing row,
  cancelled-before-write request, and injected real SQLite trigger abort of a
  multi-field request; assert no successful event or partial row/timestamp.

Create `repository/sqlite/workspace_field_updates_test.go`:

- `TestWorkspaceFieldUpdatesStorage`: nullable-presence scanner/binder checks,
  all allowlisted supplied fields, untouched raw NULL columns, empty/false
  values, fresh timestamp, missing ordinary/fenced errors, direct complete-write
  compatibility, and actual trigger-aborted assignment rollback. Do not mirror
  a query builder's internals; assert persisted rows and returned observations.

Create `handlers/workspace_field_updates_integration_test.go`:

- `TestRegisteredWorkspaceFieldUpdatesHTTP` and
  `TestRegisteredWorkspaceFieldUpdatesWS`: construct real services over two
  independent SQLite handles, call `RegisterWorkspaceRoutes`, submit HTTP via
  the registered engine and WS via `Dispatcher.Dispatch`. Count/gate the service
  snapshot rather than guessing access-read counts. Both overlap directions
  must complete with successful responses and persisted union. Capture real
  bus events with bounded delivery observation; each event matches its own DTO
  projection, accounting for RFC3339 event timestamps. The held writer's
  response includes the already committed other edit; the earlier response is
  not required to equal the final row.
- Exercise omitted versus JSON null, empty request, each blank default clear,
  false, empty text, malformed values, nonpositive timeout, unauthorized and
  persistence-abort outcomes. Verify no success event on rejected mutation.
  HTTP supplies admitted placement; WS does not gain a unit field, and unknown
  unit input follows its existing decoding behavior. Preserve legacy ignored
  Visibility rather than writing or validating it as a new field.

Create `repository/sqlite/workspace_field_updates_postgres_test.go`:

- `TestPostgresWorkspaceFieldUpdatesPhysicalConcurrency`: use the existing
  isolated PostgreSQL-schema harness, two native backend PIDs plus an observer,
  and a real transaction row lock. Start the competing new field update, prove
  its actual `pg_locks` wait, commit a distinct field from the lock holder, then
  join. Exercise both name/policy directions and observe combined returned and
  persisted values. Avoid service-local serialization as a concurrency proof.
- `TestPostgresWorkspaceFieldUpdatesCompatibility`: all nullable ID clears and
  untouched SQL NULL, bool/empty/default values, same-field and mixed controls,
  missing errors, exact success and conflict at the SQL mutation boundary after
  a real intervening writer, existing whole-row success/conflict, and a real
  CHECK-constraint-aborted mixed update preserving every value/timestamp.
  Use time values roundtripped by the actual PostgreSQL driver. Missing DSN may
  follow the harness's local skip, but does not satisfy hosted acceptance.

The existing
`TestPluginsWorkspaceAdminUsesValidatedVersionedDomainOperations` exercises the
actual exact consumer and must continue to pass. Extend only if it does not
cover a material changed behavior; no adapter redesign.

## Verification after release

These are **planned commands, not permission to execute now**. Run from repo
root with each cwd rooted independently. Obtain ROOT's one GLOBAL LOCAL HEAVY
release before any Go/lint/dependency command. Record fresh receipts and native
handles; execute and actually join each original process sequentially. A red
assertion is expected in TDD; resource/timeout/transport/unknown/out-of-scope
failures checkpoint ROOT immediately, with no automatic retry. Validate that
each named new test exists and runs; -run matching zero cases is not success.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -json -timeout=5m -run '^TestWorkspaceFieldUpdatesConcurrentSQLite$' ./internal/task/service)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -json -timeout=5m -run '^Test(WorkspaceFieldUpdates(ConcurrentSQLite|PresenceAndDefaults|ExactFenceSQLite|AdmissionAndFailures|Storage)|RegisteredWorkspaceFieldUpdates(HTTP|WS)|WorkspaceIdlePolicyDefaultsAndPartialUpdates|WorkspaceLifecycleEventsCarrySavedIdlePolicy|WorkspaceIdlePolicyMigrationDefaultsExistingRows)$' ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -json -timeout=5m -run '^TestPluginsWorkspaceAdminUsesValidatedVersionedDomainOperations$' ./internal/backendapp)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m golangci-lint run --concurrency 2 --allow-serial-runners --timeout 5m ./internal/task/models ./internal/task/repository ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/orchestrator/executor)
```

The first command is the fresh independently authored red run, then rerun that
case for green within the same approved TDD validation allocation. The second
command covers final targeted logic and unchanged idle-policy compatibility;
the third checks the exact native consumer. Do not repeat these passed final
commands without new edits/failures. If backendapp code/tests become changed,
add that actual changed package to scoped lint before the completion record.

PostgreSQL only after ROOT releases the environment/resource scope; do not
create a database/service/volume during design or assume local infrastructure.
The following guard makes absent configuration a non-proof failure rather
than a skip-only successful command. Never log the DSN.

```bash
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -json -timeout=5m -run '^TestPostgresWorkspaceFieldUpdates(PhysicalConcurrency|Compatibility)$' ./internal/task/repository/sqlite)
```

If local PostgreSQL is not released, record that absence without counting it
as PASS; require the actual hosted repository cases in the existing configured
PostgreSQL job. Add a narrow step under the existing Windows native matrix
condition (`matrix.suite == 'native'`), after the existing native SQLite
database-path checks, with `timeout-minutes: 30` for cold compilation, scoped
GOMAXPROCS=2/GOMEMLIMIT=512MiB, and this command. Windows uses the workflow/Go
timeouts; GNU timeout is the Linux local wrapper, not a Windows executable.

```bash
go test -trimpath -tags fts5 -race -p=1 -count=1 -json -timeout=5m -run '^Test(WorkspaceFieldUpdates(ConcurrentSQLite|PresenceAndDefaults|ExactFenceSQLite|AdmissionAndFailures|Storage)|RegisteredWorkspaceFieldUpdates(HTTP|WS))$' ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers
```

Require actual new-case RUN/PASS in the native log. Shared scalar SQL/scanning
and file-backed SQLite are applicable on native Windows; no new OS behavior or
browser build is involved. If native compilation/fixtures reveal a critical
unsupported assumption, checkpoint ROOT rather than dropping coverage or
replacing it with a build check.

Cheap documentation checks from the root are authorized in DESIGN:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/workspaces docs/plans/preserve-workspace-settings-updates
git status --short -- docs/specs/workspaces docs/plans/preserve-workspace-settings-updates
```

Also call `.github/scripts/pr-docs.cjs`'s `validateCoverage` with these four
artifact contents, the work order as a changed path, and the real anticipated
service path. This is a read-only prospective reference preflight, not a claim
that production changed. Verify every AC/REQ and design reference resolves,
each work-order REQ appears in its design, and its design is declared by plan.
No remote fetch or FETCH_HEAD mutation is needed.

Subsequent actual backend PR-fixup alone requires one full CHANGED `./...` lint
against the exact GitHub API base, GOMAXPROCS=2/GOMEMLIMIT=1GiB, concurrency2,
allowserial, CLI5m/GNU6m/kill10. Do not run it at initial implementation/design.
Set `WORKSPACE_SETTINGS_API_BASE_SHA` to the base SHA freshly read from the
actual PR API snapshot, verify the object exists locally, and retain that
snapshot in the receipt. Do not substitute a moving remote ref or fetch into
foreign refs/FETCH_HEAD. After an actual correction and ROOT resource release:

```bash
(cd apps/backend && test -n "${WORKSPACE_SETTINGS_API_BASE_SHA:-}" && git cat-file -e "${WORKSPACE_SETTINGS_API_BASE_SHA}^{commit}" && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run --concurrency 2 --allow-serial-runners --timeout 5m --new-from-rev "$WORKSPACE_SETTINGS_API_BASE_SHA" --whole-files ./...)
```

HOSTED, review, and MERGE remain separately controlled by the plan's
standing exact-head gates; these checks do not grant those actions.

## Files likely touched

- `apps/backend/internal/task/models/workspace_field_updates.go` (new).
- `apps/backend/internal/task/repository/interface.go`.
- `apps/backend/internal/task/repository/sqlite/workspace_field_updates.go`
  (new), `workspace.go` (minimal shared projection/scanner only).
- `apps/backend/internal/task/service/service_resources.go`.
- New service, repository SQLite/PostgreSQL, and registered-handler test files
  identified above.
- `apps/backend/internal/task/service/service_resources_test.go` (stub method).
- `apps/backend/internal/task/handlers/process_handlers_test.go` (mock method).
- `apps/backend/internal/orchestrator/executor/executor_mocks_test.go` (mock).
- `.github/workflows/backend-tests.yml` (narrow applicable native test step).
- These four requirement/design/plan/work-order artifacts (results/status).

Read-only compatibility inputs: `service_requests.go`,
`service_unit_placement.go`, `service_access*.go`, `service_events.go`,
`workspace_handlers.go`, exact plugin/settings-domain adapters, other workspace
SQL writers, frontend save builder/action, and existing full-write fixtures.
Do not modify them merely because they were audited. Any necessary extra
production change outside this boundary checkpoints ROOT before broadening.

## Dependencies and parallelism

No preceding work order. Implementation requires ROOT's later release;
heavy/hosted/merge require their separate releases. `sequential` in this same
primary; no native delegates or persistent platform workers.

## Risks

Normalization must preserve clearing intent; admission must not translate an
ignored or omitted unit into a stale write. Exact comparison must be SQL-bound
with no timestamp rounding, and returned-row scanning must not reuse stale
loaded values. Mocks require complete interface compatibility. Physical
concurrency fixtures need independent connections and owned terminal cleanup.

## Inputs

- [Requirement and criteria .1-.8](../../specs/workspaces/requirements/workspace-settings-updates.md).
- [Design and writer inventory](../../specs/workspaces/system-design/workspace-settings-updates.md).
- Existing `workflow_field_updates.go`, service overlap tests, registered
  workflow transport tests, PostgreSQL physical-lock tests, and backend test
  harness are patterns; workspace tests must be independently authored.
- Existing org-unit/access setup in `service_test.go` and
  `service_runner_switch_test.go`; native and PostgreSQL workflow contracts.
- `apps/backend/AGENTS.md`, `/tdd`, and backend test guidance at implementation.

## Results

ROOT sealed and reviewed the four-file design package at
2026-10-07T10:08:00.839621Z. All sealed hashes and the baseline were verified
before status edits. A later authoring-only release produced independent tests
against existing APIs. ROOT then released full implementation, exclusive
LOCAL-HEAVY and normal hooked commit/push/ready-PR after approved local checks.
HOSTED and MERGE remain unreleased.

Causal RED compiled and executed the existing service before production edits.
Original native session `57012`, terminal chunk `524b78`, exited 1 and was
actually joined at 2026-10-07T10:23:06.486795Z with fresh group members empty.
Name/policy and defaults/description each failed in both overlap directions;
sequential/explicit-false control passed. Receipt and complete log:
`/tmp/kandev-child70-local-c4e67ceb-20261007/red.{native.json,log}`.

The reviewed field seam, required repository method, minimal shared scanner,
service intent/admitted-unit mapping, required mocks and native test step are
implemented. Focused GREEN original native session `46076`, terminal chunk
`7e71aa`, exited 0 and was actually joined with fresh group members empty. Both
overlap families and their control passed. Exact argv/cwd/caps/UTC/cutoffs are
recorded in the sibling `green.native.json` and complete `green.log`.

Final targeted original `48819`, terminal `897fad`, exited 0 and was actually
joined with fresh group `2066912` empty at 2026-10-07T10:31:08Z. Ten top-level
tests ran/passed: seven new tests (ConcurrentSQLite, PresenceAndDefaults,
ExactFenceSQLite, AdmissionAndFailures, Storage, RegisteredHTTP, RegisteredWS)
and three existing idle-policy controls. All selected subtests passed with no
skips. Separate existing exact-plugin consumer original `51986`, terminal
`a88d0f`, ran/passed and exited 0, joined with fresh group `2073367` empty.

One isolated local PG16 fixture used the cached digest-pinned hosted harness
image, task/session labels, private localhost binding and only a tmpfs PGDATA
mount. Ownership receipt preceded readiness and assertions. Original PG run
`67445`, terminal `5093ec`, exited 0 and was joined with fresh group `2083090`
empty. Both overlap directions physically waited on holder PIDs 76/79 from
writer PIDs 77/80; the exact-fence case waited on PID 85 from writer 86. Both
new top-level PostgreSQL cases and every subtest actually ran/passed, no skips.

Backend-guide-required SQL guard (`39457`/`516816`) and store conformance
(`13664`/`e5394e`, SQLite and PG) exited 0 and were actually joined with fresh
empty groups. These required persistence checks supplement the approved scoped
commands; no initial full backend lint, browser suite or product build ran.

Owned container `fb24fd2e597022e85f7620d94f6b0ee109b15dc64e7ebe024d66601e5086cc6c`
was removed once. Cleanup original `cb1f11` exited 1 and joined because its
absence assertion expected uppercase `No such object`; Docker 29 emitted
lowercase. Fresh reconciliation `d6ad05` confirmed exact-ID inspect exit 1,
empty JSON list, and the lowercase no-such-object response. No cleanup replay,
replacement fixture or foreign-resource operation occurred. The ownership
receipt records the known assertion discrepancy and fresh physical absence.

Initial scoped lint original `57562`, terminal `090115`, exited 4: its log
reported `0 issues` followed by the CLI timeout error. This is a failed check,
not PASS. The original was actually joined with fresh group `2095865` empty;
argv/caps/UTC/fixed CLI5m/GNU6m/kill10 cutoffs and the full log are retained in
`lint.native.json` / `lint.log`. No duplicate or automatic resource retry ran.
The timeout is checkpointed to ROOT for explicit further release.

ROOT reviewed the original timeout receipt and explicitly authorized exactly
one recovery of the same six-package scoped lint with retained caches,
GOMAXPROCS=2/GOMEMLIMIT=1GiB, concurrency two, allow-serial-runners and unchanged
CLI5m/GNU6m/kill10 cutoffs. Recovery original `26123`, chunks `82f26e` / `7a05b5`,
actually exited 0 (`0 issues`), was joined, and fresh group `2119898` was empty.
No second recovery, scope expansion, cache deletion or test replay occurred.
The 1GiB exception applies only to this recorded recovery.

At preparation for normal publication, the work order remains in_progress
because hosted exact-head PostgreSQL and native RUN/PASS are still required.
All locally selected product tests, required persistence checks, and the
released lint recovery passed. The absent-commitlint guard authorized the one
pinned pnpm9.15.9 frozen install for active normal hooks. Commit/push/ready-PR
and canonical association follow the existing release after final cheap docs
checks and normal hooks. Actual publication and hook evidence are recorded in
the primary task plan and external command receipts, without a polish commit.
HOSTED and MERGE remain separately unreleased; no hosted/native-Windows or
complete-delivery success is claimed. Protected ROOT proof remains read-only
and was not replayed. All receipts and complete logs are under
`/tmp/kandev-child70-local-c4e67ceb-20261007/`.


### Hosted Windows compilation correction

At published head `e5f7d44b76c6253f2343d17639cbcae713e1ada6`, Backend
(windows, native), run `37609831297`, job `112754592876`, failed only added
step 5, `Test Windows workspace settings persistence`, at its six-minute step
limit. The complete raw job log contains dependency downloads followed by the
step timeout, before any test JSON. No new Windows test RUN/PASS or assertion
failure is claimed. This is an owned CI placement/resource failure.

ROOT released a minimal workflow-only correction: move this added step after
the existing native SQLite database-path checks and allow 30 minutes for cold
compilation. Its exact seven-case selector, trimpath/fts5/race/p1/count1/json,
GOMAXPROCS=2/GOMEMLIMIT=512MiB and Go assertion timeout `-timeout=5m` remain
unchanged, as do the existing steps and their order and the 90-minute job limit.
No product implementation, fixture or assertion changes are required. Passing
Go/PostgreSQL/SQL-guard/conformance/lint checks are not replayed for this
workflow/documentation-only correction. Cheap workflow contracts, YAML parsing,
reference coverage, documentation and whitespace checks plus normal applicable
commit hooks validate the correction.

The original hosted observer `37995` was deliberately interrupted only after
ROOT's correction release. Native terminal chunk `bb2f55` actually joined with
exit 143 and no all-terminal verdict; wrapper group `2166032` and child group
`2166033` were freshly empty. Hosted workflows were not manually cancelled.
Raw failure evidence and the interrupted observer receipts remain under the
same task-owned receipt directory. Historical completed old-head reviews stay
historical. Corrected-head HOSTED and MERGE require separate ROOT releases;
the work order stays in_progress pending actual new-head hosted RUN/PASS.


### Shared E2E collection dependency

ROOT proved that the published workspace-settings branch retained the exact
main1204 fixture blob `69892d488cdb021cb69304d906d3e3b1f65b5e68` for
`apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts`. Existing
actual E2E Build run `37601580900`, job `112727237749`, failed manifest
collection on these identical bytes: duplicate `const session` bindings at
lines 293/301 in the desktop disabled-continuation try block. Accepted raw
parser evidence is the causal RED; reproducing that failure would be redundant.

ROOT released only the second binding and its three immediate references to
`recoverySession`, with resulting blob exactly
`d9737fa2caaf3733390b822d81ebe5f4b0fca235`. Both navigations, waits, assertions
and traces remain intact. This repairs a shared validation dependency without
changing workspace-settings product code or continuation behavior. No sibling
worktree is read or changed. Validate exactly one affected pinned Playwright
Chromium `--list` collection (all 15 tests), fixture ESLint max-warnings zero,
affected documentation/actual 19-path coverage/whitespace and normal applicable
hooks. No browser, backend build/runtime, full E2E, broad typecheck, install or
passing Go/PostgreSQL/lint replay is authorized for this lexical correction.

Original corrected-head observer `67235` was deliberately interrupted after
this release and actually joined terminal chunk `ac00e8`, exit 143/no verdict;
wrapper group `2225126` and child group `2225130` were freshly empty. The one
full-review request `6036753119` remains historical to `da6dfe4` until actual
new-head coverage is inspected; it must not be repeated blindly. Hosted
workflows were not manually cancelled. The exact external walkthrough model
failure remains FAILED/verification not_run and was separately accepted by
ROOT as NONREQUIRED: it is excluded from the six required contexts. This sole
named exception permits no other failed-check bypass. All required contexts,
actual Backend/Frontend/E2E parent success, current full review and new native/
PostgreSQL RUN/PASS still gate delivery. New-head HOSTED and MERGE require
separate releases after normal publication and fresh physical return.


Dependency validation: initial command preparation `b775f9` used a nonexistent
pnpm `bin/` suffix and stopped before any check started; the existing pinned
9.15.9 executable was located without installation. Collection original
`36307` (`916ef4` / `9bd034`) actually exited 1 and joined: variadic
`--project chromium` consumed the following spec path as another project,
before discovery. This remains FAILED and is not causal product RED. ROOT
explicitly released one corrected invocation using the file first,
`--project=chromium`, GNU6m/kill10 and NODE_OPTIONS max-old-space-size4096.
Corrected original `63222` (`76c6b4` / `868433`) actually exited 0 and joined;
all 15 Chromium tests were listed in one file. This proves collection only,
not browser/runtime assertion execution. No third collection attempt occurred.

Fixture ESLint original `77481` (`4064bc` / `a6c39b`) exited 0 and joined.
Cheap original `56457` (`cde4bb` / `7772d7`) exited 0 and joined: exact approved
fixture blob, the other 17 original PR blobs unchanged, catalog/specification
validation, actual 19-path delivery coverage and whitespace passed. Original
wrapper and child groups were freshly empty before the corrected collection.
The accepted identical-byte hosted parser failure remains defect evidence;
no pre-fix parser replay, browser/backend/build/typecheck/installation or
passing product validation replay occurred. Normal applicable commit hooks
and publication/physical-return receipts follow in the primary task plan.

### Immutable-main conflict resolution (authoring checkpoint)

ROOT proved a single content conflict between published head
`88794cbd885cf4dedc2f337af91d1ea0efd7a03c` and immutable main
`330e02a47808c11ca315ae30456fcce7f4806db5` in the shared continuation fixture.
The current-head observer `38496` was deliberately stopped only through its
owned child group `2330056`, then actually joined at terminal chunk `418fa2`
with exit 143 and no all-terminal verdict. Wrapper group `2330054` and child
group `2330056` were freshly empty. Hosted workflows were not cancelled.

The normal `git merge --no-commit --no-ff` of that exact immutable main
returned the expected conflict, synchronous chunk `afdfdf`, exit 1. Its
original command and groups were joined and freshly absent. The fixture was
resolved to the exact incoming blob
`1140158f5d5920aac805c3432d7c788316318757`, which supersedes the prior lexical
binding repair with upstream setting/cleanup behavior. There are no remaining
unmerged index entries. HEAD remains `88794cbd`; MERGE_HEAD is `330e02a` and
the merge remains pending, without a commit or push.

The staged initiative diff against immutable main now has 18 paths: the
fixture matches main exactly and is no longer an initiative change. Static
inventory verified every index entry outside those 18 paths matches incoming
main. Before this work-order record, 17 prior owned blobs were byte-identical.
The repository interface whole-file blob changed because the automatic merge
retained incoming recovery-artifact imports and separate optional recovery
interfaces. Its WorkspaceRepository block is byte-identical to published
`88794cbd`; the only interface delta against incoming main is the existing
`UpdateWorkspaceFields` method. This is static compatibility evidence, not a
compilation or runtime result. Protected discovery proof hash/mode is unchanged.

The actual product-parent trigger gap remains separate from check success.
GitHub documents that conflicted pull requests do not trigger `pull_request`
workflows, while `pull_request_target` can run. This mechanism matches the
observed dirty/conflicted PR and target-only runs; it does not independently
prove the complete trigger cause. After a later exclusive local-heavy release,
perform only ROOT-approved validation and normal hooks/publication, then
inspect naturally triggered product workflows at the new frozen head. No
manual dispatch or retry is authorized to bypass the conflict.

This checkpoint authorizes source resolution and static inventory only.
No product checks, collection, installation, hooks, commit or push ran.
GLOBAL LOCAL-HEAVY remains owned by child69 and serial MERGE remains unreleased.
The work order stays in_progress; prior failed checks, interrupted observers,
review evidence and the exact nonrequired walkthrough exception stay historical.

ROOT later released exclusive local-heavy70 for the bounded merge validation
and normal publication. The one affected Chromium collection original `54077`
(`29e51c` / `d95fb1`) actually joined exit 0 and listed all 15 tests in one
file. Fixture ESLint original `80286` (`d4c1e9` / `a01414`) actually joined
exit 0. Both original wrapper/child groups were freshly absent before the
next check. Collection proves discovery only; no browser assertions ran.
ROOT independently accepted the automatic interface integration: among the
17 prior non-order/nonfixture owned files, only the interface whole-file blob
changes for incoming imports/optional interfaces; the other 16 remain exact
`88794cbd`. No owned WorkspaceRepository behavior or product logic changed.
No Go/PostgreSQL/backend lint replay is required solely for those upstream
bytes. Remaining bounded documentation/reference/18-path coverage/whitespace
checks, normal active merge hooks, commit and push are recorded separately;
new-head HOSTED and serial MERGE still require later ROOT releases.

### Current aggregate review: durable design provenance

CodeRabbit App `347564` completed substantive full review `5442207743` at
head `ca09e52749d084375e87b1fbb592d342535ccaad`, covering all 18 initiative
files. Aggregate comment `6036333535` has matching source/covered head and
`kind: reviewed`. Its concrete request to remove machine-local evidence from
the living design is valid traceability hygiene. ROOT reclassified the
initial deferred nitpick disposition and released this two-file docs-only
correction. The design now retains the durable two-service stale-snapshot
cause in both orderings. Source/test references and product contracts remain
unchanged. Preserve the exact qualification provenance here:

The ROOT qualification at baseline
`1204f0e5488418d0d9aa9aaaf6a31988ccdfcd8f` proves this service/SQLite cause
with two services and independent handles in both held-read directions. Its
receipt, native result, qualification, and log are under
`/tmp/kandev-root-workspace-settings-discovery-20261007/`. The protected candidate
is `/tmp/kandev-root-workspace-settings-overlap-candidate_test.go`, mode `0400`,
SHA256 `3305080be7e81392cf56003631ff9de92a66322497b32f09b0bc9a10640dc26b`.
Native session `40825`, chunks `8c5a88` / `8f83cd`, was actually joined at exit
1; qualification records empty groups `1854400` / `1854404`. Sequential and
explicit-false controls had no failures. This is not executed REST, WebSocket,
PostgreSQL, or browser proof. Keep these artifacts read-only and independently
author permanent tests after release; never replay or import the candidate.

Current observer `35199` was stopped only through its verified owned child
group `2552235`, then actually joined at terminal chunk `d32036`, exit 143
and no all-terminal verdict. Wrapper `2552234` and child group `2552235` were
freshly empty. No hosted workflow was cancelled. Current-head native,
PostgreSQL and product-parent successes were not inferred from progress.
The prior completed semantic review and aggregate finding remain historical
after the docs correction; the finding is addressed by this bounded change.
There is no inline finding thread to fabricate or resolve. The separate
docstring coverage warning is optional/nonrequired and receives a grounded
no-change disposition; no comment sweep or product validation replay is
authorized. Affected documentation/catalog/reference coverage/whitespace,
normal active docs-only hooks and publication results are recorded in the
primary task plan. New-head HOSTED and serial MERGE require later releases.
