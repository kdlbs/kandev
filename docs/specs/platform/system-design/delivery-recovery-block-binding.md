---
status: current
system: platform
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
---

# Delivery recovery block binding

This design covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5 and 006.9.
The repair package is [delivery block binding](../../../plans/delivery-recovery-block-binding/plan.md).

An open `agent_delivery` block bound to a submission belongs to that incident
until the incident is settled or recovered. Two rules keep it that way:

- **The binding cannot change.** `upsertSessionRecoveryBlockTx` never changes
  the consumer or submission of an open bound delivery block. A conflicting
  write from another consumer or for another submission only refreshes
  `updated_at`. An unbound open block (empty submission) may still gain a
  binding.
- **Settling a notice preserves the terminal binding.**
  `ClearAgentDeliveryRecovery` matches the notice by session, incarnation,
  submission, and generation, then settles it in one transaction. If a matching
  terminal effect is already `block_resolved` or `completed`, it also resolves
  the submission's open block with that effect's stream, sequence, turn,
  outcome, and `durable_delivery_<outcome>`. This allows a terminal event on a
  reattached stream to replace the notice's older stream. If no terminal effect
  exists yet, Clear settles only the notice; later terminal settlement resolves
  the matching block.

The resolved source block is the evidence the
[historical terminal](session-recovery-convergence.md#historical-terminal-incidents)
check requires. A later interrupted prompt therefore recovers through the
existing Resume action.
