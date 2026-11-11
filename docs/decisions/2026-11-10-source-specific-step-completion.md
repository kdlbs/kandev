# ADR-2026-11-10-source-specific-step-completion: Bind completion to its source

**Status:** accepted
**Date:** 2026-11-10
**Area:** workflow

## Context

ADR 0015 deduplicates signals for an entire workflow step.
Entry attribution later protects against leaving and returning to that step.
Neither rule distinguishes two native turns within one entry.

Issue [#5173](https://github.com/kdlbs/kandev/issues/5173) exposes this distinction.
A focused reproduction through canonical output projection confirms that a later correction retains the earlier handoff.
The duplicate acknowledgement also contains no transition proof.

## Decision

Deduplicate by authenticated source turn and workflow entry.
Keep the first payload within that source.
Permit a later current turn to replace an unconsumed signal before the source entry changes.
Keep native verification without another completion call neutral.
Preserve the existing cancellation of pending completion when the user sends new work.

Fence transition commitment with the exact signal identity, in addition to the existing entry guard.
Retain one bounded receipt on the source session after consumption.
Record committed transition proof in the same transaction as the move.
Report the current route separately from that historical proof.

Do not reopen a committed source entry through a stale signal.
Do not replace an independent Review recipient with the source session.
Do not claim prompt delivery from a database transition receipt.

This proposal amends ADR 0015's step-wide duplicate rule and extends its asynchronous acknowledgement contract.
The [requirements](../specs/tasks/requirements/workflow-completion-outcomes.md) and
[design](../specs/tasks/system-design/workflow-completion-outcomes.md) define the contract.

## Consequences

Later completed corrections can reach the configured destination without duplicate transitions.
Retries can distinguish acceptance from committed movement.
The implementation adds JSON metadata and transaction checks, without a new table.
One retained receipt is sufficient for the latest source, but does not provide complete signal history.
Older unmatched requests must fail safely with an unproven or stale result.

## Alternatives considered

- **Keep the first signal for the entry:** Discards later native corrections, as the reproduction demonstrates.
- **Replace on every call:** Lets retries overwrite accepted content and makes network duplication change reviewer input.
- **Use timestamps or payload hashes:** Neither establishes authenticated source identity or request order.
- **Change generic task state to match the step name:** Conflates runtime activity with workflow routing.
- **Use best-effort history as proof:** A history write can fail independently of the committed move.
- **Add a signal-history table:** Adds schema and retention work beyond the latest-source recovery contract.
