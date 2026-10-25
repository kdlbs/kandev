---
created: 2026-09-08
status: implemented
requirements:
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-001
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-002
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-003
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-004
system_design:
  - ../../specs/workspaces/system-design/configured-push-target.md
legacy_specs: []
---

# Implementation Plan: Configured Push Target

## Overview

The workspace push contract can publish only to `origin` or to a remote that a
contribution binding selects, and push preflight reports success without
contacting a remote at all on the ordinary path. This work package adds two
optional inputs to the push and push-preflight contracts, an explicit push
target and an expected branch, and makes preflight validate the destination it
would actually use.

Implementation runs in four work orders. Task 01 lands the vocabulary the rest
of the package needs: the request options carried into the operator, the eleven
error codes, the five result fields, and the pure resolution and validation
helpers, with no behavior change. Task 02 rewires the push path onto those
helpers, which is where the refusal ordering and the two-point expected-branch
verification take effect. Task 04 carries the two inputs across the HTTP
surface, the runtime client, and the backend action handler; it touches files
disjoint from Task 02 and depends only on Task 01's signatures, so the two run
in the same wave. Task 03 lands last because preflight consumes the shared
resolution and guard call sites that Task 02 establishes.

This order exists so that no work order changes a destination decision and its
transport in the same pass: the operator owns every new decision, and the
transport work carries values it does not interpret.

## Scope

### In scope

- An optional explicit push target on the push and push-preflight contracts,
  accepted as a configured remote name or as a remote URL resolved against
  already-configured remotes.
- An optional expected branch on both contracts, verified twice on the push
  path and once on preflight.
- The fixed refusal ordering and the eleven stable error codes.
- Push preflight validating the destination-branch refspec against the resolved
  remote, including the `origin` path that reports blanket success today.
- The five optional result fields, with the push/preflight asymmetry the design
  requires.
- Transport plumbing for both inputs across agentctl HTTP, the runtime client,
  and the backend workspace Git action handler.

### Out of scope

- Exact-OID `--force-with-lease=<ref>:<oid>`. Force keeps emitting the bare
  lease and stays refused under contribution routing.
- Auto-push policy. This package only makes such a caller expressible.
- Creating, renaming, retargeting, or deleting a remote.
- Any change to remote-contribution or contribution-destination routing, their
  force refusal, or their existing preflight behavior.
- Any web, desktop, or mobile control. The existing push control keeps sending
  neither input, so this package ships no user-visible surface.
- Preflighting the empty-remote baseline refspec.
- Pushing tags, multiple refspecs, or deleting a remote branch.

## Technical approach

### Operator primitives (`internal/agentctl/server/process`)

A new `git_push_target.go` owns everything the resolution and guard need, so the
decision logic sits in one file rather than growing `Push`:

- `PushOptions{Force, SetUpstream bool; Remote, ExpectedBranch string}` replaces
  the two positional booleans on `GitOperator.Push` and is added to
  `GitOperator.PushPreflight`, which takes no options today.
- Error-code constants for the eleven codes the design tables, alongside the
  existing `emptyRemote*ErrorCode` constants.
- `configuredRemotes(ctx)` enumerates names with `git remote`, then reads each
  name's `--get-all remote.<name>.pushurl` and `--get-all remote.<name>.url`.
  The effective push URL set is the push URL list when non-empty, otherwise the
  single-element fetch URL list, otherwise empty. A read failure yields
  `push_remote_config_unreadable` rather than an absent or unmatched target.
- `resolvePushTarget` applies the trim, discriminates name from URL with
  `securityutil.IsValidBranchName`, and returns a remote **name**. A
  caller-supplied URL never reaches a Git command line, which is what keeps
  `runGitCommand`'s existing argument allowlist unchanged.
- `currentBranch(ctx)` reads `git symbolic-ref HEAD` and trims `refs/heads/`.
  A non-zero exit is the detached state, reported as an empty branch. This
  replaces `getCurrentBranch`'s `rev-parse --abbrev-ref HEAD` on the new paths,
  which returns the literal `HEAD` when detached and cannot satisfy
  AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.11. `symbolic-ref HEAD` needs no
  flag and `HEAD` is already a safe literal, so the allowlist is untouched.

### Expected-branch validation (`internal/common/securityutil`)

The strict allowlist the expected branch must pass is `IsValidBranchName` plus
a reject on a trailing `/`, on `//`, and on the four symbolic pseudo-refs. The
existing `IsValidDefaultBranchName` composes exactly those checks but reaches
them through `IsValidBaseBranchRef`, which first strips an `origin/` prefix, so
an expected branch of `origin/main` would validate as `main` and pass. A new
`IsValidExpectedBranchName` applies the same checks without that strip.

### Push path (`Push`, `internal/agentctl/server/process/git.go`)

`Push` gains a `resolvePushPlan` step that evaluates the whole refusal list
before any remote is contacted, then keeps the existing structure. Two ordering
changes are load-bearing:

- Empty-remote first publication runs only when the resolved remote is `origin`
  and no contribution routing applies.
- `getUpstreamRef` is read today after first publication and inside the argument
  build. It moves ahead of the second expected-branch verification, so that the
  branch read is the last Git command before the push
  (AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.12).

Refspec selection is `HEAD:refs/heads/<expected>` with an explicit target and an
expected branch, `HEAD:refs/heads/<current>` with an explicit target alone, and
unchanged on every path that names no target. The `HEAD:refs/heads/` form is
already admitted by `runGitCommand`.

### Preflight (`PushPreflight`)

The contribution-destination and remote-contribution branches keep their current
bodies and result shape. The `g.remoteContribution == nil` early `result.Success
= true` return is replaced by the resolved-remote dry-run, defaulting to
`origin`, reporting `push_no_remote_configured` when no explicit target, no
contribution routing, and no `origin` remote exist. A successful preflight to
which contribution routing did not apply reports `pushed_remote` and
`pushed_branch`.

### Transport

- `internal/agentctl/server/api/git.go`: `GitPushRequest` and
  `GitPushPreflightRequest` gain `remote` and `expected_branch`, bound and
  forwarded without interpretation.
- `internal/agent/runtime/agentctl/git.go`: both result structs gain the five
  fields; `GitPush` and `GitPushPreflight` carry the two inputs.
- `internal/agent/handlers/git_handlers.go`: the `worktree.push` payload gains
  both fields and forwards them.
- `internal/agent/runtime/lifecycle/manager_startup.go` keeps passing neither,
  so the startup remote-contribution preflight is unchanged.

### Output redaction

`runGitCommand` already routes every `push` invocation, dry-run included,
through `sanitizeGitPushOutput`, so the redaction
AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.18 requires is satisfied by
construction on both new paths. No second sanitization is added; see
[Open questions](#open-questions).

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| 001.1, 004.7 | `TestResolvePushTargetTrimsAndTreatsEmptyAsAbsent` in `internal/agentctl/server/process/git_push_target_test.go` |
| 001.3, 001.15 | `TestResolvePushTargetDiscriminatesNameFromURL`, `TestResolvePushTargetRejectsCaseAndPrefixVariants` in `git_push_target_test.go` |
| 001.4 | `TestResolvePushTargetReportsUnknownRemoteName` in `git_push_target_test.go` |
| 001.5, 001.6 | `TestResolvePushTargetMatchesSingleEntryPushURL`, `TestResolvePushTargetPicksFirstByByteOrder` in `git_push_target_test.go` |
| 001.7 | `TestResolvePushTargetReportsUnmatchedURL` in `git_push_target_test.go` |
| 001.16 | `TestResolvePushTargetReportsUnreadableRemoteConfig` in `git_push_target_test.go` |
| 001.17 | `TestResolvePushTargetReportsFanoutRemote`, `TestGitOperatorPushNamedFanoutRemotePublishesToEveryPushURL` in `git_push_target_test.go` |
| 002.2 | `TestIsValidExpectedBranchName` in `internal/common/securityutil/git_test.go` |
| 002.11 | `TestGitOperatorCurrentBranchReportsDetachedHeadAsEmpty` in `git_push_target_test.go` |
| 001.2 | `TestGitOperatorPushWithoutOptionsIsUnchanged` in `internal/agentctl/server/process/git_push_expected_branch_test.go` |
| 001.8 | `TestGitOperatorPushNeverCreatesRemote` in `git_push_expected_branch_test.go` |
| 001.9, 001.10 | `TestGitOperatorPushRefusesTargetUnderContributionRouting`, `TestGitOperatorPushRefusesTargetWithSetUpstream` in `git_push_expected_branch_test.go` |
| 001.11, 001.12 | `TestGitOperatorPushToNamedRemoteLeavesUpstreamUnset`, `TestGitOperatorPushToNamedRemoteSkipsBaselinePublication` in `git_push_expected_branch_test.go` |
| 001.13 | `TestGitOperatorPushToNamedRemoteStaysNonForce` in `git_push_expected_branch_test.go` |
| 001.14 | `TestGitOperatorPushResultAndLogsCarryNoRemoteURL` in `git_push_expected_branch_test.go` |
| 001.18 | `TestGitOperatorPushRedactsCredentialsOnExplicitTargetPath` in `git_push_expected_branch_test.go`; `TestPushPreflightRedactsCredentialsInRemoteOutput` in `git_push_preflight_test.go` |
| 002.1, 002.9 | `TestGitOperatorPushExpectedBranchAloneOnlyGates` in `git_push_expected_branch_test.go` |
| 002.3, 002.4 | `TestGitOperatorPushRefusesExpectedBranchMismatch`, `TestGitOperatorPushRefusesDetachedHeadAsMismatch` in `git_push_expected_branch_test.go` |
| 002.5, 002.12 | `TestGitOperatorPushVerifiesBranchAsLastReadBeforePush` in `git_push_expected_branch_test.go` |
| 002.6 | `TestGitOperatorPushReportsMismatchAfterBaselinePublication` in `git_push_expected_branch_test.go` |
| 002.7, 002.8, 002.10 | `TestGitOperatorPushPublishesHeadToExpectedBranch`, `TestGitOperatorPushRefusesDetachedHeadWithTargetOnly`, `TestGitOperatorPushCreatesMissingDestinationBranch` in `git_push_expected_branch_test.go` |
| 003.1, 003.7 | `TestPushPreflightAppliesSameRefusalOrdering`, `TestPushPreflightReportsValidatedRemoteAndBranch` in `internal/agentctl/server/process/git_push_preflight_test.go` |
| 003.2, 003.3 | `TestPushPreflightValidatesNamedRemote`, `TestPushPreflightValidatesOriginWithoutTarget` in `git_push_preflight_test.go` |
| 003.4 | `TestPushPreflightReportsNoRemoteConfigured` in `git_push_preflight_test.go` |
| 003.5 | `TestPushPreflightMutatesNothing` in `git_push_preflight_test.go` |
| 003.6 | `TestPushPreflightReportsRemoteRefusalOutput` in `git_push_preflight_test.go` |
| 004.1, 004.2 | `TestGitOperatorPushRefusalOrdering`, `TestGitOperatorPushRefusalsAreNoOps` in `git_push_expected_branch_test.go` |
| 004.3 | `TestGitOperatorPushReportsOperationInProgress` in `git_push_expected_branch_test.go` |
| 004.4, 004.5 | `TestGitOperatorPushRepeatedIdenticalRequestSucceeds`, `TestGitOperatorPushDoesNotEscalateRejectedNonForcePush` in `git_push_expected_branch_test.go` |
| 004.6 | `TestGitOperatorPushOmitsTargetFieldsWithoutExplicitTarget` in `git_push_expected_branch_test.go` |
| 004.8 | `TestGitPushRequestScopesTargetToSelectedRepo` in `internal/agentctl/server/api/git_handlers_test.go` |
| 001.1, 002.1 transport | `TestHandleGitPushBindsRemoteAndExpectedBranch` in `internal/agentctl/server/api/git_handlers_test.go`; `TestGitPushSendsPushOptions` in `internal/agent/runtime/agentctl/git_test.go`; `TestWsPushForwardsPushOptions` in `internal/agent/handlers/git_handlers_test.go` |

## E2E tests

None. The design places every web, desktop, and mobile surface out of scope,
and the existing push control keeps sending neither input, so this package adds
no user-visible behavior for a browser test to exercise. Evidence is the Go
unit and integration coverage above, which drives real Git repositories through
the existing `setupEmptyRemoteTaskRepo` fixture pattern.

## Work orders

- [x] [Task 01: Push-target and expected-branch primitives](task-01-resolution-primitives.md)
- [x] [Task 02: Explicit push target and expected-branch verification on the push path](task-02-push-path.md)
- [x] [Task 03: Push preflight validates the resolved destination](task-03-preflight.md)
- [x] [Task 04: Carry both inputs across the push transport](task-04-transport.md)

Dependency order: Task 01, then Tasks 02 and 04 together, then Task 03.

## Verification results

All four work orders are implemented and verified on the neo SSH runner.

- `make -C apps/backend lint` — 0 issues.
- `go test -tags fts5 ./internal/agentctl/server/process/...` — all push, preflight,
  resolution and empty-remote tests pass.
- `go test -tags fts5 ./internal/agentctl/server/api/... ./internal/agent/runtime/agentctl/
  ./internal/agent/handlers/... ./internal/agent/runtime/lifecycle/...
  ./internal/common/securityutil/...` — all new and existing push tests pass.

Four tests fail identically on this runner both with these changes and on a
clean worktree at `HEAD`, so they are pre-existing and environment-specific, not
regressions: `TestRepairManagedRuntimeCacheClearsPreviousStderr`,
`TestManagedRuntimeCacheRepairUsesAgentEnvironmentAndExactTree`, the Kubernetes
PVC and `WorktreePreparer` cases in `internal/agent/runtime/lifecycle`, and
`TestBuildAuthMethodsIdentityAgentOverridesEnvironment`.
`TestWorkspaceGitAdmissionWaitDoesNotConsumeTimeout` is load-sensitive and fails
under parallel suite runs on clean `HEAD` too.

Two ordering guarantees are proved directly rather than by inspection. A PATH
shim records every git invocation, so
`TestGitOperatorPushVerifiesBranchAsLastReadBeforePush` asserts that
`symbolic-ref HEAD` is the command immediately preceding `push` on both the
explicit-target and default paths. A second shim moves `HEAD` during
empty-remote first publication, so
`TestGitOperatorPushReportsMismatchAfterBaselinePublication` exercises the
second-verification window that the first verification cannot cover.

## Risks

- `Push` is already long and carries contribution routing, force refusal, and
  empty-remote publication. Adding resolution and two verification points
  in-line would breach the backend function-length limit; the plan therefore
  puts resolution and the guard in `git_push_target.go` and has `Push` call a
  single `resolvePushPlan`.
- Moving `getUpstreamRef` ahead of the second verification changes the order of
  existing Git reads on the path that names no target. The existing
  `git_empty_remote_test.go` cases are the regression net for that path and must
  stay green unmodified.
- Replacing preflight's blanket `result.Success = true` makes a previously
  inert call contact `origin`. The startup remote-contribution preflight is not
  affected because it runs only when a binding exists, but any future caller on
  the ordinary path now performs network I/O.
- A configured remote whose name fails `IsValidBranchName` cannot be addressed
  by the name form and is skipped during URL matching. This is consistent in
  both directions, but it means such a remote is unreachable through this
  contract.
- The five result fields are `omitempty`, so `current_branch` reported as empty
  for a detached `HEAD` is omitted from the JSON rather than present and empty.
  Consumers must read a missing key as the detached state.

## Resolved questions

- The system design's Security section claimed `Push` assigns Git's output to
  the result without sanitizing it. `runGitCommand` in fact sanitizes every
  invocation whose subcommand is `push`, dry-run included, so
  AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.18 was already satisfied on both new
  paths and no new redaction call was needed. The design's Security section has
  been corrected to describe the existing behavior, and
  `TestGitOperatorPushRedactsCredentialsOnExplicitTargetPath` plus
  `TestPushPreflightRedactsCredentialsInRemoteOutput` guard it against
  regression.
- The plan proposed `IsValidExpectedBranchName` because
  `IsValidDefaultBranchName` strips an `origin/` prefix. Implementation
  confirmed the two disagree on inputs such as `origin/-dash`, and the new
  validator checks the value that is actually used in the comparison and the
  refspec. `TestIsValidExpectedBranchName` pins the distinguishing case.
