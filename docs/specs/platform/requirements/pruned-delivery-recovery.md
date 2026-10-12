---
status: active
system: platform
created: 2026-10-11
owners:
  - kandev
---

# Pruned delivery recovery requirements

## Overview

Platform owns durable delivery and restoration of existing conversations. This
extends [restart reconciliation](durable-restart-reconciliation.md) to a missing
delivery interval that neither backend receipt nor supported journal reads can supply.
Restoration can preserve a conversation without claiming its unavailable output
was recovered. The original instruction must remain non-replayable.

## Terminology

- **Missing interval:** A contiguous interval acknowledged and pruned by the
  authenticated journal but absent from the backend's durable receipt history.
- **Historical loss notice:** Persistent evidence that output from an earlier
  delivery stream is unavailable, separate from a current admission block.

## Requirements

### REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001: Continue existing conversations with honest loss evidence

**Intent:** A power loss must not permanently strand a saved native conversation
or turn missing output into fabricated receipt, completion, or repeated work.

**User story:** As a user, I want my saved conversation restored after a crash,
while knowing whether part of its previous output is unavailable.

#### Acceptance criteria

- **AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.1:** Delivery acknowledgment shall require backend receipt and applied effects that remain durable across a supported host power loss. A failed durability boundary shall prevent acknowledgment and preserve retryable progress.
- **AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.2:** Startup shall preserve the verified owner's actual generation stream identity and expose a specific receipt-gap failure rather than an unrelated ACP connection error. Undiscovered capability, an absent generation, or a missing checkpoint shall not imply legacy delivery or permission to skip history.
- **AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.3:** An unavailable acknowledged interval shall produce durable, inspectable evidence identifying its stream and missing range. Receipt, projection, and retirement progress shall describe actual persisted work. An expired journal cursor with complete backend receipt shall instead recover from that receipt without declaring output lost. If complete original events become available through validated journal replay, normal receipt/projection shall take precedence over the loss fallback and no unavailable-output notice shall remain for that interval.
- **AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.4:** After uniquely verifying the original owner has terminated, every unresolved submission's identity and payload, current ownership, and saved native state, recovery shall automatically restore the same native conversation when the affected historical stream is fully acknowledged and has no retained suffix. The existing explicit Resume action shall use the same guarded recovery. Missing recovery metadata or an absent block shall not prevent detection. Active, ambiguous, or unverifiable owners and independent admission restrictions shall remain blocked.
- **AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.5:** Successful recovery shall preserve session, incarnation, workspace, canonical history, native conversation, and recorded submission outcomes. A new delivery generation shall admit only new instructions. Recovery shall not dispatch a prompt, invent missing events or terminal outcomes, repeat tools, drain paused queues, or treat the missing range as received or projected.
- **AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.6:** Repeated recovery and another restart shall reuse the same recorded recovery progress, preserve the historical loss notice, and keep the restored conversation usable. A crash between store writes shall not allocate multiple successors, clear a newer block, or claim restoration before native identity is verified.
- **AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.7:** Desktop and phone shall expose the specific missing-output cause through the existing inline recovery surface. After successful restoration, the ordinary composer shall be available with a historical notice of unavailable output and uncertain prior work. No acknowledgment form or repeated continuation instruction shall be required. Details shall remain inspectable after reload without obstructing unrelated sessions.
- **AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.8:** Unsupported peers, an unavailable native conversation, unacknowledged retained output, conflicting submission evidence, or unknown process termination shall expose a specific blocked result. These conditions shall not trigger journal deletion, conversation replacement, or downgrade of unresolved durable work.

## Out of scope

- Reconstructing the contents of erased events or guaranteeing external tool rollback.
- Automatically continuing surviving agents across an irrecoverable receipt gap.
- Discarding a retained suffix to make an otherwise ineligible stream recoverable.
- Recovery after loss of both the native conversation and its delivery evidence.

## Design and delivery

- [System design](../system-design/pruned-delivery-recovery.md).
- [Implementation package](../../../plans/pruned-delivery-recovery/plan.md).

## Related no-gap recovery regression

Matching current receipt coverage must use normal native Resume. Journal unknown
versus stale SQL dispatch, an old empty generic admission block, and absent or
verified restored historical metadata must not strand a successfully loaded
conversation. Preserve old uncertainty and cursors, verify authenticated retirement,
and refuse independent live work or unresolved metadata. This applies the existing
durable restart reconciliation contract; it does not authorize loss recovery.
