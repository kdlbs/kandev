---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATION-RETRIES-001
created: 2026-09-29
owners:
  - kandev
---

# Automation Retry System Design

## Purpose and boundaries

The Office automation service owns retry policy, durable retry-group identity, attempt scheduling, operation recovery, and retry-history projection. The existing task/session system owns task execution and session lifecycle; this design records the exact task and turn bindings needed to reconcile automation runs but does not replace those contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATION-RETRIES-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Persistence](#persistence), [Security](#security) |

## Components and responsibilities

- `internal/automation` validates and persists `RetryPolicy`, admits retry groups and attempts, claims due work, projects history, and coordinates cancellation.
- The automation lifecycle worker periodically discovers due automations, obtains a durable run claim, and dispatches the attempt through the existing automation service.
- Retry admission and task operations persist the intent before creating a task. The operation ledger makes task creation recoverable across process interruption.
- The WebSocket/API handlers expose retry policy, group controls, and bounded history to the web client.
- `retry-policy-section.tsx` edits the retry policy. `runs-section.tsx` composes attempt rows into the configured attempt or timeline view. The history hook owns cursor progression and refresh timing.

## Data and contracts

- `RetryPolicy` stores mode (`disabled`, `finite`, `infinite`), canonical decimal `max_retries` and `delay_seconds`, backoff (`fixed`, `exponential`), and history mode (`attempts`, `timeline`). Disabled mode normalizes retry-only fields to fixed, zero-delay, attempts mode. Finite mode requires a retry count of at least one. Missing policy defaults to disabled.
- A `RetryGroup` identifies the original firing and carries the current generation and state. Each `AutomationRun` attempt carries its group ID and generation, attempt number, policy snapshot, trigger snapshot, launch snapshot, and exact `retry_scheduled_at` value where scheduled.
- Retry task intents and operation rows persist the expected task-creation action before side effects. Retry claims, operation leases, and outbox leases use database state and expiration rather than process-local ownership.
- Retry-history responses are bounded and use an opaque continuation cursor plus a high-water mark. The service supplies enough group and attempt data for either view without making the client infer ownership from adjacent rows.
- Generic webhook deliveries require `X-Webhook-Secret` and `X-Kandev-Delivery-ID`. The delivery ID is unique per automation receipt. Retry snapshots retain the legacy payload when no safe pointers are configured; interpolation, dedup-key, and repository-selector paths resolve against that payload. When pointers are configured, only selected, bounded values are retained. The whole webhook body is limited to 1 MiB.

## Control flow

1. Policy updates are normalized and persisted with the automation. Switching from disabled to finite supplies a valid retry count before saving; disabled mode hides or locks retry-only controls.
2. A failed eligible attempt advances its retry group generation transactionally, records the next attempt and its exact due timestamp, and writes any required publication intent to the outbox.
3. The lifecycle worker scans due work at a bounded cadence. A database claim serializes instances. Capacity deferral releases the claim with a later due time instead of repeatedly retrying at the scheduler tick rate.
4. Admission persists a task intent and operation before invoking task creation. Completion binds the resulting task/session/turn to the exact run and acknowledges the matching outbox lease.
5. Recovery reads pending or expired work, acquires a time-bounded lease, validates the current group generation, resumes only the durable operation still owned by that lease, and releases or acknowledges only with the matching token.
6. The web client requests bounded history pages. Timeline rows are projected from complete retry groups before status filtering and counts; deleting a visible group expands to all underlying attempt IDs. Standalone runs remain in the ordinary run history.
7. Refresh follows the next scheduled retry due time and stops when no visible retry is pending. Cursor traversal tracks every seen cursor and has a finite page bound.

## Failure and recovery

- A transient provider failure creates a retry only while the group generation and policy permit one. Replayed failure signals return the already-created next attempt rather than duplicating it.
- A live retry-operation lease is not reclaimed during startup. Expired operation and outbox leases can be reclaimed by another instance; token and generation checks reject stale workers.
- If task creation succeeded before a crash, recovery resolves the existing task through the durable intent/idempotency identity instead of creating another one.
- A failed stop leaves the run and its ownership records available for retry or reconciliation. Disabling an automation fences new admission before it attempts to stop currently open runs.
- Capacity exhaustion schedules a later retry without consuming the attempt or making a tight claim/deferral loop.
- A history request failure is surfaced to the user. It does not erase prior rows or loop forever on a repeated cursor.
- Permission requests that cannot be answered because the original turn no longer exists terminalize the run through its task binding rather than leaving an admitted run open.

## Persistence

Retry policy and lifecycle state live in the automation tables, runs, retry groups, task intents, operation ledger, and retry-event outbox. Migrations establish indexes for due retries and group/attempt identity on both supported SQL dialects. Indexed queries preserve PostgreSQL placeholder rebinding and context cancellation. Recovery is restart-safe and works across multiple backend instances; no process-local scheduler state is authoritative.

## Security

Webhook secrets are compared before accepting a receipt and are never copied into retry snapshots or history. Delivery identifiers and webhook bodies are bounded. Configured JSON pointers constrain which payload values enter retained retry input. Retry APIs authorize the automation in its owning workspace, and the client cannot supply task, generation, lease, or retry-group ownership identities.
