---
id: "02-load-unloaded-last-prompt"
title: "Load and align an unloaded last prompt"
status: done
wave: 2
depends_on:
  - "01-resolve-last-prompt-outside-window"
plan: "plan.md"
requirements:
  - REQ-UI-PINNED-PROMPT-AVAILABILITY-001
acceptance_criteria:
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.2
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.6
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.7
  - AC-UI-PINNED-PROMPT-AVAILABILITY-001.8
system_design:
  - ../../specs/ui/system-design/pinned-prompt-availability.md
---

# Task 02: Load and align an unloaded last prompt

## Summary

Make the scroll-to-last-prompt control reach a prompt that is not in the loaded
window: when the resolved prompt is absent from `messages.bySession`, request the
window around the prompt, merge it, place the prompt row at the transcript's
start-aligned position, and report the jump with the existing in-progress
indication.

## In scope

- A local `PendingMessageScrollTarget` in `task-chat-panel.tsx`, created inside
  the shared `scrollToLastPrompt` handler that the status-bar control and the
  anchored bar's own button both call, for a prompt the panel resolved as unloaded
  (absent from `messages.bySession`), which is also the condition under which the
  pending-scroll machine can act on it.
- A second `usePendingMessageScroll` instance owning that target, wired with
  `messageId: null` so its identity comes from the target alone, leaving the
  host-provided prompt-history target untouched, with an explicit settlement-mode
  option so only this instance settles around responses by identity rather than by
  panel visibility, and an ownership rule so the local target never competes with
  the panel's other target owners (the host inputs and the Dockview store's
  target recorded for this session and panel id).
- The jump-in-progress indicator covering the local jump.

## Out of scope

- Changing the around request shape or pagination metadata. The merge itself takes
  the three-case rule this package introduces (strictly newer replaces; equal keeps the
  cached row; an unparseable incoming version keeps it; a cached row whose own version is
  unparseable is replaced, with the cached-unknown branch firing first, so a pair of
  unparseable versions takes the comparator's branch order and replaces the cached row),
  because `mergeWindowRows` calls the shared
  `isIncomingMessageAtLeastAsFresh` comparator, which Task 01 leaves as the corrected
  three-case rule, so the window merge inherits it rather than restating it. This work order also owns the layer below it: the selected union
  goes through `mergeMessages`' `reconcileMessages`, which reuses the cached row when
  the signatures match, and `signatureOf` currently treats any truthy `updated_at` —
  malformed included — as authoritative, so a changed row whose malformed
  `updated_at` is unchanged would be discarded after the rule selected it. The
  signature therefore needs the content-hash fallback for an unparseable
  `updated_at` (an `updated_at` that `messageTimestampNanoseconds` cannot parse), with a regression
  in `message-signature.test.ts` that changes content while keeping
  the malformed value and asserts the new row survives `reconcileMessages` — the panel
  suites cannot host it, because they stub `mergeMessages`. The merge rule's own
  regressions live in `load-message-window.test.ts` (whose `message()` fixture must gain
  `session_id: SESSION`, because the mandated session filter drops a response row that carries
  none, so its four existing around cases keep their current expectations only once their rows
  identify the session): with an equal `updated_at` and changed
  content, the cached row's CONTENT is the one in the union handed to `mergeMessages` (its
  object identity does not hold, because the merge adopts a `prompt_index` the incoming row
  newly supplies, `{ ...current, prompt_index }`, when the cached row has none) and the same
  suite pins that newly-supplied-ordinal case directly, since `mergeWindowRows` keeps
  `current` whenever the predicate is false and the shared helper is a boolean that cannot
  carry the carve-out; for the both-unparseable pair the cached-unknown branch fires first, so
  the INCOMING row is the one in that union.
  The same direct suite pins the loader's two other terminal kinds, which the
  component suites cannot execute because they mock `loadMessageWindowAround`: a
  held target-containing response whose guard turns false before it resolves
  returns `stale` and merges nothing, and a current response that omits the target
  returns `deleted-target` and merges nothing. The same suite pins the loader's session
  filter: a response carrying a strictly newer user row for another session neither merges
  that row nor passes the target check, so it cannot resolve as this session's last prompt.
  The same suite pins the non-contiguous union `AC-UI-PINNED-PROMPT-AVAILABILITY-001.7` requires: a retained newest window
  and an around response whose spans do not tile the run, asserting the target and
  newest rows both survive, the known middle ids stay absent, and the pagination
  metadata is unchanged.

- The availability decision and the resolution of the last prompt (Task 01).
- The unloaded edge classification (Task 03).
- Anchored-bar copy, expansion, and bounding.

## Acceptance

- Activating the control for an unloaded prompt whose row is absent from
  `messages.bySession` issues exactly one around request for that prompt, shows
  the `transcript-jump-loading` indication while that request is in flight,
  merges the response, places the prompt row at the transcript's start-aligned
  position (viewport top plus that row's `scroll-margin-top`), reasserts once
  250 ms later at most, and clears the local target after the scroll.
- For an unloaded prompt the handler makes no immediate `scrollToMessage` call:
  the activation's counter is exactly the machine's three calls (the pre-merge
  attempt, the post-merge start-aligned scroll, and its single 250 ms reassertion)
  plus one consumption, and the panel suite asserts that count. The handler still
  scrolls immediately for a prompt present in `messages.bySession`.
- The local target never competes with the panel's other target owners: while
  `pendingScrollToMessageId` or `pendingScrollTarget` is active, or while the
  Dockview store holds a live target for this session and panel id, no local target
  is created; a host input or matching Dockview target appearing while one exists
  drops it and clears its indication; and the local attempt path is gated on all
  three being absent, so exactly one owner scrolls and consumes and neither clears
  the other's state. Activation while another owner's jump is in flight is an
  intentional no-op, and the next activation after that target settles performs the
  jump: activating the control while such an owner is live creates no local target
  and issues no around request, and the activation after it clears issues exactly
  one around request and lands the prompt.
- The local target is created only for a prompt the panel resolved as unloaded,
  which already means "absent from `messages.bySession`", so the machine always
  has work to do: the control cannot stall on a jump it refuses to act on, and a
  cached prompt never gains a target or a jump indication.
- The local instance runs while the transcript is on screen: it takes the
  transcript's own visibility flag, so a host that renders a visible transcript
  while passing `isVisible={false}` without a panel id (thread conversation,
  kanban hover preview) completes the jump, and only a hidden dockview tab defers
  and resumes.
- A rejected around request consumes the local target, including while the panel
  is inactive, clears the jump indication, and leaves the control available for
  another attempt. A rejection while the panel is hidden settles the same way: one
  consumption, the target and indication cleared, and reactivation issuing no
  around request.
- The around merge unions the windows without tiling them: with a retained newest
  window and an around response whose spans leave a gap, the target and the newest
  rows both survive, the known middle ids stay absent, and the pagination metadata
  is unchanged (`AC-UI-PINNED-PROMPT-AVAILABILITY-001.7`), asserted directly in
  `load-message-window.test.ts`.
- A merge-time transition settles reachably: if the around merge changes which rows the
  transcript renders, the merge leaves the local target inert rather than settling it (the merged row is in the window,
  so the attempt stops at the machine's loaded guard without consuming), and the bar never stays mounted with a height reservation for a prompt no
  post-filter row represents. A panel regression flips the matching launch-error filter
  during the jump and asserts the observable outcomes: no stale bar or height reservation, no scroll, and no second
  request.
- A jump's identity is frozen at activation: rows the around merge reveals, including a
  user prompt newer than the target that then resolves as the session's last prompt,
  re-target nothing, so the post-merge scroll and the single consumption stay bound to the
  activated prompt and the pin follows the window afterwards. A panel regression asserts
  that sequence (newer row revealed mid-jump: lands on and consumes the activated prompt,
  and the control then reports the newer prompt).
- The around loader keeps only rows whose `session_id` is the session it was asked for,
  before the target check and before the merge, so a foreign row cannot enter the window
  or the prompt fan-out and cannot become this session's last prompt.
- The instance is wired with `messageId: null` so its identity comes from the
  target alone, and it reuses the panel's window-derived `readinessKey` plus
  `isInitialMessagesLoading`, the only inputs that re-run the effect after the
  merge: an id-derived key cannot retrigger it and leaves `completedAround` set
  with no post-merge scroll attempt.
- After the target clears (rejected, deleted, or landed), a later transcript
  revision issues no further around request: the instance is wired with
  `messageId: null`, so no live identity key survives consumption and re-issues the
  request on an effect re-run.
- A target whose around response confirms the prompt no longer exists clears the
  local target without a retry loop — including when that response arrives while
  the panel is inactive, which settles by identity rather than visibility, so
  reactivation issues no second request — and consumes the target so no request
  fires without a new activation. This package mutates no prompt-projection or
  transcript state: prompt-history reconciliation stays event-sourced, so a delete
  the client missed leaves the entry, the control stays available, and the next
  activation re-confirms the deletion with exactly one request. A second activation while the local jump is
  in flight takes a new token that invalidates the first target before its guard
  runs: the first response never merges, scrolls, or consumes; the newest target
  alone settles, with `merged` performing the start-aligned scroll, one reassertion
  and one consumption, and `deleted-target` or a rejection consuming it without
  retry. A
  `merged` response that settles while the panel is inactive is retained as the
  completed window: the flow issues exactly one around request, on the pre-hide
  activation, and reactivation itself yields the two scroll calls (the activation
  attempt and its single 250 ms reassertion) and one consumption, with no second
  request. The mode splits the current single validity predicate: identity
  (mounted plus session, message, and target key) settles the request's success
  and failure paths, the existing visibility guard stays on the scroll attempt and
  the reassertion scheduler, and `completedAround` is retained while hidden only
  for this identity mode — the host mode keeps today's clearing. The settlement
  mode is an option on this instance only: a host-provided pending target and the
  dockview prompt-history path keep their existing visibility-gated settlement,
  asserted in `task-chat-panel.scroll-target.test.tsx`. The local target is
  dropped on session switch and panel unmount, and deferred — not dropped — while
  the panel is inactive, resuming when it becomes visible again, matching the
  reused machine's `!isVisible` path; unmount leaves no target, no pending
  reassertion, and no later around request. Creating it never adds a transcript row or
  changes `oldestCursor`/`hasMore`. The merge keeps the previously loaded newest
  rows (the window is merged per id into the union, never replaced, with the strict
  freshness rule above so an equal-version row keeps the cached content), and the around
  response is the target row plus the rows newer than it, so a window that held no
  user row gains the target — the session's newest stored user prompt, which keeps
  every other user row in that window older — and the scroll-to-start affordance can
  appear; that id can never move to an older row. A
  run longer than the retained newest window plus one around window leaves the rows
  between them unloaded while the transcript renders the union in order. Both
  effects are accepted for a user-initiated jump.
- `AC-UI-PINNED-PROMPT-AVAILABILITY-001.8`'s second half is this work order's: the
  same single handler serves the phone, where the transcript shell mounts the panel
  with no `panelId` and the shared `transcriptIsVisible` stays true, so an unloaded
  prompt is reached on both surfaces. The phone composition itself (no anchored
  bar) is unchanged by this work order.
- A session removed and recreated under the same id invalidates the local jump. The
  target identity and the around guard carry the session generation at creation, so
  the purge's generation bump drops the target, its `transcript-jump-loading`
  indication, its `completedAround`, and any pending reassertion before a merge or a
  settlement, and an in-flight response whose generation no longer matches is
  discarded rather than merged into the new incarnation. Tests cover both orders:
  recreation before the request is issued (no request, no indication) and
  recreation while the request is in flight (no merge, no scroll, no consumption).
  The local target's ordinary lifecycle is asserted too: an ordinary session
  switch and a panel unmount both drop it, with the reassertion timers flushed and
  a late around response settling nothing (no merge, no scroll, no consumption),
  while a merely hidden panel still keeps the target and resumes it. The existing
  `task-chat-panel.scroll-target.test.tsx` unmount cases cover the Dockview host
  owner only, so they cannot stand in for these.

## ASCII UI preview

`UI-01: Desktop transcript, last prompt outside the loaded window` (jump state;
full preview in [plan.md](plan.md#ui-01-desktop-transcript-last-prompt-outside-the-loaded-window)):

```text
+---------------------+--------------------------------------------+----------+
| sidebar             | [chat] [Changes]        [ loading... ]     | <- existing
|                     +--------------------------------------------+    jump
|                     |  last prompt row, aligned at viewport top   |    indicator
|                     |  (loaded on demand by this task)            |
|                     +--------------------------------------------+          |
|                     |  [ composer ]   [^ last prompt]            |          |
+---------------------+--------------------------------------------+----------+
```

Fixed regions: the view-tab row, the sidebar, the right column, and the composer
status bar; the jump indicator is absolutely positioned over the top-right of the
panel body (`task-chat-panel.tsx`), below the view-tab selector, so it does not
scroll with the transcript. Scrolling: the transcript body
(`.chat-message-list`), whose `around` window this task grows and whose `scrollTop`
it sets so the prompt row lands at its `scroll-margin-top` offset. The prompt row
shown here is loaded by this task, not new UI.

Phone (`UI-02`; the phone keeps one inline composition, so the phone shows this
task's loading indication and the same landed row with no anchored bar):

```text
+----------------------------------+
| < Task            [ loading... ] |  fixed: jump indication
+----------------------------------+
| ^ last prompt row                |  scrolling: .chat-message-list
|   (loaded on demand)             |  (lands at scroll-margin-top)
|                                  |
+----------------------------------+
| (no anchored bar on phone)       |  fixed: composer row
| [ composer ]   [^ last prompt]   |  <- this task's control, mid-jump
+----------------------------------+
```

ACs: `AC-UI-PINNED-PROMPT-AVAILABILITY-001.2` (activation loads and aligns) and
`.8` (the phone reaches the same value). Targeted rendered check: Task 04
asserts the landed row's scroll-margin-adjusted alignment on desktop and on the
phone project.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/task-chat-panel.last-prompt.test.tsx components/task/task-chat-panel.scroll-target.test.tsx hooks/domains/session/load-message-window.test.ts lib/state/slices/session/message-signature.test.ts lib/state/slices/session/session-slice.merge-messages.test.ts lib/state/dockview-panel-actions.prompt-history-panel.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/task-chat-panel.tsx components/task/task-chat-panel.last-prompt.test.tsx components/task/task-chat-panel.scroll-target.test.tsx hooks/domains/session/load-message-window.ts lib/state/slices/session/message-signature.ts)
```

## Files likely touched

- `apps/web/hooks/domains/session/load-message-window.ts`
- `apps/web/hooks/domains/session/load-message-window.test.ts`
- `apps/web/lib/state/slices/session/message-signature.ts`
- `apps/web/lib/state/slices/session/message-signature.test.ts`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/task-chat-panel.last-prompt.test.tsx` (new)
- `apps/web/components/task/task-chat-panel.scroll-target.test.tsx`

## Dependencies

- Task 01 supplies the resolved last prompt the control acts on.
- Task 01 depends on this work order in release terms: it offers the control for
  an unloaded prompt before this path exists, so the two land atomically. Neither
  should be merged or shipped without the other.

## Risks

- The target's identity must include the session generation: the panel's existing
  guard compares mounted state, session, message, and target key, and the session
  term is the id alone, so a session removed and recreated under the same id passes
  every term. Carry the generation that the purge bumps (`sessionId\u0000generation`
  is already how `requestPromptMessages` isolates a recreated session's read) and
  drop the target, indication, `completedAround`, and reassertion when it changes,
  or an old target or an in-flight response can load or settle into the new
  incarnation.
- Two `usePendingMessageScroll` instances plus the Dockview consumption path must
  not fight: the local target must never be created while a host input or a live
  Dockview target for this panel is active, must be dropped with its indication
  when one appears, and its attempt path must stay gated on all three, so the other
  owners keep precedence and no instance clears another's state. Regressions render
  a host input and a Dockview target against a local target, and
  `dockview-panel-actions.prompt-history-panel.test.ts` stays in the run because
  the store target is one of the three owners.
- Around settlement for the local instance must not be gated on visibility: today
  `isCurrentTarget()` includes the panel's visibility, so a `deleted-target` (or
  `merged`) response arriving while the panel is hidden is treated as stale, the
  target survives, and activation re-requests the window. Settle this instance on
  mounted plus session/message identity, keep visibility on the scroll attempt
  alone, and cover both hidden cases in the component test. Make it an option
  rather than a shared-default change: `scroll-target.test.tsx` and the dockview
  prompt-history flow pin the existing visibility-gated deferral for host-owned
  targets, and `mobile-prompt-history-panel.spec.ts` leaves and returns to Chat.
- A deleted-prompt response must consume the target, rather than retrying the
  around request on every render.
- The availability flag and the machine's loaded check must stay the same
  condition (`messages.bySession` membership): the panel only creates the target
  for a prompt it resolved as unloaded, or a cached prompt leaves a target that
  never produces a scroll, an around request, or an indicator.
- The local instance must take `transcriptIsVisible` (`panelId === null ||
  isVisible`), not the raw `isVisible` prop: hosts such as `ThreadConversation`
  and the kanban hover preview mount the panel with `isVisible={false}` and no
  panel id while their transcript is rendered and clickable, and the raw prop
  would defer their jump permanently. A component test covers one such host.
- The local target must be created in the shared `scrollToLastPrompt` handler, not
  in the status-bar button's own closure: the anchored bar's button calls the same
  callback, and in the `above` state it is the prominent affordance, so a
  status-bar-only path would leave the bar's button scrolling nothing.
- Panel inactivity must not be implemented as a target drop: the reused machine
  cancels its reassertion while hidden and resumes the same target on
  `isVisible`, and asserting a drop would require a new panel effect that
  contradicts the shared defer semantics.
- The row's start alignment is the scroll-margin-adjusted position the transcript
  already uses; asserting a literal viewport-top alignment in this work order's
  tests would contradict `resolveLastPromptEdge`'s sibling behavior for a loaded
  prompt.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/system-design/pinned-prompt-availability.md` sections
  "Unloaded prompt navigation" and "Failure and recovery".
- Existing patterns: the `usePendingMessageScroll` /
  `requestMessageWindowAround` path in
  `components/task/task-chat-panel.tsx`, `hooks/domains/session/load-message-window.ts`.

## Results

Verified: unloaded-target, window-merge, and panel tests passed in the 23-file
focused suite (360 tests total); web typecheck passed.
