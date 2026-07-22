---
id: "01-persistence-migration"
title: "Persistence and legacy migration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 01: Persistence And Legacy Migration

## Inputs

- Spec: Data Model, Persistence Guarantees, legacy/new-workspace/copy scenarios.
- ADR 0047: deployment/workspace/user ownership and the temporary legacy exception.
- Patterns: `apps/backend/internal/github/store.go`,
  `internal/integrations/secretadapter`, and ADR 0027.

## Acceptance

- The four connection/auth tables and workspace ownership columns are created by a replayable,
  SQLite/Postgres-compatible migration with uniqueness and state constraints.
- Migration creates `legacy_shared` only for workspaces present during upgrade, backfills task PR
  and watch ownership, and fails closed on ambiguous/missing ownership.
- Workspace delete/reset removes owned connection data and secrets; workspace copy never copies
  automation or personal auth.

## Files Likely Touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_connections.go` (new)
- `apps/backend/internal/github/store_connections_test.go` (new)
- `apps/backend/internal/github/copy.go`
- `apps/backend/internal/github/copy_test.go`
- `apps/backend/internal/backendapp/e2e_reset.go`

## Verification

```bash
cd apps/backend && rtk go test ./internal/github -run 'TestStore.*(Connection|Legacy|WorkspaceBackfill|Copy)'
cd apps/backend && rtk go test ./internal/backendapp -run 'Test.*E2EReset.*GitHub'
```

## Output Contract

Report schema and migration behavior, secret-key ownership, tests run, files touched, blockers, and
follow-up risks. Set this task to `done` and update `plan.md` only after acceptance passes.
