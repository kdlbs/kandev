---
created: 2026-09-22
status: implemented
requirements:
  - REQ-UI-PINNED-PROMPT-AVAILABILITY-001
system_design:
  - ../../specs/ui/system-design/pinned-prompt-availability.md
legacy_specs: []
---

# Implementation Plan: Pinned Prompt Availability

## Overview

Keep the session's last prompt, the pinned prompt, and the scroll-to-last-prompt
control available when a long agent run has pushed that prompt out of the loaded
transcript window. Today the panel derives the last prompt only from the loaded
window (`getLastUserMessageId(allMessages)`), so a window with no user row yields
`null` and both affordances disappear while the transcript still holds the
prompt elsewhere.

Order: resolve the last prompt from the session's prompt projection plus the
loaded window (Task 01), make the control load and align an unloaded prompt
(Task 02), classify the pinned prompt's edge without a rendered row (Task 03),
then prove the flow in a browser on desktop and phone (Task 04).

Task 01 is an internal prerequisite, not a shippable step on its own: it widens
availability to an unloaded prompt, but only Task 02 supplies the path that loads
and aligns one, so between the two the control is offered and does nothing when
activated — worse than today's absence. Tasks 01 and 02 therefore land atomically
(one PR). Task 03 depends on both: without it the existing tracker reports
`visible` for an absent row, so the bar stays closed and the direction falls back
to its default — incomplete rather than worse, but also not a shippable state on
its own. The full package lands as one change.

## Scope

### In scope

- Last-prompt resolution across the prompt projection and the loaded window.
- Control availability and direction for an unloaded last prompt.
- On-demand loading and start alignment of an unloaded last prompt.
- Edge classification for a last prompt that has no rendered row.
- Desktop and phone browser evidence for the whole flow.
- One-sentence wording update in `docs/public/tasks-and-workflows.md`
  ("Navigate long chat transcripts") for the extended availability, with the
  docs-maintainer validation commands (Task 04).

### Out of scope

- Backend, persistence, transport, pagination-cursor, and transcript-window
  changes.
- The pinned prompt's rendering, bounding, expansion, and mobile absence.
- Prompt-history panel pagination, prompt search, and saved-prompt aliases.
- Adding last-prompt affordances to surfaces that do not have them today
  (quick chat, run transcripts).

## Technical approach

### 1. Last-prompt resolution (Task 01)

- New `apps/web/lib/session-last-prompt.ts` owns the pure logic:
  `isStoredUserPrompt` (user, not the synthetic row), `isValidPromptMessage`
  (stored prompt plus a parseable `created_at`), `comparePromptOrder`
  (delegating to `compareMessageTimestamps`, which returns `null` when either side
  is unparseable, and breaking ties by `id`),
  `findLastStoredUserPromptIndex`, and `resolveLastPromptMessage`. It imports no
  component module, so `message-list-shared.tsx` can delegate to it without an
  import cycle; `findPromptNeighbors` therefore lives in
  `message-list-shared.tsx` with `getItemKey`. The unparseable-as-older rule lives
  in the resolver alone: an unparseable row orders before a parseable one, and two
  unparseable rows order by `id`, while the walk treats an item holding an
  unparseable non-synthetic message as unordered. The store never reaches that
  branch, because `isValidPromptMessage` filters such rows before sorting.
- `apps/web/lib/state/slices/session/prompt-message-actions.ts` delegates its
  `isValidPrompt` to `isValidPromptMessage` and its `sortPromptMessages` to
  `comparePromptOrder` through `(left, right) => comparePromptOrder(left, right) ?? 0`,
  because `Array.sort` needs a numeric comparator and the filter makes `null`
  unreachable there. The store's parseable-timestamp gate is therefore preserved,
  not widened: unparseable user rows still never enter
  `messagePrompts.bySession`. The authoritative-projection marker lives on the same
  slice (`apps/web/lib/state/slices/session/session-slice.ts` and its type
  declarations), is set only by the transcript panel's authority-carrying install,
  and is purged with the session; no other prompt-projection writer is added by this
  package.
- `apps/web/components/task/chat/message-list-shared.tsx` keeps the exported
  `getLastUserMessageId` and the module-private `findLastUserMessageIndex` (whose
  `-1` contract `getStreamingAgentMessageId` uses) as delegates.
- `apps/web/components/task/task-chat-panel.tsx`:
  - reads `useSessionPrompts(resolvedSessionId, { firstLoadOnly: true })` for the
    prompt projection, keyed on an authoritative-projection marker rather than on
    cache presence: transcript merges/prepends fan their user rows into
    `messagePrompts.bySession`, so an older-only cache must not suppress the
    newest-prompt read. The marker is a new per-session map on the prompt slice,
    initialized by boot hydration and purged by the session-removal purge that
    drops `bySession` and bumps the generation (a reincarnated session id therefore
    inherits no authority). It is set through a dedicated authority-carrying
    install performed only by the transcript panel's own read, because the
    prompt-history panel calls the same hook and keeps using the shared
    `replacePromptMessages`: authority travels with the caller, not with the store
    action. That install upserts the fetched page into the existing per-session
    array and leaves `hasMore`/`oldestCursor` untouched whenever the session already
    held a cache, writing that page's meta only for an empty cache, so a read over a
    non-authoritative older-page cache cannot discard the pages the user paged in or
    move the prompt-history cursor. `fanOutTranscriptPrompts`,
    `prependPromptMessages`, and the prompt-history read
    never set the marker. The marker
    governs the read policy alone; the resolver's admission term is the observed-prompt
    set, which the authority install and the live create path write, so a fan-out,
    older-page, or prompt-history cache cannot present a stale older prompt while the
    first read is in flight, has failed, or returned nothing, deleting the newest
    observed row hands resolution to the newest surviving observed row, and a cache row
    ordering strictly newer than the recorded newest key is admitted — the key outlives
    its row — so a prompt fetched before this client observed it still resolves once the
    window loses it. The read keeps its loading state request-local and its
    install advances neither the shared cursor nor `refreshGenerationBySession`, so
    the prompt-history panel's in-flight older-page request stays valid. A deletion
    that lands mid-flight must also win over either installer: the slice carries a
    per-session deleted-prompt-id set that the removal path writes before its early
    returns, that every row-writing action (`upsertPromptMessage`,
    `replacePromptMessages`, `prependPromptMessages`) consults, and that the purge
    clears, so no writer can reinstate a deleted id — including the prompt-history
    panel's replacing read, which stays mounted beside the transcript. The writers
    follow one matrix: the authority install (its own named action) filters, sorts,
    merges, and sets the marker, applying the response's metadata only when the
    session has no cache entry at all — a cleared but initialized cache keeps its
    cleared metadata, so a stale response can neither resurrect a cursor nor
    re-enable `hasMore` — with request-local loading and no
    `refreshGenerationBySession` advance; the page writers apply the incoming
    metadata and then repair a filtered cursor — incoming or current — to the oldest
    retained row, clearing the cursor with `hasMore` false when a non-empty page
    retains no row; a zero-row response applies the metadata it carried — the exhausted page's
     `has_more` false and empty cursor — because keeping the prior cursor and `hasMore` would
     leave the older-page path permanently armed;
    row events (`upsertPromptMessage`, `updatePromptMessage`) insert when the id is
    absent and never touch pagination metadata, so an update arriving during a held
    authority read cannot be overwritten by it, while their existing
    refresh-generation bump stays because it is what invalidates an in-flight
    prompt-history page; every merge — row events, the authority install, and the
    around window — replaces a cached row only on a strictly newer version, keeps it
    on an equal version or an unparseable incoming one, and replaces a cached row
    whose own version is unparseable, matching the comparator's branch order rather
    than its `>=` (the same hazard the around-window merge would otherwise inherit); and `removePromptMessage`
    closes the same rule, repairing a cursor naming the removed id and clearing both
    when no row remains. Boot hydration is a writer too: `mergeInitialState` may carry
    `messagePrompts`, so the merge — which the slice's `defaultSessionState` replaces
    in the composed store — hydrates only whitelisted cache and metadata fields and
    forces the marker, the observed-prompt record (ids and newest key), and deleted-id maps empty. A one-prompt session whose last prompt is deleted therefore
    stops advertising an older page the lazy-load path cannot request.
  - resolves `lastPromptMessage` from the raw cached window
    (`messages.bySession[resolvedSessionId]`) plus the projection, treating
    the synthetic task-description row (`created_at: ""`) as neither a candidate
    nor a neighbor;
  - derives `lastPromptMessageId` and the unloaded flag (a resolved prompt absent
    from `messages.bySession` while that window is non-empty), the same `messages.bySession`
    membership the machine needs before it loads an around window, so no resolved prompt is
    offered that the machine must refuse. Availability is not conditioned on the machine
    being free: while another owner's target is live the control stays visible and inert,
    and a prompt whose deletion the next activation confirms was still offered;
  - offers the control when the resolved edge is eligible or the prompt is
    unloaded, so a rendered prompt keeps today's behavior exactly;
  - gates the anchored bar's mount source and its `anchoredBarHeight` reservation on a
    non-empty cached window (`messages.bySession[resolvedSessionId]`), not on the rendered
    `allMessages` length, because the synthetic task-description row can make `allMessages`
    non-empty while the cache is empty and would otherwise mount a closed bar against
    `AC-UI-PINNED-PROMPT-AVAILABILITY-001.9`, AND on the resolved prompt being unloaded or
    renderable in the panel-built post-filter list, so a cached-but-unrendered prompt mounts
    no bar and reserves nothing (it reports content height on mount whatever its open state);
  - passes the resolved prompt and the unloaded flag through to `MessageList` for
    the edge tracker.

### 2. Unloaded prompt navigation (Task 02)

- `task-chat-panel.tsx` keeps the immediate `scrollToMessage` for a loaded
  prompt; that call lands the row at the transcript's existing start-aligned
  position, which is the viewport top plus the row's `scroll-margin-top`.
- For an unloaded prompt the panel records a local `PendingMessageScrollTarget`
  (session, prompt id, monotonic token, `hostPanelId: panelId ?? "pending"`, and
  the session's generation at creation); for a
  prompt present in `messages.bySession` the shared `scrollToLastPrompt` handler
  scrolls immediately and creates no target, so an unloaded activation's call
  count is entirely the machine's.
  Because the unloaded flag already means "absent from `messages.bySession`", the
  machine always has work to do and never stalls on a jump it refuses to act on.
- A second `usePendingMessageScroll` instance owns that target, wired with
  `messageId: null` so its identity comes from the target alone (a fallback message
  id would leave a live identity key after consumption and re-issue the around
  request on every effect re-run) and reusing the panel's window-derived
  `readinessKey` plus `isInitialMessagesLoading`, the only inputs that re-run the
  effect after the merge (an id-derived key cannot retrigger it, leaving
  `completedAround` set with no post-merge attempt), so the existing
  host-provided prompt-history target keeps its own identity and consumption. It
  is driven by the transcript's own visibility (`transcriptIsVisible`), not the
  raw `isVisible` prop, so a host that renders a visible transcript while passing
  `isVisible={false}` (thread conversation, kanban hover preview) completes the
  jump; only a hidden dockview tab defers. Ownership is arbitrated across all
  three owners: no local target is created while either host input
  (`pendingScrollToMessageId`, `pendingScrollTarget`) is active or while the
  Dockview store holds a live target for this session and panel id, a host input
  or matching Dockview target that appears while a local target exists drops it and
  clears its indication, and the local attempt path is gated on all three being
  absent, so only one owner ever scrolls or consumes. Activation while another
  owner's jump is in flight is an intentional no-op, scoped in the requirement's
  compatibility section.
  The existing machine issues
  `loadMessageWindowAround` (`around=<prompt-id>`), merges the window into
  `messages.bySession`, scrolls with `align: "start"`, reasserts once after 250 ms,
  and consumes the target.
- The jump-in-progress indicator (`transcript-jump-loading`) covers the local
  jump as well. Around settlement for the local target is by identity, not
  visibility: a confirmed `deleted-target` consumes the target even from a hidden
  panel, so no request fires without a new activation, and no prompt-projection or
  transcript state is mutated (prompt-history reconciliation stays event-sourced;
  a missed delete leaves the entry, and the next activation re-confirms it with one
  request); a merged window is retained for the deferred scroll, so
  reactivation never re-requests a window whose outcome is already known. A second
  activation while the jump is in flight takes a new token that invalidates the
  first target before its guard runs (the first response never merges, scrolls, or
  consumes), and only the newest target settles: `merged` gets the normal
  start-aligned scroll, one reassertion and one consumption, `deleted-target` or a
  rejection consumes it without retry. The full
  visible
  sequence is one around request, three `scrollToMessage` calls on the activation's
  counter, all of them the machine's (the pre-merge attempt, the post-merge
  start-aligned scroll, and its single 250 ms reassertion), and one consumption; the
  hidden-settlement phase
  produces two of those calls (the reactivation scroll and its reassertion), so a
  phase-scoped assertion scopes its counter. The option is carried
  by the local instance only; the host-provided pending target and the Dockview
  prompt-history path keep their existing visibility-gated settlement. The merge
  can move `firstMessageId` and therefore the scroll-to-start affordance; that is
  accepted for a user-initiated jump. `loadMessageWindowAround` then hands the
  freshness-selected union to `mergeMessages`, whose `reconcileMessages` pass reuses
  the cached row when the two signatures match, and `signatureOf` treats any truthy
  `updated_at` — malformed included — as authoritative; the signature must therefore
  fall back to the content hash for an unparseable `updated_at`, or the three-case
  rule is undone one layer below for exactly the malformed-timestamp rows it exists
  to replace.

### 3. Unloaded prompt edge (Task 03)

- `message-list-shared.tsx` adds the pure `resolveUnloadedPromptEdge`
  (container rect plus nearest older/newer neighbor rects, two-pixel tolerance)
  and `findPromptNeighbors`, with `visible` for the no-neighbor input. The walk
  lives here rather than in the lib module because it resolves row keys through
  `getItemKey`, which this module owns; putting it in `lib/session-last-prompt.ts`
  would force a component ⇄ lib import cycle.
- `message-list-native.tsx`'s `useTranscriptEdgeTracking` takes the resolved
  last-prompt message, the panel's unloaded flag, and the rendered transcript
  items. It classifies with `resolveUnloadedPromptEdge` only when the prompt has
  no row element and the flag is set; a cached-but-unrendered prompt keeps
  today's `visible`, so the new geometry path cannot add an affordance the jump
  cannot serve.
- `findPromptNeighbors` walks render items, not messages, so a message collapsed
  into a `turn_group` still bounds the prompt through its group's row
  (`getItemKey`), and bounds by containment (an item holding any message on a
  side) so a group straddling the prompt supplies both bounds. Each row key is
  measured with the container-scoped `findMessageRow` (now exported from
  `message-list-native-scroll.ts`), and the prompt row itself is
  resolved through the same container-scoped helper rather than today's
  document-global `getElementById`, so neither a neighbor's duplicate row nor
  another mounted transcript's copy of the prompt row can supply this container's
  rects, no fourth row-lookup variant is introduced, and a foreign prompt row
  cannot bypass `resolveUnloadedPromptEdge`. An item is unordered when it holds at
  least one
  non-synthetic message without a parseable `created_at` (the mixed item counts,
  not only the all-unparseable one), and an unordered item makes a missing bound
  inconclusive, which reports `visible` rather than a direction.
- Geometry follows chronology: the transcript renders older rows above newer
  ones, so the case that places the prompt's unloaded slot inside the viewport is
  an older bound fully above the container and a newer bound fully below it.
- The walk runs per measurement with the rendered item list as an effect input,
  so a prompt id that settles before the rows arrive is re-classified once they
  render.

## ASCII UI preview

Views below are labeled and shared by the work orders that change UI. Structural
requirements: the pinned prompt renders inside the transcript's existing
anchored bar; the control stays in the composer status bar; no new surface,
control, or copy is introduced. Spacing and wording are illustrative; the
prompt text shown is the user's own stored prompt.

**Regions (both views).** Fixed: the sidebar, the right column, the view-tab
selector, the anchored bar, and the composer status bar. Scrolling: the
transcript body (`.chat-message-list`, the only scroll owner in both
compositions). The bar overlays the top of the scrolling region and reserves
`--anchored-bar-h`, which the row `scroll-margin-top` consumes; the composer sits
outside the scrolling region.

**Evidence for the `CURRENT` view.** Source, not a render:
`apps/web/components/task/task-chat-panel.tsx` derives the last prompt from
`allMessages` only (`getLastUserMessageId`) and gates both affordances on that id;
`apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts` asserts the current
appearance and hide behavior for a prompt that stays inside the window. The
seeded unloaded state in these previews therefore shows no affordance today; Task
04's scenarios are the rendered evidence once implemented.

### UI-01: Desktop transcript, last prompt outside the loaded window

Entry point: a session whose newest message page contains no user row (long
agent run after the last prompt). Current behavior versus the plan. ACs:
`AC-UI-PINNED-PROMPT-AVAILABILITY-001.1`, `.2`, `.3`, `.7`; targeted rendered
check: Task 04's desktop scenario (control visible, bar open with the prompt's
stored text, activation lands the row at its `scroll-margin-top` offset, no
prompt row before activation).

```text
CURRENT
+---------------------+--------------------------------------------+----------+
| sidebar             | [chat] [Prompt history] [Changes]          | right    |
|                     +--------------------------------------------+          |
|                     |                                            |          |
|                     |  tool call: read file ...                  |          |
|                     |  filler message 120 ...                    |          |
|                     |  (no user prompt row is loaded)            |          |
|                     |                                            |          |
|                     +--------------------------------------------+          |
|                     |  [ composer ]                              |          |
|                     |  (no scroll-to-last-prompt control)        |          |
+---------------------+--------------------------------------------+----------+

PLANNED
+---------------------+--------------------------------------------+----------+
| sidebar             | [chat] [Prompt history] [Changes]          | right    |
|                     +--------------------------------------------+          |
|                     | [^] LAST-PROMPT-MARKER-9F2Q please handle  | <- anchored
|                     |     this scrolled-past regression ... [v]  |    bar
|                     +--------------------------------------------+          |
|                     |  tool call: read file ...                  |          |
|                     |  filler message 120 ...                    |          |
|                     |                                            |          |
|                     +--------------------------------------------+          |
|                     |  [ composer ]              [^ Scroll to    | <- control,
|                     |                              last prompt]  |    up arrow
+---------------------+--------------------------------------------+----------+
```

Requirements in this view: the anchored bar shows the stored last prompt with
its existing collapse/expand control; the status-bar control is present and
points up. Illustration only: bar height, prompt truncation, and icon art.

### UI-02: Phone transcript, last prompt outside the loaded window

Entry point: the same session on a phone viewport. ACs:
`AC-UI-PINNED-PROMPT-AVAILABILITY-001.1`, `.2`, `.8`; targeted rendered check:
Task 04's phone scenario (no bar with the setting enabled, control present, the
row absent before activation, one activation reaching it at its
`scroll-margin-top` offset).

```text
+----------------------------------+
| < Task            [ status bar ] |  fixed: task chrome + status bar
+----------------------------------+
| tool call: read file ...         |  scrolling: .chat-message-list
| filler message 120 ...           |  (single scroll owner; dvh-sized,
| (no user prompt row is loaded)   |   safe-area padded by the shell)
|                                  |
+----------------------------------+
| (no anchored bar on phone)       |  fixed: composer row
| [ composer ]   [^ last prompt]   |  <- existing control, up arrow,
+----------------------------------+     >=44px touch target
```

Requirements in this view: no anchored bar, the existing control is present and
points up, and activating it loads and aligns the unloaded prompt. Illustration
only: phone chrome, status-bar contents, and icon art.

## Mobile design contract

- **Desktop outcome and mobile entry point:** the same value on both surfaces
  (reach the newest stored prompt when it is outside the loaded window); the
  phone entry point is the existing scroll-to-last-prompt control in the
  composer status bar of the phone conversation shell.
- **Nearest shipped mobile exemplar:** `apps/web/components/task/mobile/session-mobile-layout.tsx`
  (phone conversation shell, dynamic viewport + safe-area handling, composer
  status row) with the interaction contract of
  `apps/web/e2e/tests/chat/mobile-last-prompt-scroll.spec.ts` and the navigation
  protocol of `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts`.
- **Hierarchy and primary action:** the transcript is primary; the control is a
  secondary status-bar action next to the existing transcript navigation
  controls, and it remains a single tap.
- **Presentation choice:** unchanged inline composition. No drawer, sheet, or
  new surface is introduced, because the action is a single control whose result
  is the transcript itself.
- **Surface rationale:** the prompt jump is a low-frequency, one-tap action on
  content the reader is already inside, so it belongs beside the existing
  controls rather than in a dedicated phone surface; the anchored bar stays
  desktop-only, as the pinning requirement already specifies.
- **Scroll owner, viewport, safe area, touch targets:** `.chat-message-list`
  remains the only scroll owner; the shell keeps dynamic viewport units and
  safe-area padding; the control keeps its existing coarse-pointer sizing at or
  above 44px and no new fixed or absolute element is added. Document-level
  horizontal overflow stays zero.
- **Shared logic versus phone presentation:** resolution, availability,
  arbitration, and the pending-scroll machinery are shared with desktop; the
  phone differs only in the absence of the anchored bar.
- **Mobile Playwright scenario:** Task 04's `mobile-last-prompt-scroll.spec.ts`
  pass proves the same value on the phone project.

## Public docs

`docs/public/tasks-and-workflows.md` ("Navigate long chat transcripts") states
the current availability rule ("When your latest prompt has fully left the
transcript viewport, **Scroll to last prompt** appears..."). This package
extends availability to a known prompt that is outside the loaded window, so the
same PR adds one sentence there: the control also appears when the latest prompt
is no longer part of the loaded transcript window (for example after a long agent
run), and selecting it loads that part of the transcript and aligns the prompt at
the top. The direction, hide, settings, and anchored-bar semantics in that
section stay unchanged, and phone behaviour is already described. Task 04 owns
the edit and its validation.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-UI-PINNED-PROMPT-AVAILABILITY-001.1`, `.5`, `.6`, `.7`, `.9` | `apps/web/lib/session-last-prompt.test.ts` (resolution, synthetic row, unparseable timestamps) and `apps/web/components/task/task-chat-panel.last-prompt.test.tsx` (availability, empty window, and the named projection fallbacks: empty, older-than-window, a failed/unavailable read, and an in-flight read — each retaining the raw loaded-window last prompt and its affordance; the cached-but-unrendered gate with no sticky bar and a zero height reservation, the same gate re-evaluated after a launch-error filter flips under an open bar with a `MessageList` mock that renders the captured `stickyPromptBar` so the real pinned bar mounts, with the sticky node and the height reading one post-filter item list; a failed or in-flight read over a non-authoritative older-only cache in a window with no user row presenting no affordance; a live prompt arriving after a successful empty read resolving with no further read; a newer unobserved page row over an older observed set still resolving while the newest cache row is not admitted before any id is observed, the same resolution surviving a replacing page that empties the cache of observed rows; a stored user prompt the visible-row filter drops still resolving from the raw cached window; a same-id authority response whose `created_at` differs never advancing the floor past an unobserved intervening row; a stale post-purge write never admitting a row into the next incarnation (asserted with a real handler/store delivery after the purge); a generation-0 observation surviving only until the purge, with a strictly newer unobserved row in the next incarnation not admitted; a held authority response that raced a newer live create never lowering the recorded key; a strictly newer row whose `session_id` differs from the keyed session rejected by every writer (`session-slice.prompts.test.ts`) and by the around loader (`load-message-window.test.ts`), with only a consumer-level assertion in the panel suite; an order tie keeping the loaded-window row unless the projection row's `updated_at` is strictly newer, with the tie fixture being an admitted clause-2 row tied with the window row in both directions; an authority read returning two rows, its newest deleted while the older survived, with the older resolved from its observed id, so losing the newest observed row never drops the affordance; an authority prompt, a live prompt, then a disjoint backfill evicting that live row from the transcript, with the live row still resolved as unloaded from its observed id; unmount before a held request resolves or rejects (no cache write, no marker, no loading write, no further request); a synthetic action-carrying cached-but-unrendered prompt (representable, with no producer today); unchanged behavior for a rendered prompt) |
| `AC-UI-PINNED-PROMPT-AVAILABILITY-001.2` | `apps/web/components/task/task-chat-panel.last-prompt.test.tsx`, the existing `apps/web/components/task/task-chat-panel.scroll-target.test.tsx`, and `apps/web/hooks/domains/session/load-message-window.test.ts` (whose own branches the component suites mock: a held target-containing response whose guard turns false returning `stale` with no merge, a current response omitting the target returning `deleted-target` with no merge, the newly-supplied-ordinal case on an equal `updated_at` (the around merge keeps the cached row's content and adopts the incoming `prompt_index`, since the shared comparator is a boolean and `mergeWindowRows` otherwise keeps `current` outright), and the non-contiguous union `.7` requires — a retained newest window and an around response whose spans do not tile the run, with the target and newest rows surviving, the known middle ids still absent, and pagination metadata unchanged): around request, start alignment, the `transcript-jump-loading` indication covering the local request, a rejection while hidden consuming the target once with the indication cleared and reactivation issuing no request, a user prompt newer than the target that the around merge reveals re-targeting nothing (the jump lands on and consumes the activated prompt), a merge that changes the rendered rows leaving the local target inert (no scroll, no consumption, no second request) with no stale bar reservation, hidden `deleted-target` settling without a second request, hidden `merged` settling with exactly one around request on the pre-hide activation and no second request on reactivation, two scroll calls (the reactivation's start-aligned scroll and its single 250 ms reassertion) and one consumption, a second activation superseding the first (new token, first response discarded, exactly one additional request, one consumption of the newest target), a host-provided target keeping its visibility-gated settlement, a session removed and recreated under the same id before the request (no request, no indication) and while it is in flight (no merge, no scroll, no consumption), the local target's ordinary lifecycle (an ordinary session switch and a panel unmount drop it with the timers flushed and a late around response settling nothing, while a hidden panel keeps and resumes it), and the around merge's equal-`updated_at` regression in `apps/web/hooks/domains/session/load-message-window.test.ts` (the cached row is the one in the union handed to `mergeMessages`) |
| `AC-UI-PINNED-PROMPT-AVAILABILITY-001.3`, `.4`, `.9` | `apps/web/components/task/chat/message-list-shared.test.tsx` (edge truth table incl. the no-neighbor row) and `apps/web/components/task/chat/message-list-native.test.tsx` (tracker behavior incl. a grouped-only window, a duplicate neighbor row outside the container, and a second transcript holding only the prompt row, all under a local `ResizeObserver` stub with stubbed container and row rects, since the tracker constructs a real observer and happy-dom's never fires) |
| `AC-UI-PINNED-PROMPT-AVAILABILITY-001.6` (shared cache) | `apps/web/hooks/domains/session/use-session-prompts.test.ts` (marker-keyed first-load-only policy: reads without the marker even when an older-only cache exists, a session generation advancing while the request is held installing nothing, setting no marker, writing no failure or loading state, and issuing no further request on success and on rejection alike, a held request whose session-subscription readiness is released and re-subscribed inside one effect replay asserting one request against the new readiness (the current readiness mock cannot express it), a second concurrent consumer joining that one in-flight request through the shared readiness object, and the authority install unioning the response's ids into the observation record while keeping the newer key, leaves an authoritative cache and its `oldestCursor` untouched, preserves the paged-in entries and the cursor when it does read over a non-authoritative older-page cache, attempts once per session generation and per connection-status change while the marker is absent — a failed read retries on the next status change, a marker-set read issues no further request on any transition, a held request whose refresh generation advances mid-flight still resolves and rejects with exactly one request each (the live row preserved, the marker set on success and unset on rejection with the next status change retrying), an authority response held across a prompt deletion (cached and uncached at deletion, through the transcript install and through the prompt-history panel's replacing read) installing without that row, still setting the marker) — while the held older-page race itself is asserted in `apps/web/hooks/use-lazy-load-prompts.test.ts` (a held request survives the transcript read and install with its rows and cursor intact; the `use-session-prompts.test.ts` suite cannot execute that path) plus the malformed-`updated_at` reconciliation regression in `apps/web/lib/state/slices/session/message-signature.test.ts` (changed content survives `reconcileMessages`; the panel suites cannot host it because they stub `mergeMessages`), and `apps/web/lib/state/slices/session/session-slice.prompts.test.ts` for the delegated store predicate and ordering and the marker's paths (set only by the transcript panel's authority-carrying install, never by fan-out/prepend/prompt-history reads, cleared by the session-removal purge, and untouched by a late hydration payload carrying `messagePrompts`; the prompt-history-owned removal path stays green as an unchanged neighbour, not as this requirement's evidence) plus the writer matrix across the four page shapes (a skipped cursor row repairs to the oldest retained row, a skipped non-cursor row keeps the incoming cursor, an all-rejected page clears cursor and `hasMore`, and a zero-row response keeps its own metadata — the exhausted page's `has_more` false and empty cursor, asserted as the terminal state the older-page path needs), a one-row deletion that empties the cache clearing both, the three-case freshness regressions through the authority install (same-id equal-`created_at`/equal-`updated_at` older content each keeping the cached row, and the `updated_at`-less pair where the cached row is replaced by design; the around-window half stays `load-message-window.test.ts`'s), a held authority response with an uncached update landing mid-flight (the update is not overwritten and pagination metadata is untouched), and a held authority response resolving after a last-row deletion (clearing nothing and re-enabling nothing when all rows are tombstoned; staying unpaginated when a valid row arrives), and `apps/web/lib/state/default-state.test.ts` (the boot constraint on `mergeInitialState`'s output — the only place a payload slice is read) with `apps/web/lib/state/hydration/hydrator.test.ts` for `hydrateState` and one `createAppStore` assertion in `apps/web/lib/state/store.test.ts` documenting that the slice's `defaultSessionState` wins |
| `AC-UI-PINNED-PROMPT-AVAILABILITY-001.8` | Phone Playwright project |

Existing suites that must stay green: `message-list-shared.test.tsx`,
`message-list-native.test.tsx`, `task-chat-panel.scroll-target.test.tsx`,
`task-chat-panel.launch-error.test.tsx`, `session-slice.prompts.test.ts`,
`use-session-prompts*.test.ts(x)`, `load-message-window.test.ts`,
`message-signature.test.ts`, `session-slice.merge-messages.test.ts`,
`default-state.test.ts`, `hydration/hydrator.test.ts`, `store.test.ts`,
`dockview-panel-actions.prompt-history-panel.test.ts`,
`message-list-native-scroll.test.ts`, `remove-task-session.test.ts`.

The main integration removes `useLazyLoadPrompts` and its test because the
built-in Prompt History panel no longer consumes older prompt pages. The pinned
feature retains the bounded latest-prompt read and its session projection.

Supporting evidence that does not key to an acceptance criterion:
`apps/web/hooks/use-processed-messages-fallback.test.ts` new coverage (rendering the hook, since the split lives in its `useMemo` and the pure-builder suite cannot observe it) of the derivation
that decides rendering (a plain user row renders; an action-carrying row moves to
the footer action list). No AC requires it: it pins the premise of the panel
suite's synthetic cached-but-unrendered fixture, so it stays in Task 01's
verification command and its files list without appearing in the AC-keyed table.

## E2E tests

`apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts` (`chromium`) and
`apps/web/e2e/tests/chat/mobile-last-prompt-scroll.spec.ts` (`mobile-chrome`)
gain one unloaded-prompt scenario each, reusing
`seedScrolledPastLastPrompt` with `trailingFillerCount: 120`. The helper's
default is 50, which stays inside the initial 100-row fetch
(`INITIAL_FETCH_LIMIT` in `hooks/domains/session/use-session-messages.ts`), so the
literal count is required for the fail-before/pass-after claim. Both scenarios
reload the task after seeding (`testPage.reload()`, `waitForLoad`,
`waitForChatIdle`) before asserting: seeded rows arrive over the live socket and
stay in `messages.bySession`, so only a fresh bounded fetch reproduces the state
where the last prompt is outside the loaded window. This is the pattern the
existing `keeps task opening bounded before last-prompt navigation` scenario
uses. Each desktop pass also asserts the newest seeded row survived the jump
(the scoped `filler message <trailingFillerCount>` row, count 1), because the
around response must union into the
cache rather than replace the newest window. The alignment assertion follows
`apps/web/e2e/tests/task/prompt-history-panel.spec.ts`: the prompt row's top
relative to the scrollport equals its computed `scroll-margin-top` within a small
tolerance, not zero. Because the anchored bar is enabled, that scenario must scope
its locators: the control lives in the chat status bar
(`chat.getByTestId("chat-status-bar").getByTestId("scroll-to-last-prompt-button")`)
and the bar has its own button inside `anchored-last-prompt-bar`, and every
assertion on the timing-dependent control, bar, and prompt row is web-first and
retrying (`toBeVisible`, `toHaveAttribute("data-state", "open")`, `toHaveCount`),
with the arrow direction read from the button's icon class. The prompt row itself
is located through its transcript row (`[id^='msg-']`) so the filter cannot match
the anchored bar's own copy of the same text; the sibling scenarios keep their
existing `getByText(marker, { exact: false }).first()` form unchanged, and the
bar's own scroll button is asserted alongside the status-bar control.

- Desktop: `AC-UI-PINNED-PROMPT-AVAILABILITY-001.1`, `.2`, `.3` and `.7`'s
  rendered-row clause only — the control is visible with no prompt row loaded, the
  anchored bar is enabled and presents the prompt, and two passes (the bar's own
  button, then a reload and the status-bar control) each load and align the prompt
  row through the shared `scrollToLastPrompt` handler; the transcript still holds
  no prompt row before each activation. `.7`'s request and pagination-cursor
  clauses are component/store evidence in Task 01, which asserts them.
- Phone: `AC-UI-PINNED-PROMPT-AVAILABILITY-001.8` and `.2` — no anchored bar, the
  control is present, and the persisted prompt row is absent before activation
  (scoped `#msg-<id>` count 0), so the pass cannot take the already-loaded path.
  One activation reaches the prompt row at the same
  scroll-margin-adjusted start position the desktop pass asserts. The row lookup
  follows the mobile `mobile-prompt-history-panel.spec.ts` (`activeChat()` row
  locator by message id, then an attached wait); the measurement follows the
  desktop `prompt-history-panel.spec.ts` (a causal settle poll on the closest
  `.chat-message-list` `scrollTop` until two consecutive reads agree, then
  `|rowTop - listTop - scrollMarginTop| <= 2`). The id comes from
  listing the session's persisted messages after seeding and selecting the user
  message whose content is `LAST_PROMPT_MARKER`. The in-flight
  `transcript-jump-loading` portion of `.2` is evidenced at the component
  boundary rather than by holding the phone's around response.

## Work orders

- [x] [Task 01: Resolve the last prompt outside the loaded window](task-01-resolve-last-prompt-outside-window.md)
- [x] [Task 02: Load and align an unloaded last prompt](task-02-load-unloaded-last-prompt.md)
- [x] [Task 03: Classify the pinned prompt edge without a rendered row](task-03-unloaded-prompt-edge.md)
- [x] [Task 04: Prove unloaded prompt availability in a browser](task-04-browser-evidence.md)

## Verification results

Verified after merging current main: 360 focused web tests across 23 files,
web typecheck, task-panel ESLint and Prettier, desktop prompt E2E (12/12),
phone prompt E2E (2/2), capture scenarios (1/1 per viewport), and public-doc
validation (62 tests, 47 published pages) passed.

## Risks

- The prompt projection read adds one `message.list?author_type=user` request per
  session view that lacks the authoritative projection marker. The first-load-only
  policy bounds it to those sessions — a cache holding only transcript fan-out or
  older-page entries still reads, which is what the marker exists for — and keeps
  the prompt-history panel's paginated pages and `oldestCursor` intact. A read that
  fails or is still in flight falls back to the raw loaded window without regressing
  today's behavior, and that fallback keeps its prompt's affordance. In the state this
  package exists to fix the fallback is empty only when no live prompt has been observed
  and the cache holds nothing but non-authoritative rows (the newest page has no stored
  user row), so no affordance appears until a read OR a live observation supplies one: a
  prompt observed live resolves on its own, before any successful read, and keeps
  resolving after reconciliation evicts its window row. A window whose only stored user
  prompt is the synthetic row presents nothing. Recovery follows the projection's existing path: a
  reconnect, a session-generation change, or a remount of the transcript panel,
  which re-runs that panel's own read. The Prompt history panel's retry cannot
  restore the transcript's resolution: its read installs a non-authoritative cache,
  its retry state belongs to its own hook instance, and the transcript effect
  re-runs only on a connection-status transition, a session-generation change, or
  its own retry. No polling loop is added.
- Only an authoritative projection may contribute a prompt. Transcript fan-out and
  older-prompt pagination write the same cache without authority, so a window whose
  newest page has no user row plus a non-authoritative older-only cache would
  otherwise resolve that stale prompt and present it as the last prompt while the
  initial read is in flight or has failed. The resolver's admission term is the newest cache row the session's
  observed-prompt set holds — the ids the panel's own successful read returned plus the
  ones the live create path recorded — together with a cache row that orders strictly
  newer than the newest observed key, whatever row that key came from, and the panel
  suite asserts the stale-cache case
  presents no affordance in three shapes: before a read, after a failed one, and
  after a successful empty response over that same older-only cache, while a live
  prompt arriving after that empty response still resolves: from the window while the
  row is there, and from its observed id once a reconciled fetched window drops it below
  the fetched boundary.
- The local target must carry the session generation, not just the session id: the
  session-removal purge deletes the caches and bumps the generation, so a session
  removed and recreated under the same id would otherwise satisfy the target's and
  the around guard's identity and let an old target or an in-flight response load
  or settle into the new incarnation. The generation term is what drops the target,
  its indication, its `completedAround`, and its pending reassertion before any
  merge or settlement; read-request dedupe
  partitions on the session generation for the same reason, but that key alone cannot
  carry the read: `requestSessionMessages` in `use-session-messages.ts` also compares
  `existing?.readiness === readiness`, because `use-session-prompts.ts`'s
  `sessionId\u0000generation\u0000refreshGeneration` key cannot tell a released readiness from a
  live one. Without the readiness identity an effect replay that unsubscribes and
  resubscribes joins the rejected request, issues zero attempts while the socket still
  looks connected, and the attempt bound's "no retry while connected" makes that
  dead end sticky; the same hook's test mock (`ready: Promise.resolve()`) cannot express
  the replay, so the readiness mock must model `getOrCreateSessionSubscriptionReadiness`'s
  identity: one promise object while at least one subscription is live (which is what keeps
  the suite's concurrent-consumer case at one request), and a fresh promise that
  `cancelSessionSubscriptionReadiness` rejects once the unsubscribe count reaches zero. A
  per-call-fresh mock passes the replay case and breaks the sharing case; a stable mock
  passes the sharing case and makes the replay case vacuous.
- The transcript first-load path must bypass the hook's refresh-generation mismatch
  retry and define all four response outcomes, or it breaks the attempt bound: the
  hook re-issues when the refresh generation advanced mid-flight (success branch
  `setRetryVersion`, failure branch likewise), and `fanOutTranscriptPrompts` bumps
  that generation through `upsertPromptMessage` on every live user-message event.
  Success on a current generation installs and marks; failure on a current generation
  leaves the marker unset with no mismatch retry (the documented status-change retry
  stays); a stale generation or unmounted consumer writes nothing. The retry stays
  with the prompt-history read, which replaces rather than upserts.
- The authority success branch ignores the refresh mismatch, so a deletion that
  lands mid-flight must be made to win explicitly, and the rule must be a
  slice-entry invariant rather than an install-level check: `removePromptMessage`
  advances the refresh generation and removes the row, the live
  `session.message.deleted` handler reaches it, and the stale response still contains
  the prompt. The transcript install is not the only writer — the prompt-history
  panel's own `useSessionPrompts` instance calls `replacePromptMessages`, which
  replaces the whole array and consults nothing, and it stays mounted beside the
  transcript, so an install-level skip alone would still be undone by that read. The
  per-session deleted-prompt-id set, written by the removal path before its
  cache-presence and not-present early returns (a prompt uncached at deletion still
  advances the generation), consulted by every row-writing action, and cleared by the
  purge, prevents resurrection from either installer. The held-response regression
  covers both installers and a prompt cached or uncached at deletion.
- The freshness rule has a second half below the merge: the transcript cache's
  `reconcileMessages` reuses the cached row when the signatures match, and
  `signatureOf` treats a malformed-but-truthy `updated_at` as authoritative, so a
  freshness-selected replacement would be discarded again. The signature takes the
  content-hash fallback for an unparseable `updated_at` (Task 02 owns it), with the
  regression above.
- The merge comparator is an authorization boundary, not a convenience: the store's
  `isIncomingMessageAtLeastAsFresh` tests its cached-unknown branch first and returns
  true for `>=`, so a held authority response with the same id and an equal
  `updated_at` replaces newer cached content, and `mergeWindowRows` shares that
  helper, giving the jump the same regression. The helper lives in
  `apps/web/lib/state/slices/session/message-timestamp.ts` and has exactly two
  consumers, the prompt-cache delegate and `mergeWindowRows`, so Task 01 owns that
  file and its ESLint block while Task 02's window merge inherits the corrected
  comparator. Both paths take the three-case rule
  above, with regressions for same-id/equal-timestamp older content and for the
  `updated_at`-less pair, whose cached row is replaced by design (the accepted
  fail-open, and the only way a content update lands against a payload carrying no
  `updated_at`).
- The read must not invalidate the prompt-history panel's in-flight older page:
  `useLazyLoadPrompts` captures `refreshGenerationBySession` and refuses a response
  when it changed, and the sentinel then replays the page; both
  `setPromptMessagesLoading(true)` and the per-row upsert bump that generation. The
  transcript read is therefore request-local for loading and its authority install
  advances neither the shared cursor nor the refresh generation, so a held
  older-page request lands with its rows and cursor intact. A held-request test
  covers it.
- Boot hydration is an authority writer and must be constrained, but the assertion
  belongs where the payload is copied: `mergeInitialState` spreads
  `initialState.messagePrompts`, so that merge is the only place a payload slice is
  read — `createAppStore` composes `...merged` and then the session slice, whose
  `defaultSessionState` re-asserts `messagePrompts`, and `buildStateOverrides` has no
  such key, so a `createAppStore`-level test would pass before any change and must
  not be added as enforcement. `mergePromptHistoryState` therefore hydrates only
  whitelisted cache and metadata fields and forces the marker, the observed-prompt record
  (ids and newest key), and deleted-id maps empty; the assertion lives where the payload is copied — `default-state.test.ts` on
  `mergeInitialState`'s output and `hydrator.test.ts` for `hydrateState` — with one
  `createAppStore` assertion in `store.test.ts` documenting that the slice default wins. Adding
  `messagePrompts` to `buildStateOverrides` would create the very hazard this
  prevents and is explicitly out of scope.
- `updatePromptMessage` is a row writer with its own hazard: it returns without
  writing when the id is absent from the prompt cache, while the authority path is
  terminal and ignores a mismatch, so an update for an uncached id during a held
  authority read could be overwritten by the stale response. It joins the row-event
  shape: tombstone-aware freshness merge, insertion when the id is absent, no
  pagination metadata. Its refresh-generation bump stays, as does
  `upsertPromptMessage`'s, because that bump is what invalidates an in-flight
  prompt-history page on a live prompt change — dropping it would weaken the guard
  the older-page rule relies on. The pinned live-mutation refresh assertions
  therefore stay green unchanged.
- A cleared-but-initialized cache is not "no cache": the removal path can leave
  `bySession[sessionId] === []`, and a held authority response resolving after that
  must neither resurrect `hasMore`/cursor from its own metadata nor inherit the
  cleared `hasMore=false` for a valid new row. The matrix names the three states (no
  entry, retained rows, cleared-empty) and the metadata each accepts.
- A projection read that fails while the socket stays connected has no retry: the
  hook's retry state is per instance and only the prompt-history panel renders it, so
  the transcript's failed state is otherwise unused and the affordance waits for a
  session-generation change, a reconnect, or a panel remount. AC .6 accepts that dead
  end and the package adds no transcript-side failure surface or new copy; a future
  transcript retry would need its own localized error affordance, which is out of
  scope here.
- The marker is client-only, so late hydration cannot set authority: the boot default
  initializes it empty and `StateHydrator`'s session merge never touches this slice,
  which is asserted rather than assumed.
- The transcript edge tracker gains geometry lookups for an unloaded prompt.
  Misclassification would show or hide the anchored bar at the wrong time, so the
  truth table and tracker tests cover no-neighbor, both-neighbors-outside, and
  single-neighbor cases; the residual case stays `visible` with the control still
  available.
- Start alignment must be asserted the way the transcript defines it: the row's
  top at the viewport top plus its `scroll-margin-top`, which is
  `calc(4rem + safe-area-inset-top)` on phone and `var(--anchored-bar-h)` on
  desktop. A literal `rowTop === listTop` assertion fails against correct
  behavior, and satisfying it would have to break the existing scroll-margin
  contract.
- The panel's unloaded flag and the pending-scroll machine's loaded check must stay
  the same condition: both are "absent from `messages.bySession`". Task 01 defines
  the flag that way, which is also why a cached-but-unrendered prompt keeps
  today's behavior instead of gaining a control whose activation is a no-op. That
  state is representable but has no producer today: the footer-action split and
  activity grouping are author-agnostic, while every writer of `metadata.actions` is
  agent-authored (the stall notice, the transient retry notice, the
  recoverable-failure and git-error cards), so no user row reaches it.
  The panel suite therefore guards that gate with a synthetic action-carrying
  cached row, and the hook-rendering `hooks/use-processed-messages-fallback.test.ts` pins the split the fixture
  depends on. Beyond the bar's own mount gate, nothing depends on the row being rendered:
  a missing element reports `visible`, so no control is offered and no target is created.
- A user-initiated jump merges an around window that is the target plus the rows
  newer than it, so a window that held no user row gains the target (the newest
  stored user prompt, older than nothing in that window) and the scroll-to-start
  affordance can appear after the jump; that id can never move to an older row.
  Accepted for a jump; resolving the prompt alone never changes the window.
- The anchored bar's non-empty-window gate must read the cached window
  (`messages.bySession[resolvedSessionId]`), not `allMessages.length`: the
  synthetic task-description row renders while the cache is empty, so an
  `allMessages`-based gate would mount a closed bar where AC .9 requires none.
- The browser scenario must reload after seeding: live rows stay in
  `messages.bySession` (nothing evicts them), so without the reload the last
  prompt is still cached and rendered and the scenario cannot reach the state it
  claims to test.
- Neighbors must be rendered rows, not messages: a message collapsed into a
  `turn_group` has no `msg-<id>` element, so a message-keyed walk would find no
  neighbor in a tool-heavy window and leave the bar closed where the prompt
  precedes every rendered row. Bounds must also be by containment, or a group
  straddling the prompt would bound neither side and the direction would invert.
- The tracker must re-resolve its bounds when the rendered item list changes: the
  prompt id arrives from the projection read, the rows from the session fetch, and
  a capture taken only on the id would leave the bar closed after the rows land.
- Geometry follows chronology (older above, newer below); the both-neighbor
  clause must be stated in that order or a test written from it would assert an
  unreachable case.
- The local target must never compete with the panel's existing target owners:
  while `pendingScrollToMessageId` or `pendingScrollTarget` is active, or while the
  Dockview store holds a live target for this session and panel id, no local target
  is created, an existing one is dropped with its indication, and the local attempt
  path stays gated, so one owner scrolls and consumes and neither clears the
  other's state. Activation during another owner's in-flight jump is an intentional
  no-op documented in the requirement's compatibility section.
- Around settlement for the local target must be keyed on identity, not
  visibility: the reused machine's current guard makes a response that arrives
  while the panel is hidden look stale, which would drop a confirmed deletion (and
  a merged window) and re-request on activation. The mode must split the single
  current predicate into an identity settlement guard for the request's success
  and failure paths, the existing visibility guard for the scroll attempt and the
  reassertion scheduler, and mode-specific `completedAround` retention (retained
  for the local identity mode while hidden, cleared for the host mode as today).
  It must be an explicit option so the host-provided pending target and the
  Dockview prompt-history path keep their existing behavior.
- A merged response that settles while the panel is hidden must be retained: the
  artifacts promise one around request on the pre-hide activation, and on
  reactivation exactly two scroll calls (the activation attempt and its single
  250 ms reassertion) and one consumption, with no second request. A component
  test must cover the hidden merged case as well as the hidden
  deletion, and the host-mode hidden settlement must stay asserted in
  `task-chat-panel.scroll-target.test.tsx`.
- The local pending-scroll instance must take the transcript's visibility
  (`transcriptIsVisible`), not the raw `isVisible` prop: thread conversations and
  the kanban hover preview render a visible, clickable transcript with
  `isVisible={false}`, and the raw prop would defer their jump forever. The
  existing host-target instance keeps the raw prop, unchanged.
- With the anchored bar enabled, two elements carry the
  `scroll-to-last-prompt-button` test id, so the desktop scenario must scope its
  locators to the status bar and the bar rather than using the file's unscoped
  form.
- Prompt ordering moves into `lib/session-last-prompt.ts` and the store delegates
  to it. A semantic drift there would change the prompt-history panel's ordering,
  so `session-slice.prompts.test.ts` stays in the verification commands.
