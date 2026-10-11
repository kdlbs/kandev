---
status: active
system: platform
created: 2026-10-11
updated: 2026-10-11
owners:
  - kandev
---

# Durable restart reconciliation requirements

## Overview

Platform owns recovery of delivery state across backend and shared-runtime restarts.
This extends [durable agent delivery](durable-agent-delivery.md), particularly
REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006 and its restore-only criteria .20 through .24.
Existing ownership, archived/Office/automation admission, journal durability, and native-state
availability restrictions remain effective. These additions define automatic repair while preserving durable owner proof.

## Requirements

### REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001: Automatic repair of provable inconsistencies

**Intent:** A recoverable mismatch after restart must not require manual database editing.

**User story:** As a user, I want my existing sessions to recover after restart, so that
inconsistent delivery records do not strand their saved conversations.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1:** Backend restart or confirmed shared-runtime replacement shall automatically reconcile uniquely provable disagreements between retained journal records, SQL submissions, recovery blocks, and execution records. This includes an abandoned journal dispatch whose matching SQL submission is uncertain, a dead execution recorded as active, stale recovery presentation after verified restoration, and both stores still dispatching without recovery metadata or block identities after host loss. A matching idle replacement that already loaded the native conversation shall be reused after historical owner death is proven.
- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2:** Repair shall preserve session, native conversation, workspace, retained output, payload identity, and historical uncertainty. It shall not resubmit an interrupted prompt, replay tools, invent a terminal outcome, release queued work, or require browser interaction.
- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3:** Recovery shall validate every unresolved submission in the affected session before acknowledging uncertainty. One matching record shall not override conflicting or unverifiable evidence. Missing ownership, process-termination, native-state, or payload evidence shall leave that session inspectably blocked while unrelated eligible sessions recover.
- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4:** Successful automatic restoration or explicit native Resume shall persist and publish a newer restored recovery snapshot with resolution of the matching delivery block. Desktop and phone shall show the ordinary available composer after recovery and reload, without an obsolete interruption banner. A distinct later instruction shall use normal admission; the old interrupted submission remains non-replayable.
- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5:** Repeated reconciliation and crashes between journal and SQL writes shall converge from retained progress without duplicate restore, fabricated completion, or clearing independent recovery causes. Restored sessions shall not repeatedly consult retired runtime handles or rediscover resolved historical recovery as new uncertainty. A newer notice for an identical source may refresh an unallocated checkpoint; allocating a candidate pins its source. Later partial failure notices shall preserve the same submission's known stream binding; existing partial fences require authenticated owner and payload evidence before repair.

## Technical design and delivery records

- [System design](../system-design/durable-restart-reconciliation.md).
- [Implementation package](../../../plans/journal-growth-restart-reconciliation/plan.md).
