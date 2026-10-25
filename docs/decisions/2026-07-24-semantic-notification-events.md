# Semantic notification events come from domain occurrences

**Status:** accepted
**Date:** 2026-07-24
**Area:** backend, frontend, desktop

## Context

The session lifecycle uses `WAITING_FOR_INPUT` for several operationally
different situations: startup readiness, the idle state after a normal turn,
and an actual clarification request. The notification service subscribed to
that state transition and translated every occurrence into “needs your input.”
Its once-per-session delivery key compounded the problem: a startup or ordinary
turn notification could consume the key before the agent asked a real question.

The product now exposes ordinary turn completion and explicit clarification as
independently selectable notification events. Their occurrence boundaries must
remain stable even if the orchestrator's lifecycle states change.

## Decision

User-facing notifications will be sourced from semantic domain events, not
inferred from generic session lifecycle state.

- `session.turn_finished` is derived from the durable completion of a specific
  agent turn and uses the turn ID as its occurrence identity.
- `session.clarification_requested` is derived from Kandev's structured
  agent-authored clarification-request message and uses the shared
  pending/request ID as its occurrence identity. Task and session identity are
  required.
- `system.update_available` is derived from the release poller's durable cached
  version and uses the release version/tag as its occurrence identity. It is
  install-wide and has no task or session identity.
- Notification idempotency is keyed by provider, semantic event type, and
  occurrence identity. The task-session ID remains recorded for routing and
  audit but is not the deduplication boundary.
- `session.waiting_for_input` is retired as an emitted notification event.
  Existing subscriptions migrate to clarification requests only.
- Local WebSocket and desktop notifications preserve the semantic event type
  and occurrence identity rather than collapsing both cases back into a
  waiting-state action.
- New-release delivery uses the same provider subscriptions, delivery history,
  and retry semantics as other notifications. It does not maintain a parallel
  update-only preference or de-duplication store.

The user-visible contract is defined by
[Semantic Notifications](../specs/platform/requirements/notifications.md).

## Consequences

- Startup and ordinary idle transitions cannot claim that the agent needs an
  answer.
- Users can opt into every completed turn without coupling that choice to
  clarification alerts.
- More than one turn or clarification in a session can notify, while replaying
  one domain occurrence remains idempotent.
- Notification persistence requires an occurrence identifier and a replayable
  migration from the legacy session-scoped unique key.
- New notification-producing features must identify a semantic domain
  occurrence and stable occurrence ID instead of subscribing to a convenient
  lifecycle state.

## Alternatives Considered

1. **Keep `WAITING_FOR_INPUT` and inspect the previous state.** Rejected because
   the same transition still represents both normal completion and
   clarification, and lifecycle changes would silently alter user-facing
   semantics.
2. **Rename the existing notification to “turn ended.”** Rejected because it
   would still fail to distinguish an explicit question and would preserve the
   once-per-session suppression bug.
3. **Send both messages from every waiting transition.** Rejected because it
   would create notifications for events that did not happen and prevent users
   from selecting the two intents independently.
4. **Deduplicate only in memory.** Rejected because replay and process restart
   require durable idempotency.
