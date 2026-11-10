---
id: "01-host-qualified-pr-base"
title: "Host-qualified PR base identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-001
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-003
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-004
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.5
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.1
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.2
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.4
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.4
system_design:
  - ../../specs/integrations/system-design/github-enterprise-hosts.md
---

# Task 01: Host-qualified PR base identity

## Summary

Make PR-task launch compare host-qualified repository identities so that GHES
and GHE.com PRs resolve their base, and PRs from another host fail closed.

## In scope

- `apps/backend/internal/orchestrator/executor/executor_resume.go`:
  `githubComparisonRepositoryFromRepository` and
  `normalizeGitHubComparisonRepository` use `githubhost.Parse` and preserve the
  canonical authority and a host-derived `RemoteURL`. Remove
  `isGitHubComparisonHost`. An empty `ProviderHost` stays github.com.
- `apps/backend/internal/github/service_pr.go`: `GetPRForAutomation` returns a
  copy stamped with `GitHubHost = resolved.host().String()`. It must not mutate
  a shared or cached value.
- `apps/backend/internal/backendapp/orchestrator.go`: `githubPRBaseFromPR` and
  `githubComparisonRepository` take the host from `pr.GitHubHost`. Empty means
  github.com, and an invalid value returns an error.

- Review remediation: preserve the live head node ID in transient `PRBase`
  data and normalize verified aliases during executor identity checks. Keep
  persisted node-ID targets and numeric repository bindings compatible.

## Out of scope

Contribution credential scopes, PR URL classification, change-link host
checks, schema, frontend, and public documentation. See the plan's exclusions.

## Regression tests (write first, confirm failure)

Executor (`executor_pr_base_identity_test.go` is about 600 lines, so put new
tests in a new `executor_pr_base_enterprise_test.go`):

- `TestGitHubComparisonRepositoryFromRepositoryPreservesEnterpriseHost`:
  `https://git.example.com:8443` and `https://tenant.ghe.com` keep their
  authority and produce a matching `RemoteURL`. It fails today with `ok=false`.
- `TestResolveTaskRepoInfo_EnterprisePRResolvesBase`: a GHES repository row
  with `pr_number` metadata and a resolver result on the same host launches
  without error and adopts the live target branch. It fails today with the
  reported error.
- `TestResolveTaskRepoInfo_RejectsPRFromOtherHost`: a GHES repository and a
  resolver result with the same owner, name, and number on github.com fail
  with a mismatch reason other than `repository_identity_missing`.
- Update `TestNormalizeGitHubComparisonRepositoryRejectsOtherHosts`: keep the
  port-on-github.com and path cases rejected. Move `github.com.evil` to an
  assertion that it normalizes to its own host and does not equal the
  github.com identity.

Provider:

- `apps/backend/internal/backendapp/pr_base_resolver_test.go`:
  `TestPRBaseIdentityConversionUsesFetchedHost` covers GHES, GHE.com, empty
  (github.com), and invalid (error) values of `pr.GitHubHost`.
- `apps/backend/internal/github/service_routing_hosts_test.go` (or a new file
  if it exceeds the size limit):
  `TestGetPRForAutomationStampsResolvedHost` checks that an Enterprise
  connection returns `GitHubHost == "git.example.com"` and that the
  client-returned PR value is unchanged.

## Acceptance

1. Each new regression fails before the correction for the stated reason and
   passes after it.
2. Existing github.com, fork, contribution, and legacy empty-host executor and
   resolver tests pass unchanged, except for the documented `github.com.evil`
   assertion change.
3. No public-host constant remains in the PR base identity path
   (`executor_resume.go` identity helpers, `githubComparisonRepository`).

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator/executor -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run 'PRBase' -count=1)
(cd apps/backend && go test -trimpath ./internal/github -run 'GetPRForAutomation' -count=1)
bash scripts/lint-go-changed $(git diff --name-only -- '*.go')
git diff --check
```

## Likely files

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_pr_base_identity_test.go`
- `apps/backend/internal/orchestrator/executor/executor_pr_base_enterprise_test.go` (new)
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/service_routing_hosts_test.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/pr_base_resolver_test.go`

## Results

Done on 2026-10-28.

- Red first: `TestResolveTaskRepoInfo_EnterprisePRResolvesBase` failed with the
  reported `pull request identity did not match the task repository binding`.
  The host-preservation, lookalike-host, conversion, and stamping tests failed
  on the public-host assumption.
- Executor identity helpers now use `githubhost.Parse` and keep the canonical
  authority. `isGitHubComparisonHost` was removed. `GetPRForAutomation`
  returns a host-stamped copy, and `githubPRBaseFromPR` builds both
  repositories from that host. An empty value means github.com, and an invalid
  value fails the conversion.
- New tests are in `executor_pr_base_enterprise_test.go`,
  `pr_base_enterprise_test.go`, and `service_pr_automation_host_test.go`. The
  `github.com.evil` case moved from the rejection test to a distinctness test.
- `go test ./internal/backendapp -run PRBase`, `./internal/github -run
  'GetPRForAutomation|Enterprise|Comparison'`, and `lint-go-changed` (0 issues)
  pass.
- `go test ./internal/orchestrator/executor` passes except for checkout-recovery
  integration tests. Those tests fail identically on unmodified `origin/main` in
  this macOS environment: `/var` symlinked temp paths are rejected as unsafe,
  and three still fail with a resolved `TMPDIR`. They do not exercise PR base
  identity. CI is the authority for them.

## PR review remediation

- Corrected the requirement references for legacy GitHub.com acceptance.
- Reproduced the node-ID versus numeric-ID mismatch at conversion and launch
  boundaries before the fix. Verified aliases now compare as the same live
  head repository. Unknown aliases and cross-host identities remain rejected.
- Added coverage for unchanged stored targets, absence of live alias evidence,
  remote-contribution launch, and exclusion of live aliases from JSON.
- Public documentation already describes Enterprise support. This correction
  restores that documented behavior without adding configuration or API fields.
- Linux verification passed: the complete executor package and the focused
  `PRBase|GetPRForAutomation|Enterprise|Comparison` tests in `backendapp`,
  `github`, and `task/models`. The executor suite took 152 seconds.
- Documentation coverage, catalog validation, and specification lint passed.
- Exact-head CI, automated review disposition, and merge remain pending.
