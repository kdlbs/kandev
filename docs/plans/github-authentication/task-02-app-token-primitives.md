---
id: "02-app-token-primitives"
title: "GitHub App token primitives"
status: completed
wave: 2
depends_on: ["01-persistence-migration"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 02: GitHub App Token Primitives

## Inputs

- Spec: GitHub App Permissions, Failure Modes, and installation-token persistence guarantees.
- ADR 0047: deployment App ownership and memory-only token minting.
- Official GitHub App JWT, installation token, and permission contracts linked from the public docs
  task.

## Acceptance

- Deployment App config validates all-or-none secret material, file/multiline private keys, public
  callback URL safety, and exposes only a non-secret availability result.
- A token-source-neutral REST/GraphQL client supports PAT, App installation, and App user tokens
  without erasing principal metadata.
- Installation tokens are minted with valid JWT claims, permission-scoped, singleflight-cached,
  refreshed before expiry, and never returned after expiry.

## Files Likely Touched

- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/common/config/config_test.go`
- `apps/backend/internal/github/pat_client.go` (renamed/refactored)
- `apps/backend/internal/github/token_client.go` (new)
- `apps/backend/internal/github/app_client.go` (new)
- `apps/backend/internal/github/app_token_cache.go` (new)
- `apps/backend/internal/github/app_client_test.go` (new)
- `apps/backend/internal/github/app_token_cache_test.go` (new)
- `apps/backend/internal/backendapp/helpers.go`

## Verification

```bash
cd apps/backend && rtk go test ./internal/common/config -run 'Test.*GitHubApp'
cd apps/backend && rtk go test ./internal/github -run 'Test(AppClient|InstallationToken|TokenClient)'
```

## Output Contract

Report config contract, permission mapping, refresh margin, cache key, tests run, files touched,
blockers, and risks. Do not wire service operations in this task.
