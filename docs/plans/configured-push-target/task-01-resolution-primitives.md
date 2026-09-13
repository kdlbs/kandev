---
id: "01-resolution-primitives"
title: "Push-target and expected-branch primitives"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-001
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-002
  - REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-004
acceptance_criteria:
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.3
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.4
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.5
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.6
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.7
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.15
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.16
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-001.17
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.2
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-002.11
  - AC-WORKSPACES-CONFIGURED-PUSH-TARGET-004.7
system_design:
  - ../../specs/workspaces/system-design/configured-push-target.md
---

# Task 01: Push-target and expected-branch primitives

## Summary

Add the vocabulary and pure helpers the rest of the package builds on: a
`PushOptions` request struct, the eleven error-code constants, the five result
fields, remote enumeration, push-target resolution, strict expected-branch
validation, and a symbolic-ref current-branch read. Push and preflight take the
new options and ignore them, so this work order changes no observable behavior.

## In scope

- `PushOptions{Force, SetUpstream bool; Remote, ExpectedBranch string}` in
  `internal/agentctl/server/process`, threaded through `GitOperator.Push` and
  `GitOperator.PushPreflight` as a pass-through with behavior unchanged.
- The eleven error-code constants from the system design's table.
- `PushedRemote`, `PushedBranch`, `ExpectedBranch`, `CurrentBranch`, and
  `BaselinePublished` on `process.GitOperationResult`, all `omitempty`.
- `configuredRemotes`, which reads each remote's effective push URL set and
  distinguishes a genuine read failure from an absent remote.
- `resolvePushTarget`, returning a remote name or one of the four resolution
  refusals, and never placing a caller-supplied URL on a Git command line.
- `currentBranch`, reading `git symbolic-ref HEAD` so a detached `HEAD` is an
  empty branch rather than the literal `HEAD`.
- `securityutil.IsValidExpectedBranchName`, the strict allowlist without
  `IsValidBaseBranchRef`'s `origin/` prefix strip.

## Out of scope

- Any change to how `Push` or `PushPreflight` behave. The options are accepted
  and ignored here; Tasks 02 and 03 own the behavior.
- The runtime client and backend handler result structs, which Task 04 owns.
- Widening `runGitCommand`'s argument allowlist. `symbolic-ref HEAD` and
  `config --get-all` are already admitted; if an implementation needs a flag
  that is not, change the approach rather than the allowlist.

## Acceptance

- `resolvePushTarget` returns the resolved remote name for the name form and
  the single-entry URL form, `push_remote_fanout` for a multi-push-URL remote
  carrying the value, `push_remote_url_unmatched` when nothing carries it,
  `push_remote_not_found` for an unknown name, and
  `push_remote_config_unreadable` when the remote configuration cannot be read.
- `currentBranch` returns the branch for an attached `HEAD` and an empty string
  with no error for a detached one, distinguished from a real read failure.
- `go build ./...` and the existing process and securityutil suites pass with
  `Push` and `PushPreflight` behavior unchanged.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process/... ./internal/common/securityutil/...
cd apps/backend && make lint
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git_push_target.go` (new)
- `apps/backend/internal/agentctl/server/process/git_push_target_test.go` (new)
- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/common/securityutil/git.go`
- `apps/backend/internal/common/securityutil/git_test.go`
- `apps/backend/internal/agentctl/server/api/git.go` (call-site signature only)

## Dependencies

None.

## Risks

- `IsValidDefaultBranchName` looks like the right validator but strips an
  `origin/` prefix, so reusing it would let `origin/main` validate as `main`.
  The new validator must not route through `IsValidBaseBranchRef`.
- Changing `Push`'s signature touches its existing call site in
  `internal/agentctl/server/api/git.go`. Keep that edit mechanical; Task 04
  owns the request-field work there.
- A remote name that fails `IsValidBranchName` cannot be safely interpolated
  into `remote.<name>.url`. Skip such remotes during URL matching rather than
  failing the whole enumeration.

## Parallelism

`sequential`

## Inputs

- Requirements REQ-WORKSPACES-CONFIGURED-PUSH-TARGET-001 and -002, and the
  system design's [Push-target resolution](../../specs/workspaces/system-design/configured-push-target.md)
  and [Data and contracts](../../specs/workspaces/system-design/configured-push-target.md) sections.
- `internal/agentctl/server/process/git.go` `runGitCommand` argument allowlist
  and `getCurrentBranch`.
- `internal/common/securityutil/git.go` existing validators.
- `setupEmptyRemoteTaskRepo` in `git_empty_remote_test.go` as the fixture
  pattern for real-repository tests.

## Results

Implemented. `PushOptions`, the eleven error-code constants, the five result
fields, `configuredRemotes`, `resolvePushTarget`, and `currentBranch` live in
`apps/backend/internal/agentctl/server/process/git_push_target.go`;
`securityutil.IsValidExpectedBranchName` lives alongside the existing
validators. `Push` and `PushPreflight` take `PushOptions` as a pass-through, so
this work order changed no observable behavior.

`IsValidBaseBranchRef`'s `origin/` strip was confirmed to matter: it and the new
validator disagree on inputs such as `origin/-dash`, so the new validator checks
the value that is actually compared against `HEAD` and used in the refspec.

`currentBranch` distinguishes a detached `HEAD` from an unreadable checkout by
falling back to `rev-parse --verify HEAD` only when `symbolic-ref` fails, so a
detached checkout reports an empty branch and a broken one still errors.

Verified with `go test -tags fts5 ./internal/agentctl/server/process/...
./internal/common/securityutil/...` and `make -C apps/backend lint`.
