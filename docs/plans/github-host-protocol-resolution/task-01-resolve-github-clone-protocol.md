---
id: "01-resolve-github-clone-protocol"
title: "Resolve GitHub clone protocol at use time"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.9
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.10
system_design:
  - ../../specs/integrations/system-design/github-authentication-01.md
  - ../../specs/integrations/system-design/github-authentication-02.md
  - ../../specs/integrations/system-design/github-authentication-03.md
---

# Task 01: Resolve GitHub Clone Protocol At Use Time

## Summary

Implement one host-aware, live Git protocol resolver. Use it for executor-inherited checkout origins
and review-repository clone URLs. Remove both backend-startup result caches. Prove that the same
long-lived runtime observes host configuration changes.

## In scope

- Add the per-host, global-fallback, default-SSH resolver with an injected command runner.
- Inject the resolver into `repoclone.Cloner` and evaluate it for each protocol-aware URL build.
- Thread context through executor URL construction and preserve the launch/resume reconciliation
  boundary.
- Remove `repositoryResolverAdapter.protocol` and resolve through a narrow injected cloner seam.
- Add deterministic regression coverage in `repoclone`, `executor`, and `backendapp`.

## Out of scope

- Credential-helper bridging or any #3072 behavior.
- Credential probing, new providers or hosts, UI/API changes, persistence, or migrations.
- Rewriting user-managed local checkout origins or changing managed-mode HTTPS.

## Acceptance

- Host-specific `github.com` `ssh` overrides global `https`.
- A supported global value applies when no supported host-specific value is available.
- SSH is the final default.
- A running backend observes a protocol change on the next Local/Worktree launch or resume and
  reconciles the Kandev-managed checkout without restart.
- `repositoryResolverAdapter` uses the same live resolution path and stores no protocol result.

## Verification

```bash
# From apps/backend:
rtk go test -race ./internal/repoclone ./internal/orchestrator/executor ./internal/backendapp -run 'Test(DetectGitProtocol|ClonerBuildCloneURLUsesCurrentProtocol|EnsureRepoLocalPathReevaluatesGitHubProtocol|RepositoryResolverAdapterUsesCurrentGitProtocol)' -count=1
```

The targeted command passed with 11 tests in 3 packages, including the total-timeout regression.

## Files likely touched

- `apps/backend/internal/repoclone/protocol.go`
- `apps/backend/internal/repoclone/protocol_test.go`
- `apps/backend/internal/repoclone/clone.go`
- `apps/backend/internal/repoclone/clone_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/adapters_test.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go`
- Executor test files whose `RepoCloner` fakes implement the context-aware URL builder.

## Dependencies

None.

## Risks

- Incorrect host normalization can query a non-existent `gh` host scope or construct a different
  clone URL. Test the command arguments and final canonical URLs separately.
- A constructor string or adapter field can reintroduce restart-only behavior. Mutable-resolver
  tests at both long-lived consumers prevent this error.

## Parallelism

`sequential`

## Inputs

- `REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001`, especially
  `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.9` and `.10`.
- The three Workspace GitHub Authentication system-design parts referenced above.
- `ADR-2026-07-27-task-git-credential-policy`.
- Issue #3069 and the confirmed isolated `GH_CONFIG_DIR` reproduction in `plan.md`.
- Existing origin-reconciliation coverage in
  `executor_resume_clone_transport_test.go` and review-repository adapter coverage in
  `adapters_test.go`.

## Results

- Replaced startup Git protocol snapshots with host-aware, context-bounded resolution. Host-specific
  `gh` configuration takes precedence over global configuration, with SSH as the final default. One
  five-second context bounds the complete host-plus-global lookup.
- Injected the resolver into the long-lived repository cloner and review-repository adapter. Executor
  origin reconciliation now evaluates it on every applicable launch or resume.
- Passed the targeted regression command with 11 tests in 3 packages and the full race-enabled
  affected-package suites with 1,316 tests in 3 packages.
- PR 3078 completed its fixup with 44 checks passed, no failures or pending checks, and no unresolved
  review threads.
