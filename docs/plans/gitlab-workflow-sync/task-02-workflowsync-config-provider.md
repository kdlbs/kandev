---
id: "02-workflowsync-config-provider"
title: "Workflow sync config provider field and schema"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/gitlab-workflow-sync.md"
---

# Task 02: Workflow Sync Config Provider Field And Schema

## Acceptance

1. `workflow_sync_configs` gains `provider TEXT NOT NULL DEFAULT 'github'` and
   `project_path TEXT NOT NULL DEFAULT ''` via idempotent `ADD COLUMN`
   migrations following the `addPollEnabledColumn` pattern, wired into
   `initSchema()`. Both are replay-safe on an existing database, and existing
   rows read back as `provider = "github"` with unchanged behavior.
2. `Config` and `SetConfigRequest` carry `Provider` and `ProjectPath`, and the
   select columns, `scanConfig`, and `UpsertConfigForWorkspace` round-trip them.
3. `Normalize()` is provider-conditional exactly as the spec's
   `## Data Model` → `### Config / SetConfigRequest` section states: empty
   provider defaults to `github`; unknown provider is `ErrInvalidConfig`;
   GitHub requires owner+name and forbids `project_path`; GitLab requires a
   multi-segment `project_path` with no spaces, no empty segments, no `.`/`..`
   segment, no leading/trailing slash, and forbids owner/name.

## Verification

```bash
cd apps/backend && go test ./internal/workflowsync/... -race
```

## Files Likely Touched

- `apps/backend/internal/workflowsync/models.go` — struct fields, `Normalize()`.
- `apps/backend/internal/workflowsync/store.go` — `createTablesSQL`,
  `initSchema`, two `add*Column` methods, `configSelectColumns`, `scanConfig`,
  `UpsertConfigForWorkspace`.
- `apps/backend/internal/workflowsync/models_test.go`,
  `apps/backend/internal/workflowsync/store_test.go`.

## Inputs

- Spec `## Data Model`.
- `store.go:60-66` (`addPollEnabledColumn`) is the migration precedent — the
  ALTER always runs and `db.IsDuplicateColumnError` swallows the replay.
- Note this package owns its own schema; it is not part of
  `internal/task/repository/sqlite/base_migrations.go`.

## Risks

- Adding the column to `createTablesSQL` alone would leave existing databases
  without it. Both the CREATE and the ALTER are required.
- Relaxing slash validation globally would let a malformed GitHub config
  through. Keep the GitHub branch's existing checks byte-for-byte.

## Output Contract

Config reads and writes carry provider identity, validation rejects
cross-provider field mixing, and both migrations pass a fresh-DB and a
same-DB replay test. No fetch behavior changes yet.
