---
spec: docs/specs/ui/requirements/prompt-history-panel.md
created: 2026-08-20
status: done
---

# Implementation Plan: Prompt-history loading message without flicker

## Overview

Make the prompt-history panel's "Loading older messages..." indicator flicker-free
and placement-aware. When the prompt rows overflow the panel (the panel
scrolls) it floats at the bottom of the panel, out of the content flow; when
the rows fit, it sits in the content flow directly under the last message. In
both modes the indicator can never reflow the rows or move the sentinel, and a
short minimum-display grace keeps it mounted across consecutive auto-loads so
it never flashes per page. The indicator keeps its `prompt-history-loading-older`
test id and the `task:loadingOlderMessages` copy. Sentinel, pagination, and
store logic are untouched.

## Confirmed root cause

`apps/web/components/task/prompt-history-panel-content.tsx` renders the
loading-older row **in-flow** as the last child of the scrollable `PanelRoot`,
immediately above the sentinel (`data-testid="prompt-history-sentinel"`):

```tsx
{isLoadingMore && (
  <div data-testid={LOADING_OLDER_TEST_ID} className="py-2 text-center text-xs ...">
    {t("task:loadingOlderMessages")}
  </div>
)}
<div ref={sentinelRef} data-testid={SENTINEL_TEST_ID} aria-hidden="true" />
```

The panel's sentinel is configured with `rearmWhileIntersecting: true` and
`joinInFlightWhileLoading: true` (`use-lazy-load-sentinel.ts`), so after every
positive page load the still-intersecting sentinel fires the next older-page
request immediately. `isLoadingMore` therefore flaps true→false once per page
while pages stream in, and each in-flow mount/unmount of the ~28 px row changes
the scrollable content height and the sentinel's geometry. The content reflow
shifts the sentinel across the intersection boundary and jitters the scroll
position, which is the visible flicker.

## Fix

- The panel root is a positioned outer wrapper (`relative overflow-hidden`)
  whose scrollable content lives in a distinct inner scroller (`h-full
  min-h-0 overflow-y-auto p-2`). This is REQUIRED for the floating placement:
  an absolutely positioned child of a scroll container anchors to the scrolled
  content and moves with it, so the floating message must anchor to the outer
  wrapper (the panel viewport) as a sibling of the scroller.
- Wrap the rows and the sentinel in a content wrapper and measure scrollability
  from the wrapper alone against the scroller's content box
  (`wrapper.scrollHeight > scroller.clientHeight - verticalPadding`,
  re-measured after every commit via `useLayoutEffect`, plus a per-commit
  `ResizeObserver` on the scroller that also attaches when the scroller first
  appears): the loading indicator's own presence can never flip the answer, so
  the mode cannot oscillate. The sentinel stays the final in-flow child of the
  wrapper, giving it a stable geometry in both modes.
- Scrollable panel: the indicator is an absolutely positioned,
  `pointer-events-none` centered chip at the panel bottom (`absolute inset-x-0
  bottom-2 z-10 flex justify-center`) anchored to the outer wrapper, out of the
  content flow and fixed to the panel viewport regardless of scroll position.
- Short panel (and the zero-entries branch, which can never scroll): the
  indicator is a plain in-flow row inside the scroller after the sentinel,
  directly under the last message.
- Flicker: a `LOADING_GRACE_MS` (400 ms) minimum-display window keeps the
  indicator mounted after each page settles, so the sentinel's re-arm loop
  (next page firing right after a positive settle) renders one continuous
  indicator instead of a per-page flash. It disappears once a settle is not
  followed by another load within the window, or when pagination ends
  (`shouldPaginate` false).
- Visibility unchanged: shown only while `shouldPaginate && (isLoadingMore ||
  grace)`. Passthrough stays an unconditional no-controls empty state.
- Older-page loads accumulate at least 10 new user prompts per sentinel
  trigger: `useLazyLoadMessages(sessionId, { minUserPromptsPerLoad })` loops
  message pages (20 messages each) until the threshold, pagination exhaustion,
  a zero-result page, or a 10-page cap. The transcript (no option) keeps its
  single-page behavior.
- While the user is pinned at the bottom, a positive settle scrolls the
  scroller back to the new bottom (`stickToBottomWhileLoading` in
  `useLazyLoadSentinel`, tracked via scroll events so content growth never
  clears the pin), keeping the re-armed sentinel in view so loading continues
  without a scroll-away. Scrolling away cancels the stick.
- No new copy: reuse `task:loadingOlderMessages`; no locale changes.

## Task waves

- **Wave 1 (single task, one component + its tests + one e2e spec):**
  `task-01-loading-overlay.md`.

## Tasks

- `task-01-loading-overlay.md` — floating overlay in
  `prompt-history-panel-content.tsx` with unit + e2e regression coverage.

## Global validation

From `apps/web`:

```bash
pnpm vitest run components/task/prompt-history-panel-content.test.tsx
pnpm run typecheck
pnpm run lint
```

Targeted e2e (desktop auto-load + mobile panel):

```bash
pnpm e2e:raw -- e2e/tests/task/prompt-history-auto-load.spec.ts
pnpm e2e:raw -- e2e/tests/task/mobile-prompt-history-panel.spec.ts
```

## Risks

- The overlay must stay below the panel toolbar (none here) and above rows
  (`z-10`); `pointer-events-none` keeps wheel/touch gestures and row
  interactions intact.
- Sentinel geometry is now constant while loading, which changes re-arm timing
  slightly (no height shift mid-flight); pagination behavior is otherwise
  identical and covered by the existing sentinel unit tests, which must stay
  green.
- Out of scope: the transcript's in-flow loading-older row
  (`message-list-shared.tsx`) stays as-is.
