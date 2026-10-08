---
id: turn-changed-files-02
title: Typed change sets and database parity
status: done
wave: 2
depends_on: []
  - turn-changed-files-01
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.6
  - AC-TASKS-TURN-CHANGES-002.8
  - AC-TASKS-TURN-CHANGES-002.9
  - AC-TASKS-TURN-CHANGES-003.3
  - AC-TASKS-TURN-CHANGES-003.5
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-004.2
  - AC-TASKS-TURN-CHANGES-004.6
  - AC-TASKS-TURN-CHANGES-004.7
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Typed change sets and database parity

## Summary

Persist a typed immutable interval independently of mutable Git status snapshots.

## Scope and owned files

- New `apps/backend/internal/task/models/turn_changes.go` and narrow task repository interfaces.
- New task repository change-set schema, reads, endpoint CAS, summary revisions, and content-link schema.
- `internal/task/repository/sqlite/base_schema.go`, `base_migrations.go`, and new focused SQLite/PostgreSQL tests.
- Conversation-delete/session-delete cascades and E2E reset cleanup for new tables.
- Proposed neutral `internal/common/turnchanges` DTOs; task-level source/checkout identity binding.

## Exclusions

No Git capture, content export, transcript components, or live-monitor replacement.

## Implementation acceptance

1. Enforce task/session/turn/environment/checkout relationships and unique interval identities; accepted start/end OIDs are write-once.
2. Fresh, upgraded, and replayed SQLite/PostgreSQL databases preserve policies, successful zero, partial, failed, expired, and overlap metadata.
3. Conditional revisions and complete/summary_complete/content_complete fields prevent duplicate finalization and preserve summaries after content expiry.

## Verification

New tests use `TurnChangeSet` and `PostgresTurnChangeSet` prefixes, with fresh-schema, old-schema upgrade, and same-schema replay fixtures.

```bash
cd apps/backend
go test -trimpath ./internal/common/turnchanges ./internal/task/models ./internal/task/repository/sqlite -run 'TurnChangeSet|PostgresTurnChangeSet|SchemaReplay' -count=1
go test -trimpath -race ./internal/task/repository/sqlite -run 'TurnChangeSet.*(Concurrent|CAS)|PostgresTurnChangeSet' -count=1
```

Set `KANDEV_TEST_POSTGRES_DSN` to execute the PostgreSQL cases. Record skips as missing evidence.
Run the existing upgrade-fixture harness for the changed schema using its checked-in manifest; record exact fixture identity and command.

## Dependencies and risks

Depends on policy DTOs from 01. No transaction spans executor I/O.
SQLite schema creation precedes migrations; dependent indexes must follow new columns.
PostgreSQL advisory lock order and cross-connection CAS require actual database evidence.

## Results

Implemented transport-neutral availability, reason, policy-resolution, and typed task change-set contracts; replayable schema statements; task-scoped persistence and endpoint CAS; relationship checks; immutable endpoint preservation; overlap summaries; and content/file relation schema. Added required-store inventory entries for all five tables. SQLite tests cover successful zero, partial, failed, expired, task/environment/session/turn ownership, duplicate identities, revision CAS, endpoint preservation, exact path bytes, schema replay/upgrade, and cascade behavior.

Validation passed:

- `go test -trimpath ./internal/common/turnchanges ./internal/task/models ./internal/task/repository/sqlite ./internal/persistence/requiredstores -run 'TurnChangeSet|TurnChangedFiles|AvailabilityContract|ReasonCodeContract|PolicyResolutionKindContract|Catalog' -count=1`
- `go test -trimpath -race ./internal/task/repository/sqlite -run 'TurnChangeSet.*(Concurrent|CAS)|PostgresTurnChangeSet' -count=1` (SQLite passed; PostgreSQL skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured).
- `go run ./cmd/sqlguard ./internal`
- `go test -trimpath -race ./internal/persistence/storeconformance -count=1`
- `go test -trimpath ./internal/persistence/storeconformance -run '^TestPreviousStableUpgrade$' -count=1 -v`: checked-in `v0.93.0` fixture (`866bd324855fda1f71e13b8863d2c71d4a11237e`); SQLite upgrade and replay passed, PostgreSQL skipped without a test DSN.

PostgreSQL behavior remains an evidence gap until a test DSN is available.

### Review follow-up (2026-10-08)

Finalization now updates a repository row only when its end endpoint is still empty and no unaccepted endpoint is supplied, or when its already accepted commit/tree/ref exactly matches. The SQLite regression accepts the end endpoint first, rejects a mismatched replacement, then finalizes successfully. Focused SQLite change-set, content, and retention tests passed; PostgreSQL remains unverified without its DSN.
