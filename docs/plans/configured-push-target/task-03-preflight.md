---
id: "03-preflight"
title: "Push preflight validates the resolved destination"
status: completed
wave: 3
depends_on: ["02-push-path"]
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-003
acceptance_criteria:
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.1
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.2
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.3
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.4
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.5
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.6
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-003.7
system_design:
  - ../../specs/workspaces/system-design/configured-push-target.md
---

# Task 03: Push preflight validates the resolved destination

## Summary

Replace push preflight's blanket success on the ordinary path with a real
dry-run against the destination the equivalent push would use. Preflight reuses
Task 02's resolution, guard, and refusal ordering, verifies the expected branch
once, and reports the remote and branch it validated whenever contribution
routing did not apply.

## In scope

- Replacing the `g.remoteContribution == nil` early `result.Success = true`
  return with a resolved-remote dry-run, defaulting to `origin`.
- Reporting `push_no_remote_configured` when there is no explicit target, no
  contribution routing, and no `origin` remote.
- Applying the same refusal ordering and codes as the push path, with a single
  expected-branch verification and no reachable
  `push_branch_mismatch_after_baseline`.
- Reporting `pushed_remote` and `pushed_branch` on every successful preflight to
  which contribution routing did not apply, with or without an explicit target.
- Reporting a remote's write refusal as a failure carrying the remote's output
  rather than an unhandled error.

## Out of scope

- The contribution-destination and remote-contribution preflight branches,
  which keep their bodies, their result shape, and their output handling.
- Preflighting the empty-remote baseline refspec. Preflight exercises the
  destination-branch refspec only.
- Any change to the startup remote-contribution preflight caller in
  `internal/agent/runtime/lifecycle/manager_startup.go`.

## Acceptance

- Preflight with no explicit target and no contribution routing dry-runs against
  `origin` for the current branch and reports the remote and branch it checked.
- Preflight against a remote that refuses the write reports failure with the
  remote's output, and creates, moves, updates, or deletes no ref anywhere.
- A checkout with no `origin` and no contribution routing reports
  `push_no_remote_configured` instead of success.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process/... ./internal/agent/runtime/lifecycle/...
cd apps/backend && make lint
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_push_target.go`
- `apps/backend/internal/agentctl/server/process/git_push_preflight_test.go` (new)

## Dependencies

Task 02, whose `resolvePushPlan` this reuses.

## Risks

- This turns a previously inert call into one that contacts a remote. The
  startup caller is unaffected because it runs only when a contribution binding
  exists, but the behavior change is real for any other caller on that path.
- A dry-run is still a `push` subcommand, so it must not be allowed to mutate.
  Assert absence of ref movement directly in the test rather than trusting
  `--dry-run`.

## Parallelism

`sequential`

## Inputs

- Requirement REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-003 and the system design's
  [Preflight](../../specs/workspaces/system-design/configured-push-target.md) section.
- `PushPreflight` at `internal/agentctl/server/process/git.go`.
- The startup caller at `internal/agent/runtime/lifecycle/manager_startup.go`.

## Results

Implemented. `PushPreflight` reuses `validateContributionState` and
`resolvePushPlan`, passing `requireConfiguredRemote: true` so a checkout with no
`origin` and no contribution routing reports `push_no_remote_configured` instead
of success. The blanket `result.Success = true` return is gone; every
non-routed preflight now dry-runs the destination-branch refspec and reports the
remote and branch it validated. A preflight under contribution routing keeps its
existing body and result shape.

Preflight verifies the expected branch once, through `resolvePushPlan`, and
never calls `verifyExpectedBranch`, so `push_branch_mismatch_after_baseline` is
unreachable from preflight. `TestPushPreflightAppliesSameRefusalOrdering`
asserts that directly.

`TestPushPreflightMutatesNothing` checks `HEAD`, the full local ref list,
upstream configuration, and both remotes across a preflight rather than trusting
`--dry-run`.

Verified with `go test -tags fts5 ./internal/agentctl/server/process/...
./internal/agent/runtime/lifecycle/...` and `make -C apps/backend lint`.
