---
id: "06-executor-credentials"
title: "Renewable executor GitHub credentials"
status: completed
wave: 4
depends_on: ["02-app-token-primitives", "03-workspace-credential-resolver"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 06: Renewable Executor GitHub Credentials

## Inputs

- Spec: executor routing, failure mode, persistence guarantee, and >1-hour task scenario.
- ADR 0047: task-scoped broker lease and no deployment/personal secret injection.
- Existing executor remote-auth handling and agentctl authenticated API conventions.

## Acceptance

- Git operations obtain workspace automation credentials through a task/session/repository-scoped,
  hashed, expiring broker lease; every request rechecks workspace scope and credential generation.
- `agentctl git-credential` implements Git's helper protocol and refreshes App tokens on demand for
  local, Docker, SSH, and remote executor paths; an isolated broker-aware `gh` shim refreshes before
  each CLI invocation.
- Launch environments, command arguments, logs, and persisted metadata contain no App private key,
  personal token, refresh token, or global fallback token; disconnect/revocation makes old leases
  unusable.

## Files Likely Touched

- `apps/backend/internal/github/credential_broker.go` (new)
- `apps/backend/internal/github/credential_broker_test.go` (new)
- `apps/backend/internal/github/controller_credentials.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`
- `apps/backend/cmd/agentctl/github_credential.go` (new)
- `apps/backend/cmd/agentctl/github_credential_test.go` (new)
- `apps/backend/cmd/agentctl/github_cli_shim.go` (new)
- `apps/backend/cmd/agentctl/github_cli_shim_test.go` (new)
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agent/remoteauth/catalog.go`

## Verification

```bash
cd apps/backend && rtk go test ./internal/github -run 'TestCredentialBroker'
cd apps/backend && rtk go test ./internal/orchestrator/executor -run 'Test.*GitHubCredential'
cd apps/backend && rtk go test ./cmd/agentctl -run 'TestGitHubCredential'
```

## Output Contract

Report broker transport for every executor, lease lifetime/revocation rules, helper configuration,
secret-leak assertions, tests run, files touched, blockers, and risks.
