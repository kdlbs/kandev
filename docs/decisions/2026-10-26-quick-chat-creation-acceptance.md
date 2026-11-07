# ADR-2026-10-26-quick-chat-creation-acceptance: Accept Quick Chat before runtime startup

**Status:** accepted
**Date:** 2026-10-26
**Area:** protocol

## Context

A repository-backed Quick Chat took 36.713 seconds to create on October 26.
The HTTP server had a 30-second response write timeout.
Its handler recorded success, but the browser showed 502.
WebSocket events independently added the retained conversation to the tab strip.
The original setup kept its unsent opening draft.

An isolated Go server and reverse proxy reproduced handler success followed by
an upstream EOF and browser 502. Live proxy logs were unavailable.

Normal task creation already separates session allocation from asynchronous
workspace and agent startup. Quick Chat can reuse that boundary.
The shipped external-ID API already supplies durable task uniqueness and
creation settlement. A separate aggregate-creation redesign remains draft work.

## Decision

Ordinary Quick Chat accepts a persisted session before runtime preparation.
Acceptance keeps the existing HTTP 200 and task/session fields.
It does not promise that an agent is ready.
Startup progress and failures belong to the accepted conversation.

New browser submissions keep a stable creation ID through uncertain responses.
The server binds that ID to its authenticated principal and workspace.
It uses a reserved external-ID namespace and the existing uniqueness constraint.
Settled and archived creations return their own state without another
allocation or launch. A later keyed POST may resume missing work for the exact
unsettled Quick Chat when the adapter resolves it before generic create and
serializes the per-creation operation. It reuses a session, settles before
dispatch, and starts only a still-`CREATED` session. A concurrent
`FoundUnsettled` create outcome remains a read-only descriptor. Status reads
never repair, start, or send work. Generic external-ID callers retain their
no-side-effects contract.

Browser structured opening messages keep their subscribed message admission path.
Terminal-backed openings keep backend delivery with the existing retained
initial-submission receipt. Creation acceptance and message acceptance remain
separate facts.
Explicit HTTP prompts from legacy structured clients also retain backend delivery.

The [opening composer design](../specs/tasks/system-design/quick-chat-opening-composer.md#startup-acceptance-amendment)
owns the detailed contract. The
[delivery package](../plans/quick-chat-startup-acceptance/plan.md) records the completed implementation and verification.

## Consequences

Repository setup no longer determines the creation response duration.
A lost response can resolve to the same conversation without another Send.
Request cancellation does not erase accepted work or its authenticated owner.

The browser must retain pending submission ownership and reconcile late results.
The adapter must preserve found/unsettled behavior and reserved-prefix isolation.
An interrupted creation can remain incomplete until an explicit same-token
POST resumes it or the user confirms deletion. This decision adds no autonomous
job queue.
Identity retention follows the task lifetime. Browser deletion retires its token.
Server tombstones after physical deletion remain outside this repair.

## Alternatives considered

- Raise the global HTTP timeout. Longer setup and proxy deadlines can still
  reproduce the fault. Other routes also inherit the changed resource policy.
- Extend only this route's deadline. That still couples creation to setup and
  cannot control every proxy or network failure.
- Reconcile an existing chat by title or recency. Concurrent chats can have
  identical names, profiles, and repositories. Those values cannot prove ownership.
- Retry an unkeyed POST after 502. The first request can already have created a
  conversation, so retry can allocate another one.
- Add a new creation table and durable worker. Existing task identity and
  initial-submission storage cover this repair. A general creation coordinator
  remains a separate draft initiative.
