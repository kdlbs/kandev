---
status: current
system: platform
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
---

# Delivery recovery block binding

This design covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5, 006.9, and 006.16.
The repair package is [delivery block binding](../../../plans/delivery-recovery-block-binding/plan.md).

An open `agent_delivery` block bound to a submission belongs to that incident
until the incident is settled or recovered. Two rules keep it that way:

- **The binding cannot change.** `upsertSessionRecoveryBlockTx` never changes
  the consumer or submission of an open bound delivery block. A conflicting
  write from another consumer or for another submission only refreshes
  `updated_at`. An unbound open block (empty submission) may still gain a
  binding. The resolved-row update guard preserves that bound identity too.
- **Settling a notice preserves the terminal binding.**
  `ClearAgentDeliveryRecovery` matches the notice by session, incarnation,
  submission, and generation, then settles it in one transaction. The notice
  stream is not part of this identity. If the latest matching terminal effect
  is `block_resolved` or `completed`, Clear also resolves the submission's open
  block with that effect's stream, sequence, turn, outcome, and
  `durable_delivery_<outcome>`. This allows a terminal event on a reattached
  stream to replace the notice's older stream. If no terminal effect exists,
  Clear settles only the notice; later terminal settlement resolves a matching
  block. A repeated Clear for the same settled identity may repair that block
  when its terminal effect arrives later.
- **Retry settles only a stale inactive binding.** When `retry_connection`
  encounters an open delivery block for a submission different from the
  settled recovery notice, it verifies the block and canonical submission in
  one transaction. `completed`, `failed`, `cancelled`, and `interrupted_unknown`
  submissions can be settled; `prepared`, `accepted`, and `dispatching`
  submissions stay blocking. If a matching terminal effect exists, its binding
  is written to the block. The session row is locked before any block update.

The resolved source block is the evidence the
[historical terminal](session-recovery-convergence.md#historical-terminal-incidents)
check requires. A later interrupted prompt therefore recovers through the
existing Resume action.
