---
id: "01-host-cli-route"
title: "Add the optional host CLI route"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.1
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.2
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.3
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.4
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.5
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.6
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.7
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
---

# Task 01: Add the optional host CLI route

## Summary

Register an optional host GitHub CLI helper for eligible Local/Worktree tasks in executor mode.
Preserve existing Git configuration and explicit credential precedence.

## In scope

- Add a small injected availability runner in `executor_host_gh_bridge.go` and wire its default through the executor.
- Extend the executor-policy branch in `configureGitCredentialBrokerForRepositories` after managed cleanup.
- Resolve and deduplicate eligible GitHub hosts from attached repository metadata through the existing identity resolver.
- Apply explicit executor-type and token gates before the optional probe.
- Execute the bounded probe without a shell, discard token output, and preserve caller cancellation.
- Compose one optional helper per host with `gitconfigenv.Merge`, after existing helpers and without a reset.
- Validate the resulting indexed block, including the combined count limit.
- Use a safely quoted absolute host CLI executable so the helper survives a changed child `PATH`.
- Mark generated helpers with an unambiguous Kandev ownership marker and remove only marker-owned entries.
- Resolve agent and executor profile credential-store variables before probing, while preserving request precedence.

## Out of scope

Runtime entry-point changes belong to Task 02. Remote credentials, global Git writes, and broker redesign are excluded.

## Acceptance

1. The real-route credential-fill regression fails before implementation and passes after the correction.
2. Eligibility, token precedence, probe failures, and indexed configuration cases pass with no token output or global writes.
3. Repeated configuration preserves meaningful user entries and does not accumulate the generated helper.

## TDD cases

Add `executor_host_gh_bridge_test.go` with the tests named in the plan.
Use a fake `gh` binary and real `git credential fill` in an isolated temporary home.
Clear credential-related inherited variables in the fixture without printing them.

Start with these exact inherited values:

```text
GIT_CONFIG_COUNT=2
GIT_CONFIG_KEY_0=notes.augment.mergeStrategy
GIT_CONFIG_VALUE_0=union
GIT_CONFIG_KEY_1=core.hooksPath
GIT_CONFIG_VALUE_1=/Users/cfl12/.locstat/git/hooks
```

After one helper is appended, assert count 3 and unchanged entries 0 and 1.
Also test inherited host entries outside `req.Env`, so final composition cannot silently duplicate the host block.
Include mixed repositories: repeated eligible host, explicit enterprise host, GitLab host, and ambiguous local metadata.
Assert zero probes for managed mode, request-token overrides, unknown executor types, and every remote executor type.
Distinguish probe timeout from caller cancellation.
Use fake tokens throughout the tests.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -run 'TestExecutorHostGHBridge|TestConfigureGitHubCredentialBroker|TestConfigureGitCredentialBroker' -count=1)
(cd apps/backend && go test ./internal/gitconfigenv -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_host_gh_bridge.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_host_gh_bridge_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`
- `apps/backend/internal/orchestrator/executor/executor.go`

Reuse `internal/gitconfigenv`, `internal/gitcredentials`, and `internal/common/subproc` without unrelated refactoring.

## Dependencies

None.

## Risks

Probe success is token availability only. Existing helper credentials retain their original precedence.
Malformed optional repository identities must not become trusted hosts.

## Parallelism

`sequential`

## Inputs

- Authentication requirement 002 and the design's “Executor host CLI bridge” section.
- `executor_credentials_test.go`: `TestConfigureGitHubCredentialBrokerSkipsExecutorInheritedPolicy`.
- `gitconfigenv/environment_test.go`: ordered composition and boundary-overlap tests.
- `repoclone/protocol.go`: the existing bounded host CLI command pattern.

## Results

- Added the optional host `gh` route for explicit Local and Worktree executor
  requests, with validated host selection, token gates, bounded no-shell probes,
  safe absolute helper commands, profile-aware credential-store selection, and managed-mode isolation.
- Preserved inherited indexed Git entries and meaningful repeated helpers while
  removing only marker-owned stale bridge entries and enforcing the combined 256-entry limit.
- Added regressions for quoted user-owned `gh` helpers, unrelated hosts, unavailable
  replacement CLIs, agent/executor profile directory mismatches, and request-level precedence.
- `TestLaunchPreparedSessionProbesEffectiveProfileCredentialStore` covers the HOME and
  GH_CONFIG_DIR mismatch cases through the production prepared-session entry point.
- `(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -run 'TestExecutorHostGHBridge|TestConfigureGitHubCredentialBroker|TestConfigureGitCredentialBroker' -count=1)` passed.
- `(cd apps/backend && go test ./internal/gitconfigenv -count=1)` passed.
- `go test ./internal/orchestrator/executor -count=1` and `go test ./internal/gitconfigenv -count=1` passed in the final affected-package run.
- `git diff --check` passed.
