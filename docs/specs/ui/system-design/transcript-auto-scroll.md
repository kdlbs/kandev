---
status: current
system: ui
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
created: 2026-08-27
updated: 2026-08-29
owners:
  - kandev
---

# Transcript Auto-scroll Stability System Design

## Purpose and boundaries

This design keeps an enabled transcript pinned during streamed updates without
forcing browser layout during each React commit. It preserves disabled-state
freezing, explicit navigation, prepend restoration, and catch-up after the user
enables auto-scroll again.

## Requirement mapping

| Requirement                         | Design section                                                                                                                        |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-TRANSCRIPT-AUTO-SCROLL-001` | [Bottom placement](#bottom-placement), [Interaction boundaries](#interaction-boundaries), [Responsive behavior](#responsive-behavior) |

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

## Interaction boundaries

The optimization does not replace geometry reads that answer a user-action
question. Scroll events can still read geometry to decide whether the reader
is near the bottom. Re-enabling auto-scroll can still compare the viewport with
content to decide whether catch-up is necessary. Pagination and prepend
restoration retain their existing measurements.

The write-only helper runs only when all existing guards allow automatic
placement:

- auto-scroll is enabled.
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

The nearest mobile exemplar is the current task Chat tab in
`apps/web/components/task/task-layout.tsx`. Existing desktop and mobile
auto-scroll Playwright scenarios remain the browser contract. Both viewports
must prove enabled bottom pinning and disabled position freezing after the
change.

## Failure modes

- If the container is not mounted, the helper performs no work.
- Chromium, Gecko, and WebKit clamp the signed 32-bit maximum to the native
  maximum scroll position.
- An offset outside WebKit's safe range can resolve to zero and move the
  transcript to the top.
- If a layout restore or explicit navigation owns the scroll, the existing
  guards suppress bottom placement until that owner releases control.

## Related decisions

- [Isolate Replaceable Session Stream Traffic](../../../decisions/2026-08-02-isolate-replaceable-session-stream-traffic.md)
