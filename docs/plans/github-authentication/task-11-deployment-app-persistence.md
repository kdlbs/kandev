---
id: "11-deployment-app-persistence"
title: "Deployment App persistence and source resolution"
status: done
wave: 8
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 11: Deployment App Persistence and Source Resolution

## Inputs

- Spec: `github_app_registration`, persistence guarantees, environment precedence, and removal
  scenarios.
- ADR: `docs/decisions/2026-07-20-managed-github-app-registration.md`.
- Patterns: `apps/backend/internal/github/store.go`, `personal_connection_repository.go`,
  `apps/backend/internal/integrations/secretadapter/`, and `common/config.GitHubAppConfig`.

## Acceptance

1. A replayable singleton metadata table and deployment-scoped flow table support SQLite and
   Postgres, while one immutable generation-addressed encrypted bundle owns all three generated
   secrets and metadata points to the active bundle.
2. Resolution is strictly `environment > managed > none`; a partial/invalid environment override is
   authoritative and never falls back to persisted credentials.
3. Failed or canceled bundle/metadata writes preserve the previous generation, inactive bundles
   reconcile safely, and managed save/deletion is rejected while any workspace uses an App
   installation.

## Files Likely Touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/deployment_app_store.go` (new)
- `apps/backend/internal/github/deployment_app_config.go` (new)
- `apps/backend/internal/github/deployment_app_store_test.go` (new)
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/common/config/config_test.go`

## Verification

```bash
rtk go test ./internal/github -run 'TestDeploymentApp(Store|Config|Source|Delete)' -count=1
rtk go test ./internal/common/config -run TestGitHubAppConfig -count=1
```

Run from `apps/backend`.

## Dependencies

None. Do not edit Task 12's manifest/protocol files.

## Output Contract

Report the schema, generation-addressed secret ID and pointer-switch behavior, orphan reconciliation,
precedence tests, files touched, commands run, blockers, and residual migration/rotation risk. Mark
this task `done` and update `plan.md` only after targeted tests pass.
