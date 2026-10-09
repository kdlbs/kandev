---
status: superseded
system: plugins
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
created: 2026-09-14
owners:
  - kandev
---

# Conversation recovery


## Storage replacement, 2026-09-16

The [source reconciliation design](conversation-source-reconciliation.md) now owns the intended conversation transport and storage replacement.
The [replacement plan](../../../plans/conversation-storage-replacement/plan.md) owns its implementation.
The durable journal, fixed-cutoff snapshots, ACKs, poison dispatch, and retention text here records the prior implementation.
It does not require the replacement to preserve those mechanisms.
Authorization, safe DTOs, lifecycle fencing, core compatibility, and visible recovery outcomes remain required.


## Boundary and references

This document refines recovery in the
[Host prerequisite design](prompt-history-extraction-host.md).
It uses the existing
[requirements](../requirements/prompt-history-extraction-host.md) and
[browser API contract](../../../plans/plugins/PLUGIN-API.md).
The [repair package](../../../plans/pr-3588-conversation-recovery/plan.md)
owns implementation evidence and work orders.

The public SDK, error codes, database schema, token TTL, retention, and plugin
ownership remain unchanged. This design does not approve the larger durable
transport decision or complete the outstanding PR review.

| Requirement | Design section |
| --- | --- |
| REQ-PLUGINS-PROMPT-HISTORY-HOST-002 | Replay grants, Core recovery, Continuation expiry |
| REQ-PLUGINS-PROMPT-HISTORY-HOST-005 | Compatibility and evidence |

## Replay grants

A subscription has two distinct positions: snapshot cutoff W and effective
registered cursor C. Replay starts after C. The snapshot grant uses W.
The initial resume grant uses C, never a later event that the client has not
acknowledged. For a fresh or replacement subscription, C equals W.

The gateway resolves retained gaps and poison delivery before it selects C.
It then registers or replaces the cursor and returns matching grants.
A mint or registration failure must release claimed replay work and must not
publish a successful subscription with inconsistent state.

`MintSessionStreamGrant` needs explicit internal inputs for these positions.
All call sites must supply them. An ACK at sequence S produces the next resume
grant at S. No public wire field changes.

`ValidateSessionResume` retains signature, expiry, identity, and lower-bound
checks. `SessionEventLog.Acknowledge` remains the authority for contiguous
advancement, retained events, and poison. Failed ACKs cannot move the cursor.

## Core recovery

`WebSocketClient` owns the per-session transport cursor and recovery generation.
The existing core data owners retain message and turn hydration and store writes.
An internal recovery coordinator connects those responsibilities with an
awaitable completion result. It is not a plugin SDK API or a general event bus.

The recovery states are live, subscribing, hydrating, and live again.
A failure remains recoverable. Removal or disposal terminates the cycle.
Only one cycle can own a session generation.

1. Pause normal event projection and capture the recovery generation.
2. Request the replacement subscription and validate the response.
3. Make transport readiness available to snapshot requests.
4. Request fresh messages and turns through the existing core read paths.
5. Commit both results only if the session and recovery generation still match.
6. Resume buffered events only after successful reconciliation.

The subscribe response is not proof of snapshot completion.
A replacement cursor can advance on the server while local projection remains
paused. The client must not treat the skipped interval as repaired until the
snapshot commit succeeds.

Current `fetchAndStoreMessagesAttempt` awaits subscription readiness and
starts turn hydration without awaiting success.
Current `ensureSessionTurnsLoaded` can return after exhausted retries.
The recovery path must obtain explicit success or failure from both owners.
It must bypass same-generation hydration reuse without breaking normal loading.
Its callbacks must not wait on the recovery completion barrier that they satisfy.

Reconciliation must repair skipped additions, updates, deletions, and turn
completion. The latest message window does not prove that older cached rows
remain valid. Invalidate or reconcile stale cached windows using existing
pagination ownership. Retain pending local messages under their existing rules.
Keep rich core message fields and active-turn reconciliation semantics.

Buffer events during reads. Prevent pre-snapshot events from replacing newer
snapshot state. Preserve existing sequence and freshness guards and define the
cutoff handoff explicitly in the implementation tests.
A transport-only unit test is not sufficient proof of this behavior.

A mounted idle session must recover without visibility change or disconnection.
For a subscribed session without a mounted data owner, retain a dirty generation.
Its next data owner must hydrate before consuming buffered updates.
Dispose abandoned generations and subscriptions without unbounded buffers.

A failed read must not release projection as healthy.
Use existing bounded retry/backoff and visible error behavior.
Session removal and lifecycle changes cancel stale completion.
Recovery must never mutate plugin page caches.

## Continuation expiry

The scope owns expiry decisions. Plugins continue to call `loadMore()` and
`retry()` without transport knowledge.

Before expiry, renewal retains the original cutoff and query fingerprint.
At known expiry, the Host must not submit the expired tuple for renewal.
It creates one fresh binding/subscription/snapshot cycle through its existing
rebind mechanisms. A fresh authorized snapshot may use a new cutoff.

Join concurrent recovery requests per scope. Increment the page revision before
invalidating the old cursor. Replace cursor, snapshot token, cutoff, and page
state as one matching revision. Notify message and turn readers through the
existing rebind listeners. Fence old renewal and page responses.

After fresh hydration, continue the original load-more intent once with the new
cursor if a next page exists. Return the number of newly projected message IDs
relative to the call's initial view. Concurrent callers share that result.
If the refreshed page is exhausted, no continuation request is necessary.

Keep committed rows until a successful replacement snapshot or terminal response.
On transient recovery failure, preserve state and expose the existing retryable
upstream error. Explicit retry must start a fresh recovery, not reuse the expired
tuple. Bound each automatic recovery cycle to one rebind and one continuation.

A non-expired token rejected for tampering or query mismatch remains
non-retryable. Do not convert all HTTP 400 responses into retries.
For expiry between dispatch and response, use the captured tuple's expiry to
permit one recovery. Server expiry checks remain strict.
Client/server clock uncertainty must not cause a retry loop.

An expired resume token on the first idle live event uses the same scope recovery
path instead of leaving `sequenceBlocked` permanent.
Do not increase token lifetime to mask recovery failures.
A timer can optimize renewal, but a suspended tab must recover on demand.

Removal stops recovery and sets terminal state.
Access revocation stays unauthorized or not found.
No recovery accepts an expired grant as authorization.
Plugin disable, unmount, session switch, or generation change cancels pending work.

## Compatibility and evidence

Desktop and mobile use the same repaired data paths and existing controls.
No new public copy, surface, storage, or message type is required.
The original package's tests remain historical evidence.
New RED/GREEN results belong to the
[repair work orders](../../../plans/pr-3588-conversation-recovery/plan.md#work-orders).
