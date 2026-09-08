---
id: "03-managed-runtime-tools"
title: "Repair managed agentctl and gh runtime tools"
status: done
wave: 3
depends_on: ["00-indexed-git-config-composition", "02-launch-resume-snapshot"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 03: Repair Managed Agentctl And Gh Runtime Tools

## Acceptance

- A standalone control `agentctl` started without broker env exposes both managed `agentctl` and
  `gh` to a later broker-enabled instance; managed Git and `gh` commands do not depend on the
  backend shell PATH.
- The managed tool directory is not activated for executor inheritance or explicit profile-token
  override instances, and a managed helper/broker failure does not fall back.
- Agentctl parent-plus-instance collection preserves host/executor indexed Git configuration and
  emits an already-forwarded task suffix only once.
- Named GitHub CLI credentials are re-resolved after at most five minutes and invalidate
  immediately on connection-generation changes.

## Verification

```bash
cd apps/backend && go test ./cmd/agentctl ./internal/agentctl/server/config ./internal/github -run 'Test.*(GitHubCLIShim|GitHubTool|CollectAgentEnv|CredentialResolver.*CLI|CredentialCache|IndexedGitConfig)'
```

## Files likely touched

- `apps/backend/cmd/agentctl/main.go`
- `apps/backend/cmd/agentctl/github_cli_shim.go`
- `apps/backend/cmd/agentctl/github_cli_shim_test.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/config_test.go`
- `apps/backend/internal/github/auth_resolver.go`
- `apps/backend/internal/github/auth_resolver_test.go`
- focused subprocess/fake-broker test helpers
- this task file and `plan.md`

## Dependencies

Tasks 00 and 02.

## Parallelism

Parallel-safe with Task 05: backend runtime/auth files and frontend Changes-panel files are
disjoint.

## Inputs

- Confirmed standalone root cause in `plan.md`.
- Spec failure/security scenarios and five-minute named-CLI refresh contract.
- Existing `installGitHubCLIShim`, `CollectAgentEnv`, and credential resolver cache patterns.
- Task 00's composition and overlap rules.

## Output contract

Report per-instance activation conditions, parent/instance Git-config composition evidence, actual
subprocess proof for `git` and `gh`, CLI cache deadline semantics, RED/GREEN results, files changed,
portability limits, and update task/plan status.
