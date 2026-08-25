# 0052 — Plugins reconcile pending agent interactions through a dedicated Host contract

- Status: accepted
- Date: 2026-08-22
- Area: backend, protocol
- Related: [0043 — Plugin host data API](0043-plugin-host-data-api.md) (the pattern
  this extends), [0047 — Plugins read conversation content](0047-plugin-host-conversation-reads.md),
  [2026-08-14 — Current turn owns active clarification](2026-08-14-current-turn-clarification-ownership.md)
  (the authority this reuses), [docs/specs/plugins/spec.md](../specs/plugins/spec.md)

## Context

A cross-workspace operator plugin can already subscribe to task, session,
message, permission and clarification events, and can read session metadata
through the Host data API. It still cannot answer the one question an attention
or inbox plugin exists to answer: **does a human owe this session a response
right now?**

Kandev settled this internally long ago. `WAITING_FOR_INPUT` also describes an
ordinarily completed turn, so session state is not evidence of an owed
response; the durable `pending_action` projection is. A live compatibility run
on Kandev 0.90.0 with `@agentclientprotocol/codex-acp` 1.6.2 produced the
concrete case: the turn completed successfully, its only permission row was
durably `approved`, and the session snapshot still read `WAITING_FOR_INPUT`. A
state-only plugin claims attention is owed; kandev's own list surfaces
correctly do not.

The public plugin surface exposed none of that:

- `pluginsdk.Task` and `pluginsdk.Session` carry no pending-action projection.
- `pluginsdk.Message` omits request metadata — `pending_id`, status, the
  options or questions needed to render a valid response, resolution state.
- `Host.Messages().Send` can send a new prompt, but there is no capability-gated
  action corresponding to the permission-response or clarification
  answer/cancel paths.

Events cannot close the gap on their own. A plugin may start after the request
was made, restart, or drop an event; without a durable read it can never
reconcile its cache against the truth. The remaining escape hatch — reading the
SQLite file directly — is the exact failure ADR 0043 exists to close.

## Decision

Add a dedicated **interaction** contract to `service Host`, gated by its own
`api_read:interactions` / `api_write:interactions` capabilities.

1. **A separate `Interactions()` accessor, not a field on existing DTOs.**
   An additive pending-interaction field on `Session` or `Message` would drag
   raw transcript metadata into DTOs that deliberately avoid it, and would
   leave the write authorization without a boundary to attach to. A dedicated
   accessor keeps both concerns in one gated place. It is also deliberately
   **not** folded into the plugin-managed-conversation extension: an ordinary
   attention or inbox plugin must not have to own an agent conversation
   lifecycle to find out that a user owes an answer.

   It is an OPTIONAL Host extension (`pluginsdk.InteractionHost`), like
   `ExecutorProfileHost` and `PluginOwnedTaskTreeHost`, so every existing
   Go-native `Host` implementation stays source-compatible.

2. **One durable read, one resolve.**
   - `ListPendingInteractions(filter, page)` returns every interaction still
     owed a response. `Interaction` carries the pending id, kind
     (`permission` | `clarification`), task/session/turn ids, normalized
     status, timestamps, and the permission options or structured questions a
     caller needs to render a valid response.
   - `GetInteraction(id)` resolves ANY interaction, terminal ones included.
     That is what makes an event-driven cache reconcilable: a replayed or
     missed event converges on the current durable result instead of
     disappearing into `NotFound`.

3. **The read reuses the pending-action authority rather than re-deriving it.**
   `ListPendingInteractions` is built on the same predicates as
   `GetPendingActionsBySessionIDs`: only the session's current durable turn
   counts, terminal sessions quarantine pending history, and only the newest
   permission row of that turn is answerable. A pinning test asserts the two
   reach the same verdict per session. A plugin that disagrees with kandev's
   own kanban card is a bug in one of them, and this makes that a test failure
   rather than a support thread.

4. **Writes route through the first-party services the native UI drives.**
   `RespondToPermission` goes through the orchestrator method the WS
   `permission.respond` action calls. `AnswerClarification` and
   `CancelClarification` go through the same `clarification.Handlers` instance
   the REST route serves, including its durable exclusive claim, live-waiter
   delivery, and detached-resume fallback. Nothing is reimplemented, so a
   plugin's response is indistinguishable from a user's and every surface
   converges through the normal events.

   `Cancel` is routed to the handler's **reject** path rather than its
   in-memory `CancelRequest`: cancellation needs the original waiter to still
   be parked, while rejection is durable-claim based and therefore also
   settles a bundle whose waiter went away in a restart. That is precisely the
   bundle a reconciling plugin is holding.

5. **The host derives the outcome; the plugin only names a choice.**
   A permission response must name one of the options the agent actually
   offered, and kandev derives approve-versus-deny from that option's recorded
   ACP kind. A plugin therefore cannot report an outcome that was never on the
   menu. The target session comes from the durable record, never from the
   request.

6. **Terminal-once, with two distinguishable failures.** The first response
   wins. A write against an already-resolved interaction returns
   `FailedPrecondition` instead of dispatching a second response; an unknown id
   returns `NotFound`. A reconciling cache needs to tell "someone else answered
   first" apart from "my id is stale", and those two codes are that
   distinction. The clarification handler's own conflict outcome maps onto the
   same `FailedPrecondition`, so a plugin branches on one code regardless of
   which layer caught the race.

## Consequences

- An attention or inbox plugin can declare `api_read:interactions` alone and
  never gain the ability to answer on a user's behalf.
- The clarification response sequence gained an exported
  `Handlers.Respond` / `Handlers.Cancel` seam; the REST route is now a thin
  shell over it. Behavior is unchanged — the HTTP status vocabulary moved into
  a typed `RespondError` the REST layer renders and the plugin adapter maps to
  gRPC codes.
- Interaction reads and the compact `pending_action` projection are two queries
  over the same durable rows. They are pinned to agree, but a future change to
  one must update both; the shared predicates (`turnAuthorityPredicate`,
  `nonTerminalSessionPredicate`) are the single source for the parts that
  matter most.
- Only the newest permission of a turn is listed, matching the projection. If
  ACP ever issues genuinely concurrent permission requests in one turn, both
  this contract and the projection need widening together.

## Alternatives considered

- **An additive pending field on `Session`/`Message`.** Rejected: it leaks
  transcript metadata into DTOs that avoid it, and leaves the write path
  without a capability boundary of its own.
- **Reuse the plugin-managed-conversation extension.** Rejected: it forces an
  inbox plugin to take on a conversation lifecycle it has no use for.
- **Events only, with no durable read.** Rejected: it cannot serve a plugin
  that started late, restarted, or dropped an event, which is the entire
  reported problem.
- **Let plugins write message rows directly.** Rejected for the same reason
  ADR 0043 rejects repository access: the agent would never unblock, and no
  surface would converge.
