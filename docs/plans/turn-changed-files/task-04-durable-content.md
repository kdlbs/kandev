---
id: turn-changed-files-04
title: Durable historical rendering and bounded retention
status: done
wave: 4
depends_on:
  - turn-changed-files-02
  - turn-changed-files-03
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-003.2
  - AC-TASKS-TURN-CHANGES-003.4
  - AC-TASKS-TURN-CHANGES-003.5
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-004.5
  - AC-TASKS-TURN-CHANGES-004.6
  - AC-TASKS-TURN-CHANGES-004.7
  - AC-TASKS-TURN-CHANGES-007.1
  - AC-TASKS-TURN-CHANGES-007.2
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Durable historical rendering and bounded retention

## Summary

Preserve bounded rendering data before ephemeral checkout loss, then expire content without losing its summary.

## Scope and owned files

- Proposed `internal/task/changes` export/retention service and task content repository methods.
- Executor per-file export from accepted OIDs, including canonical and ignore-whitespace patches, modes, and bounded old/new rendering content.
- Database content-addressed compression, digest/size validation, links, retention leases, and atomic expiry.
- Existing maintenance/storage mutation integration and backend construction/lifecycle ownership.
- Owned executor ref cleanup receipts and reconnect cleanup; conversation deletion/reset cleanup.

## Exclusions

No whole-checkout copy, public retention settings UI, user-repository GC, live-workspace fallback, or binary image preview promise.

## Implementation acceptance

1. Verified durable content supports patches and context expansion after executor destruction, task archive, and backend restart.
2. Age/task/install limits remove only eligible unleased content, preserve summaries, and deduplicate shared payloads without premature deletion.
3. Oversized/truncated/missing export data remains visibly partial or unavailable; cleanup never reports missing content as fully ready.

## Verification

Planned tests use `TurnChangeContent`/`TurnChangeRetention` prefixes. Delete fixture executor directories before historical reads.
Use a fake clock for expiry and a real database for export/restart/cleanup races.

```bash
cd apps/backend
go test -trimpath ./internal/task/changes ./internal/task/repository/sqlite -run 'TurnChangeContent|TurnChangeRetention|PostgresTurnChangeContent' -count=1
go test -trimpath -race ./internal/task/changes ./internal/task/repository/sqlite -run 'TurnChange(Content|Retention).*(Lease|Concurrent|Cleanup)|PostgresTurnChangeContent' -count=1
```

Execute PostgreSQL cases with `KANDEV_TEST_POSTGRES_DSN`. Assert decompression and aggregate-budget limits too.

## Dependencies and risks

Depends on 02 and 03. Retention defaults are proposed in the design: 30 days, 1 GiB install, 128 MiB task, 32 MiB turn export.
Record payload and backup growth. Ref deletion requires export acknowledgement; executor loss requires explicit unavailable content.

## Results

Implemented bounded per-file exports from immutable Git endpoints, durable content-addressed compressed storage, integrity-checked historical reads, retention leases, and age/task/installation expiry. Export loss is persisted as partial or unavailable while summaries remain intact. The existing one-minute session reconciliation sweep applies the reviewed 30-day, 128 MiB task, and 1 GiB installation defaults.

Focused validation passed:

```bash
cd apps/backend
go test -trimpath ./internal/task/changes ./internal/task/repository/sqlite -run 'TurnChangeContent|TurnChangeRetention|PostgresTurnChangeContent' -count=1
go test -trimpath -race ./internal/task/changes ./internal/task/repository/sqlite -run 'TurnChange(Content|Retention).*(Lease|Concurrent|Cleanup)|PostgresTurnChangeContent' -count=1
go test -trimpath -race ./internal/task/service -run 'TurnChangeRetentionSweep' -count=1
go test -trimpath ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl -run 'TurnCheckpoint' -count=1
```

The PostgreSQL round-trip and retention case was skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured. SQLite cleanup, lease, deduplication, expiry, and historical export coverage passed.

### Review follow-up (2026-10-08)

Export receipts now report the bytes and completeness actually retained under the aggregate turn limit. Comparison file summaries persist before export, so transport or export failure keeps file metadata with unavailable content. Focused content/retention tests passed, including aggregate limits and partial payloads. PostgreSQL coverage remains unavailable without its DSN.
