---
id: "06-database-maintenance-command"
title: "Provide safe database retention maintenance"
status: pending
wave: 3
depends_on: ["04-github-auth-backoff-observability", "05-bounded-history-storage"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 06: Provide safe database retention maintenance

## Intent

Give operators a backup-gated command to report, execute, compact, and roll back bounded-storage cleanup.

## Acceptance

- The native maintenance database command supports dry run, explicit retention settings, per-table candidate/reclaim estimates, and fails closed when a verified backup is absent.
- Execution removes only selected redundant payloads/snapshots/revisions in transactions, stages/validates compaction with VACUUM INTO or equivalent, atomically replaces only after success, and prints rollback steps.
- The command reports SQLite/WAL and table-storage measurements without contents, credentials, or destructive default behavior.

## Files likely touched

- apps/backend/cmd/kandev/main.go
- apps/backend/internal/launcher/*.go
- apps/backend/internal/system/storage/*.go
- apps/backend/internal/persistence/snapshot.go
- New maintenance package/command and *_test.go files
- apps/backend/internal/common/config/catalog.go
- apps/backend/internal/common/config/source.go

## Dependencies

Tasks 04 and 05.

## Parallelism

Sequential. It consumes finalized retention data and health/configuration contracts.

## Verification

~~~bash
cd apps/backend && go test ./internal/persistence ./internal/system/storage ./internal/launcher ./cmd/kandev -run 'Test.*(Maintenance|Retention|DryRun|Backup|Vacuum|Compact|Rollback).*' -count=1 -v
cd apps/backend && go test ./internal/persistence ./internal/system/storage ./internal/launcher ./cmd/kandev -count=1
git diff --check
~~~

## Output contract

Report dry-run versus execution row/byte counts, backup verification, staged-compaction failure preservation, successful restore/rollback, command output redaction, and test results. Update task and plan status.

## Results

Pending.

