# ADR-2026-09-28-repeated-unclassified-fallback: Opt-in repeated unclassified fallback

**Status:** accepted
**Date:** 2026-09-28
**Area:** backend

## Context

[Issue #4015](https://github.com/kdlbs/kandev/issues/4015) proposes recovery from repeated unknown failures in task dynamic routing.
The user accepted this direction after reviewing the manual-recovery behavior and safety boundaries.
The implementation is delivered in the linked plan and its three completed work orders.

The [provider policy decision](2026-08-17-provider-error-classes-and-policies.md) requires manual recovery for all unclassified errors.
That rule prevents automatic replay from ambiguous evidence, but also stops recovery from repeated, proven safe startup failures.

## Decision

Add a disabled-by-default threshold exception for individual dynamic candidates in Kanban/task sessions.
This decision supersedes only the unconditional unclassified-stop rule for this narrow scope.
Transient and hard remain the only configurable error classes with retry and reset-wait pipelines.

Separate evidence of replay safety from semantic classification.
Count only distinct attempts with identical bounded diagnostic identity and authoritative evidence of no output or effects.
A repeated message alone never authorizes recovery.
The workflow step can veto the exception.
Below the threshold, each failure still needs manual recovery.
At the threshold, use the existing fenced continuation and successor-launch path.

The [agent design](../specs/agents/system-design/dynamic-unclassified-fallback.md) defines eligibility, persistence, and reset rules.
The existing provider catalogue retains its classifications and fallback flags.

## Consequences

Operators can recover from a consistently unusable candidate without broad unknown-error fallback.
Persisted counts need attempt deduplication and explicit reset semantics.
Unknown execution scope, uncertain message identity, or missing effect evidence prevents automatic advancement.
The first version exposes configuration through existing APIs and workflow documents, without new visual controls.

## Alternatives Considered

- Manual recovery only: preserves the existing boundary but leaves the reported repeated-failure case unresolved.
- Immediate unknown-error fallback: one failure does not establish repetition and can hide configuration faults.
- Automatic retries until threshold: adds a retry owner and behavior outside the accepted scope.
- Shared candidate health penalties: can affect unrelated tasks with different environments.
- Same candidate without matching diagnostics: combines unrelated failures and weakens the evidence requirement.
