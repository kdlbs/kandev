---
id: "01-preserve-profile-scripts"
title: "Preserve ordinary profile script intent end to end"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
acceptance_criteria:
  - AC-EXECUTORS-PROFILE-EDITOR-001.5
  - AC-EXECUTORS-PROFILE-EDITOR-001.8
  - AC-EXECUTORS-PROFILE-EDITOR-001.9
  - AC-EXECUTORS-PROFILE-EDITOR-001.10
  - AC-EXECUTORS-PROFILE-EDITOR-001.11
  - AC-EXECUTORS-PROFILE-EDITOR-001.12
system_design:
  - ../../specs/executors/system-design/profile-editor.md
---

# Task 01: Preserve ordinary profile script intent end to end

## Summary

Carry the two script-presence intents from ordinary built-in profile saves into
an atomic store update and acknowledge that save's committed script pair and
timestamp. Prove the bounded contract through real service/store, registered
transport, launch projection, and native database/platform boundaries.

## Release and execution authority

ROOT explicitly released implementation after the completed DESIGN handoff;
child75 now holds the exclusive global local-heavy grant. The original gate was:
do not start production or permanent test edits until
ROOT sends a later explicit implementation INTERRUPT to task
`4bdb9b77-f970-4631-a4de-4ed4776f42a5`, SAME primary session
`58b5b0b8-dba3-47cc-a898-2f6f91d23963` and current profile/executor.
No agents, sessions, recursive tasks, model switch, or operator approval question.
Read the live task plan and this manifest before execution; preserve its system
marker, user edits and authority/resource barriers using current versions.

After release mark this sole work order `in_progress`. Use `/tdd`, record
meaningful permanent RED before production edits, then minimum GREEN. Mark it
`done` and the manifest `implemented` only when every required check has actual
evidence. Reconcile paired draft lifecycle with the existing implemented editor
package before promotion; do not reinterpret its UI scope or results.

## In scope

- New narrow script intent model, required repository seam, focused atomic
  storage method, ordinary built-in service selection and committed result use.
- Permanent independent SQLite causal regressions and required controls.
- Actual registered REST/WS and settings-domain update, event acknowledgement,
  and saved-row `Executor.applyProfile` projection.
- Real physical PG row-wait behavior, mutation-scoped conformance and Windows CI.
- Only causally required public/convention clarification and ordinary delivery
  after checks, subject to the manifest's resource/observer/merge gates.
- ROOT-authorized hosted prerequisite: test-only deterministic older persisted
  timestamps in the existing workspace presence/storage fixtures that failed on
  Windows. Reread the persisted row before the mutation; retain timestamp-order,
  matched/stale CAS, stored equality and field assertions. No production clock or
  workspace behavior changes. This belongs to this sole work order.

## Out of scope

Everything excluded by the manifest: particularly other field omissions, exact
CAS/legacy/plugin redesign, runtime execution, running resources, frontend/E2E,
credential semantics, timestamp changes, migrations, generic writer frameworks,
unrelated mocks/tests beyond the authorized hosted fixture prerequisite, and sibling worktrees.

## Acceptance

1. All four disjoint ordinary-save interleavings preserve stored and returned
   scripts, with explicit clear/both-present/same-script controls and own-commit
   pair/timestamp in responses/events. Exact, legacy, plugin, authorization,
   config/env and unrelated-field semantics retain their existing behavior.
2. Registered REST/WS and settings-domain partial saves with a real DB and the
   actual saved-row launch projection prove the user effect. Query/result/commit
   failure, precommit cancellation, rollback and missing rows emit no success;
   actual PG statement row waits and native Windows named functional RUN/PASS
   establish supported portability without SKIP or compile-only evidence.
3. Task-defined checks and document/reference gates pass with retained original
   results, all handles joined and groups gone. Authorized ready-PR delivery
   satisfies the manifest's six-context, reviewer coverage, automation and
   local-heavy return gates; merge still needs a separate ROOT grant.

## Implementation sequence

1. Preserve the accepted private proof unchanged. Author NEW permanent regression
   tests rather than copying/replaying it. Use two independent actual SQLite
   stores and services, pinned distinct physical connections, shared file-backed
   DB, and channel gates only around the captured `GetExecutorProfile` snapshot.
   Use production factory/fixture conventions. Join test goroutines via cleanup
   on fatal paths and close stores only after workers settle.
2. `TestExecutorProfileScriptsIndependentSQLite` covers held name-only versus
   acknowledged prepare, held name-only versus cleanup, prepare versus cleanup,
   cleanup versus prepare. `TestExecutorProfileScriptsPresence` covers serial
   omissions, current null decode, explicit clear for each script, both supplied,
   and explicit same-script last commit. Run the anchored RED command below;
   record four expected failures and passing controls, not an arbitrary error.
3. Add `ExecutorProfileScriptIntent` in a focused models file and the required
   method described by the design. Keep legacy full/exact methods and plugin
   path untouched. Use the current pre-write profile for other fields. New store
   method writes supplied scripts only; capture its pair/time, drain/close rows,
   commit and then update the model. No post-write reread or full-write fallback.
4. Add `TestExecutorProfileScriptsOwnCommit`: gate delivery of an actual committed
   store result, allow another independent real save to finish, then confirm the
   first response/event still reports its own pair/timestamp. Capture events by
   actual publisher payload and operation, not arbitrary sleep. This result gate
   is separate from the captured-read-only causal regression. Cover each script
   and both pair members; do not assert global ordering/coherence of other fields.
5. Add `Storage`, `Failures` and `Compatibility` tests. Cover reachable query,
   scan/iteration/close and commit failures using existing focused SQL/driver
   seams only when needed. Existing typed JSON inputs cannot cause marshal
   failures: handle returned errors normally, without production marshaler hooks
   or invented impossible fixtures. Prefer behavioral evidence over exhaustive
   branch mirrors. Real rollback/constraint,
   cancelled admission and missing-row cases assert storage and absent success
   events. Commit uncertainty yields error/no success, never an invented rollback
   promise. Check connection reuse after settled failures. Seed fixed saved
   `updated_at` instants for matching/stale CAS, including a timestamp-only
   intervening commit. Never depend on rapid clock ticks or alter production time.
6. Use existing controls plus focused real-fixture tests for full legacy script
   replacement, exact guarded updates, plugin restriction/no seam call, K8s and
   remote-Docker member denial, invalid K8s config, Sprites token preserve/remove,
   and global env-reference rejection. Other fields continue existing semantics;
   this is no inventory-wide writer program. Required explicit aggregate doubles
   receive adapters in NEW focused files, not appended oversized baselines.
   Supported active fixtures retain faithful behavior or delegation; only
   unexercised unsupported doubles fail closed.
7. In NEW handler/backendapp test files exercise `RegisterExecutorProfileRoutes`
   via actual Gin PATCH `/api/v1/executors/:id/profiles/:profileId` and dispatcher
   `ws.ActionExecutorProfileUpdate`. Exercise registered settings dispatch into
   `executor_profile` in `settings_domain_operations.go`. Use real service/DB,
   independent competing script writes, decoded omission and explicit empty,
   stored/response/event assertions, rejected admission and missing row. Only
   identity/message transport may be substituted; do not call private handlers
   directly as registered-route proof. Config-mode MCP retains its existing
   exact-version tests and operator-key preservation.
8. `TestExecutorProfileScriptsApplyProfile` uses a real stored row saved through
   the corrected service and invokes actual `applyProfile`, asserting setup,
   cleanup and nonempty cleanup metadata. Do not simulate provision or promise
   remote cleanup execution. Keep the source projection unchanged.
9. Add mutation-scoped SQLite/task-store conformance in a NEW focused conformance
   test file. No generic full-store audit. Add NEW PG behavior tests with real
   independently pinned holder/writer/observer physical connections and distinct
   backend PIDs. Observe `pg_blocking_pids` and an ungranted transaction lock
   while the actual UPDATE waits, then commit the holder. Assert pair/timestamp
   preservation and explicit last commit, clear, exact timestamp fence after
   row wait, legacy writes, rollback/cancellation/missing row and connection reuse.
   Avoid synthetic timing/sleeps as row-wait proof. Reuse isolated PG fixture
   helpers; an absent DSN may skip ordinary local runs but required PG evidence
   must show named RUN/PASS and no SKIP.
10. Add **Test Windows executor profile scripts**, native matrix only, six-minute
    job step, PowerShell explicit exit-code propagation, GOMAXPROCS 2/GOMEMLIMIT
    512MiB, anchored functional selector matching the non-PG command below,
    `-trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -v`. Save actual hosted
    test names RUN/PASS. Preserve **Test Windows profile enabled omission** and
    any incoming **Test Windows executor profile enabled** or sibling checkout
    stage without renaming/dropping them. Existing fixed PostgreSQL package
    allowlist includes the store package; retain it and prove new named cases run.
11. Add the brief partial-save public clarification specified by the manifest
    and, if the required seam makes conventions incomplete, one concise scoped
    backend AGENTS note. Run cheap document checks and exact task checks. Record
    actual commands/results, sync artifacts, and follow already authorized normal
    hooks/PR delivery only after checks and resource gates pass.

## Verification

All commands run from repo root using independently rooted subshells. Each heavy
original runs alone after ROOT release and the global local-heavy grant. Retain
each native session/functions-cell ID, actually join it once, record its terminal
exit/log and verify fresh group gone. No duplicate run to replace a lost result.
Timeout/resource/transport/unknown/out-of-scope failures require saved diagnosis
and ROOT checkpoint before recovery. Nonzero exit with empty log or zero lint
issues is failure, never pass. These commands are planned, not executed here.

If and only if fresh worktree `apps/node_modules` is absent, run ONE pinned
frozen install before any pnpm/hook use; do not replace/install tool versions:

```bash
(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
```

RED, then GREEN of the same permanent regression group after correction:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestExecutorProfileScripts(IndependentSQLite|Presence)$' ./internal/task/service -v)
```

Complete non-PG functional group (also the exact new Windows stage selector):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^(TestExecutorProfileScripts(IndependentSQLite|Presence|OwnCommit|Storage|Failures|Compatibility|SettingsDomain|ApplyProfile)|TestRegisteredExecutorProfileScripts(HTTP|WS))$' ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers ./internal/backendapp ./internal/orchestrator/executor -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^(TestUpdateExecutorProfile(WritesMutableFieldsAndReportsMissing|IfUnmodifiedRejectsStaleProfile|AppliesSuppliedFields|KeepsSpritesTokenWhenOmitted)|TestMergeSpritesTokenEnvVarsPreservesUnlessExplicitlyRemoved|TestRemoteDockerMutationsRequireAdmin|TestPluginExecutorProfilesAcceptKandevCredentialKeys|TestPluginExecutorProfileKandevKeysAreClearedAndNeverSentToTheProvider|Test(HTTP|WS)KubernetesProfile(MutationsReturnForbiddenWithoutInternalLog|ValidationReturns(BadRequest|Validation)WithoutInternalLog)|TestExecutorProfileHandlersRejectUserNamespacesAndPreserveOperatorConfig|TestHandleUpdateExecutorProfile(PreservesOperatorConfigDuringOrdinaryConfigUpdate|UsesCASWhenConfigIsOmitted))$' ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers ./internal/mcp/handlers -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestExecutorProfileScriptsStoreConformance$' ./internal/persistence/storeconformance -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go run -trimpath -tags fts5 -p=1 ./cmd/sqlguard ./internal)
```

If adapters introduce different necessary focused tests, name them in the exact
selector before running; cover every changed test suite, not an empty match.
Assert each expected top-level name actually RUN/PASS in the retained logs.
Do not broaden tests when these pass without a grounded new concern.

Private PG allocation requires a saved exact owner/container/name/image/mount
receipt BEFORE allocation. Use tmpfs and no volumes/binds, or an explicitly
named owned volume. Record actual mounts after creation. Populate
`KANDEV_TEST_POSTGRES_DSN` privately without displaying credentials; use existing
testutil isolated database cleanup. Stop/close/join tests before deleting only
proved-owned resources and credential files. Foreign/unproved volume `2c48...`
and paused work stay untouched. With the owned DSN already set, run sequentially:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestPostgresExecutorProfileScripts(PhysicalConcurrency|Compatibility)$' ./internal/task/repository/sqlite -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestExecutorProfileScriptsStoreConformance$' ./internal/persistence/storeconformance -v)
```

Scoped conformance must actually exercise this mutation on SQLite and PG when
DSN is supplied. Preserve production schema/history and adapter catalog.
PG no-SKIP and real statement-wait logs are mandatory before delivery.

Initial prepublication lint covers actual changed and required compatibility
packages only, using immutable admitted base `bef6699b47d64768cec6aba278c0b00ebea97e6e`.
Run the following with the actual affected package list recorded at execution:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers ./internal/backendapp ./internal/orchestrator/executor ./internal/persistence/storeconformance --new-from-rev=bef6699b47d64768cec6aba278c0b00ebea97e6e --concurrency=2 --allow-serial-runners --timeout=5m)
```

Full changed-code lint below is required before an actual Go PR fixup, rather
than an additional mandatory initial scan. `PR_BASE_SHA` must be saved from the
actual immutable PR base; never substitute moving main. Hosted full lint still
must pass. The full CHANGED fixup lint has the same 1GiB budget:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev="$PR_BASE_SHA" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Cheap checks, using the existing Node path, run at design and after document
changes; public validators become required when the public clarification lands:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/executor-profile-script-preservation
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node --test scripts/validate-public-docs.test.mjs
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node scripts/validate-public-docs.mjs
```

Run `.github/scripts/pr-docs.cjs` exported `validateCoverage` against exact
changed files and current artifact contents to prove work-order, manifest,
requirement/AC, and design coverage; catalog/spec lint alone is insufficient.
For DESIGN, the actual docs-only diff is exempt, so exercise reference coverage
with the planned source trigger, explicitly without treating it as an actual edit:

```bash
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'NODE'
const fs = require('node:fs');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/executors/requirements/profile-editor.md',
  'docs/specs/executors/system-design/profile-editor.md',
  'docs/plans/executor-profile-script-preservation/plan.md',
  'docs/plans/executor-profile-script-preservation/task-01-preserve-profile-scripts.md',
];
const plannedTrigger = 'apps/backend/internal/task/service/service_resources.go';
const result = validateCoverage({
  changedFiles: [...paths.map(filename => ({filename, status: 'modified'})),
    {filename: plannedTrigger, status: 'modified'}],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
process.stdout.write(JSON.stringify({mode: 'planned-source design preflight', ...result}) + '\n');
if (!result.ok || result.status !== 'covered' || result.workOrders.length !== 1) process.exitCode = 1;
NODE
```

After implementation replace planned source classification with the actual diff.
No frontend build, browser run, or generic full audit.

Normal hooks remain active. Delivery follows the manifest and live task plan:
all-local-heavy RETURN before the sole hosted observer; saved fixed deadlines,
complete required contexts and parent SUCCESS, named Windows/PG RUN/PASS,
fresh errors-empty/zero actionable threads, and full authenticated configured
CodeRabbit current-head coverage. No merge until ROOT's separate serial grant.

## Files likely touched

- `apps/backend/internal/task/models/executor_profile_script_intent.go` (new)
- `apps/backend/internal/task/repository/interface.go` (`ExecutorRepository` only)
- `apps/backend/internal/task/repository/sqlite/executor_profile_script_update.go` (new)
- `apps/backend/internal/task/service/service_resources.go` (`UpdateExecutorProfile` only)
- `apps/backend/internal/task/service/executor_profile_scripts_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/executor_profile_scripts_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/executor_profile_scripts_postgres_test.go` (new)
- `apps/backend/internal/task/handlers/executor_profile_scripts_test.go` (new)
- `apps/backend/internal/backendapp/settings_executor_profile_scripts_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_profile_scripts_test.go` (new)
- `apps/backend/internal/persistence/storeconformance/executor_profile_scripts_test.go` (new)
- New focused `executor_profile_script_mock_test.go` files only in packages whose
  explicit aggregate doubles need the required seam; audit affected implementations
  with `rg`, never append to oversized `*_test.go` baselines.
- `.github/workflows/backend-tests.yml` (new native stage, existing stages retained)
- `docs/public/executors.md` (brief clarification)
- `apps/backend/AGENTS.md` (only a causally necessary scoped seam convention)
- The four artifacts in this design package (lifecycle/results updates)

Read-only consumers/control sources include legacy `executor_profile.go`,
`executor_profile_handlers.go`, `settings_domain_operations.go`,
`config_executor_handlers.go`, `executor_provider_profiles.go`, and
`executor_state.go`. No production consumer edits are planned beyond the ordinary
service branch and required store interface/implementation.

## Dependencies

None. Sole work order, sequential. Sibling74 PR4308 is no prerequisite. Check
static compatibility of overlapping interface/service/workflow/scoped guidance
near merge and preserve incoming native stages; do not access its worktree,
revert other edits, rebase moving main or add unrelated implementation.

## Risks

- Result closure/commit/cancellation ownership and driver differences can falsely
  acknowledge snapshots if not asserted through actual DB and event paths.
- Aggregate interfaces affect package compilation; compatible focused test-only
  methods must remain fail-closed, and integration tests must use real storage.
- Fixed persisted timestamps are necessary for reliable exact-CAS portability.
- PG/Windows must execute actual tests; selectors that match nothing are failures.
- Broadening to other fields or full-editor drafts would exceed the accepted scope.

## Parallelism

`sequential`; same primary session only, no delegation.

## Inputs

- [Requirement](../../specs/executors/requirements/profile-editor.md), AC .5 and .8 through .12.
- [Design](../../specs/executors/system-design/profile-editor.md#partial-save-script-persistence).
- [Manifest](plan.md), accepted read-only qualification/receipt/log and consumer audit.
- Existing `workspace_field_updates.go`/PG physical-wait patterns; existing
  executor profile service/repository/handler tests and real settings dispatch
  fixtures; `/tdd` backend-tests reference and scoped backend guidance.
- Existing [SQLite writer admission ADR](../../decisions/2026-10-05-sqlite-writer-transaction-admission.md).

## Results

Implementation released after ROOT reviewed the original four-artifact design
package. Permanent regression evidence was authored independently; the private
accepted proof was neither copied nor replayed. Original run receipts/logs live
in `/tmp/kandev-child75-executor-scripts-20261007/` and the live task plan.

- RED: native92984 joined exit1, group299650 gone. All four causal stored/returned
  script assertions failed; explicit-last-commit and serial presence controls passed.
- GREEN: native69646 joined exit0, group306857 gone. The same regression group passed.
- Expanded functional native39195 joined exit1, group334494 gone: service/event
  controls, real registered REST/WS, settings-domain dispatch and actual
  `applyProfile` projection passed. A new PG fixture compile typo blocked only
  storage; corrected `time.Hour()` to `time.Hour` after joining, without replaying
  passing functional packages.
- Storage/SQLite conformance native68088 joined exit0, group357896 gone: real
  deferred-FK commit error, query failure/rollback, cancellation, missing row,
  caller-value preservation and connection reuse passed.
- Existing compatibility native59554 joined exit0, group360240 gone: legacy/full
  exact CAS, config-mode MCP guard, K8s/remote-Docker authorization and validation,
  Sprites env/token and separate plugin credential controls passed.
- SQLguard native40935 joined exit0, group363807 gone.
- PG native7843 joined exit0, group368147 gone: eight ordinary physical row-wait
  interleavings and fixed-history exact row-wait plus controls passed. Nine real
  distinct backend PID pairs and the blocking observer were recorded. Its combined
  selector missed conformance; that empty match is not counted as evidence.
- PG-only conformance native42247 joined exit0, group371180 gone using
  `^TestExecutorProfileScriptsStoreConformance$/^pgx$`; actual subtest RUN/PASS,
  no SKIP. SQLite conformance was not replayed.
- Private PG owner/image/tmpfs-only intent was recorded before allocation and
  literal container/mounts afterward. Allocation and cleanup originals joined;
  only owned container `f9b8f8f3f04094e56958e9c2c98a5f7f42fe7df46d75a8082f1846388267ea7e`
  was removed. Its absence and credential removal were verified. Foreign resources
  and paused work were untouched.
- Faithful aggregate handler fixture native86673 joined exit0, group373796 gone:
  existing five HTTP/WS update/admission/error cases passed.
- Catalog (360 decisions/1,434 specs), all-spec lint, public-validator 62 tests and
  47 pages passed. Harness tests19 passed. The scoped guidance initially exceeded
  its line budget by one; an existing wrapped sentence was consolidated. All203
  harness files and the active focused harness hook now pass at300 lines.

- Initial scoped lint native99848 joined exit0, group376360 gone, actual zero
  issues in297.227s within the five-minute CLI limit. Nine changed/required
  compatibility packages were checked against admitted immutable `bef6699b` with
  concurrency2/allow-serial, GOMAXPROCS2 and GOMEMLIMIT1GiB.
- Actual-diff documentation coverage passed with one work order and `errors: []`.

- One frozen install native32173 joined exit0, group408000 gone; 935 packages
  reused, no downloads or lock/tool changes, actual pnpm9.15.9. Its Node
  dependency deprecation warning is recorded in the original install log; no
  recovery or unrelated tool change was attempted.

Normal commit hooks/publication,
hosted native Windows/PG evidence and the sole terminal observer remain pending.
No UI/browser/E2E/build work was required. Merge authority remains NONE.

Hosted prerequisite diagnosis on frozen `97f5f009d`: job113051537292,
run37696518985, failed only the later existing Windows workspace stage.
`TestWorkspaceFieldUpdatesPresenceAndDefaults` line240 and storage helper line105
require strict timestamp advancement between immediate writes. Exact baseline
fixture and producer bytes match admitted `bef6699b`; the producer uses ordinary
`time.Now().UTC()`. Actual timestamps were not printed, so the failure proves the
strict-order assertion failed, without proving equal versus backward clock values.
The new Windows executor-script stage and all11 named cases actually RUN/PASS,
no SKIP, before that failure. ROOT permits only older persisted fixture versions
and rereads in the two existing test files, two affected anchored tests and one
full changed-code lint against immutable published PR base `c40f6d96`.
Original native91568 was lost during machine interruption; identity-checked
obsolete observer424436 may be terminated under ROOT's explicit direction, with
termination/group-absence evidence and no fabricated exit/join. Its original log
is retained. After real corrective publication, original-head semantic/native
proofs become historical; new-head gates must run again.

Fixture correction verification: only anchored
`^(TestWorkspaceFieldUpdatesPresenceAndDefaults|TestWorkspaceFieldUpdatesStorage)$`
ran in service/storage under the admitted race/resource flags. Native31030
actually joined exit0, group651796 gone (128.078s including compilation), both
names RUN/PASS. Format/diff/spec/catalog/actual one-work-order coverage passed.
The mandatory full changed-code lint original10791 actually joined exit124,
group671825 gone after360.237s; empty diagnostics do not establish a clean scan.
ROOT authorized exactly one identical warm recovery, original25906/group716355,
against immutable published PR base `c40f6d96` with the same limits and retained
caches. Recovery25906 actually joined terminald56bd6 exit0, group716355 gone,
230.324s and actual0issues. No initial passing script/route/PG suite replay.
