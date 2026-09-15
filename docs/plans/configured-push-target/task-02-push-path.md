---
id: "02-push-path"
title: "Explicit push target and expected-branch verification on the push path"
status: completed
wave: 2
depends_on: ["01-resolution-primitives"]
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-001
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-002
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-004
acceptance_criteria:
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.2
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.8
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.9
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.10
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.11
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.12
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.13
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.14
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.18
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.3
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.4
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.5
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.6
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.7
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.8
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.9
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.10
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.12
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.1
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.2
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.3
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.4
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.5
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.6
system_design:
  - ../../specs/workspaces/system-design/configured-push-target.md
---

# Task 02: Explicit push target and expected-branch verification on the push path

## Summary

Wire `GitOperator.Push` onto Task 01's primitives: evaluate the fixed refusal
list before contacting any remote, skip empty-remote first publication for a
resolved remote other than `origin`, and verify the expected branch twice, the
second time as the last Git read before the push command. A request that names
neither input performs the same push it performs today.

## In scope

- A `resolvePushPlan` helper that evaluates the refusal list in the design's
  fixed order and yields the resolved remote, the refspec, and the
  set-upstream decision.
- Refusing `push_remote_contribution_conflict` and
  `push_remote_upstream_unsupported` before any remote read.
- Restricting empty-remote first publication to the `origin` path.
- Moving `getUpstreamRef` ahead of the second verification so the branch read is
  the last Git command before the push.
- The second verification, reporting `push_branch_mismatch_after_baseline` with
  `baseline_published` set when first publication published a baseline in this
  request, and `push_branch_mismatch` otherwise.
- Refspec selection: `HEAD:refs/heads/<expected>` with a target and an expected
  branch, `HEAD:refs/heads/<current>` with a target alone, unchanged otherwise.
- Populating `pushed_remote` and `pushed_branch` on a successful push only when
  the request carried an explicit target.
- Extending the push completion log with the resolved remote, destination
  branch, and whether an expected branch was supplied and matched.

## Out of scope

- `PushPreflight`, which Task 03 owns even though it reuses `resolvePushPlan`.
- Transport plumbing, which Task 04 owns.
- Contribution routing behavior, its force refusal, and the empty-remote
  publication mechanics themselves, all of which stay as they are.
- Adding a second output sanitization. `runGitCommand` already redacts every
  `push` invocation.

## Acceptance

- A push naming a configured remote other than `origin` publishes
  `HEAD:refs/heads/<branch>` to it, sets no upstream, and publishes no baseline.
- An expected branch that does not match the current branch, including a
  detached `HEAD`, refuses the push with the expected and current branches
  reported and no remote contacted.
- A request naming neither input produces the same Git commands and the same
  result shape as before this task, with `git_empty_remote_test.go` unmodified
  and green.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process/...
cd apps/backend && make lint
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_push_target.go`
- `apps/backend/internal/agentctl/server/process/git_push_expected_branch_test.go` (new)

## Dependencies

Task 01.

## Risks

- `Push` is near the backend function-length limit already. Keep
  `resolvePushPlan` and the second verification in `git_push_target.go` rather
  than growing `Push` in place.
- The second verification can follow a completed baseline publication, so the
  refusal is deliberately not side-effect-free. Do not add a rollback; report
  `baseline_published` instead.
- Reordering the upstream read is observable to any test that asserts Git
  command sequence on the `origin` path. Existing empty-remote tests must pass
  unmodified; if one fails, the reorder is wrong rather than the test.

## Parallelism

`parallel-safe`

## Inputs

- The system design's [Control flow](../../specs/workspaces/system-design/configured-push-target.md)
  and [Refusal ordering](../../specs/workspaces/system-design/configured-push-target.md) sections.
- `Push` at `internal/agentctl/server/process/git.go`, and
  `prepareEmptyRemotePublication` in `git_empty_remote.go`.
- `git_empty_remote_test.go` as the regression net for the unchanged path.

## Results

Implemented. `Push` now calls `validateContributionState`, then
`resolvePushPlan`, which evaluates the whole refusal list before any remote is
contacted. Empty-remote first publication is gated on `plan.baselineEligible`,
so it runs only for `origin` with no contribution routing. `resolveSetUpstream`
moves the upstream read ahead of the second verification and skips it entirely
on the explicit-target path.

The ordering guarantee is proved rather than asserted: a PATH shim records every
git invocation, and `TestGitOperatorPushVerifiesBranchAsLastReadBeforePush`
checks that `symbolic-ref HEAD` immediately precedes `push` on both the
explicit-target and default paths. A second shim moves `HEAD` during first
publication so
`TestGitOperatorPushReportsMismatchAfterBaselinePublication` exercises the
post-baseline refusal, confirming `push_branch_mismatch_after_baseline`,
`baseline_published`, and that the task branch stays unpublished while the
baseline remains.

The existing `git_empty_remote_test.go` cases pass unmodified, which is the
regression evidence that the path naming no target is unchanged.

Verified with `go test -tags fts5 ./internal/agentctl/server/process/...` and
`make -C apps/backend lint`.
