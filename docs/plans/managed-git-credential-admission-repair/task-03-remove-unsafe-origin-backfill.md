---
id: "03-remove-unsafe-origin-backfill"
title: "Remove unsafe origin backfill"
status: done
wave: 3
depends_on: ["02-centralize-session-admission"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 03: Remove Unsafe Origin Backfill

## Acceptance

- Startup migrations do not infer or persist `provider_host` from only a provider and remote URL.
- Incomplete legacy rows, including rows with nullable identity fields, do not stop startup schema
  replay.
- Runtime identity resolution can still infer public `github.com` in memory from a safe persisted
  remote.
- No replacement migration or duplicate provider-specific inference path is added.

## TDD Sequence

1. Add a startup migration regression with incomplete legacy provider metadata and nullable fields.
   Assert that startup succeeds and `provider_host` remains empty.
2. Run the focused SQLite repository test and record RED because the PR migration writes the host
   or fails while it scans a nullable value.
3. Remove the credential-origin migration registration, implementation, and migration-specific
   tests. Keep the startup regression at the base migration boundary.
4. Run the SQLite repository package test and record GREEN.
5. Confirm that the specification and accepted provider-origin decision describe the same
   persistence boundary.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite -run 'Test.*Migration.*ProviderHost' -count=1
cd apps/backend && go test ./internal/task/repository/sqlite -count=1
```

## Files Likely Touched

- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/repository_credential_origin_migration.go`
- `apps/backend/internal/task/repository/sqlite/repository_credential_origin_migration_test.go`
- `apps/backend/internal/task/repository/sqlite/repository_credential_origin_migration_postgres_test.go`
- a focused base migration regression test under
  `apps/backend/internal/task/repository/sqlite/`
- this task file and `plan.md`

## Dependencies

- Task 02 must make runtime admission safe without relying on a persistent backfill.

## Parallelism

Sequential. This final task removes the temporary persistence mechanism after the runtime contract
is complete.

## Inputs

- ADR `docs/decisions/2026-07-20-repository-provider-origin-identity.md`.
- The runtime public-host inference in `gitcredentials.ResolveRepositoryIdentity`.
- `baseMigrations` and startup migration replay tests.

## Output Contract

Report the RED persistence change or scan failure, deleted migration files, retained startup
regression, GREEN commands, files changed, and remaining risks. Update this task and the parent plan
with the recorded results.

## Recorded Results

- RED: startup migration changed an empty plugin repository `provider_host` to public GitHub from
  only its provider label and remote URL.
- The new credential-origin migration, registration, and provider-specific tests were removed.
- A base migration regression now preserves the empty provider host for partial legacy identity.
- GREEN: the focused regression passed once.
- GREEN: `go test ./internal/task/repository/sqlite -count=1` passed 598 tests.
