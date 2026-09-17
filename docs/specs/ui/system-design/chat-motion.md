---
status: current
system: ui
requirements:
  - REQ-UI-CHAT-MOTION-001
  - REQ-UI-CHAT-MOTION-002
  - REQ-UI-CHAT-MOTION-003
---

# Chat Motion System Design

## Boundary and mapping

| Requirement | Design section |
| --- | --- |
| REQ-UI-CHAT-MOTION-001 | Live content rendering |
| REQ-UI-CHAT-MOTION-002 | Preference and effective motion |
| REQ-UI-CHAT-MOTION-003 | Scroll integration |

UI owns presentation only. Reuse the existing native transcript; do not replace
its placement engine or broaden the shared Markdown renderer's default behavior.
See [requirements](../requirements/chat-motion.md),
[auto-scroll design](transcript-auto-scroll.md), and
[implementation plan](../../../plans/chat-motion/plan.md).

## Preference and effective motion

Preserve [ADR 0046](../../../decisions/0046-settings-route-save-coordinator.md)
and the existing route save coordinator; the row joins Appearance's contributor.

Follow `lib/settings/rich-output-motion.ts` and
`lib/state/slices/ui/rich-output-motion-actions.ts`, using a separate proposed
`chatMotion` state with `enabled` and `savedEnabled`. Add a typed local-storage
key `kandev.settings.chatAnimations`, default true, accepting booleans only.
Use the existing storage wrapper's fallback on unavailable/corrupt storage.
No backend schema, account PATCH field, or runtime registry entry is required.

Extend `AppearanceState`, saved-state construction, patch/rebase/signature
helpers, and `general-settings.tsx` preview/commit/restore handling. Preserve
in-flight-save rebasing and the existing account-save failure behavior before
committing local values. Wire slice actions through `ui-slice.ts`, UI types,
and `app-state-types.ts`. Register the row in settings discovery.

A proposed `use-chat-motion` hook combines the current preference with a live
`prefers-reduced-motion: reduce` subscription. The effective value is false
until client preference/media initialization is known, preventing a first-paint
flash for users who opted out. Keep CSS reduced-motion suppression as a second
layer. A preference/media change cancels effects and reveals text immediately;
re-enabling only affects future arrivals. Do not persist OS-derived values.

Place “Chat animations” beside the rich-output animation control. Suggested
localized description: “Animate incoming text, new chat items, and scrolling
on this device. Respects reduced motion.” Include all five supported catalogs,
generate Traditional Chinese with the repository converter, and refresh pseudo.

## Live content rendering

`message-list-native.tsx` and `MessageItem` in `message-list-shared.tsx` supply
live-arrival eligibility using stable `getItemKey` identities. Track a baseline
only after initial history/refresh settles. Reset on session change. Pagination,
refetch and hidden-panel updates advance the baseline without animating.
A new child within an existing grouped tool/turn item needs its own stable
message identity: group-key stability must not suppress that child's entrance.
Keep bookkeeping bounded to the loaded transcript, clearing removed identities.

Add a chat-scoped presentation context for eligibility and effective motion.
Enter eligible row content with opacity 0 to 1 and translateY 3 px to 0 over
160 ms, ease-out, without stagger backlog. Animate an inner presentation wrapper,
not the measured/anchored row. Avoid double animation of a parent group and child.
No height, margin, blur, bounce, or exit animation. Stable keys prevent replay.

`messages/agent-message-content.tsx` and `thinking-message.tsx` opt into an
optional chat-only text transform in `MemoizedMarkdown`; all other callers keep
its current behavior. Transform Markdown text nodes before React renders them,
using source positions in the same normalized string that is parsed, stable
message identity, and the previous normalized content to identify append-only
suffixes. Split at text-node boundaries, never raw Markdown tokens. Exclude code,
preformatted text and custom structured renderers. If source positions are
missing or an update rewrites prior content, render the affected text immediately
instead of guessing a boundary or replaying the paragraph.

Animate only newly appended prose runs, opacity 0.6 to 1 over 140 ms. Publish
all text in the same commit; no per-character spans or artificial streaming
buffer. Keep previous runs' identities stable while active and compact settled
runs without replay. While text is selected, retain existing spans and render
additional text statically; compact after the selection is released. Coalesce rapid arrivals to at most one new run per rendered frame. Preserve Unicode, links, inline emphasis, whitespace, and textContent.
Changing Markdown structure can reconcile nodes; source rewrites use the static
fallback. Do not mutate React-owned DOM or add live-region copies. Existing
comment-selection tests must prove span boundaries do not change offsets. Only
marked motion spans use the motion renderer; unmarked spans retain the caller
renderer.

## Scroll integration

`useAutoScroll` and its helpers in `message-list-native-scroll.ts` remain the
policy authority. Add one small cancelable follow driver per visible native
scroll container. Existing allowed live-follow calls request a target instead
of synchronously writing the bottom when motion is effective. Frame callbacks
read current geometry, then perform one scroll write, easing toward a moving
bottom target and landing exactly within 300 ms of the last growth. New content
updates the target of the active driver, never queues another animation. Retargeting
preserves elapsed frame time so continuous growth cannot stall scroll progress.
Use existing resize notifications for late content growth. Keep synchronous
content-size reads out of message commits (the existing stability invariant).

Follow intent is distinct from instantaneous distance to bottom while the driver
runs: its own intermediate scroll events must not incorrectly clear intent.
Actual wheel/touch/keyboard/scrollbar input cancels it and returns control to
existing near-bottom logic. Ordinary content clicks do not clear follow intent;
pointer presses interrupt only in the native scrollbar gutter. Keys handled by
interactive or editable descendants do not cancel following. Explicit scroll-to-message navigation cancels the
follow driver and retains `useProgrammaticScrollGuard`. Thread effective motion
through `handleScrollToMessage`, transcript search, and chat controls in
`simple/task-chat.tsx`, avoiding application-wide `scroll-behavior: smooth`.
Search highlighting uses the row returned by its panel-scoped navigation owner,
so duplicate message IDs in another mounted panel cannot receive the flash.

Initial placement, pagination compensation, unread positioning, and Dockview
restoration continue using immediate writes. Retain native scroll anchoring while
history is loading or refreshing; suppress it only for eligible live motion so
cached-history layout changes retain their existing placement behavior. They cancel any active driver
before applying their position. Auto-scroll disabled, panel hidden/unmounted,
session switched, and programmatic locks also cancel it. Disabling motion while
following snaps to the latest target only if existing follow policy still allows
it; otherwise it cancels without moving the viewport. Existing work-start policy
continues to decide whether a new turn requests bottom placement.

## Phone composition and accessibility

Reuse the shipped Appearance settings surface for the switch and the
full-height session chat layout for transcript motion. The curated mobile
language's dense-viewer pattern applies: Chat remains the single internal
vertical scroll owner, composer remains outside it, dynamic viewport and safe
areas remain intact. Settings retain their own scrolling page and Save/Reset
controls. This frequent reading surface needs no new drawer or navigation step.
Share preference and motion logic across viewports; use wrapped help text and
44 px coarse-pointer setting hit areas while retaining desktop density.
Touch scrolling and keyboard appearance must cancel or resize following through
the existing viewport policy. Motion wrappers remain pointer-inert and do not
change focus order, announcements, hit testing, or overflow geometry.

## Verification and failure behavior

Unit tests cover default/storage, draft save/rebase/cancel, live reduced-motion,
append classification, grouped rows, Markdown rewrites, Unicode/selection, and
frame-driver cancellation/retargeting with a deterministic frame clock. Browser
tests assert actual intermediate opacity/scroll positions and final content,
not just CSS classes. Cover sustained burst streams, long histories, user scroll
interruption, hidden tabs, session changes, and disabled motion on desktop/phone.
Record a rendered desktop/phone check and a sustained-stream trace: no idle frame
loop, no accumulating animations/spans, no new synchronous layout in commit path.

If an animation API or text-position mapping is unavailable, content renders
statically. Use existing logging/debug facilities only; no new telemetry or
security boundary is introduced. This local preference follows an established
pattern and does not need a new ADR.
