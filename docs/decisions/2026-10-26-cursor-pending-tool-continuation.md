# ADR-2026-10-26-cursor-pending-tool-continuation: Delegate pending tool recovery to Cursor

**Status:** accepted
**Date:** 2026-10-26
**Area:** backend, protocol, frontend

## Context

[Issue #4801](https://github.com/kdlbs/kandev/issues/4801) reports transport loss
with completed and pending foreground tools. The completion-only policy refuses
continuation before its first attempt. The UI reports that retry stopped.

The [native probe report](https://github.com/kdlbs/kandev/issues/4801#issuecomment-6418179442)
shows that a command can survive prompt settlement without a terminal ACP result.
One continuation repeated the command after a denied diagnostic request.
The report establishes neither generic reconciliation nor prevention of overlap.

After reviewing that evidence, the user explicitly requested automatic retry
with pending tools. The agent must use its conversation to inspect or retry work.
This is a product policy choice, not confirmation of native execution guarantees.

## Decision

Permit versioned automatic continuation after recognized transport loss on a
retained Cursor runtime despite valid pending foreground tools. Preserve the
same conversation and exact internal `continue` instruction. Cursor owns any
subsequent tool inspection and retry. Do not require a native tool-result query.

Keep the original-request replay fence, one recovery owner, existing budget,
human priority, permission checks, generation checks, and background exclusions.
Failed/cancelled or malformed tool evidence remains ineligible. Runtime loss
ends this pending-tool policy; completed-tool restoration remains unchanged.

This amends the pending-tool exclusions in the
[October 5 decision](2026-10-05-hidden-completed-tool-continuation.md) and
[October 8 decision](2026-10-08-unconditional-interruption-continuation.md).
It adds no opt-in or confirmation step. V1/V2 retain their wire meanings.

## Consequences

The reported mixed-tool turn can continue without a user message. The host can
prove bounded continuation dispatch and native identity. It cannot promise that
Cursor checks every tool, receives its missing result, or avoids duplicate effects.
An original tool and a new model-selected action can overlap. Persisted history
and synthetic cancelled cards are not proof of execution state.

Tests must distinguish host prompt replay from actions the agent chooses later.
Public documentation must state this boundary without claiming reconciliation.
Existing native evidence is sufficient to document the risk; successful native
reconciliation is not an implementation gate for the chosen policy.

## Alternatives Considered

- Require native reconciliation: the tested ACP surface did not provide it;
  this retains the intervention the user explicitly wants to remove.
- Wait a fixed time: tool duration is unbounded and elapsed time proves no outcome.
- Kill tools before continuing: partial effects remain and native ownership is absent.
- Remove every safety check: pending tools do not authorize permission bypass,
  stale callbacks, or original-request replay.

## Related contracts

- [Requirements](../specs/platform/requirements/provider-interruption-continuation.md)
- [Design](../specs/platform/system-design/provider-interruption-continuation.md)
- [Plan](../plans/cursor-pending-tool-continuation/plan.md)
