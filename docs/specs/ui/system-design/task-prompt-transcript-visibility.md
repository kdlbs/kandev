---
status: current
system: ui
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
created: 2026-08-24
owners:
  - kandev
---

# Task transcript history visibility system design

## Purpose and boundaries

This design defines how the task transcript distinguishes a bounded newest
window from the visible conversation start. It keeps task opening bounded,
loads older history only after upward navigation, and preserves prompt `#1` as
the visible start even when older internal rows remain on the backend.

The design changes no backend query, persistence rule, message order, or API
field. It keeps transcript pagination separate from Prompt History pagination:
Prompt History can request user messages with the `author_type=user` filter,
while transcript navigation can request an around window for a message that is
not loaded. Both projections use the existing cursor metadata, message API,
and `prompt_index` field.

## Requirement mapping

| Requirement                                    | Design sections                                                                                                                                                                                             |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001` | [History state](#history-state), [Opening boundary](#opening-boundary), [Upward pagination](#upward-pagination), [Failure and recovery](#failure-and-recovery), [Responsive behavior](#responsive-behavior) |

## Components and responsibilities

- The session message fetch owns one bounded newest-window request and records
  when authoritative pagination metadata has initialized.
- `messages.metaBySession[sessionId]` owns raw `hasMore`, `oldestCursor`,
  initial/refetch loading, older-page loading, and history initialization.
- `useSessionMessages` exposes the raw history state to chat composition.
- `useProcessedMessages` filters and groups stored messages. It may synthesize
  the task-description fallback only after the session start is known.
- `useLazyLoadMessages` owns the prompt-`#1` visible boundary and older-page
  requests. It also owns opt-in per-consumer accumulation targets and keeps raw
  pagination available to explicit recovery consumers.
- The native transcript consumes `useLazyLoadMessages`. Prompt History consumes
  its own user-message request and pagination state.
- `TaskChatPanel` supplies the native transcript with the host's real visibility
  state. Dockview hosts use panel activation; mobile and other direct hosts use
  their existing mounted visibility.
- The native transcript owns upward intent, committed sentinel geometry, scroll
  restoration, host-visibility recovery, and retry-control visibility.
- `SessionPanelContent` restores the saved scroll offset after a hidden panel
  receives non-zero geometry. The native transcript waits for that restore to
  commit before it asks the sentinel to re-evaluate the viewport.
- `useLazyLoadSentinel` owns observer lifecycle, current-geometry eligibility,
  request serialization, and guarded retries from lifecycle or user intent.
- Transcript navigation first uses loaded messages. When a selected prompt is
  absent, it requests an around window and merges the result before scrolling.
- `requestOlderMessages` keeps transcript request coordination and raw cursor
  metadata unchanged. Prompt History has a separate request coordinator and
  cursor metadata.

## History state

The frontend distinguishes these facts:

- **History initialized:** An authoritative newest-window response or boot
  payload supplied pagination metadata for the session.
- **Raw history remains:** The backend reported `has_more: true`.
- **Visible start loaded:** A loaded user message has `prompt_index === 1`.
- **History exhausted:** History is initialized and raw history does not remain.

Default store values are not proof that history is exhausted. The session
message metadata therefore records initialization separately from `hasMore`.
Session deletion purges this state with the rest of the session transcript.

The task-description fallback is eligible only when all of these conditions
are true:

1. History is initialized.
2. Raw history is exhausted.
3. No visible stored user message exists.
4. The task description is non-empty.

A tool-only newest window with `has_more: true` does not satisfy this contract.
It renders only the stored rows in that window until the user navigates upward.

## Opening boundary

The session message fetch requests only the newest bounded window when a task
opens. It does not paginate backward to locate a prompt. A live WebSocket row
can join that contiguous newest window through the existing reconciliation
rules.

The latest-window reconciliation contract remains unchanged:

- An overlapping fetched suffix joins the cached contiguous window.
- A disjoint cached prefix is removed before the fetched suffix becomes the
  new authoritative window.
- The oldest cursor always names the oldest row in the contiguous window.

The absence of a user row in the newest window neither starts background
pagination nor authorizes the task-description fallback.

## Visible pagination boundary

Transcript pagination remains available while raw history remains and loaded
messages do not contain user prompt `#1`. Loading prompt `#1` immediately
removes the transcript sentinel and retry control, even if raw backend history
contains internal rows before that prompt.

Payloads without prompt ordinals use raw exhaustion as the compatibility
boundary. Session search can continue through the explicit raw loader when a
backend search hit precedes the visible transcript start.

## Text-aware load batches

The backend pagination contract remains cursor-based and counts every persisted
row. Each request still asks for 20 raw rows and prepends every returned row so
tool activity remains available in the transcript.

The native transcript opts into a 20-text-part target on
`useLazyLoadMessages`. One `loadMore` invocation snapshots the loaded text-part
count, requests consecutive raw pages, and returns only after the delta reaches
the target or a normal stop condition applies. A text part has type `message`
or `content`; a missing type counts for legacy compatibility. The predicate
does not count tool, thinking, progress, status, or other activity rows, and it
does not discard those rows from the store or renderer.

The accumulation loop preserves the existing bounded pages-per-load safeguard.
Prompt `#1`, raw exhaustion, a zero-result page, or the safeguard can end a
batch before 20 text parts arrive. The returned progress count remains the
total number of raw rows prepended so the sentinel's no-progress recovery
contract is unchanged.

Prompt History continues to use its separate user-prompt target. Raw search,
drain, and backfill consumers retain their existing page semantics; text-aware
batching is opt-in only for the native transcript.

## Upward pagination

The transcript starts a load cycle when the user reaches the oldest loaded
edge. Before each request it captures a stable rendered-row anchor. After the
page commits, scroll restoration keeps that anchor at the same visual position.

A hidden Dockview panel can keep its transcript mounted while its scroll root
has zero geometry. When that panel becomes visible, the native transcript
waits until `SessionPanelContent` has restored the saved scroll offset, then
performs one current-geometry eligibility check. If the restored sentinel is
inside the preload region and pagination is otherwise eligible, that check
starts or resumes the normal load cycle. It does not use an intersection entry
captured while the panel was hidden.

An upward wheel, touch, keyboard, or scrollbar action at the hard top can
produce no `scroll` event because `scrollTop` cannot decrease below zero. The
native transcript therefore treats directional input at that boundary as
fresh pagination intent and checks current sentinel geometry. Input and panel
activation use the same request guards and cannot bypass loading, visible-start,
failure-recovery, or session-epoch stops.

Continuation uses committed sentinel geometry, not the identity of the oldest
standalone row:

1. Re-evaluate the sentinel against the transcript's preload region after
   React layout and scroll restoration commit.
2. Continue when the sentinel remains in that region and visible pagination
   still has more history.
3. Stop when inserted content moves the sentinel outside the region. A later
   upward reach starts the next cycle through the normal observer path.
4. Stop at prompt `#1`, raw exhaustion, a stale session, or no progress.

This rule crosses both collapsed activity pages and short standalone pages. It
also avoids cascading through content that has already moved above the user's
current preload region. A stale IntersectionObserver entry is not sufficient
to continue; the decision uses geometry from the current sentinel and current
scroll root after commit.

## Failure and recovery

A rejected request or a zero-row response with visible history still remaining
sets a transcript-local recovery state. While that state is active, the normal
automatic cycle is disarmed and the explicit older-page retry control is
visible. A successful retry clears recovery state and resumes the geometry
rule. Session changes, prompt `#1`, and raw exhaustion also clear it.

Panel activation does not retry a request that already entered recovery state.
The lifecycle check repairs a missed observer transition only; request failure
and no-progress outcomes still require the explicit retry control. Repeated
active renders and resize notifications do not create additional pagination
intent.

Routine successful pagination does not render the retry control. The control
keeps the existing localized label and has a coarse-pointer hit area of at
least 44 pixels without increasing fine-pointer desktop density.

## Observability

The `messages:pagination` namespace records page start and settle events
without message content. Settle events include the session, trigger, count,
raw and visible boundaries, committed sentinel geometry, continuation, and
stop reason. Recovery entry and exit use distinct reasons so a real request
failure is distinguishable from leaving the preload region.

## Responsive behavior

Desktop and mobile continue to use the existing full-height task Chat surface,
shared store, shared hooks, and one vertical transcript scroll owner. Prompt
History uses the same row selection callback on both surfaces. A selected
prompt returns the phone to Chat and uses the around-window path when the row is
not loaded. The recovery button uses the same placement on both surfaces with a
coarse-pointer touch target.

Dockview activation recovery is desktop-specific because inactive desktop tabs
remain mounted with zero geometry. The eligibility and hard-top gesture rules
remain in the shared native transcript path. Mobile has no hidden Dockview tab,
so it keeps mounted visibility and must retain the same upward pagination,
anchor, and failure-recovery behavior.

## Around-window navigation

The chat panel sends `GET /api/v1/task-sessions/:id/messages?around=<message-id>&sort=desc&limit=N` after it confirms that the target belongs to the active session. The backend returns the target and newer rows in newest-first order. The chat panel merges the response into `messages.bySession`, waits for the target row to render, and then scrolls to it. The target token is session-scoped and is cleared only after a successful scroll or a confirmed missing target. A stale response cannot consume a newer selection.

The nearest mobile exemplar is `apps/web/components/task/task-layout.tsx` and
the existing `mobile-message-pagination.spec.ts` flow. The mobile scenario
must prove the same continuous loading, true-start fallback, recovery, and
anchor behavior as desktop.

## Test boundaries

- Processed-message tests cover an uninitialized window, a tool-only window
  with older history, exhausted legacy history, an empty description, and a
  loaded stored prompt.
- Session-state tests cover initialization through boot hydration, newest
  fetch, purge, and refetch without inferring exhaustion from defaults.
- Sentinel tests cover committed in-region continuation, out-of-region stop,
  stale observer entries, one-shot lifecycle rechecks, hard-top intent,
  no-progress recovery, successful retry, and session reset.
- Native transcript tests cover hidden-to-visible recovery, recovery-control
  visibility, and prepend anchor preservation.
- Lazy-load hook tests cover mixed `message`, `content`, legacy, and tool rows,
  the 20-text-part target, every early stop, and unchanged raw/single-page
  consumers.
- Desktop and mobile browser tests seed more than twenty older pages between
  the newest window and the latest user prompt. They prove that no synthetic
  initial prompt or routine retry control appears and that one upward
  navigation reaches the stored prompt without a click.
- Desktop and mobile browser tests also place standalone tool activity ahead of
  older text so the first raw page leaves the preload region. One upward reach
  must still cross that page and reveal the targeted older text batch.
- Existing prompt-`#1`, hidden pre-prompt, and inactive-session reconciliation
  scenarios remain covered.
- A desktop browser test restores two session tabs with the target tab inactive,
  suppresses the fresh observer entry that can be missed during restoration,
  activates the target, supplies upward input at the hard top, and proves that
  the next request carries an older-page cursor instead of repeating only the
  newest 100-row window.
