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

## Out of scope

Repository/storage changes, schema/index/migration changes, concurrent-writer
serialization, existing duplicate repair, exporter changes, the legacy
`office/service` facade, import/sync changes, execution-profile naming,
frontend/locales, browser tests, dependency changes, and broad verification.

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

This work changes the shared API save outcome only. Existing desktop and phone
callers receive the same rejection and persisted state. No browser composition,
frontend state, or mobile-specific interaction changes, so no Playwright or
layout work is required. Existing public guidance already asks for unique
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
