# ADR-2026-11-05-capacity-known-command-failures: Continue capacity recovery after known command failures

**Status:** accepted
**Date:** 2026-11-05
**Area:** backend, protocol

## Context

The reported Codex conversation refused capacity recovery twice with a retained runtime and zero retry dispatches.
Read-only database inspection and native ACP frames show terminal command failures, with no pending tools in either turn.
The pictured turn contains 31 successful tools and six commands with nonzero exit codes.
Each unsuccessful command has matching terminal identity and no signal termination.
An earlier turn contains 43 successful tools and one such command failure.

The current capacity ledger treats every failed tool as blocking work.
A normal test or search command can therefore disable recovery for the rest of its turn.
The existing scheduler already provides exponential backoff and a bounded recovery window.
The missing contract distinguishes known unsuccessful commands from uncertain tool outcomes.

## Decision

Permit capacity continuation after confirmed terminal command failures on a tested retained Codex ACP conversation.
Use a new version of the typed capacity evidence contract.
Require current prompt identity, terminal execute-tool status, matching terminal identity, an integer nonzero exit code, and no cancellation or signal termination.
Keep ambiguous failures, failed non-command tools, pending work, unresolved permissions, and unaccounted background work ineligible.

Retain real failed results in conversation history.
Use the existing capacity instruction, which continues unfinished work and prohibits repetition of completed actions.
Keep original-prompt replay, runtime preservation, provider ownership, cancellation, and dispatch uncertainty boundaries intact.
Reuse the existing eleven-attempt backoff episode and thirty-minute admission window.
Do not restore a lost runtime or switch models through this exception.

Older versions retain their existing failed-tool policy.
OpenCode V1 gains no new failed-tool authority from Codex evidence.
An older host cannot accept the new support version without its predicates.

This amends the unsuccessful-command exclusion in the [completed-tool capacity decision](2026-10-05-capacity-continuation-after-completed-tools.md).
It does not amend the separate Cursor pending-tool policy.
Implementation and validation results are recorded in the [delivery package](../plans/turn-error-continue-capacity/plan.md).

## Consequences

A failed test or search command no longer blocks a later capacity retry when its terminal outcome is known.
The agent still decides the next action from history. Terminal exit evidence does not prove rollback or exactly-once effects.
Partial effects remain possible, so the continuation instruction retains its existing review requirement for uncertain outcomes.
New protocol shapes require explicit evidence before they can qualify.

## Alternatives considered

- Keep every failed command ineligible: this preserves the reported manual recovery burden despite known terminal outcomes.
- Permit every failed tool: this also admits ambiguous failures and tools without terminal command evidence.
- Permit pending tools: this changes a separate ownership boundary and is unnecessary for the reported turns.
- Replay the original request: this can repeat completed or partially completed actions.
- Add another retry scheduler: the existing owner already supplies backoff, cancellation, and bounded dispatch.
