---
created: 2026-09-22
status: draft
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
legacy_specs: []
---

# Implementation Plan: Fork PR base resolution

## Overview

Resolve a PR base using its target repository and branch before worktree
preparation. Preserve ordinary repository defaults and existing push routing.
Issue: [#3857](https://github.com/kdlbs/kandev/issues/3857).

Deliver provider identity first, materialization second, and recovery guards
with integration evidence last. All work orders are pending and sequential.
This package does not authorize implementation or delegation.

## Confirmed root cause

Investigation used commit `c4262c644d` on 2026-09-22.

- `readRemoteDefaultBranch` reads only `refs/remotes/origin/HEAD`. This matches
  its documented repository-default purpose, but cannot identify a PR target.
- `Executor.resolvePRBaseForLaunch` uses the attachment repository's owner/name
  even when `repoInfo.ComparisonTarget` names an upstream PR repository.
- `githubPRBaseResolver.ResolvePRBaseBranch` drops all information except
  `PR.BaseBranch`. GitHub's internal `PR` model does not retain the base OID.
- Lifecycle preparation and `worktree.CreateRequest` do not carry the existing
  `ComparisonTarget`. `resolveBaseRefWithFallback` can choose origin's default.
- `recoverTaskLaunchBranch` calls the origin resolver and the manual base-update
  operation. The latter clears comparison targets, including same-name updates.

A PR number alone is insufficient because numbers are repository-scoped.
A branch name alone is insufficient because fork and upstream can share it.

## Reproduction evidence

Two temporary tests exercised current production methods and failed as expected:

```text
go test ./internal/orchestrator/executor -run '^TestRepro3857' -count=1 -v
base=main comparison=upstream/widget:release
calls=[{workspaceID:workspace owner:fork-owner repo:widget number:42}]
FAIL: PR lookup used fork namespace instead of authoritative upstream namespace

go test ./internal/worktree -run '^TestRepro3857' -count=1 -v
base="main" stored="main" error=<nil>
FAIL: unresolved PR base accepted origin default
```

Both commands ran from `apps/backend`. The tests were removed after capture.
The executor fixture attached `fork-owner/widget`, stored a valid upstream
comparison target, and recorded the provider resolver call. The worktree
fixture had a real Git repository, origin HEAD pointing to `main`, a missing
`upstream-release` base, and PR number 42.

The second test proves an unsafe branch-only fallback. It does not establish
that every PR task is cross-repository. The first test supplies that identity.
Neither test reproduces the external agent hint in issue #3856.

## Requirement conformance and decisions

The workspace system owns worktree base identity and materialization.
Criteria .11 and .13 already cover live bases and missing-branch fallback.
They omit repository identity. Draft criteria .15 through .19 close that gap.
The design amendment follows the existing repository-qualified comparison ADR.
No new ADR or schema migration is necessary.

Confirmed scope is investigation and a fix package, conditional on clear evidence.
The source and temporary tests establish the defect. No material product choice
remains unresolved for this package. Existing PR URL tasks attach upstream,
so the issue's fork-origin topology is not universal.

## Scope

### In scope

- Exact attachment/PR matching and repository-qualified provider base results.
- Validated base OID observations and verified materialized commits.
- Host creation/recreation and remote materialization contract propagation.
- No implicit fork-default substitution for an explicit upstream target.
- Recovery guards, compatibility tests, and implementation-time public docs.

### Out of scope

- The reconciliation hint's producer, wording, and counts in issue #3856.
- Automatic merge, rebase, reset, push, or rewriting origin.
- New credentials, expanded repository access, or a new persistence format.
- New UI controls, translations, or changes to manual comparison selection.
- Changing background comparison readiness or valid worktree reuse policy.

## Technical approach

Use the [design amendment](../../specs/workspaces/system-design/worktree-base-refresh.md#proposed-amendment-repository-qualified-pr-bases).
The existing origin-default resolver remains a low-level repository operation.
Callers with PR context must use qualified identity before fallback.

Reuse `ComparisonTarget` identity and deterministic comparison refs. Retain
base OIDs only in provider observations and preparation results. Materialize
under the existing credential route and verify the selected commit.

Extract only the reusable materialization primitive into `internal/common/gitbase`.
Keep agentctl scheduling, error projection, and workspace ownership unchanged.
Reject default recovery for an explicit cross-repository binding before writes.

## Tests

All names below are planned permanent regressions, not existing passing tests.

| Criteria | Evidence |
| --- | --- |
| .11, .15 | `executor_pr_base_resolver_test.go`: `TestResolveTaskRepoInfo_PRBaseUsesTargetRepository`; colliding PR numbers and retarget cases |
| .15, .16 | `github/pr_base_identity_test.go`: `TestPRBaseIdentityConversions`; REST, GH CLI, GraphQL, incomplete fields |
| .16, .17 | `worktree/manager_pr_base_test.go`: `TestCreateWorktree_QualifiedPRBase`; two local bare repositories with different `main` commits |
| .16, .17 | `common/gitbase/materialize_test.go`: `TestMaterializeQualifiedBase`; exact ref, OID mismatch, collisions, auth, missing ref, cancellation |
| .15-.17, .19 | `agentctl/server/api/workspace_materialize_pr_base_test.go`: `TestMaterializeRepository_QualifiedPRBase`; remote boundary round trip |
| .18 | `orchestrator/task_launch_recovery_pr_base_test.go`: `TestRecoverTaskLaunch_ForkPRDefaultPreservesTarget`; zero writes and zero relaunch |
| .15-.19 | `backendapp/pr_base_integration_test.go`: `TestForkPRBasePreparationEndToEnd`; provider fixture through request wiring to real Git |
| .19 | Existing live-default, stacked-PR fallback, comparison-target, and contribution tests plus a mixed-repository regression |

## End-to-end evidence

This backend repair uses a Go integration fixture instead of a browser-only test.
The fixture creates upstream and fork bare repositories with different sentinel
commits, then prepares a task through the production wiring. Assert target OID,
checkout head preservation, unchanged origin/push routing, and persisted target.
Repeat with upstream unavailable and with one valid sibling repository.
Existing rendered comparison behavior remains unchanged.

## Work orders

- [ ] [Task 01: Preserve qualified PR base identity](task-01-pr-base-identity.md)
- [ ] [Task 02: Materialize the qualified PR base](task-02-qualified-materialization.md)
- [ ] [Task 03: Guard recovery and prove integration](task-03-recovery-and-integration.md)

## Companion packages

- [Fork PR comparison targets](../fork-pr-comparison-targets/plan.md): implemented;
  reuse its identity, remote, and manual-selection invariants.
- [Noninteractive comparison Git](../noninteractive-comparison-target-git/plan.md):
  preserve credential environment and asynchronous status materialization.
- [Stacked PR base retargeting](../worktree-refresh-fail-closed/task-05-support-stacked-pr-base-retargeting.md):
  preserve same-repository fallback and provider retargeting coverage.

These historical packages keep their recorded results. This package owns all
new implementation status, regression cases, and command results.

## Verification results

- Investigation: both temporary reproductions failed for the expected reason.
- Temporary tests: removed. No production or permanent test changes made.
- `python3 scripts/list-docs.py validate`: passed, 299 decisions and 1112 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Relative document links and work-order requirement/acceptance IDs: resolved.
- `git diff --check`: passed. All four package files are present in Git status.
- GitHub assignment: verified as `carlosflorencio`.
- Implementation checks: pending; exact commands are in each work order.

## Risks

- Legacy PR-number-only metadata lacks a trustworthy namespace. Never guess
  upstream from remote names or choose the first historical association.
- Provider retargeting can race a fetch. OID mismatch must remain an explicit
  error until a fresh retry supplies consistent evidence.
- Private upstream targets can remain unreadable with existing credentials.
  Fail required preparation without expanding authority.
- Sharing materialization must preserve agentctl's asynchronous scheduling.
- Successful comparison hydration does not prove preparation used the same ref.
  Verify producer-to-consumer wiring, not only helper outputs.
