# ADR-2026-10-09-retained-delivery-record-reconstruction: Reconstruct verified delivery associations

**Status:** accepted (implementation pending)
**Date:** 2026-10-09
**Area:** backend, protocol

## Context

An older session can retain a submitted prompt in agentctl while its canonical
backend submission and recovery metadata are absent. Native resume does not
resolve that discrepancy. PR #4380 keeps such sessions blocked.

Backend SQL remains the authority for conversations, admission, and workflow
effects. The authenticated journal retains the immutable delivery payload and
its observed state. Neither store alone proves that interrupted tools completed.

## Decision

Permit an authorized recovery operation to reconstruct a missing delivery
association from uniquely verified journal evidence. Keep this operation separate
from prompt admission and native continuation.

Require independent backend session, incarnation, and generation evidence.
Validate the complete candidate set and immutable payload before a conditional
transaction creates the submission, provenance, recovery projection, and matching
block binding. Preserve existing records and independent blocks.

Reconstruction does not grant process-control authority. Preserve unknown
execution or process identity as unknown. Continuation retains the existing
termination and ownership gates, even after the record repair succeeds.

Use the existing backend-to-runtime authenticated boundary. Do not expose
journal payloads through browser recovery responses or generic diagnostic logs.

The [system design](../specs/platform/system-design/durable-agent-record-recovery.md)
defines the evidence matrix, transaction, and failure behavior.

## Consequences

Verified older sessions can enter normal reconciliation without manual database
or journal edits. Unprovable sessions remain blocked with a specific cause.
Record reconstruction and successful continuation remain separate outcomes.

The change adds bounded evidence inspection and a conditional repository write.
It does not introduce a second transcript store or an automatic resend policy.

## Alternatives considered

- Leave every missing record blocked: safe, but rejects recoverable installations
  despite sufficient retained evidence.
- Trust the newest journal entry: candidate order cannot prove ownership or
  exclude another unresolved prompt.
- Clear all recovery blocks after native resume: hides unresolved delivery and
  permits duplicate effects.
- Recreate the old payload from chat text: composed prompts and attachments can
  differ from displayed text. This breaks immutable submission identity.
- Import all journals during startup: expands authority and creates an unbounded
  migration across unrelated sessions.

## Related decision

[Durable harness session boundaries](2026-09-10-durable-harness-session-boundaries.md)
retains authority over the separation between SQL, delivery journals, and native state.
