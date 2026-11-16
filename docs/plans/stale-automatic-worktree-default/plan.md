---
created: 2026-11-16
status: complete
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/workspaces/system-design/worktree-base-selection.md
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
legacy_specs: []
---

# Fix plan: Stale automatic worktree default

## Overview

Restore automatic-default recovery when pull-before-worktree encounters a
deleted base. One work order owns the error evidence, resolver correction,
regressions, existing launch controls, and matching documentation.

Keep the repair in the two existing manager files. Reuse `ErrRemoteRefMissing`,
the default resolver, and the fallback refresh helper. Add no error type,
recovery module, candidate-search algorithm, or request field.

Issue: [#5335](https://github.com/kdlbs/kandev/issues/5335).
Confirmed assignee: `carlosflorencio`.
Investigation baseline: `8b46ceba098d20ddb11de01d6d9d9d947edc71e5`.
The issue status was recorded as open during the investigation. This work did
not update the issue or retry a live task.

## Confirmed root cause and evidence

MCP default filling retains automatic selection intent. The attachment policy
and repository default reach `buildWorktreeCreateRequest` in lifecycle preparation.
For ordinary tasks, `resolveBaseRefWithFallback` calls `pullBaseBranch` before
the missing-base policy gate. That helper supplies no substitute candidate.

With no usable local base, `handleBaseFetchFailure` returns a required-refresh
error for `missing_remote_ref`. The resolver returns that error before checking
permissive policy or calling `ResolveRemoteDefaultBranch`. A stale saved default
therefore prevents recovery even when `origin/HEAD` identifies a usable branch.

A temporary Go overlay exercised the real `Manager.Create` path with existing
fixture helpers. Each case used an independent repository and local bare origin.
The fixture published `main`, set `origin/HEAD` to `origin/main`, and omitted
`retired-base` locally and remotely. The request used that missing branch as both
base and saved fallback.

| Case | Observed result |
| --- | --- |
| Automatic policy, refresh disabled | Recovered to `main`, matched its commit, recorded a fallback warning |
| Automatic policy, refresh enabled | Failed with `required refresh of "retired-base" failed (missing_remote_ref): git command failed` |
| Strict policy, refresh enabled | Failed with the same classification and persisted no worktree |

Diagnostic command, run from `apps/backend`:

```bash
go test -trimpath -tags fts5 -overlay=<temporary-overlay.json> ./internal/worktree -run '^TestIssue5335Probe$' -count=1 -v
```

The command exited 1 because the automatic refresh-enabled case failed its
recovery assertion. Both controls passed. The temporary overlay and fixture
resources were removed. No production or permanent test files changed.
This is native Git evidence on Linux with Go 1.26.0, not a live-instance retry
or a macOS runtime test.

The permanent primary regression is
`manager_automatic_base_recovery_test.go::TestCreateWorktree_AutomaticDefaultMissingRemoteRecovers`.
Its refresh-enabled case failed before the production change and passes now.

## Requirement conformance and assumptions

The active [requirement](../../specs/workspaces/requirements/worktree-base-refresh.md)
already defines selection eligibility under `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27`.
Criteria `.6`, `.8`, `.10`, `.13`, and `.25` through `.28` constrain failure,
refresh, cancellation, and attachment admission. No new requirement is needed.

Workspaces owns this repair because it owns the source ref for materialization.
Task launch consumes that contract. The
[selection design](../../specs/workspaces/system-design/worktree-base-selection.md#host-refresh-recovery)
now states the missing error-ordering and replacement-verification rules.
The accepted explicit-selection and local-refresh ADRs remain authoritative.
No new ownership, credential, persistence, or public API boundary is introduced.

The issue and active contract settle the outcome: automatic/inherited recovery
continues only from proven absence. Explicit selection stays strict. A different
branch after a required fetch needs its own successful refresh and verification.
There are no material open questions for this package.

## Scope

### In scope

- Reuse `ErrRemoteRefMissing` for confirmed absence without parsing error text.
- Let eligible ordinary host preparation reach existing default recovery.
- Verify replacement freshness, commit identity, warnings, and final progress.
- Keep forbidden failure categories and explicit selections strict.
- Reuse existing launch and mixed-attachment controls.
- Align public worktree guidance when the correction is implemented.

### Out of scope

- Finding how the stale repository default was originally saved.
- Issue #5203's stale-commit behavior when the selected ref exists.
- Changing task creation, provenance storage, or default-branch persistence.
- New retries, credential routes, dependency packages, migrations, flags, or UI.
- PR retargeting, provider-managed refresh, or remote executor script changes.
- Changing valid reuse, empty-remote bootstrap, or local-history preservation.
- Restarting or retrying the reported live task.

## Technical approach

### Missing-ref evidence

Reuse the existing `ErrRemoteRefMissing` sentinel.
In `manager_git.go::handleBaseFetchFailure`, include it in the safe error cause
only when the fetch proves absence and no usable selected local ref exists.
Keep `ErrGitCommandFailed` available through error inspection.
Context cancellation and execution deadlines take precedence over text classification.
Keep authentication and exhausted-lock identities intact.
No new error type is needed. The existing `confirmedRemoteRefMissingError`
also unwraps to `ErrInvalidBaseBranch`, so do not use that wrapper to change
required-refresh error identities.

Do not mark a ref-verification failure after a successful fetch as proven remote
absence. Do not recover from a generic command error containing similar words.
The existing policy classifier, rather than diagnostic-only categories, supplies
the missing-ref evidence.

### Resolver ordering and candidates

Make a small change in `manager_lifecycle.go::resolveBaseRefWithFallback`.
Before returning a refresh error, use `errors.Is(err, ErrRemoteRefMissing)` and
the existing selection policy to admit eligible non-PR host preparation.
Qualified targets remain on their earlier strict route.

Continue through the existing configured/live-default sequence in place.
Keep its ordering and failure behavior, including its skip when the saved
fallback equals the requested branch. Do not add candidate iteration or broaden
recovery when a different configured fallback also fails.

Let `resolveFallbackRef` distinguish this required recovery from its existing
callers through a local boolean argument. For this path, call the existing
`pullBaseBranchWithPolicy` with required refresh and no recursive substitute.
Its ref and ancestry checks already verify the replacement.
When the persisted candidate is absent or equals the missing request, discover
the origin's current HEAD from `git ls-remote --symref origin HEAD`; cached
`origin/HEAD` may still name the deleted branch, and `remote set-head --auto`
cannot resolve a default branch without its local tracking ref. Refresh the
replacement through its explicit remote-tracking destination refspec so a
narrow `remote.origin.fetch` mapping cannot make a stale cached commit look
refreshed.
Refresh-disabled and provider-handled calls retain their current behavior.

On success, reuse the existing fallback finishers and warning fields.
The worktree base and its commit must identify the resolved branch.
The final sync event must report completion after fallback refresh.
On failure, retain the safe cause and normal failed progress outcome.

Make only the wiring changes this needs. Extract a small local helper only if
the edited function exceeds an enforced limit. No new source module is planned.

### Compatibility by route

| Route and identity | Intended result | Evidence and limit |
| --- | --- | --- |
| Host origin fetch, automatic/inherited branch-bound selection | Proven missing base can recover to a refreshed default | New real Git manager and backend launch tests |
| Host fetch, explicit or ambiguous selection | Original failure, no other-base checkout | Strict and invalid-provenance controls |
| Selected local branch exists | Existing same-ref best-effort refresh and ancestry rules | Existing local-only and refresh tests |
| Refresh disabled | Existing permitted recovery, no new fetch policy | Paired regression and existing live-default test |
| Numbered PR | Existing configured-fallback rules only | Existing stacked-PR tests |
| Qualified PR target | Exact repository and branch remain strict | Existing qualified-base tests in the worktree suite |
| Provider callback with `RemoteSyncHandled` | Existing provider route, no second host fetch | Existing provider controls in the worktree suite |
| SSH, containers, Kubernetes, or other external materialization | No claim of new script recovery | No live remote-executor evidence in this package |

## Tests

New test identifiers are planned additions. Criteria suffixes refer to
`AC-WORKSPACES-WORKTREE-BASE-REFRESH-001`.

| Criteria | Test and evidence |
| --- | --- |
| .27 | `TestCreateWorktree_AutomaticDefaultMissingRemoteRecovers`: automatic/inherited permission, refresh on/off, stale saved default equal to the selected base |
| .6, .19, .27 | `TestCreateWorktree_AutomaticDefaultFallbackRequiresVerifiedRefresh`: publish a newer sentinel commit after cloning, require refreshed replacement and matching worktree `HEAD` |
| .6, .19, .27 | `TestCreateWorktree_AutomaticDefaultDeletedRemoteTrackingRefUsesRefreshedFallback`: delete the requested remote branch while retaining its cached tracking ref; require the refreshed default commit and completion warning/progress |
| .6, .19, .27 | `TestCreateWorktree_AutomaticDefaultDiscoversCurrentRemoteDefault`: recover from a stale cached `origin/HEAD` and an absent symref with an uncached current default |
| .6, .19, .27 | `TestCreateWorktree_AutomaticDefaultFallbackUsesFetchedCommitWithNarrowRefspec`: require the fetched commit when `remote.origin.fetch` excludes the fallback branch |
| .10, .14, .27 | `TestCreateWorktree_AutomaticDefaultRecoveryRejectsUnprovenFailure`: auth, network, timeout, cancellation, lock, generic failure, and verification uncertainty on original or candidate |
| .25, .26, .27 | `TestCreateWorktree_AutomaticDefaultRecoveryKeepsStrictSelection`: explicit selection, including a same-name saved default, still fails |
| .6, .8, .28 | Existing `TestBaseBranchSelection_*` launch and mixed-attachment controls |
| .1, .3, .13, .17, .18 | Existing local-refresh, strict-base, stacked-PR, provider, and qualified-target controls |

Manager regressions use
`apps/backend/internal/worktree/manager_automatic_base_recovery_test.go`.
Use existing real Git fixtures and scripted failures instead of a new test framework.
Failure cases assert no resulting worktree, no persistence, no creation callback,
and no fallback base mutation. Recovery warns with both branch names and no
raw command output or credentials.

## E2E tests

The new `Manager.Create` tests use real Git repositories, fetches, and worktrees.
They provide integration evidence at the boundary this fix changes.
Task creation and policy propagation do not change, so do not add another
launch harness or repeat their provenance tests.
Run the existing `TestBaseBranchSelection_*` backendapp tests for launch admission
and mixed-attachment regression coverage. All required attachments must still
resolve before runtime admission, including two attachments of one repository.

This correction changes backend logic without frontend copy, layout, touch,
navigation, or viewport behavior. Existing desktop and phone surfaces consume
the same warning/error fields. No Playwright file, UI preview, or mobile
composition is required. This package makes no rendered-browser claim.

## Documentation

The implementation updated `docs/public/git-operations.md` (explanation) and
`docs/public/executors.md` (reference). Their refresh summaries distinguish
strict missing selections from eligible automatic-default recovery. They state
that a confirmed missing automatic base can use a refreshed fallback. They keep
explicit selections and unrelated failures strict. The Git Operations summary
and selected-base section agree. The public coverage inventory cites the manager
source and regression test.

## Companion package inventory

The linked package directories and their work orders were inventoried on
2026-11-16. Their existing results remain historical.

| Package | Recorded plan status | Relationship |
| --- | --- | --- |
| [Explicit selection](../explicit-worktree-base-selection/plan.md) | completed | Preserve provenance and strictness |
| [Required refresh](../worktree-refresh-fail-closed/plan.md) | implemented | Preserve required refresh and PR fallback |
| [Local refresh](../local-worktree-best-effort-refresh/plan.md) | completed | Preserve same-selected-ref recovery |
| [Fork PR bases](../fork-pr-base-resolution/plan.md) | done | Preserve qualified identity |
| [PR-link launch](../pr-link-fork-launch/plan.md) | done | Preserve provider attachment matching |
| [Lock recovery](../git-refresh-lock-recovery/plan.md) | done | Preserve retry and context rules |
| [Checkout settings](../preserve-repository-checkout-defaults/plan.md) | in_progress | Separate repository-write repair, not a dependency |
| [Recovery diagnostics](../recovery-diagnostics/plan.md) | complete | Preserve fixed safe diagnostics |
| [Invalid-ref diagnostics](../git-invalid-ref-diagnostics/plan.md) | complete | Invalid local objects cannot authorize substitution |

This work adds no tasks or browser scenarios to those packages.
Only the new work order records this repair's implementation and fresh results.

## Work orders

- [x] [Task 01: Recover after a missing automatic base fetch](task-01-recover-after-missing-base-fetch.md)

Task 01 is complete. There are no dependencies or parallel work orders.

## Package validation

Run all commands from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/stale-automatic-worktree-default
git status --short -- docs/specs/workspaces/system-design/worktree-base-selection.md docs/plans/stale-automatic-worktree-default
```

Run the repository's PR-documentation preflight against the planned production
path as well as the new work order:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/stale-automatic-worktree-default';
const workOrder = `${dir}/task-01-recover-after-missing-base-fetch.md`;
const files = [workOrder, `${dir}/plan.md`,
  'docs/specs/workspaces/requirements/worktree-base-refresh.md',
  'docs/specs/workspaces/system-design/worktree-base-selection.md',
  'docs/specs/workspaces/system-design/worktree-base-refresh.md'];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/backend/internal/worktree/manager_lifecycle.go', status: 'modified' },
    { filename: workOrder, status: 'added' },
  ],
  fileContents: Object.fromEntries(files.map(file => [file, fs.readFileSync(file, 'utf8')])),
});
console.log(JSON.stringify({ ok: result.ok, status: result.status,
  errors: result.errors, workOrders: result.workOrders }, null, 2));
if (!result.ok) process.exit(1);
NODE
```

The work order owns exact implementation checks, including focused race tests,
changed-file Go lint, and public-doc validators. Implementation and package
validation results are recorded below.

## Verification results

Implementation: complete. Changes remain uncommitted. No push, PR, or live-task
retry was part of this work.

Implementation verification:

- RED: the primary regression failed on the refresh-enabled case with the
  expected `required refresh ... missing_remote_ref` error. The refresh-disabled
  control passed before the production change.
- GREEN: `go test -trimpath -tags fts5 ./internal/worktree -run '^TestCreateWorktree_AutomaticDefault' -count=1 -v` passed the new recovery tests. The primary regression also passed with the work-order command.
- Review correction RED: the new deleted-remote-tracking-ref regression selected the stale `origin/retired-base` sentinel commit after the remote branch was deleted.
- Review correction GREEN: after cached requested refs were disallowed as success during required-fallback recovery, `(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run '^TestCreateWorktree_AutomaticDefault' -count=1 -v)` passed, verifying refreshed `main`, the warning, the default commit, and completed final progress.
- PR review RED: regressions reproduced a cached `origin/HEAD` pointing to the deleted base, an absent symref with an uncached new default, and a stale fallback tracking ref left by a narrow fetch mapping.
- PR review GREEN: after discovering the default from the origin symref and fetching to an explicit tracking ref, the same targeted command passed all cases, including the three review regressions.
- `go test -trimpath -race -tags fts5 ./internal/worktree -count=1`: passed.
- `go test -trimpath -race -tags fts5 ./internal/backendapp -run '^TestBaseBranchSelection_' -count=1`: passed.
- Changed-file Go lint reported zero issues for `manager_git.go`,
  `manager_lifecycle.go`, and `manager_automatic_base_recovery_test.go`.
- Public-doc tests passed (63 tests), and the public-doc validator checked 55 pages.
- `python3 scripts/list-docs.py validate` checked 453 decisions and 2,018 specifications.
- Specification lint tests passed (36 tests), and full specification lint passed.
- The PR-documentation preflight returned `covered` with no errors.
- `make -C apps/backend build` passed. The host had no codesign tools, so the
  Darwin agentctl helper artifacts remained unsigned.
- `git diff --check` passed.

Design-stage evidence:

- Temporary `TestIssue5335Probe`: two controls passed and the reported case
  failed for the expected required-refresh error.
- GitHub assignment to `carlosflorencio` succeeded and a fresh read confirmed it.
- Five existing compatibility controls passed: live-default recovery, strict
  selection, same-ref local fallback, stacked-PR fallback, and fatal transient
  PR refresh failure.
- `python3 scripts/list-docs.py validate`: 453 decisions and 2,018 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The PR-documentation preflight returned `covered` for the work order, with no errors.
- Every local Markdown target exists. Whitespace checks produced no diagnostics.
- Git status contains only the selection-design amendment and two new package
  files. All changes remain unstaged and uncommitted.

Exact compatibility command, from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run '^(TestResolveBaseRefWithFallbackUsesLiveRemoteDefault|TestCreateWorktree_ExplicitMissingBaseFailsClosed|TestCreateWorktree_MissingRemoteBaseUsesRefreshedFallback|TestCreateWorktree_PRBaseTransientRefreshFailureRemainsFatal|TestPullBaseBranch_MissingRemoteRefUsesLocalBase)$' -count=1 -v)
```

## Risks

- A generic error match can turn authentication or uncertain state into substitution.
- A best-effort candidate fetch can admit an unrefreshed replacement branch.
- Repeated stale candidates can create duplicate attempts or default-discovery loops.
- Recovery policy must remain attached to its exact repository row.
- Provider-handled refresh must never gain a second host credential route.
- Native Git fixtures do not prove macOS or live remote-executor behavior.
