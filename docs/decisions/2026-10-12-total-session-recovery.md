# ADR-2026-10-12-total-session-recovery: Preserve outcomes while converging admission

**Status:** accepted
**Date:** 2026-10-12
**Area:** protocol

## Context

Graceful shutdown can release a background ACP call before its stop channel is
closed. A typed RPC error then becomes a journal failure while SQL records
uncertainty. Native restoration succeeds, but label equality strands admission.
Legacy recovery fences can also lack their stream binding or carry a restored
notice for a different earlier incident. Independent incident exceptions do not
establish a recovery contract.

## Decision

Separate historical outcome from admission. Keep journal labels, immutable
payloads, SQL uncertainty, terminal references and receipt cursors. A loaded
native conversation and authenticated complete inventory permit retirement of
matching uncertain history, including an unproven historical terminal label,
without asserting completion or replaying the payload. Actual terminal evidence,
known SQL failures and independently active work retain their authority.

Use one lifecycle submission assessment for native retirement, repeated adoption
and automatic already-loaded recovery. Commit missing fence associations only
with authenticated current stream and exact inventory proof, under existing
SQL admission locks. Publish original-child stop intent before adapter teardown;
bounded settlement survives cancellation. Keep existing exact dead-owner proof,
restore checkpoints, gap archives and independent admission rules.

## Consequences

Recovery is restartable across journal retirement and SQL fence resolution.
Legacy labels remain readable and are not interpreted as terminal proof alone.
Incomplete or conflicting evidence yields a precise refusal; no storage surgery
or fabricated receipt is a supported recovery method. New state additions must
extend the finite disposition and persistence interruption tests.

## Alternatives considered

- Normalize every failure to unknown: erases genuine deterministic failure.
- Clear every recovery fence: overrides independent incidents and live owners.
- Replay the old instruction: duplicates external effects.
- Make the stores atomic: unavailable across backend and agentctl failure domains.

## Related contracts

- [Requirements](../specs/platform/requirements/durable-restart-reconciliation.md).
- [Convergence design](../specs/platform/system-design/session-recovery-convergence.md).
- [Honest delivery loss](2026-10-11-honest-delivery-loss-recovery.md).
