---
id: "01-enforce-agent-name-admission"
title: "Enforce Office agent name admission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AGENT-NAMES-001
acceptance_criteria:
  - AC-OFFICE-AGENT-NAMES-001.1
  - AC-OFFICE-AGENT-NAMES-001.2
  - AC-OFFICE-AGENT-NAMES-001.3
  - AC-OFFICE-AGENT-NAMES-001.4
  - AC-OFFICE-AGENT-NAMES-001.5
  - AC-OFFICE-AGENT-NAMES-001.6
system_design:
  - ../../specs/office/system-design/agents-01.md
---

# Task 01: Enforce Office agent name admission

## Summary

Restore the Office identity name check on ordinary updates and report a
duplicate name through the existing HTTP validation response. Prove the
registered API preserves the two persisted identities and a previously valid
export, while valid saves continue to work.

## In scope

- Independently author the service and registered HTTP/export regressions
  below before changing production code; reuse repository-local test patterns.
- Add `ErrAgentNameAlreadyExists`, wrap it in the existing name helper, and
  validate the update with its current workspace and excluded target ID before
  optional reporting validation can return.
- Map that sentinel to `agent_name_already_exists` in
  `agentValidationErrorCode`; keep operational errors separate.
- Update this work order, the plan results, and the paired contract statuses
  only to reflect actual completed implementation and verification. These
  existing specification files also contain broader draft identity material;
  do not promote their entire lifecycle status based on this narrow repair.
- Correct the existing mobile profile-deletion fixture's three repeated
  positional assumptions by resolving its exact seeded profile and owner.
- Carry the reviewed shared review-toggling fixture's checkout reset and exact
  two-file precondition, preserving its existing scroll assertions.

## Out of scope

Repository/storage changes, schema/index/migration changes, concurrent-writer
serialization, existing duplicate repair, exporter changes, the legacy
`office/service` facade, production import/sync changes, execution-profile naming,
frontend/locales, browser changes outside the named profile and review fixtures,
dependency changes, and broad verification.

## Acceptance

1. The direct service and registered HTTP tests reject a pre-existing
   same-workspace exact-name collision; HTTP returns `400` with the stable
   conflict code. Fresh repository reads show both original rows and
   timestamps unchanged, including other fields in the refused PATCH, and
   the original manifest contents and revision are preserved.
2. Unchanged-name, omitted-name, unoccupied-name and cross-workspace saves
   succeed and persist on the same identity. Exact-string, soft-delete, and
   global execution-profile controls retain their existing namespace behavior;
   duplicate create remains refused with the same client validation code.
3. The independently authored regression fails for the expected collision
   behavior before production changes. After the minimal service/handler fix,
   the named regressions, affected package suite, changed-Go lint, spec/link/
   coverage checks, and diff check pass with actual results recorded.

## Regression specification

Use new test files rather than extending the already substantial
`service_test.go` or `handler_routing_test.go`.

### Service evidence

`TestAgentNameUniqueness_Service` in `service_name_uniqueness_test.go` uses
`newTestAgentService` and `createAndGetAgent`, or their existing profile-store
variant when seeding a global execution profile. Use valid Workers and fresh
fixtures per case. Cover:

- Alpha and Beta in workspace A: direct update Beta to Alpha returns an error
  matching `ErrAgentNameAlreadyExists`; reread both rows, not the caller's
  mutated model. Include a valid reporting parent in a collision case.
- Duplicate create is refused and matches the same sentinel; the list keeps
  the original identities and row count.
- Saving Beta as Beta and saving another valid field succeeds with the
  original ID. A truly unused name also succeeds.
- Alpha exists only in workspace B: Beta in workspace A can become Alpha;
  workspace B's row is unchanged.
- With Alpha in workspace A, `alpha` and `Alpha ` remain distinct valid names
  with their exact bytes preserved. A soft-deleted Alpha does not reserve its
  name. A global execution profile named Alpha does not reserve Alpha in A.

Use real repository/storage behavior for these cases. Do not broaden name
semantics to make a test pass or create a new concurrency fixture.

### Registered HTTP, persistence, and export evidence

`TestAgentNameUniqueness_HTTPExport` in `handler_name_uniqueness_test.go`
constructs the real AgentService and SQLite repository with the existing
settings and Office schemas. Mount `agents.RegisterRoutes` under
`/api/v1/office` on a Gin test router. Construct a real
`config.NewConfigService` using that same repository; export uses no config
loader or writer. No mock repository or separately reconstructed export is
acceptable for the central regression.

The `reject_occupied_name` subtest shall:

1. Create Workers Alpha and Beta through AgentService in workspace A. Snapshot
   both persisted rows, their IDs and timestamps, and a successful manifest
   containing exactly one agent config per identity.
2. PATCH Beta to Alpha with another valid change such as an icon or budget.
   Include a valid reporting parent so name validation cannot hide behind
   `validateReportsTo`'s early return.
3. Assert `400`, a non-empty error and `agent_name_already_exists`. Independently
   reread both identities and the workspace list: IDs, names, timestamps and
   co-submitted fields equal the pre-request snapshots, and row count remains
   two. Do not stop at the response or inspect only the in-memory request model.
4. Call `ExportManifest` again and require success, unchanged revision and
   path/content entries, including one `.kandev/agents/Alpha.yml` and one
   `.kandev/agents/Beta.yml`. Decode their YAML and assert the original distinct
   agent configurations. Arrange nonfatal independent assertions so the red
   run exposes status, persistence, and blocked-export consequences.

Add independent `unchanged_name`, `omitted_name`, `unused_name`, and
`cross_workspace_name` HTTP subtests. Require `200`, the original identity ID
in the response and reread row, and valid exported names/configurations. For
cross-workspace reuse, export both workspaces and prove the foreign Alpha row
is unchanged. Explicitly persist another valid field in the unchanged and
omitted-name controls so they prove a successful save.

`TestAgentNameUniqueness_HTTPCreate` in the same file submits a duplicate POST
through the registrar and asserts `400`, the same stable code, and no inserted
row. This covers the shared helper's intended create classification change.

No exported sentinel exists before the fix. For the initial red run, assert
the behavioral service error/HTTP outcome without referencing an undefined
symbol; add `errors.Is` assertions when the sentinel is introduced. A compile
failure from a planned symbol is not acceptable regression evidence.

## Verification

The following commands are for the later implementation phase only. Run from
the repository root, sequentially. Retain actual process handles and exit
results. Use the narrow command for red and green, then run the affected
package once for final coverage of existing callers.

```bash
(cd apps/backend && timeout --kill-after=10s 10m env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags=fts5 -p=1 -count=1 -timeout=8m ./internal/office/agents -run '^TestAgentNameUniqueness_')

(cd apps/backend && timeout --kill-after=10s 15m env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags=fts5 -p=1 -count=1 -timeout=12m ./internal/office/agents)

timeout --kill-after=10s 7m env -u GOMEMLIMIT GOTOOLCHAIN=local GOMAXPROCS=2 bash scripts/lint-go-changed apps/backend/internal/office/agents/service.go apps/backend/internal/office/agents/handler.go apps/backend/internal/office/agents/service_name_uniqueness_test.go apps/backend/internal/office/agents/handler_name_uniqueness_test.go

python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/office-agent-rename-uniqueness
```

The lint command deliberately clears the test memory limit so
`scripts/lint-go-changed` uses its canonical `3GiB` default. If an environment
provides GNU timeout under a different executable name, use that GNU tool with
the same bounds. No dependency install or build is needed to run the Go
regression itself. Publication hooks follow the repository's existing fresh
worktree dependency requirements.

Run the lightweight delivery-coverage preflight below after all four artifact
edits, from the repository root. The runtime paths are prospective for design
validation; this does not claim those paths have changed yet. At completion,
also confirm actual changed runtime paths are within the owned-file list.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/specs/office/requirements/agents.md',
  'docs/specs/office/system-design/agents-01.md',
  'docs/plans/office-agent-rename-uniqueness/plan.md',
  'docs/plans/office-agent-rename-uniqueness/task-01-enforce-agent-name-admission.md',
];
const runtime = [
  'apps/backend/internal/office/agents/service.go',
  'apps/backend/internal/office/agents/handler.go',
  'apps/backend/internal/office/agents/service_name_uniqueness_test.go',
  'apps/backend/internal/office/agents/handler_name_uniqueness_test.go',
  'apps/backend/internal/workflowsync/circuit_test.go',
  'apps/backend/internal/workflowsync/service_test.go',
  'apps/web/e2e/tests/settings/mobile-agent-profile-delete.spec.ts',
  'apps/web/e2e/tests/review/review-viewed-toggling-flow.ts',
];
const result = validateCoverage({
  changedFiles: [...documents, ...runtime].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(documents.map(file => [file, fs.readFileSync(file, 'utf8')])),
});
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors, workOrders: result.workOrders }));
if (!result.ok) process.exitCode = 1;
NODE
```

Check all four files' Markdown links and frontmatter design references resolve;
the requirement/acceptance IDs must exist in the referenced requirement and be
declared by the plan's system design. `git diff --check -- docs/specs
docs/plans/office-agent-rename-uniqueness` and the scoped status check complete
the design-only gates. Neither Go command, changed-Go lint, nor a test of the
specification tooling runs during design.

## Files likely touched

Owned production files:

- `apps/backend/internal/office/agents/service.go`
- `apps/backend/internal/office/agents/handler.go`

Owned new test files:

- `apps/backend/internal/office/agents/service_name_uniqueness_test.go`
- `apps/backend/internal/office/agents/handler_name_uniqueness_test.go`

Additional owned existing tests for required CI synchronization correction:

- `apps/backend/internal/workflowsync/circuit_test.go`
- `apps/backend/internal/workflowsync/service_test.go`

Additional owned existing test for required CI profile fixture correction:

- `apps/web/e2e/tests/settings/mobile-agent-profile-delete.spec.ts`

Additional owned existing test for required CI review fixture correction:

- `apps/web/e2e/tests/review/review-viewed-toggling-flow.ts`

Owned design/delivery records:

- `docs/specs/office/requirements/agents.md`
- `docs/specs/office/system-design/agents-01.md`
- `docs/plans/office-agent-rename-uniqueness/plan.md`
- `docs/plans/office-agent-rename-uniqueness/task-01-enforce-agent-name-admission.md`

Read-only dependencies: `apps/backend/internal/office/repository/sqlite/agents.go`,
`apps/backend/internal/office/config/service.go`, the existing test helpers,
Office route composition, and the existing public agent/export guides.

## Dependencies

None. Execute one work order sequentially.

## Risks

The existing uniqueness helper's conflict becomes recognizable for both create
and update. Keep the operational-error branch unclassified, the self exclusion
and exact-name predicate intact, and the reporting-parent path covered.
Admission remains a read followed by a write; this work promises no new
concurrent-write guarantee or duplicate-data repair.

## Mobile and public documentation

The product change affects the shared API save outcome only. Existing desktop and phone
callers receive the same rejection and persisted state. No browser composition,
frontend state, or mobile-specific interaction changes are needed. Required-CI
fixture corrections retain existing browser assertions and need no layout work.
Existing public guidance already asks for unique
agent names and describes manifest export; no public docs change is needed.

## Parallelism

`sequential`

## Inputs

- [Office agent requirements](../../specs/office/requirements/agents.md),
  `REQ-OFFICE-AGENT-NAMES-001` and all six acceptance criteria.
- [Identity design: name admission](../../specs/office/system-design/agents-01.md#workspace-agent-name-admission).
- [Persistence guarantees](../../specs/office/system-design/agents-03.md#persistence-guarantees).
- `apps/backend/AGENTS.md`, `apps/backend/internal/office/AGENTS.md`, and
  `.agents/skills/tdd/references/backend-tests.md`.
- `agents/service_test.go` for real repository/service fixtures and
  `agents/handler_reports_to_test.go` / `handler_routing_test.go` for registered
  Gin handler conventions. Independently author this defect's test scenarios.

## Results

Implemented the two-file service/handler correction and independently authored
the two planned test files. The tests cover the six acceptance criteria,
including a real registered HTTP request, complete persisted model/timestamp
comparisons, co-submitted fields, a valid reporting parent, real manifest
contents/revision, accepted-save controls, and the existing namespace rules.

The exact commands in Verification were run sequentially on 2026-10-26:

- Targeted regression before production changes: exit 1 for the intended
  behavioral failures. The registered PATCH returned 200, changed Beta to Alpha
  together with the icon/reporting parent, and made export fail with duplicate
  `.kandev/agents/Alpha.yml`. Duplicate POST returned 500. Direct update accepted
  the collision. The tests compiled and executed.
- Targeted regression after the fix: exit 0; package result 1.338s, with race
  detection. Typed conflict assertions were added when the sentinel existed.
- Affected package: exit 0; package result 2.084s, with race detection.
- Changed-Go lint: exit 0, `0 issues`; canonical 3GiB default, not the test
  memory limit.
- One frozen pnpm workspace install from `apps`: exit 0, pnpm 9.15.9; no
  manifest/lockfile changes. Both existing git hooks are active.
- Final catalog/specification checks: passed. All 24 local links and all
  frontmatter file references resolve. Delivery coverage reports `covered`
  without errors, and final `git diff --check` passes.

Storage, exporter, concurrency behavior, frontend, and public docs are unchanged.
The broader requirement/design files remain draft as required by this work
order; the narrow delivery is complete. Hosted CI and review are external
delivery gates and are not claimed by these local results.

### CEO name-precedence remediation

The accepted name-conflict ordering finding is implemented and verified.
Independent `TestAgentNameUniqueness_HTTPCEOPrecedence` cases cover duplicate-CEO
POST and unparented Worker-to-CEO rename PATCH requests. Both must return the
existing name-conflict HTTP 400/code before CEO-count rejection. Distinct-name
controls must retain the current CEO guard. Every case compares original saved
identities, row count, and real manifest contents/revision after rejection.

The targeted RED command, before changing production, is:

```bash
(cd apps/backend && timeout --kill-after=10s 10m env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags=fts5 -p=1 -count=1 -timeout=8m ./internal/office/agents -run '^TestAgentNameUniqueness_HTTPCEOPrecedence$')
```

After expected behavioral RED, put existing name admission before CEO-count
rejection in both validators. Preserve required name/role validation, CEO
reporting-root validation, the distinct-name CEO guard, and the exact repository
predicate. Run the original full name-regression selection, affected package,
changed-Go lint, and documentation commands from Verification afterward.
Remediation results on 2026-10-26:

- The targeted CEO command compiled and failed with exit 1 before production
  changes. Both colliding-name requests returned HTTP 500 with the CEO-count
  error instead of HTTP 400/name-conflict code. Both distinct-name controls
  passed and all persisted-row/manifest preservation checks passed.
- The minimal correction moved the existing name checks before CEO validation
  in both validators; it added no new predicate or persistence behavior.
- Original full name-regression selection: exit 0, race enabled, Go result
  1.451s.
- Affected agents package once after the correction: exit 0, race enabled, Go
  result 2.168s.
- Changed-Go lint: exit 0, `0 issues`; canonical 3GiB default.
- Catalog/specification checks passed. All 24 local links and frontmatter paths
  resolve; delivery coverage is `covered` without errors; diff check passed.

No dependency reinstall or broad suite was run. The earlier successful results
remain the initial-slice history. The existing requirement is unchanged.

### Required CI fixture completion correction

The required Backend Postgres packages job exposed
`TestSyncDueConfigs_FirstKnownFingerprintRearmsSuspendedConfig` failing at the
applier-count assertion. The test uses isolated in-memory SQLite, even in that
cohort. A second diagnosed fixture,
`TestPermanentFailureSuspendsPollingUntilManualRecovery`, shares the same gap:
saved suspension precedes automatic-worker completion.

Add only an existing-state completion wait in both named tests after the saved
suspension check and before subsequent provider, metric, or clock operations.
Use `require.Eventually` with the existing one-second/one-millisecond bounds;
read `automaticInFlight["ws-1"] == nil` under `automaticMu`. Preserve every
existing assertion and timeout. No production change, new helper, sleep,
selector adjustment, or broader fixture repair belongs to this correction.

Run the following unchanged selection once as a baseline, then once after the
waits. A passing baseline means the hosted failure was unreproduced locally.

```bash
(cd apps/backend && timeout --kill-after=10s 5m env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags=fts5 -p=1 -count=20 -timeout=4m ./internal/workflowsync -run '^(TestSyncDueConfigs_FirstKnownFingerprintRearmsSuspendedConfig|TestPermanentFailureSuspendsPollingUntilManualRecovery)$')

(cd apps/backend && timeout --kill-after=10s 6m env GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -race -tags=fts5 -p=1 -count=1 -timeout=5m ./internal/workflowsync)

timeout --kill-after=10s 7m env -u GOMEMLIMIT GOTOOLCHAIN=local GOMAXPROCS=2 bash scripts/lint-go-changed apps/backend/internal/workflowsync/circuit_test.go apps/backend/internal/workflowsync/service_test.go
```

Results on 2026-10-27:

- Unchanged two-case baseline, count 20: exit 0, Go result 1.249s. The hosted
  failure was not reproduced locally; no repeat was used to seek a failure.
- The two-case selection after the waits, count 20: exit 0, Go result 1.248s,
  race detection enabled.
- Affected workflowsync package once: exit 0, Go result 1.322s, race enabled.
- Changed-Go lint over the two test files: exit 0, zero issues, canonical
  3GiB limit.
- Catalog/specification checks passed, all 24 local links resolve, delivery
  coverage is covered without errors, and gofmt/diff checks passed.

The Office agents suite and already-passing browser selections are not replayed
for this test-only correction. Hosted CI remains a separate delivery gate.

### Required CI mobile profile fixture correction

The existing mobile profile-deletion fixture assumed that the first returned
agent owned a profile. Required CI failed repeatedly at `profile.name` before
the deletion UI was exercised. Per-test setup instead guarantees the exact
`seedData.agentProfileId` and removes other global test profiles; an empty
agent family is valid API data.

The prepared test-only correction resolves that ID across returned agents,
asserts exactly one owner/profile match, and uses its owner and model for the
two disposable-profile factories in the same file. Every existing UI action,
assertion, selector, and timeout is preserved. No shared fixture, API ordering,
production code, or public documentation changes are included.

List the mobile project selection and require exactly three cases, then run the existing file once,
headless, one worker, retries zero, through the guarded managed runner with
verified existing assets:

```bash
cd apps/web
timeout --kill-after=10s 2m pnpm e2e:raw --project=mobile-chrome e2e/tests/settings/mobile-agent-profile-delete.spec.ts --list --retries=0 --workers=1
timeout --kill-after=10s 7m pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/settings/mobile-agent-profile-delete.spec.ts --retries=0 --workers=1 --reporter=line
```

On 2026-10-27, the listing confirmed exactly three cases and the edited file
passed all three once (11.0s), headless with one worker and retries zero. The
guarded managed runner admitted the existing assets; no build or install was
repeated. The original hosted failures remain the failure evidence.

The separate pseudo-office workspace-skills case failed its locale setup
assertion, then passed its built-in retry. Its cause is unknown; it did not
reach the copy scan. One exact existing desktop case with initial-attempt trace
is diagnostic only, with no pseudo-locale or product edit. Existing
Office and workflowsync passing suites are not repeated for this fixture fix.

The exact unchanged desktop diagnostic passed once (5.6s), with one worker,
retries zero, headless, and initial-attempt tracing enabled. Its trace records
an English HTML shell and boot locale without the cookie, followed by a reload
request carrying `kandev_locale=pseudo` and a pseudo HTML shell and boot locale.
The prior hosted initial request evidence remains unavailable, so its cause
is still unknown. No pseudo-locale source was edited.

### Required CI review fixture correction

Carry only `apps/web/e2e/tests/review/review-viewed-toggling-flow.ts` from reviewed
commit `6b84f031a530bdcbdf78da42b97203df6fbd5afe`. Its SHA256 is
`9294e777f7747b1863c3690bf44668cd1813bb630464f25a6096a80f57281610`.
The fixture invokes the existing seeded-checkout reset and asserts exactly the
target and trailing file headers before the unchanged scroll sequence. No
production utility, shared helper implementation, or other source file is
carried from that commit.

Reuse the reviewed producer's limited ordered three-desktop/one-mobile passing
results; no review-checkbox case is replayed here. Retain earlier hosted flaky
verdicts and the unresolved observation about marking multiple files. This
fixture correction does not claim a general product scrolling repair.

Final fixture delivery checks passed on 2026-10-27: documentation catalog and
specification lint, all 24 local links, delivery coverage without errors, exact
carried-file identity, and the scoped diff check. The seven existing managed
assets retain their original hashes. Hosted CI and review remain external
delivery gates; these limited results do not erase prior hosted failures.
