---
status: draft
system: office
requirements:
  - REQ-OFFICE-IDENTITY-GRADUATION-001
  - REQ-OFFICE-IDENTITY-GRADUATION-002
  - REQ-OFFICE-IDENTITY-GRADUATION-003
  - REQ-OFFICE-IDENTITY-GRADUATION-004
created: 2026-09-25
owners:
  - kandev
---

# Office Session Identity Graduation System Design

## Purpose and boundaries

The [session identity design](task-session-identity-01.md) owns participant
binding and the Office-only transaction guard. This design describes the later
retirement release already required by
[Office session identity graduation](../requirements/session-identity-graduation.md).
The first default-on release was v0.94.0; this work does not introduce a new
database constraint or migrate existing session rows.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-IDENTITY-GRADUATION-001` | Unconditional binding |
| `REQ-OFFICE-IDENTITY-GRADUATION-002` | Unconditional decision resolution |
| `REQ-OFFICE-IDENTITY-GRADUATION-003` | Persistence and compatibility |
| `REQ-OFFICE-IDENTITY-GRADUATION-004` | Retired identity |

## Unconditional binding and decision resolution

Remove `OfficeSessionIdentity` from the orchestrator service configuration and
the dashboard's flag setter. Office run preparation always chooses the run's
participant agent identity when present, with the existing runner-seat and
execution-profile fallback cases retained. Agent decisions always use the
validated calling session; the Tasks-owned fallback for a decision without one
remains unchanged. Keep the Office-only in-transaction live-pair guard and
live-preferring session lookup. Do not add a table-level unique index, dedup
migration, or repair of historical rows.

## Retired identity

Remove the profile entry, `FeaturesConfig.OfficeSessionIdentity`, active runtime
registration, feature-response field, frontend default, and live startup
catalog classification. Append the exact key and environment variable to the
append-only retired identity registry. Old stored overrides remain in the
database but no longer affect behavior; an explicit old environment value is
also inert. The `features.office` release toggle remains independent and off
by default.

## Verification

Replace flag-on/off tests with permanent participant-binding and decision
re-evaluation tests, including reviewer/approver quorum, no participant
identity, non-Office task, pre-existing duplicate rows, and concurrent creation
under the supported SQLite/PostgreSQL locking model. Add a retirement contract
test that seeds a stale false override and environment value and checks the
runtime definitions and feature response. No new rendered UI is introduced.
