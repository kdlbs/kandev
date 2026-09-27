# ADR-2026-08-19-durable-administrative-turn-settlement: Settle Administrative Turns by Exact Identity

**Status:** accepted
**Date:** 2026-08-19
**Area:** backend, workflow, protocol, security

## Context

An accepted workflow completion signal can outlive the provider lifecycle event that normally ends its turn. Treating session `RUNNING` as sufficient evidence of live implementation work can then block a same-profile successor, while clearing the in-memory owner risks closing a successor or accepting late frames.

## Decision

Kandev persists one completion intent for an exact task/session/turn/step identity. Normal provider completion, bounded reconciliation, and a narrowly authorized stale-settlement request share one compare-and-set settlement operation. A transition's on-entry prompt is identified by the committed task-step-transition row, so duplicate callbacks return the same control delivery and an obsolete transition cannot prompt the current session.

`settle_stale_session_kandev` is distinct from `stop_task_kandev`. It can settle only one proven-stale turn and only for a same-workspace peer session, direct parent, or session with server-recorded spawn supervision. It fails closed when activity, tools, reservations, cancellation, or successor evidence exists.

## Consequences

Completion and destination dispatch survive a missing provider terminal event and restart without turning timeouts into authority. The implementation adds persisted intent, control-event, and transition-delivery records plus conservative reconciliation.

## Alternatives Considered

Flipping old `RUNNING` sessions by age can interrupt real work and cannot protect successor turns. Clearing `activeTurns` only repairs a cache and allows late-frame corruption. Widening the all-session stop tool grants more authority than stale recovery needs.
