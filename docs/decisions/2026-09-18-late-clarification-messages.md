# ADR-2026-09-18-late-clarification-messages: Answer earlier questions as new messages

**Status:** accepted
**Date:** 2026-09-18
**Area:** frontend, workflow

## Context

Current-turn ownership prevents stale tool responses from resuming obsolete work.
Users still need to answer questions after the original waiter or turn ends.
Removing the stale panel alone loses that conversation affordance.

## Decision

Preserve current-turn authority for the original tool response. Offer late
answers as ordinary user messages in the question's original conversation.
Use existing message admission, steering, queue ordering, and retry identity.
An affirmative submission receiving a recognized inactive response preserves
its answers and continues through that message path. Closing sends nothing.

The decision extends the presentation of the
[current-turn ownership rule](2026-08-14-current-turn-clarification-ownership.md).
It does not grant historical questions operational authority. Ordinary messages
remain subject to existing session availability and other live input barriers.

## Consequences

Users can answer earlier questions without resurrecting stale tool waits.
Message admission must retain drafts and stable identities across failures.
The UI must distinguish tool delivery, new-message delivery, and queue admission.
The Inbox History tab can retain its read-only boundary and link to conversation.

## Alternatives Considered

- Reopen historical tool requests: rejected because this restores obsolete
  workflow barriers and can resume the wrong turn.
- Remove inactive controls without a late-answer path: rejected because users
  lose the ability to respond to questions the agent asked.
- Add a separate late-answer transport: rejected because ordinary message
  admission already owns sending, queueing, and duplicate prevention.
