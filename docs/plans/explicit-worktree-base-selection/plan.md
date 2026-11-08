---
created: 2026-11-07
status: completed
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
  - ../../specs/workspaces/system-design/worktree-base-selection.md
legacy_specs: []
---

# Implementation plan: Explicit worktree base selection

## Overview

Make a missing explicitly selected worktree base fail preparation before
another branch becomes a runnable workspace. First record source intent on
attachments. Then propagate and enforce the policy through worktree preparation.
Both work orders are implemented and their focused checks pass.

Issue: [#5142](https://github.com/kdlbs/kandev/issues/5142).
Confirmed assignee: `carlosflorencio`.
Investigation baseline: `52309e613fc9a76d38a9f891b74b4d8b32bff265`.

## Confirmed root cause and evidence

`lifecycle.buildWorktreeCreateRequest` always assigns `req.DefaultBranch` to
`FallbackBaseBranch`. `worktree.Manager.resolveBaseRefWithFallback` does not
know whether the base was supplied or inherited. It substitutes the configured
fallback and can also discover a live remote default. Earlier provider-refresh
and numbered-PR helpers can substitute before the final branch-existence check.

MCP `inheritedRepoInputs` and `Service.inheritParentRepositories` retain branch
text without durable selection intent. `explicitRepoInputsWithDefaults` also
fills an omitted base before downstream consumers can distinguish omission.
The existing manual comparison marker covers later picker changes, not all
task-creation selections. It cannot replace creation provenance.

A temporary `TestIssue5142Repro_ExplicitMissingBase` used real native Git:

1. Create a bare remote and clone a cache containing `main`.
2. Publish `feature/qualified-base` afterward with a distinct sentinel commit.
3. Confirm the selected branch exists remotely and is absent from the cache.
4. Call the real `worktree.Manager.Create` with that base, fallback `main`,
   and `PullBeforeWorktree=false`.
5. Require an error, a nil worktree, and zero persisted worktrees.

The command failed for the expected behavioral reason:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run '^TestIssue5142Repro_ExplicitMissingBase$' -count=1 -v)
```

Observed: nil error, one persisted worktree, base `main`, and `HEAD` equal to
cached `main` rather than the selected commit. Its warning named both branches.
The throwaway test and its temporary Git fixtures were removed after diagnosis.
No production or permanent test change forms part of this design turn.

The permanent regression will be
`manager_explicit_base_test.go::TestCreateWorktree_ExplicitMissingBaseFailsClosed`.
It must fail behaviorally before enforcement, not merely fail compilation.

## Requirement conformance and assumptions

The defect violates existing `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6` and
`.8`. Criteria `.25` through `.28` fill the missing selection-policy and legacy
compatibility contract. They qualify branch substitution under `.13` and `.19`.
The original same-selected-ref refresh criteria remain intact.

Workspace ownership follows the source-ref contract. Task launch and executor
adapters implement that workspace contract. No parallel UI requirement is needed.

The requested outcome, deliberate retry, and absence of automatic repair follow
the issue. Existing tests and MCP comments establish inherited-base recovery.
The conservative design choice is to treat ambiguous legacy bases strictly.
The proposed [ADR](../../decisions/2026-11-07-explicit-worktree-base-selection.md)
records its compatibility cost. No material factual question remains open.

## Scope

### In scope

- Preserve explicit, inherited, and automatic-default selection intent.
- Store branch-bound intent through existing attachment metadata.
- Enforce strict missing-base resolution in every worktree substitution path.
- Retain separately identified inherited/default recovery and its warnings.
- Preserve attachment identity through single- and multi-repository preparation.
- Prove failure admission, deliberate repair/retry, and compatibility controls.
- Update public Git recovery guidance with the implemented behavior.

### Out of scope

- Issue #4477's prepared-route retry and PR-base overwrite behavior.
- New fetch retries, credential fallback, transport changes, or branch resets.
- New API request fields, database columns, migrations, feature flags, or UI.
- Changing valid reuse, remote-executor checkout scripts, or empty-remote bootstrap.
- Reopening historical work packages or replacing their recorded test results.

## Technical approach

Task 01 records the proposed `BaseBranchSelection` contract in attachment
metadata and derives strict policy for ambiguous non-empty bases. It covers
task creation, both inheritance producers, MCP default filling, branch addition,
branch-policy snapshots, explicit edits, and database reopen. This compatible
expansion does not yet change worktree-manager behavior.

Task 02 carries the derived `BaseBranchFallbackPolicy` through executor,
backendapp, and lifecycle request copies. `FallbackBaseBranch` remains a
candidate. Both early fallback helpers and live-default discovery obey policy.
The guard precedes new worktree effects. Existing required-refresh and context
errors retain their identity. A failed strict request cannot reach fallback
self-healing or agent startup.

Keep new policy helpers in focused files. Existing large launch and manager
files need small wiring edits, not further nested decision trees.
The [selection design](../../specs/workspaces/system-design/worktree-base-selection.md)
owns metadata shape, copy boundaries, and recovery semantics.

### Provider and transport compatibility

| Source/transport | Identity | Intended behavior | Evidence and limit |
| --- | --- | --- | --- |
| Local or managed cache, refresh disabled | Supplied branch | Missing base stops without network/default discovery | Native remote/cache fixture reproduces reported condition |
| Host `origin` fetch | Supplied branch | Existing same-ref local recovery without different-base substitution | Pull/ref-selection controls and missing-base regression |
| GitHub/GitLab/plugin managed refresh | Persisted attachment plus existing credential route | Successful refresh does not authorize another branch | Adapter and callback tests without new provider-auth claims |
| Inherited/default branch-only recovery | Server-authored branch-bound metadata | Existing verified fallback and warning | Real Git fixtures plus producer/persistence tests |
| Qualified PR base | Validated repository and branch | Existing strict target materialization | Existing qualified-PR and mixed-attachment controls |
| Legacy non-empty base | Source intent unknown | Strict missing-base handling | Missing/malformed/stale metadata controls |
| Other remote executors | Existing selected checkout contract | No script or credential change | Shared type compilation without live Docker/SSH/Sprites evidence |

## Tests

All test identifiers below are planned additions unless marked existing.
Full names and exact commands belong to the work orders.

| Criteria suffixes | Evidence |
| --- | --- |
| .27 | `TestBaseBranchSelection_*` in models, service, SQLite, and MCP handlers |
| .6, .25, .26 | `TestCreateWorktree_ExplicitMissingBaseFailsClosed` with distinct remote/cache commits |
| .25, .27 | Explicit-default equality, manual reselection, unknown/stale metadata, and replacement controls |
| .13, .19, .27 | Existing inherited fallback, live-default, and stacked-PR tests with intentional opt-in |
| .1, .3, .10 | Existing local-only refresh and cancellation tests |
| .8, .28 | Mixed valid/missing attachments, including the same repository twice |
| .26, .28 | `TestBaseBranchSelection_LaunchRejectsWrongBase` and `TestBaseBranchSelection_RepairAndRetry` |

## E2E tests

Use a native backend integration in
`apps/backend/internal/backendapp/base_branch_selection_integration_test.go`.
Drive service-created persisted attachments through actual executor/lifecycle
adapters and real Git preparation. Replace only external agent execution with
an observable fake. Assert the stored launch failure and zero agent starts.
After an explicit fixture fetch, retry the same task.
Assert the selected commit, sentinel bytes, unchanged attachment base, and
successful admission.

Repeat the failing admission with one valid attachment and one unresolved
explicit attachment. Test separate repositories and two rows for one repository.
Do not manufacture a launch policy and call that producer coverage.

No Playwright spec or UI preview is required. Existing desktop and phone error
surfaces remain unchanged. The backend integration verifies their shared failure
source. This does not claim rendered browser validation.

## Companion package inventory

| Existing package | Recorded status | Relationship |
| --- | --- | --- |
| [Required refresh](../worktree-refresh-fail-closed/plan.md) | implemented | Preserve strict PR refresh and bounded launch errors |
| [Local refresh](../local-worktree-best-effort-refresh/plan.md) | completed | Preserve recovery to the same selected local branch |
| [Fork PR bases](../fork-pr-base-resolution/plan.md) | done | Preserve repository-qualified target strictness |
| [PR-link launch](../pr-link-fork-launch/plan.md) | done | Preserve attachment/provider identity matching |

Those results remain historical. This package records fresh results for its
changed paths and compatibility cases. It adds no browser scenario or new work
scope to those completed packages.

## Work orders

- [x] [Task 01: Record base selection intent](task-01-record-selection-intent.md)
- [x] [Task 02: Enforce base selection before launch](task-02-enforce-selection.md)

Executed Task 01, then Task 02 in the primary session after the explicit
implementation request. Results are recorded in each work order.

## Package validation

Run each work order's exact commands from the repository root.
Then run the specification and diff gates listed in Task 02.
The following local preflight uses a planned production path to exercise actual
PR-documentation coverage even while the workspace contains only design files:

```bash
node <<'NODE'
const fs = require('node:fs');
const path = require('node:path');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/explicit-worktree-base-selection';
const orders = fs.readdirSync(dir).filter(name => /^task-\d{2}-.+\.md$/.test(name));
const files = [path.join(dir, 'plan.md'), ...orders.map(name => path.join(dir, name)),
  'docs/specs/workspaces/requirements/worktree-base-refresh.md',
  'docs/specs/workspaces/system-design/worktree-base-refresh.md',
  'docs/specs/workspaces/system-design/worktree-base-selection.md'];
const result = validateCoverage({
  changedFiles: [{ filename: 'apps/backend/internal/worktree/worktree.go', status: 'modified' },
    ...orders.map(name => ({ filename: path.join(dir, name), status: 'added' }))],
  fileContents: Object.fromEntries(files.map(file => [file, fs.readFileSync(file, 'utf8')]))
});
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors,
  workOrders: result.workOrders }, null, 2));
if (!result.ok) process.exit(1);
NODE
```

## Verification results

Implementation completed on 2026-11-08. All work-order commands passed:

- Focused Task 01 tests across models, service, SQLite, MCP, and backendapp.
- `go run ./cmd/sqlguard ./internal` and `./internal/persistence/storeconformance`.
- `TestCreateWorktree_ExplicitMissingBaseFailsClosed` with real Git, including
  the strict live-default control and multi-attachment rollback.
- The full worktree, lifecycle, executor, and backendapp package suites.
- Backendapp launch integration for failure admission, repair/retry, a valid
  sibling, and two attachments of one repository.
- Public documentation validation and specification lint.
- `bash scripts/lint-go-changed` over every changed Go file: `0 issues`.

Review corrections verified on 2026-11-08:

- Single-repository resume now copies the exact attachment's derived policy and
  matching default candidate; ambiguous or unbound session bases remain strict.
- `AddBranchToTask` captures base omission before LocalPath/GitHubURL resolution,
  so automatic-default intent no longer depends on the repository locator.
- Rolling back an unpersisted prepared worktree evicts its cache aliases. A
  partial multi-repository failure, repair, and same-task/same-session retry now
  admits both attachments without the stale `worktree not found` recreation.

The diagnostic command failed for the expected wrong-base behavior before the
guard, and now passes with the strict result.

Design-package checks passed on 2026-11-07:

- `python3 scripts/list-docs.py validate`: 438 decisions and 1,938 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- The package's local PR-documentation preflight: `covered`, both work orders,
  zero reference errors. New criteria sit within their owning requirement.
- Local Markdown targets exist. The catalog discovers the selection design
  and proposed ADR.
- `git diff --check` passed. Git status contains only the seven design-package
  files, with no staged changes or retained reproduction test.

Three existing worktree controls also passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run '^(TestCreateWorktree_BaseBranchFallback_RecoversWhenRequestedMissing|TestCreateWorktree_BaseBranchFallback_NotUsedWhenBaseExists|TestCreateWorktree_PullEnabledUsesLocalOnlyBase)$' -count=1 -v)
```

These design-stage results established the defect and the existing
compatibility controls. They do not substitute for the implementation and
integration checks recorded above, which are now complete.

## Risks

- Missing a request copy can erase permissive intent or leak it to a sibling.
- A final-only guard misses early refreshed-ref and PR fallback paths.
- Clearing the configured candidate misses live remote-default substitution.
- Legacy strictness deliberately changes missing-base recovery for unknown rows.
- A same-name explicit edit must revoke inherited permission.
- Reusing the manual comparison marker for all creation choices can change
  unrelated provider-retarget behavior. Keep the contracts separate.
- Real-provider authorization and remote executor operation are outside the
  temporary native Git evidence.
