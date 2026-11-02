---
status: active
system: platform
created: 2026-10-11
updated: 2026-10-12
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

### REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002: Convergent interrupted-session recovery

**Intent:** All recovery entry paths apply the same honest disposition after hard
crash or graceful shutdown, without repeated incident-specific database repair.

**User story:** As a user, I want my saved conversation to become usable when its
owner is safely recoverable, even when recovery bookkeeping is inconsistent.

#### Acceptance criteria

- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.1:** Graceful shutdown during an active durable prompt shall preserve uncertainty when teardown releases the call without terminal proof. Deterministic provider rejection and retained terminal effects remain authoritative; request cancellation cannot prevent durable settlement.
- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.2:** Native recovery shall preserve differing historical journal labels and SQL uncertainty while acknowledging exact unproven history. It shall neither invent completion/cancellation nor redispatch the old instruction, including queue-origin instructions.
- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.3:** Restart, runtime replacement, ordinary resume launch, explicit Resume, and new prompt/queue admission shall consume one consistent recovery disposition. Complete authenticated inventory and source-owner proof are required; live or unknown independent ownership remains protected.
- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.4:** Missing stream associations, absent notices and stale already-restored earlier notices shall converge through guarded exact proof and atomic resolution. Repeated restart and interruption during recovery shall reuse retained progress and admit a distinct new instruction when all independent gates pass.
- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.5:** Every supported reachable cross-store state shall have a tested safe disposition at dispatch, terminal, acknowledgement, teardown, restoration and publication boundaries. Incomplete/conflicting evidence shall surface the actual cause and supported recovery action without fabricating certainty.
- **AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.6:** A delivery notice already marked recovered whose execution was later lost shall not strand the session. Explicit Resume on the current ready native execution shall restore that notice through the same authenticated retirement transaction, preserving its historical execution, submission, payload, and unknown outcome. A settled notice, foreign generation or stream, or missing retirement evidence shall keep recovery blocked.

## Technical design and delivery records

- [System design](../system-design/durable-restart-reconciliation.md).
- [Implementation package](../../../plans/journal-growth-restart-reconciliation/plan.md).
- [Proposed pruned-history recovery](pruned-delivery-recovery.md) defines an
  additional restoration contract when acknowledged events are absent from
  both stores. Current reconciliation does not establish that missing receipt.

- [Convergence design](../system-design/session-recovery-convergence.md).
- [Convergence implementation](../../../plans/session-recovery-convergence/plan.md).
