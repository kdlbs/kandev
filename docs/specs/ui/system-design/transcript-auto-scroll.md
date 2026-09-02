---
status: current
system: ui
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
created: 2026-08-27
updated: 2026-09-02
owners:
  - kandev
---

# Transcript Auto-scroll Stability System Design

## Purpose and boundaries

This design keeps an enabled transcript pinned during streamed updates and
restores that bottom-follow intent when a persistent desktop session panel
becomes visible. It does not force browser layout during each React commit. It
preserves disabled-state freezing, reader-owned positions, explicit navigation,
prepend restoration, and catch-up after the user enables auto-scroll again.

## Requirement mapping

| Requirement                         | Design section                                                                                                                        |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-TRANSCRIPT-AUTO-SCROLL-001` | [Bottom placement](#bottom-placement), [Persistent-panel visibility lifecycle](#persistent-panel-visibility-lifecycle), [Interaction boundaries](#interaction-boundaries), [Responsive behavior](#responsive-behavior) |

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
keeps `isVisible` true and retains its current initial, enabled, and disabled
scroll behavior through the shared coordinator.

The nearest mobile exemplar is the current task Chat tab in
`apps/web/components/task/task-layout.tsx`. Existing desktop and mobile
auto-scroll Playwright scenarios remain the browser contract. Both viewports
must prove enabled bottom pinning and disabled position freezing after the
change.

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

## Related decisions

- [Isolate Replaceable Session Stream Traffic](../../../decisions/2026-08-02-isolate-replaceable-session-stream-traffic.md)
