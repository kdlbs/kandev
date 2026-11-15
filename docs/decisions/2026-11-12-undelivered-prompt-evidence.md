# ADR-2026-11-12-undelivered-prompt-evidence: Require local non-delivery evidence

**Status:** accepted
**Date:** 2026-11-12
**Updated:** 2026-11-15
**Area:** backend

## Context

A missing acceptance callback can mean either an unsent request or a lost reply
after admission. Main now recovers eligible SQL-only unknown history through
serialized retirement (PR #5312), with later stale-history fixes (PR #5320).

## Decision

Use typed local control-flow evidence to settle a request as failed only when no
write was attempted and no acceptance callback was observed. Never retry after a
lost response merely because the connection closed. Preserve current-owner SQL
conditions and block new admission while an unknown outcome remains unresolved.

Initial and follow-up dispatch use the same conditional SQL settlement. A failed
submission with a settlement timestamp and no terminal identifiers is the local
non-delivery receipt. Retention uses that receipt after its existing grace and
protection checks; it does not fabricate terminal or workflow effects.

Reuse the existing Resume contract for historical uncertainty. Keep its unknown
outcome and non-replayable retirement record. Do not add the previously proposed
negative-receipt state, capability, or endpoint now that upstream handles recovery.

## Consequences

Local failures no longer create false uncertainty. Ambiguous effects stay unknown.
The change needs no peer capability upgrade; historical recovery uses the support
already present on main. Conditional SQL failure keeps recovery protection.

## Alternatives considered

- Treat a missing callback as failure: unsafe when the acceptance reply is lost.
- Retry every disconnect: may resend work that agentctl already admitted.
- Add a parallel historical recovery protocol: unnecessary after PR #5312.

## References

[Design](../specs/platform/system-design/undelivered-prompt-recovery.md).
[Plan](../plans/undelivered-prompt-recovery/plan.md).
