---
id: "15-migration-coexistence"
title: "Migration and coexistence (M8)"
status: pending
wave: 9
depends_on:
  - "14"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-MIGRATION-001
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-MIGRATION-001.1
  - AC-EXECUTORS-PLUGIN-MIGRATION-001.2
  - AC-EXECUTORS-PLUGIN-MIGRATION-001.3
  - AC-EXECUTORS-PLUGIN-MIGRATION-001.4
  - AC-EXECUTORS-PLUGIN-MIGRATION-001.5
  - AC-EXECUTORS-PLUGIN-MIGRATION-001.6
  - AC-EXECUTORS-PLUGIN-MIGRATION-001.7
  - AC-EXECUTORS-PLUGIN-MIGRATION-001.8
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-operations-migration.md
---

# Task 15: Migration and coexistence (M8)

## Summary

Add environment enumeration, the authority field, the resumable freeze-drain-switch operation, and rollback that keeps retained plugin environments readable.

## Size

L: ~600 source, ~900 test lines.

## In scope

- Source is a profile of one of `local_docker`, `remote_docker`, `sprites`, `ssh`, `k8s`; `POST` and `GET /api/v1/executors/profiles/:profile_id/migration` for start, rollback (allowed in freezing, draining, switching), and step; no automatic drain timeout; `authority` written only on records the operation touches; no live-scenario gate in the host.
- Read-only enumeration route; `authority` metadata with tolerant decode; `transitionAuthority` under the cleanup claim.
- Migration state machine in the new `executor_profile_migrations` table (row revision CAS), with `checkMigrationFreeze` in the common launch entry that built-in launches pass, evaluated in the transaction that creates the environment record; the `migrated_to` write and the `done` step in one revision-guarded transaction; `executor_migrating` as the refusal.
- Rollback and retention reads.

## Out of scope

- Converting records between authorities; removing any built-in.

## Acceptance

- No environment has two authorities; the operation resumes after a restart; rollback loses nothing.

## Failing-first tests

- Freeze races a new launch: the earlier launch drains, the later is refused (sqlite and PostgreSQL race).
- Backend restart in each step resumes without a double freeze (barrier e2e).
- Rollback with retained plugin environments: not adopted, still listed, survive plugin disable; rollback from `done` removes `migrated_to`, and a plugin environment created after the switch stays resumable.
- Rollback racing an in-flight drain destroy: none starts after the rollback commits, the started one finishes.
- A launch resolved to the source before `switching` that reaches its transaction after `done` aborts and re-resolves once; no built-in record after `done`. A resume of a stopped source-profile session during `draining` is refused `executor_migrating`.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/plugins/... ./internal/task/... -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `apps/backend/internal/agent/runtime/lifecycle`
- `apps/backend/internal/task`
- Risk: concurrency on both dialects; add the PostgreSQL race run (`KANDEV_TEST_POSTGRES_DSN`) before marking done.
