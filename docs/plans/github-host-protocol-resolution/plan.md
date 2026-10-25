---
created: 2026-08-27
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
system_design:
  - ../../specs/integrations/system-design/github-authentication-01.md
  - ../../specs/integrations/system-design/github-authentication-02.md
  - ../../specs/integrations/system-design/github-authentication-03.md
legacy_specs: []
---

# Fix Plan: Resolve GitHub Clone Protocol Per Host At Use Time

## Overview

Replace the backend-startup Git protocol snapshots with one host-aware resolver. Evaluate this
resolver when Kandev constructs a clone URL or an executor-inherited managed-checkout origin. Then
route the long-lived cloner and review-repository adapter through it. The tests prove that all paths
observe configuration changes without a restart.

## Confirmed root cause

`repoclone.DetectGitProtocol` runs only `gh config get git_protocol`. Thus, a global `https` value
wins when `gh config get -h github.com git_protocol` reports the effective `ssh` preference.
`backendapp/main.go` passes that incorrect startup result into one long-lived `repoclone.Cloner`.
`backendapp/orchestrator.go` stores a second startup result on `repositoryResolverAdapter`.
Executor-inherited origin reconciliation selects the wrong transport. It cannot observe later `gh`
configuration changes until the backend restarts.

The smallest reproduction uses an isolated `GH_CONFIG_DIR`. Set global `git_protocol=https` and
host-specific `github.com` `git_protocol=ssh`. The current detector reads `https`.
`gh config get -h github.com git_protocol` reads `ssh`.

## Scope

### In scope

- Resolve the `github.com` clone protocol from host-specific configuration first. Use global
  configuration second. Use SSH by default when neither scope yields `ssh` or `https`.
- Resolve the protocol at clone URL construction and Local/Worktree launch or resume origin
  reconciliation, not backend startup.
- Remove the cached protocol field from `repositoryResolverAdapter` and route it through the same
  live resolver as `repoclone.Cloner`.
- Preserve canonical HTTPS for managed task credentials and preserve user-managed local origins.
- Add deterministic tests with injected command and protocol resolvers.

### Out of scope

- Executor-mode credential-helper or token bridging tracked by #3072.
- Discovering or validating the actor behind inherited SSH agents or credential managers.
- GitHub Enterprise Server or hosts other than the existing `github.com` contract.
- Frontend, API, persistence, schema, or workspace-policy changes.

## Technical approach

### Host-aware protocol resolver

- In `apps/backend/internal/repoclone/protocol.go`, make protocol resolution accept a context and a
  provider host. Query `gh config get -h <host> git_protocol` first. Then query
  `gh config get git_protocol`. Accept only `ssh` and `https`. Otherwise, return the existing SSH
  default. Bound the complete host-plus-global resolution operation with one five-second command
  context.
- Isolate command execution behind an injected runner. Unit tests control the results and assert
  command order without reading the developer's `gh` configuration. Keep one five-second command
  timeout for the complete resolution operation.

### Use-boundary resolution

- In `apps/backend/internal/repoclone/clone.go`, store an injected protocol resolver, not a resolved
  protocol string. Make protocol-aware URL construction resolve the normalized provider host on
  every call.
- In `apps/backend/internal/backendapp/main.go`, construct the cloner with the live resolver and
  remove the startup call to `DetectGitProtocol`.
- In `apps/backend/internal/orchestrator/executor/executor.go` and
  `executor_resume.go`, thread the operation context through the protocol-aware `RepoCloner` URL
  builder. The shared launch and resume path continues to reconcile only Kandev-managed GitHub
  checkouts. Managed mode remains explicitly HTTPS.
- In `apps/backend/internal/backendapp/orchestrator.go`, replace the adapter's concrete cached
  protocol field with a narrow cloner interface that builds the URL at `ResolveForReview` use time.
  Production wiring supplies the same `repoclone.Cloner`. Tests supply a deterministic fake.

## Tests

- `TestDetectGitProtocolPrefersHostConfiguration` in
  `apps/backend/internal/repoclone/protocol_test.go` is the primary regression. Before the
  correction, the command runner observes no host-specific lookup. The result is HTTPS.
  Companion cases cover global fallback, unsupported values, command failures, and SSH default.
- `TestClonerBuildCloneURLUsesCurrentProtocol` in
  `apps/backend/internal/repoclone/clone_test.go` mutates an injected resolver between calls and
  proves one long-lived cloner does not cache a result.
- `TestEnsureRepoLocalPathReevaluatesGitHubProtocol` in
  `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go` performs two
  preparation passes with different resolver results. It proves that the managed origin changes
  without executor recreation. Existing managed-HTTPS and user-managed-local cases cover the
  exclusions.
- `TestRepositoryResolverAdapterUsesCurrentGitProtocol` in
  `apps/backend/internal/backendapp/adapters_test.go` performs two resolutions through an injected
  fake and proves the adapter has no independent startup cache.

## Execution checklist

- [x] Audit the current plan, specifications, work order, and working tree.
- [x] Implement the live host-aware protocol resolver and use-time URL construction.
- [x] Update executor origin reconciliation and review-repository wiring.
- [x] Add and pass the specified regression tests.
- [x] Update plan/work-order verification results.
- [x] Commit with hooks, push the branch, and create a ready-for-review PR.
- [x] Wait 15 minutes, run PR fixup, and report the final PR state.

## Work orders

- [x] [Task 01: Resolve GitHub clone protocol at use time](task-01-resolve-github-clone-protocol.md)

## Verification results

- `rtk go test -race ./internal/repoclone ./internal/orchestrator/executor ./internal/backendapp -run 'Test(DetectGitProtocol|ClonerBuildCloneURLUsesCurrentProtocol|EnsureRepoLocalPathReevaluatesGitHubProtocol|RepositoryResolverAdapterUsesCurrentGitProtocol)' -count=1` passed with 11 tests in 3 packages, including the total-timeout regression.
- `rtk go test -race ./internal/repoclone ./internal/orchestrator/executor ./internal/backendapp` passed with 1,316 tests in 3 packages.
- PR 3078 at `662d3c98d3ddd560ff46cab83e2d8cf965571f65` completed with 44 checks passed, 0 failed, 0 pending, and 0 unresolved review threads. GitHub reports `MERGEABLE / CLEAN`.

## Risks

- Adding context to protocol-aware URL construction touches executor test fakes. All implementations
  must preserve the same host and operation context.
- A `gh` lookup now occurs on each relevant URL construction. One five-second bound covers the
  complete host-plus-global resolution, and the shared `gh` subprocess throttle limits the cost.
  Correctness requires no process cache.
- Provider-host normalization must pass `github.com`, not an HTTPS URL, to `gh --host`. Clone URL
  construction must retain the canonical persisted provider origin.
