---
status: draft
system: agents
requirements:
  - REQ-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001
created: 2026-10-31
updated: 2026-10-31
owners:
  - cfl
---

# Dynamic Circuit Health System Design

## Requirement mapping

| Requirement                             | Design section                                         |
| --------------------------------------- | ------------------------------------------------------ |
| `REQ-AGENTS-DYNAMIC-CIRCUIT-HEALTH-001` | Admission, persistence, API, and control read lifetime |

## Admission and persistence

The existing dynamic engine owns resource circuits. Fixed-profile admission reads
the same registry before launch or prompt delivery and reports a suspension
without selecting a different profile. Failure attribution uses the current
execution and route generation. Account holds require owner release; expired
timed and restored half-open observations do not become permanent fixed-profile
blocks. Recording an unrouted classified failure does not authorize a successor.

The lifecycle resolver captures the same profile snapshot used for the execution
environment. Its existing resource key is retained with the execution/profile
pair and propagated by trusted runtime events. Failures and clean completions
use that captured resource, not later profile settings. An unknown retained key
cannot charge or reset today's credential; account holds keep their owner-release
contract. Explicit profile secret references supply their binding scope even
when the controller-only billing projection is absent from the store read.

A pre-provider circuit refusal preserves queued prompt identity and recorded
history. Unattempted sessions wait rather than fail. The existing admission sweep
rechecks only recorded waits; ordinary queue auto-run and session-incarnation
guards still apply. Automation continuations retain their existing run, prompt,
and retry tuple and resume through the existing dispatcher after suspension
expires or is released. No new attempt, run, worker, or retry budget is created.

The original `features.dynamicCircuitBackoffEscalation` option is constructed at
startup and remains OFF in every shipped profile. Enabled escalation retains its
existing bounded ladder and reset semantics. The additive `failure_count` column
belongs to `dynamic_resource_circuits`; it introduces no new ledger. Snapshot
copies are collected under the registry mutex, sorted by resource key, and carry
the known observation time. An unknown old timestamp remains absent. Existing
pending durable-write handling and hold rollback remain in place.

## Read API

`GET /api/v1/routing/circuits` follows the existing agent-settings read boundary.
Its narrow controller reader exposes only snapshots. The backend adapter maps
resource scope and UTC times; it has no routing, release, probe, or dispatch
method. A configured inert engine returns an empty list. A missing reader returns
503, and a read failure returns an error rather than an empty success.

## Control read lifetime

The current integrated-control providers dashboard mounts one installation-wide
circuit section when an authorized workspace snapshot exists. It uses that
snapshot object as its read owner. The request is cancelled on replacement or
unmount; an old owner's result is never displayed for a new owner. The existing
control foreground/manual refresh creates the next read, with no new timer.

The wire validator rejects malformed states, counts, and times. Rendering keeps
the original circuit-state ordering, localization, narrow resource identifiers,
and distinct loading/denied/unsupported/error/empty states. Transient failures
retain only the same authorized owner's dated snapshot and mark it stale;
denied, unsupported, and changed-owner reads never reuse those values.
The port uses the existing dashboard
layout rather than importing the older aggregate/control helper implementation.
P6/K18 provider-budget policy values are not inferred from these circuit rows.
