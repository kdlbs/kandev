# ADR-2026-09-17-coalesce-streamed-agent-plans: Coalesce streamed agent plans

**Status:** accepted
**Date:** 2026-09-17
**Area:** protocol

## Context

Some ACP agents stream a plan by repeatedly updating the `plan` input of one
tool call. Kandev currently converts every snapshot into an `agent_plan` event
without the source tool-call identity, then persists every event as a new
conversation message. A single plan can therefore appear as many cumulative
cards in live and replayed transcripts.

The plan remains useful before the tool call completes, distinct plan
invocations must remain distinct, and existing stored transcripts cannot be
assumed to contain correlation metadata.

## Decision

Every agent-plan event produced from a tool update carries that source
`ToolCallID`. The durable conversation boundary identifies one plan stream by
session, turn, and tool call, and upserts its latest accepted snapshot into one
`agent_plan` message.

The web transcript projection uses the durable correlation identity when it is
present. For historical uncorrelated rows, it collapses only contiguous
same-turn `agent_plan` messages whose content forms a strict cumulative prefix
chain. It does not rewrite stored historical rows.

## Consequences

Live updates and replay use the same durable message identity, so reloading does
not reveal intermediate snapshots. Separate tool calls retain separate cards.
The ACP-to-conversation path must preserve tool-call identity and test
idempotent updates.

Legacy cleanup is deliberately conservative. Historical snapshots that are not
contiguous prefixes remain visible rather than risking accidental data loss.
The compatibility projection can be removed only after the retained history no
longer contains uncorrelated snapshots.

## Alternatives Considered

- Persist only the final tool input. This removes duplication but withholds a
  useful plan while a long-running tool call is still in progress.
- Deduplicate only in the frontend. This hides the symptom in one projection
  while retaining duplicate durable messages for reload, pagination, APIs, and
  future clients.
- Coalesce by content similarity or turn alone. This can merge independent plan
  invocations and makes identity depend on mutable text.
- Rewrite all historical rows. This adds destructive migration risk for a
  rendering compatibility issue that can be handled conservatively at read
  time.
