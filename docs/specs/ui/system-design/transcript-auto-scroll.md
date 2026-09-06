---
status: current
system: ui
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
created: 2026-08-27
updated: 2026-09-04
owners:
  - kandev
---

# Transcript Auto-scroll Stability System Design

## Purpose and boundaries

This design keeps an enabled transcript pinned during streamed updates and
restores that bottom-follow intent when a persistent desktop session panel
becomes visible or a task switch rebuilds Dockview for another environment. It
does not force browser layout during each React commit. It preserves
disabled-state freezing, reader-owned positions, explicit navigation, prepend
restoration, and catch-up after the user enables auto-scroll again.

## Requirement mapping

| Requirement                         | Design section                                                                                                                                                                                                                                                                                        |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-TRANSCRIPT-AUTO-SCROLL-001` | [Bottom placement](#bottom-placement), [Persistent-panel visibility lifecycle](#persistent-panel-visibility-lifecycle), [Environment-change rebuild lifecycle](#environment-change-rebuild-lifecycle), [Interaction boundaries](#interaction-boundaries), [Responsive behavior](#responsive-behavior) |

## Bottom placement

`message-list-native-scroll.ts` owns one helper that places the native scroll
container at its maximum vertical offset with a write-only scroll command. The
common message-update and work-start paths call this helper. They do not read
`scrollHeight`, `clientHeight`, a bounding rectangle, or computed style before
the write.

The helper writes `2_147_483_647`, which is the largest signed 32-bit integer.
Chromium, Gecko, and WebKit clamp this positive offset to the current maximum.
The helper must not write `Number.MAX_SAFE_INTEGER`. WebKit resolves that value
to the top of the scroll container.

The existing near-bottom reference remains the decision input. The helper does
not add a second animation frame, smooth scrolling, or a resize observer.

Tests instrument the common append path. A `scrollHeight` getter fails the test
if that path reads it. The scroll setter records the WebKit-safe offset.

## Persistent-panel visibility lifecycle

Desktop Dockview renders panel content through persistent portals. An inactive
session transcript can therefore be mounted and receive messages while its
scroll container has zero or detached geometry. `usePanelActive` supplies the
authoritative active-tab signal to `TaskChatPanel`, which passes it to the
native transcript scroll coordinator as `isVisible`.

Geometry-based initialization and divider placement do not consume their
one-time completion state while `isVisible` is false. When a transcript first
becomes visible after an inactive mount, its placement runs after the generic
`SessionPanelContent` absolute-offset restore. The coordinator uses two
animation frames because that generic restore uses one frame after its
`ResizeObserver` reports non-zero geometry.

The unread-divider settling deadline starts when the transcript is first
visible and resets for each hidden-to-visible activation. Time spent in an
inactive persistent portal does not consume the four-second reassertion window.
The deadline remains fixed during that visit, so later layout changes can
reconcile the divider only while the activation is still settling.

`SessionPanelContent` remains a generic panel component. Its queued restore
captures the element's `scrollTop` when scheduled and applies the saved offset
only if that value is unchanged when the frame runs. This cooperative stale-
write guard lets a newer transcript or user scroll remain the active owner.

The post-restore placement follows this ownership order:

1. A pending Dockview layout restore or explicit message-navigation target
   retains ownership and suppresses automatic placement.
2. A visit-scoped unread divider is placed at its target and marks the reader
   away from the bottom.
3. A disabled auto-scroll preference or an enabled reader who previously moved
   away from the bottom keeps the saved absolute offset.
4. An enabled reader with bottom-follow intent receives the shared write-only
   bottom placement.

Hidden message and work-state updates do not write to a zero-geometry scroll
container and do not recompute near-bottom state from hidden geometry. If the
reader had bottom-follow intent before the panel became inactive, activation
performs one guarded catch-up after the panel restore. If the reader had moved
away from the bottom, activation leaves the generic absolute-offset restore in
place.

The visibility lifecycle uses component refs only. It does not add persisted
scroll state or change the existing per-session disabled-position storage.

## Environment-change rebuild lifecycle

`buildEnvSwitchAction` distinguishes an environment-changing task switch from
same-environment session activation before it mutates Dockview. A cross-
environment switch arms a tokened initial-placement request for the incoming
session. The request is independent of `pendingChatScrollTop`: the latter keeps
owning same-transcript layout rebuilds, while the environment-switch request
must never capture or replay the outgoing session's absolute offset.

The incoming native transcript treats a matching request as a reactivation even
when its panel remains logically visible through the rebuild. The request owns
two bounded placement phases whenever cached rows are already available and no
unread-divider target owns entry:

1. After Dockview restoration makes the panel measurable, provisional
   placement resolves against the incoming session's cached rows. Enabled
   auto-scroll selects the newest cached message; disabled auto-scroll selects
   the incoming session's saved offset. This phase does not clear the request.
2. After the latest-window refresh settles, final placement resolves against
   the reconciled rows and clears the request conditionally.

Both phases use `scheduleAfterPanelRestore` and re-check the live session,
visibility, and competing owners before writing. This prevents cached content
from appearing at the browser's default `scrollTop = 0` while preserving a
final correction when the refresh changes the current window. A transcript
without cached rows keeps the existing loading presentation and performs only
the final phase. An active unread-divider target keeps its existing
visit-scoped placement and does not receive the provisional bottom or saved-
offset write.

Each placement resolves ownership in this order:

1. A pending explicit navigation target or same-transcript layout restore keeps
   priority.
2. Disabled auto-scroll restores
   `transcriptAutoScroll.scrollTopBySessionId[sessionId]`, with
   `getStoredAutoScrollTop(sessionId)` as its storage fallback.
3. Enabled auto-scroll places the transcript at its current native maximum.

Final completion conditionally clears the same token so a superseded switch
cannot consume a newer request. While a matching request remains pending, the
older-history intersection sentinel is blocked. Releasing that block is an
eligibility transition, not proof that the sentinel remains at the top. Before
replaying an intersection observed while blocked, `useLazyLoadSentinel` invokes
the consumer's current-geometry predicate. The transcript starts pagination
only if the current sentinel and scroll root remain inside the preload region.
The same current-geometry rule applies when a stale request hands an observed
intersection to a replacement view.

Same-environment session switches return before arming this lifecycle.
Maximize, un-maximize, preset, and custom-layout rebuilds retain the existing
`pendingChatScrollTop` path and do not request session initialization.

## Interaction boundaries

The optimization does not replace geometry reads that answer a user-action
question. Scroll events can still read geometry to decide whether the reader
is near the bottom. Re-enabling auto-scroll can still compare the viewport with
content to decide whether catch-up is necessary. Pagination and prepend
restoration retain their existing measurements.

The write-only helper runs only when all existing guards allow automatic
placement:

- auto-scroll is enabled.
- The transcript is visible, or a visibility-activation catch-up is running
  after panel restoration.
- The reader is near the bottom.
- No user programmatic scroll lock is active.
- No layout restoration is pending.
- No environment-switch initial placement is pending for the session, except
  when that matching request is executing its own provisional or final write.

Disabled auto-scroll continues to restore the captured `scrollTop`. New
content cannot move that frozen position through application code or browser
overflow anchoring.

## Responsive behavior

Desktop and mobile use the same native transcript, store state, and bottom
placement helper. The transcript remains the only vertical scroll owner. No
control, copy, layout, or touch target changes.

The inactive persistent-portal state exists only in the desktop Dockview
workbench. Mobile renders one selected `TaskChatPanel` and replaces its session
input instead of keeping inactive session panels mounted. Mobile therefore
keeps `isVisible` true and retains its initial, enabled, and disabled scroll
behavior through the shared coordinator. A mobile task switch with cached rows
must also place the incoming transcript before a background refresh can expose
the browser-default top, but it does not consume Dockview's environment-switch
placement request.

The nearest mobile exemplar is the current full-height task Chat tab selected
by `TaskLayout` in `apps/web/components/task/task-layout.tsx`. The transcript is
the only vertical scroll region; the task header and composer retain their
existing safe-area handling. Desktop and mobile Playwright scenarios must prove
immediate cached placement, final refresh reconciliation, enabled bottom
pinning, and disabled position restoration.

## Failure modes

- If the container is not mounted, the helper performs no work.
- If a transcript is mounted but inactive, one-time placement remains pending
  until the panel becomes active and measurable.
- If a panel becomes inactive again before the post-restore frames complete,
  the scheduled placement is cancelled.
- Chromium, Gecko, and WebKit clamp the signed 32-bit maximum to the native
  maximum scroll position.
- An offset outside WebKit's safe range can resolve to zero and move the
  transcript to the top.
- If a layout restore or explicit navigation owns the scroll, the existing
  guards suppress bottom placement until that owner releases control.
- If a newer environment switch supersedes a pending placement, the older
  token cannot clear or position the newer session.
- If the latest-window refresh is slow, cached rows remain at their provisional
  position and automatic older-history pagination stays blocked.
- If the incoming history refresh fails, the request resolves against the
  session history that remains available; it does not fall back to the outgoing
  session's offset.

## Observability

Debug logging records provisional placement, final placement, and request
completion with the session identifier, placement token, selected owner, and
scroll geometry. Pagination start logging retains its trigger and geometry so
diagnostics can distinguish a real top reach from a rejected stale
intersection. Message content and saved offsets are not logged.

## Related decisions

- [Isolate Replaceable Session Stream Traffic](../../../decisions/2026-08-02-isolate-replaceable-session-stream-traffic.md)
