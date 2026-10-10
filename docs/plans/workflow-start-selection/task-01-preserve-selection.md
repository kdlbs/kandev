---
id: "01-preserve-selection"
title: "Preserve selection through ordinary step edits"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-START-SELECTION-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-START-SELECTION-001.1
  - AC-TASKS-WORKFLOW-START-SELECTION-001.2
  - AC-TASKS-WORKFLOW-START-SELECTION-001.3
  - AC-TASKS-WORKFLOW-START-SELECTION-001.4
  - AC-TASKS-WORKFLOW-START-SELECTION-001.5
  - AC-TASKS-WORKFLOW-START-SELECTION-001.6
system_design:
  - ../../specs/tasks/system-design/workflow-start-selection.md
---

# Task 01: Preserve selection through ordinary step edits

## Summary

Preserve ordinary caller omission of `is_start_step` through the existing
transaction and refresh the successful response/event flag from that Tx.
Retain explicit/full-model and exact Host behavior, with focused permanent
regressions that exercise production callers and actual SQL.

## In scope

- Own the controller, narrow service/repository entry points, private SQL helper,
  and necessary focused test files listed below.
- Audit the concrete dependencies and unchanged public adapters before coding.
- Cover both stale observed flags, explicit/latest selection, demotion results,
  publisher payload/counts, actual resolver selected/fallback IDs, rollback,
  and access controls. Preserve task placement through unchanged source wiring.
- Add small independent-store SQLite and env-gated PostgreSQL regressions for
  the changed method, and anchored native CI execution after ROOT coordination.
- Add a concise backend `AGENTS.md` note for the new narrow ordinary-update seam
  during implementation, so the existing full-model convention remains clear.

## Out of scope

Other fields' patch semantics, migration, broad unchanged test replay, global
writer/event ordering, new routes, UI/E2E, install in DESIGN, foreign worktrees,
delegates, new tasks/sessions, protected-proof reuse, commits/PR in DESIGN.
Provider, gateway, and task-service production edits and new gateway hub or
task-creation fixtures are excluded.

## Acceptance

1. Fresh real-controller omission regressions fail before production changes
   on wrong saved/returned selection, then pass through the presence seam.
2. Registered caller responses and events reflect the transaction result;
   explicit promotion/clear and all required failure cases retain atomicity.
3. Changed SQL executes on actual SQLite, configured PostgreSQL, and native CI;
   exact/full-model compatibility passes, with every original handle joined.

## Sequential implementation

1. After a later explicit ROOT reviewed-package implementation request in this
   primary, mark this work order `in_progress`. Compare owned blobs once if the
   source base differs; do not rebase for drift. Obtain ROOT's separate GLOBAL
   local-heavy grant before checks/install/hooks. No delegation or model switch.
2. Read the design's caller inventory, exact Host path, and existing helpers.
   Confirm `service.Service` and `Controller` use concrete dependencies;
   `adapters.WorkflowRepo` and `settingsWorkflowController` do not need signature
   changes. Inventory any new references on the implementation base. Keep
   workflow provider, transport registries, bootstrap, and exact Host adapters
   compiling without optional fallback paths.
3. Author fresh `controller/start_selection_test.go` regressions first. Use
   existing provider lookup as a deterministic post-snapshot seam, a real SQL
   workflow lookup, two actual Services, and actual repository rows. Supply
   only `{ID, Name}` through `Controller.UpdateStep`. Do not call a nonexistent
   new method in RED, manually make a stale full model as the only regression,
   copy the protected candidate, or alter permanent code to manufacture RED.
   Run only `TestWorkflowStartSelectionControllerOmission`; preserve both
   initial-A->B and initial-B->A failures on flags, response, demotion list,
   and real resolver. The latter must inspect flags even if positional fallback
   accidentally returns the same ID.
4. Implement the design's `*bool` intent seam and narrow private helper change.
   Nil never demotes and preserves the current SQL column. Explicit wrappers
   stay explicit, exact predicates stay exact. Capture persisted flag inside
   the Tx and install it on the response model only after commit. Extract small
   helpers if lint limits require it; no universal patch model or writer rewrite.
5. Complete the cases below, then run only the anchored checks. Coordinate any
   `.github/workflows/backend-tests.yml` addition through ROOT with Child71;
   preserve both native regressions without inspecting its worktree. Update
   result evidence and statuses only from actual terminal outcomes.

## Test cases and ownership

These names now correspond to permanent tests; terminal execution evidence is
recorded in Results. Hosted native and PG execution remain separate gates.

| New file / test | Cases and evidence | AC suffix |
| --- | --- | --- |
| `workflow/controller/start_selection_test.go`: `TestWorkflowStartSelectionControllerOmission` | Both observed directions using real name-only caller and second-service explicit write; saved flags, own returned flag, empty demotions, real resolver | `.1`, `.3`, `.5` |
| Same: `TestWorkflowStartSelectionControllerControls` | Nonoverlap name-only on selected/unselected steps; explicit true/false, false on unselected leaves other selected, supplied value equal to snapshot remains explicit, latest explicit write wins its operation; real `ResolveStartStep` asserts selected ID and, after explicit clear, actual first-positional fallback ID | `.1`, `.2`, `.5` |
| `workflow/repository/start_selection_test.go`: `TestWorkflowStartSelectionSQLiteIndependentStores` | File-backed DB through `db.OpenSQLite` and separate `OpenSQLiteReader`, two distinct repository stores. The controller fixture separately uses two real Services. Real competing promotion commits after caller snapshot and before omitted update. Also hold native writer Tx, start omission, prove wait at admission via established observable barrier, commit selection then join; assert both directions and fresh rows | `.1`, `.3` |
| Same: `TestWorkflowStartSelectionRepositoryRollback` | Trigger rejects target name UPDATE after true promotion has demoted previous start; saved target/previous rows and timestamps unchanged, no successful result. Missing/deleted target cannot leave demotions. An AFTER UPDATE trigger makes the flag unreadable; integer capture fails and the transaction restores all rows without a production failpoint | `.4` |
| Same: `TestWorkflowStartSelectionLegacyAndExact` | Direct `UpdateStep`/`UpdateStepWithDemotedStartSteps` full-model false/true remain explicit; exact current success, stale workflow and stale step reject; rejected exact promotion restores previous start and timestamps | `.2`, `.4`, `.6` |
| `workflow/handlers/start_selection_test.go`: `TestWorkflowStartSelectionRegisteredREST` | Production `RegisterRoutes` + `httptest` PUT with JSON omitting flag, both causal directions through provider seam. Assert HTTP body, saved rows, exactly one edited-step event with committed flag and no demotion; explicit promotion emits actual demotion + target, explicit clear emits only target; failed write emits none | `.1`-`.4` |
| Same: `TestWorkflowStartSelectionAccessFailures` | Existing scoped identities/checkers: foreign and missing share classification, sync/workspace read-only rejection, invalid supplied data, no SQL mutation and no events | `.4` |
| `mcp/handlers/config_workflow_start_selection_test.go`: `TestWorkflowStartSelectionRegisteredMCP` | Production `Handlers.RegisterHandlers` and guarded config-mode dispatcher, `ws.ActionMCPUpdateWorkflowStep`; real omitted JSON field and real SQL/provider fixture. Both directions, response envelope, event flags/counts, explicit controls and denied/missing/failing-write cases. Pin is_start_step:null as omission; retain existing REST complete_task_on_enter:null rejection | `.1`-`.4` |
| `workflow/repository/start_selection_postgres_test.go`: `TestWorkflowStartSelectionPostgresIntent` | Existing `setupPostgresDecisionTestRepo`/`testutil.PostgresDSNFromEnv`, isolated schema, actual omission both directions, explicit controls, missing and rollback; validate rebound CASE/integer SQL | `.1`, `.2`, `.4`, `.6` |
| Same: `TestWorkflowStartSelectionPostgresWaitsForCurrentFlag` | Real multiple connections, blocker Tx owns target row via `FOR UPDATE`; start omitted update with stale model, prove wait using `pg_blocking_pids`/`pg_locks`, change selection within blocker and commit. Join omission and assert latest persisted flag/returned result in both directions. Bound context, always release/join on failure; no new production lock required | `.1`, `.3` |

Use new small test files instead of expanding oversized legacy files. Reuse
existing fixture constructors where their scope fits; never mutate the protected
ROOT candidate or its archive. Independent stores and PG are new evidence beyond
the accepted proof. Use real factory handles for native locking, never shared
in-memory connections as independent-store evidence. Keep assertions outside
worker goroutines; return errors/results through owned channels and join them.

REST and guarded WS/MCP tests use the actual `stepevents.Publisher` and assert
exact event payloads and counts. The design's consumer audit records unchanged
gateway mapping and ordinary task-creation routing into `ResolveStartStep`.
Together with actual selected/fallback resolver IDs, these support `.3` and `.5`
at the changed boundary. No new gateway or task-create execution is planned or
claimed, and this work order does not alter those consumers.

Existing narrow reference tests:
`repository/sqlite_test.go:TestExactWorkflowStepWritesFenceStoredVersions` and
`backendapp/plugins_workspace_admin_test.go:TestPluginsWorkspaceAdminUsesValidatedVersionedDomainOperations`.
Run those exact controls once because the shared helper is changed. Do not add
broad legacy suite replays or unrelated test-value audits.

## Verification

Not authorized during DESIGN. Run from repo root sequentially after ROOT's
later implementation request and heavy grant. Retain the original command
handle/PID, log, terminal result, and owned group-gone observation for every
execution. Use bash with login disabled. No automatic retry on resource,
timeout, transport, unknown, or extra-scope failure; durably checkpoint ROOT.
Routine causal-fixture/style corrections rerun only the affected check.

RED first (expected causal failure before production edits):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/workflow/controller -run '^TestWorkflowStartSelectionControllerOmission$' -count=1 -timeout=4m -v)
```

GREEN and focused compatibility, each original executed once after final code:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/workflow/controller ./internal/workflow/repository ./internal/workflow/handlers ./internal/mcp/handlers -run '^TestWorkflowStartSelection(ControllerOmission|ControllerControls|SQLiteIndependentStores|RepositoryRollback|LegacyAndExact|RegisteredREST|AccessFailures|RegisteredMCP)$' -count=1 -timeout=4m -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/workflow/repository ./internal/backendapp -run '^Test(ExactWorkflowStepWritesFenceStoredVersions|PluginsWorkspaceAdminUsesValidatedVersionedDomainOperations)$' -count=1 -timeout=4m -v)
```

With a ROOT-approved disposable PostgreSQL DSN already configured, require the
variable to be present; then execute and require actual RUN/PASS, no SKIP:

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Configure the approved disposable PostgreSQL DSN}"
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/workflow/repository -run '^TestWorkflowStartSelectionPostgres(Intent|WaitsForCurrentFlag)$' -count=1 -timeout=4m -v)
```

Required persistence gates are not broad product-suite replays:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/persistence/storeconformance -count=1 -timeout=4m)
```

Scoped lint, after resolving the actual PR base SHA from the API into
`workflow_pr_base` (not a stale remote ref). If no PR exists, use the approved
implementation baseline and record that provenance. If backend PR finding
remediation occurs later, substitute full changed `./...` lint once with
GOMEMLIMIT=1GiB, the exact API PR base, and the same bounds.

```bash
: "${workflow_pr_base:?Set the observed PR base or approved implementation baseline}"
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m golangci-lint run ./internal/workflow/controller/... ./internal/workflow/service/... ./internal/workflow/repository/... ./internal/workflow/handlers/... ./internal/mcp/handlers/... --new-from-rev="$workflow_pr_base" --concurrency=2 --allow-serial-runners --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/workflow-start-selection
```

For native hosted Windows execution, add one anchored workflow regression step
under `test-windows` matrix `suite == 'native'`, using the new controller,
repository, REST and guarded MCP test names above:

```bash
go test -trimpath -tags fts5 -race -p=1 -count=1 -json -timeout=4m -run '^TestWorkflowStartSelection(ControllerOmission|ControllerControls|SQLiteIndependentStores|RepositoryRollback|LegacyAndExact|RegisteredREST|AccessFailures|RegisteredMCP)$' ./internal/workflow/controller ./internal/workflow/repository ./internal/workflow/handlers ./internal/mcp/handlers
```

Do not replace Child71's step. Observe actual native RUN/PASS at the pushed head;
cross-compilation, no-tests-to-run and SKIP do not prove this boundary. Existing
Postgres jobs include `internal/workflow/repository`; verify the new PG cases
actually run at that head in the configured jobs. Update native commands to the
actual implemented names before execution, with no wildcard that silently misses
tests. Changes to these commands need truthful package/result reconciliation.

## Files likely touched

- `apps/backend/internal/workflow/controller/controller.go`
- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/workflow/repository/sqlite.go` and small helper file if required
- New test files in the test matrix above
- `apps/backend/AGENTS.md`
- `.github/workflows/backend-tests.yml` (ROOT-coordinated native step only)
- This requirement/design/plan/work order for status and actual results

## Dependencies

None between work orders. Execution depends on ROOT's later reviewed-package
interrupt, GLOBAL local-heavy grant, configured disposable PG, and native CI
coordination. DESIGN does not confer any of those authorities or MERGE authority.

## Risks

Preserve omission independently from observed value; do not restore the bug by
calling the full-model method. Transaction flag capture must settle before
projection. PG lock probes need bounded contexts and original goroutine joins.
Existing exact/full-model semantics must survive shared-helper extraction.

## Parallelism

`sequential`. One existing primary session; no delegates or extra platform tasks.

## Inputs

- [Requirement](../../specs/tasks/requirements/workflow-start-selection.md), all six ACs.
- [Design](../../specs/tasks/system-design/workflow-start-selection.md), including caller/adapters inventory and accepted proof limitations.
- `workflow/controller/reorder_test.go`, `workflow/handlers/step_events_test.go`,
  `mcp/handlers/config_workflow_handlers_test.go`,
  `workflow/repository/reorder_postgres_test.go`.
- `.agents/skills/tdd/references/backend-tests.md` and scoped backend guidance.
- Live Kandev task plan for identity, barriers, resource and delivery gates.

## Results

Implementation is in progress after ROOT accepted the revised package and
released the same primary. The design turn had no production/test/Go/install/DB
fixture/commit/publication work; the following results are later execution.

- Independently authored real caller RED: both omission directions failed on
  stored/returned flags, demotions and resolver before production changes.
- Initial controller GREEN passed. After actual workflow metadata scans were
  corrected to QueryRow.Scan, both omission and controller control tests passed.
- Expanded GREEN first found fixture errors: SQLX could not map workspace_id
  into the task model; current exact controls incorrectly picked a positional
  default row and initially used a SQL-generated timestamp; the REST null
  control targeted a flag that both transports decode as omission. Fixtures and
  the design's overbroad null wording were corrected without product changes.
- Final affected repository/REST/MCP controls passed. Null-as-omission positive
  cases retain current selection and emit one target update; existing REST null
  rejection is pinned on complete_task_on_enter. Access/read-only/invalid and
  target-write failure emit no events and leave stored rows/timestamps unchanged.
- Independent SQLite-store tests passed in both directions with an observed
  native BEGIN wait. Target-write, integer flag-capture and missing-target
  rollback passed; no production failpoint was added.
- PostgreSQL 16 cases actually ran and passed, with no SKIP: both intent
  directions, rollback, full/exact controls, and both physical row-lock waits
  observed through pg_locks/pg_blocking_pids before holder commit.
- Existing exact repository and actual Host adapter reference tests passed.
- sqlguard passed. Store conformance passed on SQLite and PostgreSQL, including
  the existing upgrade fixtures; no cases skipped. Scoped lint passed with zero
  issues at the approved implementation baseline, concurrency 2 and serial
  runners enabled, within its original bounded execution.
- Each completed original native handle was joined and its owned process group
  freshly observed gone before the next heavy. Private receipts/logs are under
  /tmp/kandev-child72-implementation-20261007. The owned PG fixture uses loopback
  ephemeral port and private tmpfs. After both original PG commands joined and
  their groups were freshly gone, the exact container was removed, absence was
  freshly verified, and both private credential files were deleted.
- Harness checks passed (19 harness validator tests, 203 files), catalog passed
  (360 decisions/1431 specs), and spec checks passed (36 validator tests/full
  lint). These are cheap documentation checks, not native CI evidence.
- The one conditional frozen apps install completed with pnpm 9.15.9 on
  Node 24.21.0; all 935 packages reused the existing cache, with no lockfile
  change. Normal hooks are active; the targeted harness hook passed.
- The initial normal commit, push and ready PR publication completed with all
  original handles joined and owned groups freshly gone. Exact head, canonical
  automation, observer and review receipts live in the external task plan.
- A PR finding identified stale local PostgreSQL wording in the manifest. The
  summary now distinguishes completed local checks from pending hosted native
  Windows and PostgreSQL CI. This correction changes documentation only; its
  affected validation and follow-up publication remain externally pending.
  Full current-head review disposition and separate ROOT merge authority remain
  pending without anticipating hosted results.

### Design checkpoint

- `python3 scripts/list-docs.py validate`: PASS, 360 decisions and 1431
  specifications validated; owning-system catalog discovers both new specs.
- `python3 scripts/lint-spec-files.py --all`: PASS.
- Repository `validateCoverage` preflight: actual four-file documentation diff
  is exempt; modeled planned controller-source trigger is covered, with the
  actual work order's requirement/design references accepted and zero errors.
  This is reference validation only, not a product check or edited source claim.
- `git diff --check`: PASS. Separate direct whitespace/final-newline check over
  all four untracked package files: PASS, since git diff omits untracked files.
- Initial bare `node` lookup failed because Node is absent from the login-shell
  PATH. The reference check used the existing explicit Node 24.21.0 binary with
  bash/login disabled; no installation or package command ran.
- All cheap commands completed synchronously with terminal handles. Working
  tree has only this four-file package, unstaged/uncommitted. No heavy authority
  was requested, acquired, or consumed.

### ROOT preliminary-review revision

Removed the planned gateway hub and task-creation fixtures and their GREEN,
scoped-lint, and native package/test entries. Retained actual publisher
payload/count assertions through registered REST and guarded WS/MCP, actual
selected/fallback resolver ID controls, unchanged-consumer source audit,
independent SQLite/PG cases, rollback/exact controls, sqlguard and storeconformance.
No new gateway/task-create execution is planned or claimed. Requirement `.5`
and the existing routing contract remain unchanged. ROOT coordinates the separate
native workflow hunk with Child71 after handoff.

Revised reference preflight: covered with zero errors. Direct four-file
whitespace, removed-test/package checks, mandatory-persistence-command checks,
and `git diff --check`: PASS. Initial catalog/spec-lint results above are
retained historical results; unchanged broad cheap checks were not replayed.
All revision commands completed synchronously; no live original handle,
production/permanent-test edit, or heavy/MERGE grant exists.
