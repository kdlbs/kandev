---
status: current
system: platform
requirements:
  - REQ-PLATFORM-UNDELIVERED-PROMPT-001
  - REQ-PLATFORM-DELIVERY-RETENTION-001
---

# Undelivered prompt recovery

## Ownership and scope

Platform owns delivery admission and recovery evidence. This extends
[interrupted recovery](interrupted-prompt-recovery.md). Criteria .1 maps to local
dispatch classification; .2 and .4 reuse native Resume; .3 adds the final admission
guard; .5 verifies the existing desktop and phone controls.

## Local non-delivery evidence

The agentctl client returns `RequestNotSentError` only before a WebSocket write is
attempted. The error preserves its cause. A missing acceptance callback, provider
error text, write error, lost reply, or timeout after a write is not non-delivery
proof. The lifecycle retry path may reconnect only after a typed unsent error.
If every retry stays local, reconnect failure retains that evidence. Once a write
was attempted, its error is returned without the earlier unsent classification.

The runtime captures the initial submission returned by durable admission and
settles a typed unsent dispatch failure before invoking the initial failure handler.
It keeps the same captured owner for both caller-admitted and runtime-admitted
initial prompts. A missing client before the RPC retains the unsent error type.

The orchestrator checks both the typed error and the acceptance callback. Only an
unaccepted, proven unsent request can settle as failed. The repository pins the
captured submission ID, session, incarnation, harness and owner generations, hash,
dispatching state, and absence of terminal sequence under the existing SQL locks.
It sets `failed`, the failed outcome, and a settlement timestamp without a terminal
event. A failed conditional write preserves recovery protection. No journal
capability, new journal state, or new agentctl endpoint is introduced.

A failed row with a settlement timestamp and no terminal identifiers is a local
non-delivery receipt. The existing bounded retention sweep can prune its redundant
payload after the grace period, without inventing terminal or workflow events.
Candidate selection and the conditional update both reject conflicting terminal
evidence and open delivery recovery blocks. Identity, hash, outcome, and claim
association remain. Ordinary failed rows without settlement proof remain protected.
This extends the [retention design](backend-agent-delivery-retention.md).

## Existing historical recovery

Main already includes missing-peer retirement from PR #5312 and stale-history
reconciliation from PR #5320. Explicit Resume writes a non-replayable retirement
record for eligible SQL-only interrupted history under the existing process-owner
serialization. It then verifies the retained evidence and resolves the incident.
The historical SQL outcome remains unknown. This change does not infer failed
execution from journal absence or replace that recovery contract.

The earlier proposal for an `absence_fencing` capability and `not_admitted` journal
state was removed after the upstream comparison. Its implementation is not part
of this change. [Decision](../../../decisions/2026-11-12-undelivered-prompt-evidence.md)
records the final boundary.

## Admission and presentation

The final prompt preparation callback checks all open unknown-outcome blocks for
the current session identity before creating another submission. It runs inside
the existing dispatch admission boundary. This preserves the original incident
binding if a stale client, provider retry, or queued dispatch tries to send again.
Other recovery causes keep their existing capability reconciliation behavior.

Existing Resume, Retry connection, and Stop controls remain unchanged. The browser
regression sends a message while recovery is blocked, waits for its asynchronous
blocked delivery status, and verifies unchanged submission IDs and block binding.
It then uses Resume, checks native identity and retirement of two SQL-only prompts,
reloads, and sends a distinct instruction. Phone checks include control geometry
and horizontal overflow. No rendered UI or localization changes are required.

## Verification

Real client and SQLite regressions cover unsent requests, lost replies, reconnect
failure, accepted callbacks, changed ownership, and admission blocking. Existing
native Resume tests and desktop/phone E2E cover integration with upstream recovery.
Run scoped race tests, SQL guard, changed-package lint, and documentation checks.
PostgreSQL remains unverified when no disposable test database is available.
