---
created: 2026-09-22
status: done
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

Provider identity, materialization, recovery guards, integration evidence, and
public documentation are complete. Issue #3856 remains outside this package.

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
They omit repository identity. Criteria .15 through .19 close that gap and are
now active as part of the delivered contract.
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
- New UI controls or translations. Explicit manual base selection remains
  authoritative across provider refreshes.
- Changing background comparison readiness or valid worktree reuse policy.

## Technical approach

Use the [design amendment](../../specs/workspaces/system-design/worktree-base-refresh.md#repository-qualified-pr-bases).
The existing origin-default resolver remains a low-level repository operation.
Callers with PR context must use qualified identity before fallback.

Reuse `ComparisonTarget` identity and deterministic comparison refs. Retain
base OIDs only in provider observations and preparation results. Materialize
under the existing credential route and verify the selected commit.

Validate a live PR's head repository against the attached checkout or the
validated contribution source. Keep explicit manual base selections marked
across provider refreshes, and fail known cross-repository resolution errors
without falling back to a bare branch.

Extract only the reusable materialization primitive into `internal/common/gitbase`.
Keep agentctl scheduling, error projection, and workspace ownership unchanged.
Reject default recovery for an explicit cross-repository binding before writes.

## Tests

These permanent regressions cover the implementation and compatibility cases.

| Criteria | Evidence |
| --- | --- |
| .11, .15 | `executor_pr_base_resolver_test.go`: `TestResolveTaskRepoInfo_PRBaseUsesTargetRepository`; colliding PR numbers and retarget cases |
| .15, .16 | `github/pr_base_identity_test.go`: `TestPRBaseIdentityConversionsRetainBaseCommitOID`; REST/GraphQL OID conversion and GH CLI command contract |
| .16, .17 | `worktree/manager_pr_base_test.go`: `TestCreateWorktree_QualifiedPRBase`; two local bare repositories with different `main` commits |
| .16, .17 | `common/gitbase/materialize_test.go`: `TestMaterializeQualifiedBase`; exact ref, OID mismatch, collisions, auth, missing ref, cancellation |
| .15-.17, .19 | `agentctl/server/api/workspace_materialize_pr_base_test.go`: `TestMaterializeRepository_QualifiedPRBase`; remote boundary round trip |
| .18 | `orchestrator/task_launch_recovery_pr_base_test.go`: `TestRecoverTaskLaunch_ForkPRDefaultPreservesTarget`; zero writes and zero relaunch |
| .15-.19 | `backendapp/pr_base_integration_test.go`: `TestForkPRBasePreparationEndToEnd`; provider fixture through request wiring to real Git |
| .17 | `lifecycle/env_preparer_worktree_pr_base_test.go`: valid sibling cannot hide a required target OID failure; cancellation returns no prepared workspace |
| .17 | `worktree/manager_pr_base_test.go`: qualified-base recreation failure preserves the existing checkout path |
| .19 | Existing live-default, stacked-PR fallback, comparison-target, and contribution tests plus a mixed-repository regression |

## End-to-end evidence

`TestForkPRBasePreparationEndToEnd` converts a fake provider response, passes
the result through backendapp's lifecycle request mapper and the lifecycle
worktree preparer, then materializes a real linked worktree from local fork and
upstream repositories. It verifies the provider's non-default target OID, PR
head, comparison metadata, unchanged fork origin refs and push URL, and disabled
push routing on the comparison remote. Separate lifecycle cases prove that a
valid sibling cannot hide a failed required target and that cancellation does
not return a prepared workspace. A recreation regression confirms failed
qualified-base preflight leaves the existing checkout untouched. Remote
materialization has its own agentctl API regression from Task 02.

## Work orders

- [x] [Task 01: Preserve qualified PR base identity](task-01-pr-base-identity.md)
- [x] [Task 02: Materialize the qualified PR base](task-02-qualified-materialization.md)
- [x] [Task 03: Guard recovery and prove integration](task-03-recovery-and-integration.md)

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
- Temporary investigation reproductions were removed. Permanent regression
  coverage now covers provider command contracts, linked legacy PR resolution,
  cross-fork identity rejection, qualified checkout reuse, and recovery writes.
- `python3 scripts/list-docs.py validate`: passed, 299 decisions and 1112 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Relative document links and work-order requirement/acceptance IDs: resolved.
- `git diff --check`: passed. All four package files are present in Git status.
- GitHub assignment: verified as `carlosflorencio`.
- Task 01 implementation: provider lookup now uses the validated target namespace,
  retains the base OID from REST and GraphQL, and checks the response against the
  exact PR and checkout identity. GH CLI `pr view/list --json` requests omit the
  unsupported `baseRefOid` field; `GetPR` reads the base SHA, ref, and repository
  identity together with `gh api`, while preserving PR details if that read fails.
  Exact linked task PR selection is scoped by repository, PR number, and checkout
  branch.
- Task 01 focused verification: `go test ./internal/github ./internal/backendapp ./internal/orchestrator/executor -run 'Test(PRBase|ResolveTaskRepoInfo|GithubPRBase)' -count=1` passed.
- Task 01 package verification: `go test ./internal/github ./internal/backendapp ./internal/orchestrator/executor -count=1` passed.
- Task 01 diff check: `git diff --check` passed.
- Task 02 implementation: a shared `internal/common/gitbase` primitive fetches
  and verifies the qualified target, while host and remote preparation keep
  PR-head materialization separate. Lifecycle copies now preserve the typed
  target through worktree and remote checkout requests.
- Task 02 focused verification: `go test ./internal/common/gitbase ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process -run 'Test(MaterializeQualifiedBase|CreateWorktree_QualifiedPRBase|MaterializeRepository_QualifiedPRBase|QualifiedPRBase|MaterializeComparisonTarget|ResolveRemoteDefaultBranch|ResolveBaseRefWithFallback|CreateWorktree_MissingRemoteBase)' -count=1`: passed.
- Task 02 full package verification: `go test ./internal/common/gitbase ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process -count=1`: passed.
- Task 02 Task 01 regression check: `go test ./internal/github ./internal/backendapp ./internal/orchestrator/executor -run 'Test(PRBase|ResolveTaskRepoInfo|GithubPRBase)' -count=1`: passed.
- Task 02 `git diff --check`: passed.
- Task 03 implementation: default recovery now rejects explicit cross-repository GitHub PR targets before repository-default or task-base writes. The comparison target and launch-error stamp remain intact; retry-launch remains unchanged, and explicit manual base selection stays authoritative across provider refreshes. Provider lookup cancellation now aborts repository resolution instead of being treated as an ordinary provider outage.
- Task 03 focused recovery and integration checks: `go test ./internal/orchestrator ./internal/backendapp -run 'Test(RecoverTaskLaunch|ForkPRBasePreparationEndToEnd)' -count=1`: passed.
- Task 03 regression checks for cancellation, mixed-repository failure, and recreation preflight: passed.
- Full affected Go package verification: `go test ./internal/common/gitbase ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/github ./internal/backendapp ./internal/orchestrator ./internal/orchestrator/executor -count=1`: passed.
- Backend build: `make -C apps/backend build`: passed for agentctl targets, Kandev, mock-agent, acpdbg, and winjob.
- Documentation checks: catalog validation passed (299 decisions and 1112 specifications); specification linter tests passed (36); full specification lint passed; public-doc tests passed (62); public-doc validation passed (47 pages).
- `gofmt -l` for changed Go files and `git diff --check`: passed.
- Updated `docs/public/git-operations.md` to explain qualified fork targets, required-fetch failures, retry actions, and unchanged push routing.
- PR review remediation: qualified materialization reuse verifies the exact
  upstream base and PR-head commit; PR-head branch restoration uses the captured
  immutable OID. Contribution and qualified-base identities are checked at both
  worktree and remote materialization boundaries. Retry-default uses the
  system-owned task-base update so it does not create a manual override.
- PR review documentation correction: criteria .15-.19 are nested under the
  owning requirement, and the plan/design/public documentation describe current
  validation and retry behavior.
- PR fixup local verification: backend build and changed-code golangci-lint
  passed. The focused review-regression command passed after the final helper
  refactor. All packages in the aggregate affected-package run passed except
  one transient orchestrator failure; the complete orchestrator package passed
  in an immediate standalone rerun. The scoped PR documentation coverage
  validator passed.

## Review corrections

- A legacy attachment with only a PR number now qualifies the live upstream
  target before materialization. The producer-to-materializer test starts
  without stored comparison metadata and verifies a real worktree uses the
  resolved upstream OID and PR head.
- A PR with the same number and head branch from another repository is rejected
  for a legacy attachment. Contribution PRs validate their head against
  `SourceRepository` while keeping the attached base repository as target.
- Invalid or ambiguous known associations and known cross-repository lookup
  failures cannot fall back to a bare stored branch. Ordinary provider outages
  retain the existing best-effort behavior.
- Explicit manual base selections are marked atomically and provider refresh
  cannot replace them. Explicit comparison-target association remains the
  operation that clears the marker.

## Review revalidation (2026-09-23)

- The post-review affected-package run passed every package except
  `internal/task/service`. It exposed that a same-branch manual choice
  persisted its marker but also triggered unchanged-base side effects. The
  service now skips those side effects when the metadata marker is the only
  change.
- Focused base-selection and comparison-target service regressions passed, and
  `go test ./internal/task/service -count=1` passed after the correction.
- The other packages in the affected-package run passed:
  `internal/common/gitbase`, `internal/worktree`,
  `internal/agent/runtime/lifecycle`, `internal/agent/runtime/agentctl`,
  `internal/agentctl/server/api`, `internal/agentctl/server/process`,
  `internal/github`, `internal/backendapp`, `internal/orchestrator`,
  `internal/orchestrator/executor`, `internal/task/repository/sqlite`, and
  `internal/task/handlers`.
- `make -C apps/backend build`: passed for agentctl targets, Kandev, mock-agent,
  acpdbg, and winjob.
- Current documentation validation passed: catalog (299 decisions and 1112
  specifications), 36 specification-linter tests, full specification lint, 62
  public-doc tests, and validation of 47 published pages.
- All 54 changed Go files passed `gofmt -l`; `git diff --check` passed.
- The workspace requirement is active and its system design is current. Review
  corrections and the final verification outcomes are recorded in all three
  work orders.

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

## PR #3878 fixup verification (2026-09-23)

- Repository identity normalization now accepts the stored public GitHub
  provider host (`https://github.com`) as well as the bare host while rejecting
  custom ports, URL paths, and lookalike hosts. The regression failed before
  the change and passed after it.
- The full GitHub-URL task creation E2E spec passed (9 tests), including local
  and worktree launch, missing-snapshot failure, and independent worktrees.
  Five selected PR watcher auto-start and cleanup E2E tests passed.
- A plugin-tooltip E2E test failed once in the combined run and passed in an
  isolated rerun. It is outside this change; no plugin code changed.
- `make -C apps/backend build e2e-plugin-package` passed. Changed-code
  `golangci-lint` reported 0 issues. The affected package suite passed across
  eight backend packages, and the complete executor package passed after its
  process-global metric assertion was made tolerant of unrelated asynchronous
  increments while retaining exact session-specific warning assertions.
- `git diff --check` passed. The GH CLI request contract, legacy linked-PR
  producer-to-materializer flow, fork identity rejection, contribution source
  validation, and base/head OID reuse regressions passed.
