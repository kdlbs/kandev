---
id: "03-workspace-credential-resolver"
title: "Workspace PAT and CLI resolver"
status: completed
wave: 2
depends_on: ["01-persistence-migration"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 03: Workspace PAT And CLI Resolver

## Inputs

- Spec: What, Identity And Routing, PAT/CLI failure modes, and first five scenarios.
- ADR 0047: explicit purpose resolution and deterministic named CLI accounts.
- Existing `factory.go`, `gh_client.go`, `service_tokens.go`, and rate/cache helpers.

## Acceptance

- Resolver requests require workspace and purpose and return a client, verified principal,
  capabilities, generation, and expiry without process-global operational state.
- PAT and named CLI replacements validate first and atomically replace only the target workspace;
  CLI commands select `--hostname` and `--user`, strip ambient token variables, and never switch
  the host account.
- `legacy_shared` preserves current precedence only for migrated rows; disconnected/new/invalid
  workspaces never fall through to it or another workspace.

## Files Likely Touched

- `apps/backend/internal/github/auth_principal.go` (new)
- `apps/backend/internal/github/auth_resolver.go` (new)
- `apps/backend/internal/github/auth_resolver_test.go` (new)
- `apps/backend/internal/github/gh_accounts.go` (new)
- `apps/backend/internal/github/gh_accounts_test.go` (new)
- `apps/backend/internal/github/factory.go`
- `apps/backend/internal/github/factory_test.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/service_connections.go` (new)
- `apps/backend/internal/github/service_tokens.go`

## Verification

```bash
cd apps/backend && rtk go test ./internal/github -run 'Test(CredentialResolver|GHAccounts|WorkspaceConnection|LegacyShared)'
```

## Output Contract

Report the resolver interface, exact CLI subprocess environment, replacement/rollback behavior,
tests run, files touched, blockers, and risks. Do not migrate domain service call sites in this task.
