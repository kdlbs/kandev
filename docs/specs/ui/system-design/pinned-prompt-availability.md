---
status: draft
system: ui
requirements:
  - REQ-UI-PINNED-PROMPT-AVAILABILITY-001
---

# Pinned prompt availability System Design

## Purpose and boundaries

This design owns which prompt the transcript's pinned prompt and
scroll-to-last-prompt control represent, and how the transcript reaches a prompt
whose row is not in the loaded window.

Adjacent contracts it uses without changing:

- [Transcript history visibility](task-prompt-transcript-visibility.md) owns the
  bounded newest window, shared message API, and around-window navigation.
- [Prompt History Extraction](../../plugins/system-design/prompt-history-extraction.md)
  owns the plugin's full-history surface. This design owns the separate
  latest-prompt projection for pinned transcript affordances.
- [Last-prompt pinning](../requirements/last-prompt-pinning-regressions.md) owns
  the pinned prompt's rendered appearance and its directional threshold.
- [Bounded user-message rendering](bounded-user-message-rendering.md) owns the
  pinned prompt's source bounding.

No backend, persistence, or transport changes: the design reads the prompt
projection and around-window query.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-PINNED-PROMPT-AVAILABILITY-001` | Last-prompt resolution, Last-prompt controls, Edge classification, navigation |

## Components and responsibilities

- `lib/session-last-prompt.ts` (new) owns the prompt predicates, ordering, and
  resolution as pure functions: `isStoredUserPrompt` (user-authored and not the
  synthetic task-description row, used by resolution), `isValidPromptMessage` (a
  stored prompt with a parseable `created_at`, the store's gate),
  `comparePromptOrder` (delegating to `compareMessageTimestamps` and then `id`),
  `findLastStoredUserPromptIndex`, and
  `resolveLastPromptMessage(windowMessages, promptProjection, observedPrompts)`, whose third
  input is the caller's generation's record, because a projection row alone cannot say
  whether the client observed it.
  An unparseable window prompt is a rendered prompt, and its edge comes from
  `resolveLastPromptEdge` with `scrollButtonEligible` governing the control,
  unchanged. It imports no
  component module, so `message-list-shared.tsx` can delegate to it without an
  import cycle.
  `lib/state/slices/session/prompt-message-actions.ts` delegates its
  `isValidPrompt` to `isValidPromptMessage` and its `sortPromptMessages` to
  `comparePromptOrder` through an explicit adapter, because `Array.sort` needs a
  numeric comparator: rows are filtered by `isValidPromptMessage` first and sorted
  with `(left, right) => comparePromptOrder(left, right) ?? 0`. The `null` branch
  is therefore unreachable at that call site, so the store's ordering is unchanged for every
  row it accepts, while the resolver handles `null` explicitly and the walk reports an
  unparseable non-synthetic item through its `unordered` term.
- `components/task/chat/message-list-shared.tsx`: keeps the exported
  `getLastUserMessageId` and the module-private `findLastUserMessageIndex` (whose
  `-1` contract `getStreamingAgentMessageId` depends on) as thin delegates to the
  new module (including the file-local `isStoredUserMessage`, which becomes a delegate to
  `isStoredUserPrompt` so the first-user lookup shares it); owns `resolveUnloadedPromptEdge` beside the existing
  `resolveLastPromptEdge`, and owns (and exports) `findPromptNeighbors` (whose bounds carry the
  `unordered` term), because the walk resolves row keys through `getItemKey`, which lives in
  this module. Keeping
  the walk here avoids a component ⇄ lib import cycle.
- `components/task/chat/message-list-native.tsx`: `useTranscriptEdgeTracking`
  receives the resolved last-prompt message, the panel's unloaded flag, and the
  rendered transcript items, and classifies the edge when the prompt row has no
  element. The tracker measures each keyed row through the container-scoped
  `findMessageRow` helper (now exported from `message-list-native-scroll.ts`).
- `components/task/task-chat-panel.tsx`: resolves the last prompt, decides control
  availability and direction, and owns the local pending-scroll request.
- `hooks/domains/session/use-session-prompts.ts`: reads the bounded latest
  user-message window after the session subscription is ready. `TaskChatPanel`
  uses the first-load-only policy, keyed by session generation; concurrent reads
  join only when they share the same readiness identity. Only a successful
  authority install establishes the projection marker. A failed read leaves the
  loaded-window fallback available, and a connection-status change or remount
  can retry while the marker remains unset.
- `lib/state/slices/session/prompt-message-actions.ts` (`messagePrompts`):
  keeps a session-scoped cache separate from transcript pagination. The
  authority install accepts only valid, same-session, non-tombstoned prompts,
  merges rows by strict freshness, and records observed ids and their newest
  key. Live creates are observed immediately; transcript fan-out can populate
  the cache but cannot establish authority. Updates preserve freshness, and
  removal tombstones an id before cache checks and removes it from the observed
  set. Session removal clears these maps and increments the generation so a
  late response cannot cross an incarnation. The API response cursor metadata
  is not consumed by a core older-page loader; full Prompt History pagination
  remains plugin-owned.
- `lib/state/slices/session/message-timestamp.ts`: `isIncomingMessageAtLeastAsFresh` becomes the
  three-case rule (Task 01); the around window inherits it.
- `lib/state/slices/session/message-signature.ts`: the signature falls back to the content hash
  when `updated_at` is unparseable, branching on `messageTimestampNanoseconds`, not
  truthiness.
- `components/task/chat/anchored-last-prompt-bar.tsx`: unchanged; it receives the
  resolved prompt's content and visibility.

## Last-prompt resolution

The panel derives one `lastPromptMessage` per render:

1. `windowLast` is the last stored user prompt of the raw loaded window
   (`messages.bySession[resolvedSessionId]`, not the processed `allMessages` a filter can
   empty), the source introduced by this design. A reconciled fetched window can drop a live
   user row from `messages.bySession` (it reaches `message-window-reconciliation` below the
   fetched boundary), so the projection has to resolve it.
2. `projectionLast` is the newest valid cached prompt the client observed,
   either from the successful latest-prompt read or a live create event. Cache
   rows populated only by transcript fan-out do not become candidates until
   the read establishes authority or a live create event observes them. The
   observation record is scoped to the session generation; its newest key is
   never lowered and survives removal of the row that established it, so a
   stale response cannot admit an older prompt after a newer live prompt.
3. The result is the newer of the two by `comparePromptOrder`, or `null` when neither
   exists. An order tie keeps the loaded-window row unless the projection row's `updated_at`
   is strictly newer.


Membership rules for the unparseable and synthetic inputs:

- `comparePromptOrder` reports `created_at` through `compareMessageTimestamps` and breaks
  ties by `id`; it returns `null` when either side is unparseable, and the resolver then
  treats the unparseable row as older, ordering two of them by `id`.
- The synthetic task-description row carries `created_at: ""` and a user author type. It
  never resolves a last prompt, and the neighbor walk treats it as absent.

The panel treats a resolved prompt as unloaded when it is absent from the
session's cached transcript window (`messages.bySession`) while that window is
non-empty. That is the machine's precondition plus a non-empty window, so the offered set
cannot exceed it. Availability reads that flag, not whether a row happens to be rendered;
`isMessageRowRendered` remains only the dockview target's own readiness check.

A cached but unrendered prompt is not an unloaded prompt and gains no control. That gate is
defensive: the footer-action split is author-agnostic, so the shape is representable
although no producer emits a user row in it today, and the panel suite guards the gate with
a synthetic action-carrying cached row.


## Last-prompt controls

- The anchored bar keeps its rule: visible only for an edge of `above`, rendering
  the resolved prompt's content.
- The anchored bar's mount source and its height reservation (`anchoredBarHeight`) are
  gated on a non-empty cached window (`messages.bySession[resolvedSessionId]`, not the
  `allMessages` length the synthetic row makes non-empty, which would mount a closed bar
  against `AC-UI-PINNED-PROMPT-AVAILABILITY-001.9`) and on the resolved prompt being unloaded
  or renderable. The panel computes one list — the visible-message filter, the footer split,
  activity grouping, and then the same pure `filterLaunchErrorItems(groupedItems,
  launchErrorOwned, launchErrorStamp, launchErrorOccurredAt)` that `NativeMessageList`
  applies before rendering, exported from `message-list-shared.tsx`. The bar's node and its
  `anchoredBarHeight` both read that list in the same render, so it cannot disagree with the
  transcript. A row is not renderable when it is footer-only, action-carrying, or absent from
  every rendered item; an in-flight local jump keeps the bar only while its target will
  produce a row.
- The scroll-to-last-prompt control's availability changes from the existing
  `scrollButtonEligible` gate to `scrollButtonEligible || (prompt resolved and
  unloaded)`. A rendered prompt keeps today's behavior: the
  control appears once it leaves the viewport. An unloaded prompt offers it whatever the
  resolved edge, `visible` included.
- The control's direction comes from `resolveLastPromptControls(edge)` alone, so an
  unloaded prompt classified `visible` points up.

## Edge classification

`useTranscriptEdgeTracking` resolves the last prompt's edge from DOM geometry:

- Prompt row present in this container: unchanged `resolveLastPromptEdge`, but the
  row is looked up through `findMessageRow(scrollRef.current, promptId)` instead of
  `document.getElementById`, so another mounted transcript's copy of that row
  cannot supply this container's geometry.
- Prompt row absent from this container and the panel's unloaded flag set:
  `resolveUnloadedPromptEdge` uses the container rect and the bounding rows' rects. `findPromptNeighbors` walks the items,
  bounds each by the messages it holds (`message` items by their message,
  `turn_group` items by their messages, decorative skipped), and returns the `getItemKey`
  keys of the older bound (the last item holding an older message) and the newer bound (the
  first holding a newer one) together with an `unordered` term, because a rendered item whose
  messages carry no parseable `created_at` supplies no bound and the row keys alone cannot
  carry that; the tracker maps `unordered` with either bound missing to `visible` before it
  classifies. Bounds are containment, not strict ordering, so a straddling
  `turn_group` can supply
  both. Each key resolves with `findMessageRow(scrollRef.current, key)`. An item is
  unordered only when it holds at least one non-synthetic message without a
  parseable `created_at`; the synthetic row is excluded by identity first. If an
  unordered item is rendered and either bound is missing, the classification is
  inconclusive and reports `visible`; a side is unbounded only when no rendered item
  holds a message on it and no unordered item is rendered. Classification order:
  - no rendered item holds a message on either side: `visible`;
  - no newer bound: `below` (the prompt is newer than every rendered row);
  - no older bound: `above` (the prompt precedes every rendered row);
  - older bound fully above and newer bound fully below: `visible` (the prompt's
    slot lies inside the viewport range);
  - newer bound fully above the container: `above`;
  - older bound fully below the container: `below`;
  - otherwise: `visible`.
  The existing two-pixel tolerance applies to each comparison. The prompt is
  orderable by contract: the cache admits an unloaded prompt only with a parseable
  `created_at`, so no defensive branch is required. The walk runs per
  measurement, and
  the item list is an effect input, so a prompt id settling before the rows
  re-classifies once they render.
- Prompt row absent without the unloaded flag (cached but unrendered): `visible`,
  as today.
- No resolved prompt: `visible`, as today.

For an unloaded prompt a `visible` classification keeps the bar closed while the
availability gate keeps the control offered.

## Unloaded prompt navigation

Activating the control keeps the loaded-prompt behavior: a resolved prompt in
`messages.bySession` scrolls immediately through the message list handle with
`align: "start"`, landing the row at the transcript's start-aligned position including its
`scroll-margin-top`; for an unloaded prompt the handler creates the local target with no
immediate call, so the flow's call count is entirely the machine's. The status-bar control
and the bar's button share one `scrollToLastPrompt` handler, and the panel creates a local
target only when the row is also absent from `messages.bySession` (a cached but unrendered
row is unreachable). The target:

- The shared `PendingMessageScrollTarget` (session, prompt id, token,
  `hostPanelId: panelId ?? "pending"`) gains an optional generation, defaulted from
  the panel, so host-built targets keep their four-field shape while a session
  removed and recreated with the same id invalidates the local target.
- The existing `usePendingMessageScroll` hook drives it as a second instance, so a
  host-provided prompt-history target on the same panel keeps its own identity, token,
  refresh key, and consumption path. That instance is wired with `messageId: null`, so its
  identity comes from the target alone; a fallback message id would leave a live identity
  key after consumption and re-issue the around request on every effect re-run. It also
  reuses the panel's window-derived `readinessKey` plus `isInitialMessagesLoading`, the only
  inputs that re-run the effect after the merge; an id-derived key cannot retrigger it,
  leaving `completedAround` set with no post-merge scroll attempt.
- `loadMessageWindowAround` requests `GET /api/v1/task-sessions/:id/messages`
  with `around=<prompt-id>`, `sort=desc`, and the limit, keeps only rows whose
  `session_id` is the requested session, checks the target against those, and merges them
  into `messages.bySession` without changing pagination metadata, so a foreign row can
  neither enter the window nor resolve.
- The machine finishes the jump: initial start-aligned scroll, the `transcript-jump-loading`
  indication while the request is in flight, one delayed reassertion (250 ms), and
  consumption on success. A confirmed deleted target is consumed without scrolling and
  without a retry loop.

If the local target exists but the panel's visibility flag is false, the machine defers the
scroll: it cancels its reassertion and resumes the same target once that flag flips back.
The local instance takes the transcript's own visibility (`transcriptIsVisible`, false only
for a hidden dockview tab), not the raw `isVisible` prop, so a host rendering a visible
transcript while passing `isVisible={false}` (the thread conversation and the kanban hover
preview, which use it only to suppress read-cursor advancement) still completes the jump.

Settlement for the local target is decided by identity, not visibility: an around response
is accepted while the target still belongs to the same session generation and message and
the panel is mounted, so a confirmed `deleted-target` consumes the target even from a hidden
panel and a `merged` response is retained as the completed window for the deferred scroll.
Only a changed generation, a superseded target, or unmount discards it, and visibility gates
the scroll attempt alone.

Identity settlement is scoped to the local unloaded-last-prompt instance alone, through an
explicit settlement-mode option carried by that instance, not by the host-provided target
on the same panel. The host pending path and the Dockview prompt-history consumption keep
their existing visibility-gated settlement, because their responses arrive for a target
the user selected in another surface, so a shared helper takes the mode as a parameter.

The mode splits three responsibilities:

- **Settlement** of the around response uses an identity guard — mounted, plus the same
  session generation, message, and target key — for the success and failure paths, so a
  result arriving while hidden still settles.
- **Scroll attempt and reassertion** keep today's visibility guard (`isVisible` stays in
  the effect and the reassertion scheduler).
- **Completed-window retention**: a response settling while hidden retains
  `completedAround` in the local mode, so activation scrolls the merged window and consumes
  the target without a second request; the host mode keeps clearing it.

The panel already owns two other transcript targets — the host-provided pending
inputs and the Dockview prompt-history target (`useScrollTargetConsumption` with
this panel's id) — and the local target never competes with either:

- The control does not create a local target while a host input is active, or
  while the Dockview store holds a live target for this session and panel id
  (`scrollTarget.sessionId === resolvedSessionId && scrollTarget.hostPanelId === panelId`).
- A host input or a matching Dockview target that becomes active while a local
  target exists drops that target and clears its indication.
- The local instance's attempt and consumption path is additionally gated on both host
  inputs being absent and no live Dockview target for this panel, so two mounted instances
  cannot act on one message list at once.

Activation while another owner's jump is in flight is a no-op: the control stays
visible, and the next activation after that owner settles creates a fresh target.

A second activation while the local jump is in flight takes a fresh token,
invalidating the first target before its guard runs: the first response is
discarded without merging, scrolling, or consuming, and only the newest settles —
`merged` performs the start-aligned scroll, its single 250 ms reassertion, and one
consumption; `deleted-target` or a rejection consumes it without a retry loop.

That state is one term in both artifacts: a target recorded for this session and panel id,
unconsumed.

Consequently the local jump issues one around request, three `scrollToMessage`
calls (the pre-merge
attempt returning false, the post-merge start-aligned scroll, and its single 250 ms
reassertion), and one consumption; the handler adds none.

The merge adds the around window — the target row plus the newer rows — to
`messages.bySession` without touching pagination metadata, and the union need not be
contiguous. A jump's identity is frozen at activation: rows the merge reveals, including a
user prompt newer than the target that then resolves as the last prompt, re-target nothing,
so the scroll and the single consumption stay bound to the activated prompt and the pin
follows the window afterwards.

## Failure and recovery

- Prompt projection read fails, is still loading, or is unavailable: resolution falls back
  to the last stored prompt of the raw loaded window, which keeps that prompt's affordance,
  and a live observation resolves on its own before any successful read. Absent a live
  observation, a window with no stored user prompt waits for a successful read; an empty raw
  window presents nothing at all, live observation or not
  (`AC-UI-PINNED-PROMPT-AVAILABILITY-001.9`). The read is attempted once per session generation and once per status
  change until it succeeds, and the marker then suppresses every later attempt: a success
  issues no later request, a failure retries on the next status change. Its in-flight join carries the readiness it
  awaited, so a replay releasing and re-subscribing it cannot strand the attempt. It never retries
  while connected, so a transient failure recovers only on a reconnect or a remount.
  The authority read's response path is terminal and four-way: a current session generation
  with success installs by authority upsert, unions the accepted rows' ids into the
  observation record and keeps the newer key, and sets the marker, ignoring the mismatch
  (the upsert preserves a live update that landed mid-flight), while the deletion-wins
  invariant keeps a tombstoned row out and a foreign-session row is rejected, and no retry
  follows. Loading stays request-local; the install advances neither cursor nor
  refresh generation. A
  current generation with failure sets the failure state, leaves the marker
  unset, and issues no mismatch retry, so the next status-change, generation, or remount
  attempt stays available; a stale generation or unmounted consumer writes nothing, sets
  no marker, and retries nothing. Only the prompt-history read keeps the mismatch retry.
- A prompt that is cached but not rendered as a transcript row is not navigable
  through the around window, so the panel does not classify it as unloaded and
  offers no control that cannot land.
- Around-window request fails: the local pending target is consumed, the jump
  indication clears, and the control stays available for another attempt.
- The around response does not contain the prompt: the local target is consumed as
  deleted, including while the panel is hidden, so reactivation issues no request and no
  request fires without a new activation. The design does not prune: reconciliation of the
  projection stays owned by the prompt-history contract, whose deletion source is session
  message events. When that event was missed the projection keeps the entry, the control
  stays offered, and the next activation re-confirms the deletion with exactly one request
  (no automatic retry). A host-provided target keeps its visibility-gated settlement.
- Session switch or panel unmount: the local target is dropped with the panel state. A
  session recreated under the same id bumps that generation, so the target, its loading
  and completed-around state, and its pending reassertion are dropped before any merge or
  settlement, and no in-flight response lands in the new incarnation. Panel inactivity
  defers it: the machine cancels its reassertion while the host is inactive and resumes
  the same target when it becomes visible.
- A stale or slower projection response cannot move the pinned prompt backwards, because
  resolution only accepts an entry newer than the loaded window's prompt.

## Persistence

None. The projection, its marker, and the observed-prompt set stay a client-side
cache: no new storage, schema, migration, or retained server state.

## Observability

The existing `transcript-jump-loading` indication covers the jump; the window and
older-page requests keep their debug loggers, and no new metric, log field, or counter
is added.

## Related contracts

- [Requirements](../requirements/pinned-prompt-availability.md)
- [Last-prompt pinning](../requirements/last-prompt-pinning-regressions.md)
- [Transcript history visibility](task-prompt-transcript-visibility.md)
- [Prompt history panel](prompt-history-panel.md)

## Test boundaries

- `lib/session-last-prompt.test.ts`: resolution and ordering — either side newer, an
  empty or invalid projection, synthetic-row exclusion, unparseable `created_at` on both
  sides, `id` tie-breaking, `isStoredUserPrompt` versus `isValidPromptMessage`, the
  unparseable-as-older rule, and the clause-2 tie fixture.
- `components/task/task-chat-panel.*.test.tsx` (the flow's component boundary):
  availability, the empty window, the projection fallbacks (including a failed or
  in-flight read over a non-authoritative older-only cache presenting no affordance, plus a
  newer unobserved page row still resolving over a recorded key and after the observed
  rows leave, and a live prompt arriving afterwards still resolving from the window or,
  once a disjoint fetch drops it, from its observed id), the cached-but-unrendered gate (no sticky bar, zero reservation) with a message list that mounts
  the real pinned bar for the filter flip, and the local jump consuming those fallbacks; its cases
  (around request, in-flight indication, start alignment, single
  reassertion, consumption, rejected request, a hidden rejection consuming the target
  once with the indication cleared and no request on reactivation, confirmed deletion,
  superseding activation, hidden settlement and reactivation counts, target drop on an
  ordinary session switch and on unmount with the timers flushed and a late around
  response settling nothing, same-id recreation, and a newer user row the merge reveals
  not re-targeting the in-flight jump); the visibility input
  (`transcriptIsVisible` versus the raw prop); arbitration against both other owners;
  and no resolution side effect on `hasMore`/`oldestCursor`.
- `lib/state/slices/session/message-signature.test.ts`: a parseable `updated_at`
  short-circuits, an unparseable one falls back.
- `hooks/use-processed-messages-fallback.test.ts`: the rendering derivation (a plain user
  message renders, an action-carrying one moves to the footer list).
- `hooks/domains/session/use-session-prompts.test.ts` covers the bounded user
  prompt read, authority marker, connection readiness, failure/retry, request
  joining, and stale session-generation responses. The store suite covers
  prompt fan-out, observed live prompts, freshness, deleted-id tombstones, and
  the authority install. Core has no prompt-history older-page hook.
- `hooks/domains/session/load-message-window.test.ts`: the loader's branches —
  a target-containing response whose guard turns false before it resolves returning
  `stale` with no merge, and a current response omitting the target returning
  `deleted-target` with no merge, a foreign-`session_id` row merged nowhere (the target-id row
  carries another session), beside the
  equal-`updated_at`, newly-supplied-ordinal, and both-unparseable pair cases. The
  component boundary mocks this loader, so only this suite executes those branches.
- `lib/state/slices/session/session-slice.prompts.test.ts`: the store predicate and
  ordering, an unparseable user row never entering `messagePrompts.bySession`, the
  marker set only by the authority install (never by fan-out, prepend, update, or the
  prompt-history read) and cleared by the purge with the observed-prompt set (asserted in
  `remove-task-session.test.ts`), that set written only by the authority install and the
  create path, pruned by the removal path (asserted on the record's own stored ids, since a
  pruned id has no resolution-visible effect while its tombstone lives) — and the page-shape
  and row-event metadata matrix. The boot constraint sits on
  `mergeInitialState` in `default-state.test.ts` and for `hydrateState` in
  `hydrator.test.ts`.
- `components/task/chat/message-list-shared.test.tsx`: the `resolveUnloadedPromptEdge`
  truth table (no bounds, both outside, single-bound, one row as both) and the exported
  `findPromptNeighbors` (a straddling `turn_group`, the unordered rule, the synthetic
  row).
- `components/task/chat/message-list-native.test.tsx`: the tracker for an unloaded prompt
  with and without bounding rows, a single-`turn_group` window classifying
  `above`, a prompt id settling before the rows and re-classifying after, the
  unchanged `visible` for a cached-but-unrendered prompt, and both foreign-row cases
  (a duplicated neighbor id, and a second transcript holding only the prompt row) —
  all under a local `ResizeObserver` stub with stubbed rects.
- Playwright, desktop (`chromium`) and phone (`mobile-chrome`): after seeding past the last
  prompt and reloading, no prompt row renders, the control is present, each affordance
  reaches the row in its own pass (the bar's button, then the status-bar control), the newest
  seeded row survives the jump, and the landing matches the panel's spec.
