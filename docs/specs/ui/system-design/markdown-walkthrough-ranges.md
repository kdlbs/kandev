---
status: current
system: ui
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-002
---

# Markdown Walkthrough Ranges

## Boundary and requirement mapping

This design extends the reusable Markdown file presentation contract in
[Markdown file editing](markdown-file-editing.md). It maps
`REQ-UI-MARKDOWN-FILE-EDITING-002`, criteria `.16` through `.19`, to range
resolution, rendered geometry, navigation ownership, and responsive lifecycle.
Walkthrough generation, stored steps, and backend APIs retain their contracts.
The existing [walkthrough layout](../requirements/walkthrough-navigation-layout.md)
owns the floating card and phone sheet.

## Integration boundary

`revealFileAtLine` opens the file and writes a walkthrough-owned pending cursor
position. Monaco and CodeMirror retain their existing provider hooks and range
indicators. Rendered Markdown uses a shared target and lifecycle through the
Preview and hybrid Edit adapters.

`useMarkdownWalkthroughRange` resolves the active step at the desktop Markdown
presentation boundary and the mobile Markdown composition. It requires the open
walkthrough's active task and compatible active session, then uses
`getWalkthroughEditorRange` and `walkthroughStepMatchesFile` for file and
repository matching. `MarkdownWalkthroughTarget` carries the file scope, task,
session, step index, normalized inclusive endpoints, and original navigation
line. Its key includes both endpoints. Primary-repository identity follows the
existing empty-repository convention; a basename does not establish repository
identity.

`MarkdownPreviewContent` and `HybridMarkdownEditor` receive the resolved target.
Experimental package imports remain below `components/editors/markdown/`.
`useRenderedWalkthroughRange` owns their shared indicator, reveal, and anchor
lifecycle. There is no new dependency, engine patch, backend state, or
persistence migration.

## Rendered range geometry

### Edit

Convert inclusive source lines with the existing source-offset helpers.
Preserve CRLF, LF, and BOM offsets. Clamp endpoints to the current document.
Use the pinned engine's public `view.rangeRects(OffsetRange)` observable.
`useHybridWalkthroughRange` clips the selection rectangles to the public
`view.measuredLayout.measurements` block bounds whose source ranges strictly
intersect the target. The engine's selection geometry can include bridge gaps
or neighboring-block rectangles; clipping prevents a preceding paragraph or
checklist from entering the section indicator. Empty offset ranges use half-open
block containment. The remaining rectangles are unioned in the
`view.overlayContainer` coordinate space.

Intersecting tables also contribute their complete measured block rectangles,
so table borders and rows are enclosed even when selection geometry covers only
text. Horizontal bounds span the editor viewport with an eight-pixel
inset on each side. The adapter converts the same geometry with
`view.coordinateSpace.capture().toClientRect` for connector publication.

Use `view.revealRangeInCenterIfOutsideViewport(range, "instant")` once for a
new target or newly active presentation. Retain and dispose the reveal handle.
For hidden syntax with no measured rectangle, use the nearest measurable block
within the same document. Never substitute another file's geometry. Empty or
unmeasurable documents have no connector until valid layout exists.

### Preview

Reuse `data-md-source-start`, `data-md-source-end`, and `readSourceLineRange`.
`useMarkdownPreviewWalkthroughRange` selects overlapping mapped blocks. A
container without its own mapped text defers to its mapped descendants. A
container with its own text remains eligible; it is removed only when descendant
ranges cover its complete intersection with the target. This preserves a list
item's own opening text while avoiding an enclosing list or blockquote when
smaller blocks already cover the requested section. If no candidate overlaps,
the nearest mapped block provides the blank-line or unrendered-source fallback.

The adapter unions the selected DOM rectangles into section bounds spanning the
Preview scroller with an eight-pixel horizontal inset. Paint coordinates include
the scroller's current offset; connector coordinates remain in client space.

The adapter observes content layout as well as the viewport. Source changes,
internal scroll, resize, and asynchronous image, diagram, or table layout update
the geometry. Reveal changes only the existing Preview scroller's position and
waits for measurable content.

### Shared presentation

The shared lifecycle creates one indicator with the existing `border-primary/70`,
`bg-primary/10`, inset outline, and small radius. The layer has `pointer-events: none` and `aria-hidden`.
It cannot cover editing actions with an interactive hit area. Preserve
`data-testid="walkthrough-editor-range"` and `data-line-range` for parity tests.
Do not use selection changes, comment markers, or source edits to paint it.

## Navigation and lifecycle ownership

Only the visible, active file presentation can reveal or publish an anchor.
Retained hidden Preview/Edit instances release their subscriptions and anchor.
The shared lifecycle requires a connected container with positive bounds before
revealing. Each rendered hook instance adds its own identity to the target key,
so old cleanup cannot clear a replacement instance's anchor. Step identity
includes both range endpoints.

The source-position hooks register mode-restoration handles before the range
lifecycle schedules its first animation frame. Mode restoration preserves the
selection; the current walkthrough target then controls initial reveal. A new
target or newly active presentation reveals once. Layout updates and source
replacement remeasure without recentering after deliberate user scrolling.
Composition settlement remains owned by the existing input transport.

The lifecycle coalesces geometry updates through animation frames. Resize
observers, captured scroll/load events, window resize, and a 150-millisecond
visibility refresh keep bounds and occlusion current. The Edit adapter also
subscribes to the public range-rectangle observable.

`file-editor-cursor.ts` retains the shared pending positions used by walkthrough,
search, and LSP navigation. Walkthrough producers attach a unique
`walkthroughOwner` token. Immediately before accepting its initial reveal, the
rendered lifecycle obtains a `pendingWalkthroughAcknowledgement` callback for
the exact file/repository key, compatible session scope, and original requested
line. Session lookup preserves the existing scoped-first, unscoped-fallback
contract.

Acknowledgement removes the observed entry only if it is walkthrough-owned and
still the same pending request. A newer request, another scope, or an ordinary
search/LSP position survives. Capturing the request in the first animation
frame includes walkthrough navigation written by the floating window's passive
effect. A handled request is removed so Source cannot replay that old jump;
existing non-walkthrough cursor consumers retain their behavior.

Cancel queued frames, reveal handles, observers, and anchors on target changes,
walkthrough close, hide, replacement, failure, and unmount. Source replacement
remeasures the current line-based target. This does not add semantic rebasing
of generated steps after arbitrary edits.

## Desktop and phone

Desktop keeps the floating walkthrough card. Phone keeps the existing
`WalkthroughFloatingWindow` sheet beside the focused Files surface.
`SessionMobileReviewDialog` receives a separate walkthrough opener from
`useMobileFileOpenHandlers`. A new Markdown walkthrough target defaults to
Preview; a retained target uses its stored mode. Same-file walkthrough navigation
activates Files without refetching, replacing a dirty draft, changing mode, or
opening a discard dialog. The chat opener retains its explicit Source/Preview
semantics. The
Markdown surface remains its own scroll owner. No new controls are added.
Use `isWalkthroughAnchorTargetVisible` for connector publication. If the sheet
obscures the target, retain the section indicator and clear only the connector.
Reuse existing safe-area spacing and 44-pixel phone controls.

## Validation and delivery

Adapter tests cover range conversion, nested blocks and parent-owned text,
geometry updates, target identity, conditional pending-request acknowledgement,
and teardown. Tests use the real engine model and local history with controlled
layout boundaries to check exact source bytes, selection, and undo after
decoration. Mobile handler tests cover default Preview, retained dirty Edit, and
independent chat navigation.

Browser coverage follows a stored walkthrough through default Preview, retained
Edit, Source, same-file steps, and repository changes. The delivery checks span
Chromium, Firefox, WebKit, and both configured phone projects, including computed
indicator styling, complete table bounds, scrolling, and occlusion.

The [implementation package](../../../plans/markdown-walkthrough-ranges/plan.md)
owns delivery and recorded results. No persistence migration is required.
