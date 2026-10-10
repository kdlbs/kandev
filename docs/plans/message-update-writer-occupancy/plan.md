---
created: 2026-10-09
status: done
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
system_design:
  - ../../specs/system-page/system-design/tool-payload-retention.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
legacy_specs: []
---

# Reduce message update writer occupancy

## Overview

Remove one unnecessary message-row write from each guarded SQLite replacement.
Preserve message results, retained-payload protection, errors, and conversation receipts.
One sequential work order implements and measures the correction.

This is a behavior-preserving optimization under existing requirements and the accepted writer-admission ADR.
No new product requirement or architecture decision is needed.
The existing Platform specification retains its draft status; this package does not promote unrelated contracts.

## Evidence and confirmed cause

`updateMessageWithPayloadGuardTx` updates `task_session_messages.id` to itself before reading retained metadata.
All three production caller paths supply a transaction from the shared writer factory.
That factory already acquires SQLite's writer at `BEGIN IMMEDIATE`.
The reservation UPDATE therefore performs additional row and trigger work without providing additional admission.

The [completed investigation](../database-writer-contention/evidence.md) measured this path on disposable databases.
Removing that statement shortened all three alternating 16 MiB workload comparisons, with substantial host variability.
Health deadlines still occurred. The optimization does not establish the writer responsible for the historical 503 incident.
The SQLite conversation revision trigger is conditional; the extra UPDATE does not necessarily increment that revision.

## Scope

### In scope

- Remove the redundant message reservation UPDATE while retaining the real transaction.
- Preserve the existing `message not found: <id>` error through SELECT error mapping on both engines.
- Cover ordinary updates, conversation-receipt updates, and changed agent-plan upserts.
- Prove that successful replacements perform one message-row UPDATE and retain transaction safety.
- Compare the implemented path with a disposable overlay that restores the old reservation statement.

### Out of scope

- Other no-op statements, including session identity locks and insertion guards.
- New transaction wrappers, pools, retries, batch policies, or schema migrations.
- Health-policy changes, probe timeouts, database migration, or production telemetry.
- Live load, deployment, historical root-cause claims, or workspace filesystem repairs.
- Rendered UI, API shape, localization, and public configuration changes.

## Technical approach

In `apps/backend/internal/task/repository/sqlite/message_payload_replay.go`, remove only the first SQLite reservation block
from `updateMessageWithPayloadGuardTx`. Map `sql.ErrNoRows` from the retained-metadata SELECT to the existing error
for both engines. Keep PostgreSQL `FOR UPDATE`, field merging, payload identity, timestamps, and actual update SQL intact.

`updateMessageWithPayloadGuard` continues to own its normal Begin/Commit/Rollback lifecycle.
`UpdateMessageWithConversationReceipt` and `updateAgentPlanMessage` continue to use their caller-owned transaction.
Do not start a nested transaction or change receipt publication, base revision, identity checks, or commit order.
The metadata SELECT remains inside the authoritative writer transaction.

| Caller / engine | Admission and identity | Result to preserve | Verification |
| --- | --- | --- | --- |
| Ordinary update / SQLite | Shared factory immediate transaction | One row update, same content and metadata | New update-count and missing-row tests |
| Receipt update / SQLite | Existing conversation mutation transaction | Same base/new revision and receipt; one row update | New count case and receipt regression |
| Agent-plan update / SQLite | Existing plan identity and conversation guards | Same identity, no-op decision, and changed receipt | New changed-upsert count case; existing plan tests |
| All callers / PostgreSQL | Existing session/identity locks and message `FOR UPDATE` | Same result and missing-row error | Existing PostgreSQL receipt/plan tests and shared assertions |
| Arbitrary injected deferred SQLite pool | Outside factory scheduling guarantee | Native errors remain errors; never retry outside authority | Do not use it to prove production admission |

Use an isolated audit table and `AFTER UPDATE ON task_session_messages` trigger in tests.
Install it after setup so it counts only the operation under test.
It must observe two row updates before the fix and one after the fix for each changed caller path.
This checks real write amplification without a timing threshold or source-string assertion.
Also verify stored content, retained metadata, and revision/receipt outcomes.
An injected failure in the actual content update must roll back both the row and audit effect.

Reuse the current database admission tests for native waiting, cancellation, rollback, and connection reuse.
Add a focused stale-retention check through the receipt path, since it shares the changed helper.
Keep the existing stale-read test for ordinary updates and externalized-payload tests.

The measurement script currently removes the reservation statement in its optional overlay.
After implementation, replace that experimental switch with `--restore-message-reservation`.
It restores the original statement only in a temporary source copy, providing a reproducible comparison against the fixed path.
Fail clearly if the expected source shape changes. Never silently compare two identical paths.
Preserve the historical measurements and explain their original command/version boundary.
Record new measurements separately in this package.

## Tests and acceptance mapping

| Criterion / invariant | Evidence |
| --- | --- |
| AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.3 | Existing externalized-payload tests and new rollback/result assertions preserve stored identities and timestamps |
| AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.4 | `TestUpdateMessagePreservesPayloadRemovalAfterStaleRead` and new receipt stale-retention case |
| AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3, .7 | Missing-row error, row-result, receipt, rollback, and engine-specific lock compatibility checks |
| Reduced redundant work | `TestGuardedMessageUpdateWritesRowOnce` fails at count 2 before the fix, then passes at count 1 |
| Existing writer scheduling | `TestSQLiteWriterTransactionAdmission`, `TestSQLiteWriterTransactionCancellation`, and related reuse tests |
| Performance evidence | Fixed 128-update benchmark, three alternating original/fixed pairs, no race detector or concurrent task-owned load |

No new browser flow is introduced. Repository integration tests cover this persistence boundary.
Public-doc audit: no CLI, operator setting, public API, or user-facing terminology changes.
The existing retention and writer-admission packages keep their recorded results and completed status.

## Work orders

- [x] [Task 01: Remove redundant message reservation](task-01-remove-message-reservation.md)

## Verification results

Design validation passed on October 9, 2026:

- `python3 scripts/list-docs.py validate`: 368 decisions and 1,493 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed; new package files also passed explicit whitespace checks.
- PR-documentation preflight: documentation-only paths exempt; the planned runtime path and work order passed cross-reference coverage.
- Work-order requirements are declared by their referenced designs, and both designs are included in this manifest.
- Source inspection confirmed all three helper callers and the existing factory admission/cancellation/reuse tests.

Implementation and its expected-red regression are complete. The fixed helper performs one message-row update per changed caller; the three alternating pairs finished 45.2%, 53.0%, and 53.8% sooner on this shared host. Four fixed-path health deadlines remain across the three contention samples, so the experiment does not establish universal availability recovery. Full test commands and raw measurements are in [implementation evidence](implementation-evidence.md). A disposable PostgreSQL 17 server was used for the PostgreSQL engine checks.

## Risks

- Removing the UPDATE without widening missing-row mapping changes the error contract.
- Reading metadata outside the transaction can restore previously removed payloads after a stale read.
- The helper serves more than the ordinary UpdateMessage path; receipt and plan coverage is required.
- Shared-host timings cannot establish a stable percentage gain or guarantee zero probe failures.
- The original 503 incident and workspace-file failures remain unresolved after this optimization.
