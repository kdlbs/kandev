---
id: "01-resolve-last-prompt-outside-window"
title: "Resolve the last prompt outside the loaded window"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PINNED-PROMPT-AVAILABILITY-001
acceptance_criteria:
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.1
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.5
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.6
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.7
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.9
system_design:
  - ../../specs/ui/system-design/pinned-prompt-availability.md
---

# Task 01: Resolve the last prompt outside the loaded window

## Summary

Derive the chat panel's last prompt from the session's prompt projection plus
the loaded transcript window instead of the loaded window alone, and offer the
scroll-to-last-prompt control when the resolved edge is eligible or the resolved
prompt is absent from a non-empty cached window, so a message page that contains
no user row no longer removes the affordance.

This work order is an internal prerequisite, not a shippable step alone: the
control it offers for an unloaded prompt does nothing until Task 02 supplies the
load-and-align path, which is worse for the user than today's absent control.
Tasks 01 and 02 therefore land as one change.

## In scope

- New pure module `apps/web/lib/session-last-prompt.ts` with the stored-prompt
  predicate, the store's valid-prompt predicate, the prompt ordering comparator,
  the last-prompt lookup, and the window/projection resolution, whose third input is the
  caller's generation's observation record (its ids and its newest key) because a projection
  row alone cannot say whether the client observed it. It imports no component module, so
  `message-list-shared.tsx` can delegate to it without an
  import cycle; the neighbor walk stays with `getItemKey` in that component module
  and is Task 03's.
- `prompt-message-actions.ts` delegates its `isValidPrompt` to the store
  predicate and its `sortPromptMessages` to the shared comparator, so the store
  and the pinned prompt share one convention without widening the store's
  parseable-timestamp gate.
- `message-list-shared.tsx` delegates the exported `getLastUserMessageId` and the
  module-private `findLastUserMessageIndex` (keeping its `-1` contract), and its
  file-local `isStoredUserMessage` delegates to `isStoredUserPrompt` so the
  first-user lookup (`getFirstUserMessageId`, consumed by scroll-to-start)
  shares the one definition instead of keeping a duplicate.
- The first-load-only option on `useSessionPrompts`, so the panel's read never
  replaces a cache the prompt-history panel has paginated, plus the dedicated
  authority-carrying install that sets the authoritative-projection marker. Only
  this read performs that install: the prompt-history panel calls the same hook and
  the same `replacePromptMessages`, so a marker set inside the store action would
  make that panel's read the transcript's authority. Fan-out, prepend, and the
  prompt-history read leave the marker unset, and the session-removal purge clears
  it.
- `task-chat-panel.tsx` reads the projection, resolves the last prompt, keeps the
  pinned-prompt content sourced from that resolution, and widens control
  availability to the unloaded case without changing the rendered-prompt case.

## Out of scope

- Loading an unloaded prompt (Task 02) and the unloaded edge classification
  (Task 03).
- Any change to transcript rows, pagination metadata, or the prompt projection
  request parameters, including its `author_type` filter and page size.
- Quick chat, run transcripts, and other hosts that do not pass last-prompt
  affordances today.

## Acceptance

- The resolved last prompt is the newer of the loaded window's last stored user
  prompt and the projection's newest valid entry. `comparePromptOrder` delegates
  to `compareMessageTimestamps` (nullable when either side is unparseable) and
  breaks ties by `id`; the resolver alone owns the unparseable-as-older rule, so
  an unparseable row orders before a parseable one, and the synthetic
  task-description row is excluded from both resolution sides. The loaded-window
  fallback is asserted by name for each projection state: an empty projection, a
  projection older than the window, a failed/unavailable read, and an in-flight read each
  retain the raw loaded window's last prompt and keep the control available; absent a live
  observation, a window with no stored user prompt waits for a successful read, while a live
  observation resolves before any read and an empty raw window presents nothing (AC .9).
  An empty window yields no affordance at all.
- The projection admits a row only through the session's observed-prompt set, whose
  ids the authority install and the live create path write for valid stored user prompts, so a cache populated by
  transcript fan-out or older-prompt pagination contributes nothing while the panel's
  first read is in flight, has failed, or returned nothing. Until an id is observed,
  nothing is admitted; once one has been, the newest cache row is also admitted when it
  orders strictly newer than the recorded newest key, which survives the row it came
  from, so a prompt fetched before this client observed it still resolves after the
  window loses it and after a replacing page or a removal empties the cache of observed
  rows. The record's ids and its floor come from the rows the merge accepted, never from the
  raw response: the strict rule can keep a cached row for a same-id incoming one, so a raw
  key would advance the floor past a real prompt or, unguarded, lower it (a regression feeds
  a same-id response whose `created_at` differs and keeps an intervening unobserved row
  resolvable). Resolution reads only its own generation's record, so a tagged write that lands
  after a purge cannot admit a row into the next incarnation. The live create path carries no
  generation, so an event delivered after a purge records a valid stored user prompt's id in whatever incarnation is
  then current: accepted, because that same call also writes `messages.bySession` and the
  prompt cache, so it can only re-admit a row the ingress already delivered, and the
  never-lowered key keeps an older delayed row from lowering the floor. The regression
  delivers an old event after a purge and recreates under the same id, asserting no
  observation, floor, or affordance CARRIED OVER from the previous incarnation. The record is per session generation: the purge clears its ids and its key with
  the marker and the deleted-id set, so nothing the previous incarnation observed,
  including the floor, admits a row in the next one (a named regression pairs a
  generation-0 observation, a purge, and a strictly newer unobserved row in generation
  1). The key is never lowered: the authority install unions the response's ids into the
  record and keeps the newer of its key and the recorded one, so a response that raced a
  newer live observation cannot re-open the guard and admit an older unobserved row (a
  regression holds a read across a live create, installs it, evicts the live row with a
  replacing page, and asserts the refusal rather than a resolution: an unobserved row whose
  version lies strictly between the response's newest and the recorded key must not resolve,
  because a still-resolves assertion would pass with the key lowered — every row a lowered
  key newly admits is older than the recorded key's own row — together with a store-state
  assertion that the recorded key stayed the newer one). Every writer keys on the
  session it is called for: the page and authority writers drop a row whose
  `session_id` differs from the keyed session, so a strictly newer foreign row cannot
  enter this session's cache or satisfy the second clause. Those writer assertions live in
  `session-slice.prompts.test.ts`, which drives the real actions, and the around loader's
  own filter in `load-message-window.test.ts`; the panel suite keeps only a consumer-level
  assertion that a foreign row never resolves. An order tie between the loaded-window row
  and the projection row keeps the window row unless the projection row's `updated_at` is
  strictly newer, so the pinned content is deterministic; `lib/session-last-prompt.test.ts`
  covers an admitted clause-2 row tied with the window row in both `updated_at` directions. The marker governs the read policy alone, never admission.
  The named projection-fallback cases are therefore asserted on two fixtures: a
  cache holding the prompt (where the loaded window's prompt must survive) and a
  non-authoritative cache holding only an older prompt in a window that has no user
  row (where no affordance appears, because presenting that stale prompt as the
  last prompt would contradict
  `AC-UI-PINNED-PROMPT-AVAILABILITY-001.6`). A successful read of an empty
  projection still installs and sets the marker, so it stays authoritative.
- The marker has exactly one writer: the transcript panel's own authority-carrying
  install. The prompt-history panel's read of the same hook leaves it unset, as do
  `fanOutTranscriptPrompts` and `prependPromptMessages`, and the session-removal
  purge deletes it with `bySession`/`metaBySession` and the generation bump, so a
  reincarnated session id inherits no authority. The suites assert the unset paths
  (a prompt-history read leaves the cache non-authoritative) and the purge path,
  not only the set path.
- The marker is client-only and its only sources are the boot default and the
  transcript panel's own install. A later hydration payload that carries
  `messagePrompts` is not merged — `StateHydrator`'s session merge does not touch
  this slice — so a payload can neither set authority nor suppress the read, and the
  panel-mounted ordering (hydration before or after the read) cannot race it. The
  payload rule is asserted where the payload is copied (`default-state.test.ts` on
  `mergeInitialState`'s output, `hydrator.test.ts` for `hydrateState`), and the
  marker-absent read is asserted in `use-session-prompts.test.ts`.
- `isStoredUserPrompt` and the store's predicate differ exactly on the
  parseable-timestamp gate: the store still rejects a user row with an unparseable
  `created_at`, and the store's existing ordering behavior is unchanged for every
  row it accepts. The store sorts through
  `(left, right) => comparePromptOrder(left, right) ?? 0` after filtering, so the
  comparator's `null` branch is unreachable there.
- The panel offers the control when the resolved edge is eligible, or when the
  prompt is resolved and absent from `messages.bySession` while that window is
  non-empty; a resolved prompt that is rendered keeps today's `scrollButtonEligible`
  gate, a cached-but-unrendered prompt gains no control, and an empty window
  presents neither affordance — the anchored bar's mount source and its height
  reservation are gated on a non-empty cached window
  (`messages.bySession[resolvedSessionId]`, not the rendered `allMessages` length,
  which the synthetic task-description row can make non-empty), so it is absent
  rather than mounted closed and reserves no offset. The pinned prompt's content
  comes from the same resolution.
- A prompt that is in the cached window is never treated as unloaded: the panel
  passes that message id to `MessageList`, and the unloaded flag is defined by
  cache membership, so it can only ever describe a prompt outside the window. A
  cached prompt whose row element is absent (a message carrying `metadata.actions`
  moves to the footer action list, and an activity-typed user message collapses
  into a `turn_group`) therefore keeps today's behavior: the rendered-row path
  reports `visible` for the missing element, which offers no control and no bar.
- The projection read uses a first-load-only policy keyed on an
  authoritative-projection marker, not on cache presence: the marker check is a
  guard clause that returns before any prompt-slice write (no
  `setPromptMessagesLoading`, no refresh-generation bump) when the marker is set,
  and otherwise reads — including when the cache already holds transcript-fan-out
  or older-page entries — while leaving an authoritative cache (including its
  `oldestCursor`) untouched. Attempts happen once per session generation and once
  per connection-status change while the marker is absent, so an attempt that fails
  retries on the next status change (the hook hydrates even while disconnected,
  which its existing test pins), and once the marker is set no further request is
  issued on any connection transition. The marker is set only by the transcript
  panel's authority-carrying install and purged with the session.
  Resolving the prompt adds
  no transcript row, changes no pagination metadata, and issues no history request
  beyond that bounded read — asserted in
  the panel suite as "resolving an unloaded prompt issues no around or history
  request and leaves `hasMore`/`oldestCursor` untouched".
- The transcript first-load path must not inherit the hook's refresh-generation
  mismatch retry, and its response path is terminal: a current session generation
  with success installs by authority upsert and sets the marker (the upsert preserves
  a live update that landed mid-flight); a current session generation with failure
  sets the read's failure state, leaves the marker unset, and issues no mismatch
  retry, so the documented next-status-change, generation, or remount attempt stays
  available; a stale session generation or an unmounted consumer writes nothing, sets
  no marker, and retries nothing. The retry exists so the prompt-history panel's
  replacing read cannot clobber a live update, and that read keeps it on both
  branches.
- The read must not invalidate the prompt-history panel's in-flight older-page
  request: it keeps its loading state request-local (never the shared
  `setPromptMessagesLoading`) and installs without advancing
  `refreshGenerationBySession`, since `useLazyLoadPrompts` refuses a response when
  that generation changed and the sentinel then replays the page. The install
  preserves the shared cursor, so the page's rows land and its cursor is unchanged —
  asserted in `apps/web/hooks/use-lazy-load-prompts.test.ts`, which imports that hook
  and already owns the refresh-generation race; `use-session-prompts.test.ts` cannot
  execute the lazy-page path, because its store mock omits
  `setPromptMessagesLoadingMore` and `prependPromptMessages`.
- A deletion must win over an in-flight response, from either installer. The
  removal path (`removePromptMessage`) already advances the refresh generation, but
  the authority success branch ignores that mismatch, and the prompt-history panel's
  own `useSessionPrompts` instance keeps calling `replacePromptMessages`, which
  replaces the array without consulting anything. The rule is therefore a
  slice-entry invariant: the store keeps a per-session deleted-prompt-id set, the
  removal path records the id **before** its cache-presence and not-present early
  returns (so a prompt uncached at deletion is still recorded, mirroring the
  unconditional revision bump beside those returns), every row-writing action
  (`upsertPromptMessage`, `replacePromptMessages`, `prependPromptMessages`) skips
  such an id, and the session purge clears the set. That purge is asserted where it is
  already hosted, in `remove-task-session.test.ts`: it deletes the prompt caches and
  bumps the generation in one action, so it is the suite that carries the
  observed-prompt set out with them, leaving a reincarnated session id with none. The
  writers follow one matrix
  rather than one shared helper, because they have different shapes: the authority
  install (its own named action) filters, sorts, merges, and sets the marker while
  preserving existing pagination metadata and applying the response's only when the
  cache was empty, with request-local loading and no `refreshGenerationBySession`
  advance; the page writers (`replacePromptMessages`, `prependPromptMessages`) apply
  the incoming metadata and then repair a filtered cursor — incoming or current — to
  the oldest retained row, clearing the cursor with `hasMore` false when a non-empty
  page retains no row; a zero-row response applies the metadata it carried (the exhausted page's
  `has_more` false and empty cursor), never the prior values, because keeping them would leave
  the older-page path permanently armed, and
  a single-row upsert never touches pagination metadata and keeps its existing
  refresh-generation bump.
  `removePromptMessage` closes the rule at the removal site: it repairs a cursor
  naming the removed id and clears both when no row remains, so a one-prompt session
  cannot advertise an older page the lazy-load path will not request. Two more
  writers join the matrix. `updatePromptMessage` is a row event, not a page: it must
  insert when the id is absent from the cache, skip tombstoned ids, and leave
  pagination metadata alone, so an update for an uncached id during a held authority
  read is not overwritten by that response. Every merge in this work order — the row
  events, the authority install, and (per Task 02) the around window — takes a
  three-case rule instead of the store's `isIncomingMessageAtLeastAsFresh` `>=`: an
  incoming row replaces a cached one only when its version is strictly newer, an
  equal version keeps the cached row, an unparseable incoming version keeps it too,
  and a cached row whose own version is unparseable is replaced (the comparator's own
  branch order, and the accepted fail-open). Regressions: same-id, equal-`created_at`
  and equal-`updated_at` older content through the authority installer (the stale
  response must not win; the cached row is kept) and the legacy pair with no
  `updated_at` on either side (the cached row is replaced, which is what
  `session-slice.prompts.test.ts` and its "advances the refresh revision" fixtures
  require). Both row-event writers keep
  their existing `refreshGenerationBySession` bump: it invalidates an in-flight
  prompt-history page on a live prompt change, and dropping it would weaken the guard
  the older-page rule relies on. The pinned live-mutation refresh assertions in
  `session-slice.prompts.test.ts` therefore stay green unchanged. Boot hydration is a writer too:
  `mergeInitialState` spreads the payload's `messagePrompts`, so the merge hydrates
  only the whitelisted cache and metadata fields and forces the marker, the observed-prompt record (ids and newest key), and
  the deleted-id map empty. The assertion lives where the payload is copied: `default-state.test.ts`
  proves it on `mergeInitialState`'s output, `hydrator.test.ts` proves `hydrateState`
  cannot set the marker, the record, or the deleted-id map, and one `createAppStore` assertion documents that the
  composition takes the slice's `defaultSessionState` and so cannot install a payload
  cache (the panel suite cannot host any of this — it stubs the store). Adding `messagePrompts` to `buildStateOverrides` would create the
  authority hazard this prevents and is out of scope. The authority metadata rule also needs the cache state
  named: no entry (apply the response's metadata), retained rows (preserve),
  cleared-but-initialized (keep the cleared metadata, so a stale response re-enables
  nothing and a valid new row waits for the next own read). Tests cover the four page
  shapes, the one-row deletion, the row-event update, a held authority response after
  a last-row deletion with and without a valid row, and the boot payload, and hold the
  response across a
  deletion in both orders, through both installers, and for a prompt cached or
  uncached at deletion.
- A projection read that fails while the socket stays connected has no recovery
  until a session-generation change, a reconnect, or a panel remount: this work
  order adds no transcript-side failure surface and no polling, and the
  prompt-history panel's retry cannot re-run this read. Tests cover a failed read
  followed by a reconnect transition (exactly one further attempt), a failed read
  followed by a non-connected transition (one attempt, no extra retry), and a
  successful read followed by a transition (zero further requests), plus unmount
  before a held request resolves and before it rejects: no cache write, no marker,
  no loading write, and no further request. The stale-generation branch is asserted
  the same way, mutating `generationBySession` while the request is held: on success
  and on rejection alike, no authority install, no marker, no failure or loading
  write, no retry, and no follow-up request.

## ASCII UI preview

`UI-01: Desktop transcript, last prompt outside the loaded window` (planned
view; full preview in [plan.md](plan.md#ui-01-desktop-transcript-last-prompt-outside-the-loaded-window)):

```text
+---------------------+--------------------------------------------+----------+
| sidebar             | [chat] [Prompt history] [Changes]          | right    |
|                     +--------------------------------------------+          |
|                     |  tool call: read file ...                  |          |
|                     |  filler message 120 ...                    |          |
|                     |  (no user prompt row is loaded)            |          |
|                     +--------------------------------------------+          |
|                     |  [ composer ]              [^ Scroll to    | <- this task
|                     |                              last prompt]  |    adds the
+---------------------+--------------------------------------------+    control
```

Fixed regions: the view-tab row, the sidebar, the right column, and the composer
status bar where this task's control lives. Scrolling: the transcript body
(`.chat-message-list`) only; the control never moves with the transcript. This
work order adds the control and the empty-window bar gate (the bar stays absent
while the window is empty); the bar's opened state for an unloaded prompt is
`AC-UI-PINNED-PROMPT-AVAILABILITY-001.3` (Task 03).

Phone (`UI-02`; the phone keeps one inline composition): the same control in the
phone composer status bar, and no anchored bar exists at any state.

```text
+----------------------------------+
| < Task            [ status bar ] |  fixed: task chrome + status bar
+----------------------------------+
| tool call: read file ...         |  scrolling: .chat-message-list
| filler message 120 ...           |  (single scroll owner)
| (no user prompt row is loaded)   |
+----------------------------------+
| (no anchored bar on phone)       |  fixed: composer row
| [ composer ]   [^ last prompt]   |  <- this task's control
+----------------------------------+
```

ACs: `AC-UI-PINNED-PROMPT-AVAILABILITY-001.1` (available with no prompt row
loaded) and `.9` (absent while the window is empty). Targeted rendered check:
Task 04's desktop and phone scenarios assert the control with no prompt row
loaded.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/state/default-state.test.ts lib/state/hydration/hydrator.test.ts lib/state/store.test.ts lib/session-last-prompt.test.ts components/task/chat/message-list-shared.test.tsx components/task/task-chat-panel.last-prompt.test.tsx components/task/task-chat-panel.launch-error.test.tsx hooks/use-processed-messages-fallback.test.ts hooks/domains/session/use-session-prompts.test.ts hooks/domains/session/use-session-prompts.stability.test.tsx lib/state/slices/session/session-slice.prompts.test.ts lib/state/slices/session/remove-task-session.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/task-chat-panel.tsx components/task/chat/message-list-shared.tsx lib/session-last-prompt.ts lib/session-last-prompt.test.ts lib/state/slices/session/prompt-message-actions.ts lib/state/slices/session/message-timestamp.ts lib/state/default-state.ts lib/state/default-state.test.ts lib/state/hydration/hydrator.test.ts components/task/task-chat-panel.last-prompt.test.tsx)
```

## Files likely touched

- `apps/web/lib/session-last-prompt.ts` (new)
- `apps/web/lib/session-last-prompt.test.ts` (new)
- `apps/web/lib/state/slices/session/prompt-message-actions.ts`
- `apps/web/lib/state/slices/session/message-timestamp.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/default-state.test.ts`
- `apps/web/lib/state/hydration/hydrator.test.ts`
- `apps/web/lib/state/store.test.ts`
- `apps/web/lib/state/app-state-types.ts`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/chat/message-list-shared.test.tsx`
- `apps/web/hooks/domains/session/use-session-prompts.ts`
- `apps/web/hooks/domains/session/use-session-prompts.test.ts`
- `apps/web/hooks/use-lazy-load-prompts.test.ts`
- `apps/web/lib/state/slices/session/session-slice.prompts.test.ts`
- `apps/web/components/task/task-chat-panel.launch-error.test.tsx`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/task-chat-panel.last-prompt.test.tsx` (new)
- `apps/web/lib/state/slices/session/remove-task-session.test.ts` (the purge assertion
  the observed-prompt set is added to)
- `apps/web/hooks/use-processed-messages-fallback.test.ts` (the split lives in the hook's
  `useMemo`, so the pure-builder `use-processed-messages.test.ts` cannot host it)

## Dependencies

None.

## Risks

- Delegating the store's prompt ordering can change the prompt-history panel's
  row order if the semantics drift; `session-slice.prompts.test.ts` stays in the
  verification block, the delegated comparator must reproduce
  `compareMessageTimestamps` plus `comparePromptIDs` for parseable rows, and the
  delegated predicate must keep rejecting rows without a parseable `created_at`.
- The store's sort needs a numeric comparator, so the delegation goes through
  `?? 0` after the filter; assigning the nullable comparator directly to
  `Array.sort` would not type-check, and coercing it silently would hide the
  contract the resolver depends on.
- The bar's non-empty-window gate must read the cache
  (`messages.bySession[resolvedSessionId]`), not `allMessages.length`: the
  synthetic task-description row renders while the cache is empty, so an
  `allMessages`-based gate would mount a closed bar against AC .9. That gate alone is not
  sufficient now that the resolver reads the raw cache: the bar and its
  `anchoredBarHeight` reservation mount only for an unloaded or renderable resolved
  prompt, because the bar reports its content height on mount whatever its open state and
  that height sets the transcript's scroll margin. Renderable means a row the transcript
  actually renders, not a grouped or cached row. The panel builds that one list itself: the
  visible-message filter, the footer-action split, and activity grouping, then the same
  pure `filterLaunchErrorItems(groupedItems, launchErrorOwned, launchErrorStamp,
  launchErrorOccurredAt)` the message list applies before rendering (already exported from
  `message-list-shared.tsx`), so the sticky node and `anchoredBarHeight` both read the same
  item list in the same render and no filter can remove the row after the bar committed its
  content height. The message list keeps filtering for its other callers. A row is not renderable when it is footer-only or
  action-carrying, or when no rendered item contains it (a renderer-null row).
  Regressions: the cached-but-unrendered shape (no sticky bar, zero reservation), and an
  open bar whose matching launch-error filter then flips (no sticky bar,
  `anchoredBarHeight === 0`), whose `MessageList` mock must render the captured
  `stickyPromptBar` so the real pinned bar mounts and its height report is ordered against the
  flip (or that case loads the real list with `vi.importActual`). Mounting alone is not enough:
  happy-dom reports `offsetHeight` 0 for every element, so the fixture must stub the measured
  height and assert the pre-flip non-zero reservation before asserting the post-flip zero, or
  the zero holds with the bar still mounted. The mock shape must also carry the `prompts` slice
  (items, loaded, loading, setPrompts, setPromptsLoading) or mock
  `hooks/domains/settings/use-custom-prompts`, because the real bar reads it through an
  unguarded selector and would throw before any assertion.
- The first-load-only policy must not suppress a read that a session genuinely
  needs. It is keyed on the authoritative-projection marker, not on cache
  presence: it reads when the marker is absent, including when the cache already
  holds transcript-fan-out or older-page entries, because cache presence is not
  proof that the newest prompts were ever fetched. The prompt-history panel keeps
  its own replace-and-paginate read.
- Resolution admits a row through the observed-prompt set alone, never through the
  marker: a non-authoritative cache (fan-out or older-page entries only) can hold an
  older prompt, and with a window that has no user row the resolver would otherwise
  select it and present it as the last prompt while the first read is in flight, has
  failed, or returned nothing. That is the fixture the panel suite asserts: the stale
  entry is ignored and no affordance appears. The admission term is the observed-prompt
  set, whose ids the authority install records for every row the response returned
  (tombstoned or filtered ids are inert) and the live create path records for the row it
  observed when that row is a stored user prompt (an agent row, a system notice, or a row without a parseable
  `created_at` neither enters the set nor advances its key), together with the newest key the set has observed; resolution takes the
  newest cache row the set holds, so a page or fan-out row the read did not return
  contributes only as the second clause below allows. A merge that keeps a cached row still
  adopts a `prompt_index` the incoming row newly supplies, mirroring the transcript cache's
  ordinal carve-out, with a regression for an equal-`updated_at` merge that newly supplies
  one.
  The second clause keeps the floor:
  once an id has been observed, the newest cache row is admitted when it orders strictly
  newer than that recorded key, and the key outlives the row it came from, so a
  replacing page or a removal that empties the cache of observed rows cannot stop the
  clause — a prompt fetched before this client observed it still resolves, and losing the
  newest observed row never drops the affordance.
- The window fallback must read the raw cached window, not the processed rows: a stored user
  prompt that a visible-row filter removes from rendering still belongs to the loaded window
  and must still resolve, so the resolver reads `messages.bySession[resolvedSessionId]`
  while the processed list stays for rendering and the edge classification (a regression
  uses a stored user prompt the visible-row filter drops, not only the footer-action case).
- The unloaded flag must be derived from `messages.bySession` membership, the same
  precondition the pending-scroll machine checks, or the control is offered for a
  jump that cannot land. A component test pins both halves: an unloaded prompt is
  offered, a cached prompt keeps the rendered-prompt gate.
- Widening availability must not show the control while the resolved prompt is
  rendered; the existing `scrollButtonEligible` path stays authoritative there.
- Suites that render the real panel with a stubbed app store must gain the
  projection state: `task-chat-panel.launch-error.test.tsx` provides an
  `appStoreState` without `messagePrompts`, so the new `useSessionPrompts` read
  throws before its assertions. Extend that mock state (or mock the projection
  hook) exactly as this work order's new suite does, and keep the suite in the
  verification command.
- A cached prompt is never an unloaded prompt, so that gate is defensive: the
  footer-action split and activity grouping are author-agnostic, so the shape is
  representable, but no producer emits a user row in it today: every writer of
  `metadata.actions` is agent-authored (the stall notice, the transient retry notice,
  and the recoverable-failure and git-error cards). The panel suite guards the
  gate with a synthetic action-carrying cached row and says so in the test, and
  the hook-rendering `hooks/use-processed-messages-fallback.test.ts` pins the split the fixture depends on.
  Beyond the bar's own mount gate, no availability or jump behavior depends on the row
  being rendered: the unloaded flag is cache membership, and a missing element reports
  `visible`, so such a prompt gains no control, the bar does not mount, and the
  pending-scroll machine never sees it.
- `task-chat-panel.last-prompt.test.tsx` must render the real panel, so it needs
  the panel-level mock shape of `task-chat-panel.launch-error.test.tsx` (MessageList,
  panel primitives, chat panel state, state provider) extended with the projection
  slice, plus the Dockview store mock shape from
  `task-chat-panel.scroll-target.test.tsx` (which is a `renderHook` suite and
  renders no panel). Because those mocks replace the transcript and composer
  children, the suite must capture their props instead of relying on rendered DOM:
  `MessageList`'s `stickyPromptBar`, `anchoredBarHeight`, and the resolved
  prompt/unloaded flag, and `ChatInputArea`'s `showScrollToLastPrompt` and
  `lastPromptScrollDirection`. Without that, the availability assertions prove
  nothing.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/system-design/pinned-prompt-availability.md` sections
  "Last-prompt resolution" and "Last-prompt controls".
- `docs/specs/ui/requirements/last-prompt-pinning-regressions.md` for the
  rendered-prompt behavior this task must not change.
- Existing patterns: `components/task/task-chat-panel.scroll-target.test.tsx`,
  `hooks/domains/session/use-session-prompts.ts`,
  `lib/state/slices/session/session-slice.prompts.test.ts`.
- `lib/state/default-state.ts` (`mergePromptHistoryState`, `mergeInitialState`) and
  `lib/state/store.ts`: the merge must be constrained to the whitelisted cache and metadata fields, with
  the marker, the observed-prompt record, and the deleted-id map forced empty, because
  the spread copies any payload key the merge does not override. That merge is the only place a
  payload slice is read — `createAppStore` composes the merged state and then the
  session slice, whose `defaultSessionState` re-asserts `messagePrompts` — so the
  acceptance's payload assertion is placed on the merge output and the store
  composition is documented rather than enforced.

## Results

Verified: resolver, session-store, and panel tests passed in the 23-file
focused suite (360 tests total); web typecheck passed.

Post-merge integration removes the core older-prompt page hook and its test;
the bounded latest-prompt read and projection remain for pinned transcript
availability. Prompt History older-page loading remains plugin-owned.
