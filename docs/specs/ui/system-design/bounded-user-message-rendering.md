---
status: current
system: ui
requirements:
  - REQ-UI-BOUNDED-USER-MESSAGE-001
---

# Bounded user-message rendering design

## Purpose and boundaries

Bound source content before Markdown normalization, parsing, highlighting, and DOM creation.
CSS clipping and React memoization do not bound the first render.
This is a local presentation change. It introduces no persistence, transport, or authorization boundary.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-UI-BOUNDED-USER-MESSAGE-001.1, .3 | Preview selection and integration |
| AC-UI-BOUNDED-USER-MESSAGE-001.2, .4 | Original content and download |
| AC-UI-BOUNDED-USER-MESSAGE-001.5 | Responsive behavior |
| AC-UI-BOUNDED-USER-MESSAGE-001.6 | Verification |

## Preview selection and integration

Add a pure helper in `apps/web/lib/utils/message-preview.ts`.
It returns a bounded prefix and an omission flag without changing the input.
Use constants of 200 logical lines and 16,000 UTF-16 code units.
Stop scanning once either budget is exhausted. Do not split a surrogate pair or CRLF boundary.
Exact-limit content is complete; one additional character or logical line marks omission.
Do not allocate an array containing every source line.

Add a shared presentation component at
`apps/web/components/task/chat/messages/bounded-message-preview.tsx`.
It selects the bounded input before invoking a Markdown render callback or raw-text renderer.
It places a localized omission notice and download action outside clipped text containers.
Keep full source strings in existing props and state; do not create a second stored message model.

Integration points:

- `messages/user-message-body.tsx`: retain `splitMessageSegments` semantics.
  Parse instruction markers before slicing, so a cut cannot expose a partial internal marker.
  Share one visible-content budget across text segments, rather than granting each segment the full budget.
  Bound each disclosed instruction body too. Keep instruction blocks closed by default.
  Raw mode bounds `rawContent || content` before creating its `pre`.
- `anchored-last-prompt-bar.tsx`: strip system tags before selection and pass only the prefix to `MemoizedMarkdown`.
  Both hidden and visible states use the bounded source. Preserve two-line clipping, overflow detection, and the 40% expanded-height cap.
  The omission notice and full-text action remain reachable outside the clipped Markdown area.
- `queued-ghost-message.tsx`: bound `visible` before `ReactMarkdown` in collapsed and expanded states.
  Keep queue mutation handlers and the full edit value unchanged.

Retain Markdown for bounded formatted previews, including the pinned bar.
A prefix can end inside a Markdown construct; the existing safe renderer handles incomplete syntax.
Do not add raw HTML support or synchronous highlighting of omitted content.
The general `MemoizedMarkdown` renderer remains unchanged for other consumers.

## Original content and download

Reuse `triggerFileDownload` from `apps/web/lib/utils/file-download.ts` with plain-text content and a fixed safe filename.
Create the Blob only after an explicit download action.
Normal mode downloads the full original display message, including text surrounding any disclosed instruction block; raw mode downloads the full raw representation.
Pinned and queued downloads use the full source after existing system-tag stripping.
Instruction disclosures download their complete instruction body.
Existing message copy actions retain their current source and behavior.

Storage, WebSocket payloads, agent prompts, queue editing, and attachments retain complete content.
All preview transformations are read-only. No database migration or backend change is required.

## Responsive behavior

Use the existing task-layout phone route and chat bubble as the entry point.
The closest shipped pattern is the inline user-message attachment action in `chat-message.tsx`.
The curated `task-layout.tsx` precedent supplies the dedicated phone conversation composition.

Desktop shows an inline notice followed by a compact download action.
Phone shows the notice followed by a full-width touch action inside the bubble.
Use `useResponsiveBreakpoint` and shared Button primitives: 28px desktop, at least 44px for phone or coarse pointers.
The transcript remains the vertical scroll owner. Do not add a modal or nested text scroller.
The existing phone shell retains dynamic viewport and safe-area handling.
No pinned bar mounts on phones; full-text access remains in the original bubble.
Localized labels wrap instead of widening the document.

## Failure and recovery

The notice remains visible after download, so the user can repeat the action.
Downloads use the existing browser helper and do not require a network request.
Switching sessions selects that session's complete source and computes its own preview.
Memoization must not preserve a stale preview or download source after content replacement.

## Verification

Use deterministic parser-input and DOM-size assertions as the primary regression gates.
Cover exact limits, long single lines, CRLF, surrogate pairs, instruction markers, raw text, and source replacement.
Verify downloaded bytes against the original source.

Desktop and phone Playwright scenarios use generated 3,921-line logs and real message submission.
Arm causal WebSocket waits before submission, then verify persistence, session switching, and a subsequent small message.
Capture timeout diagnostics. Keep strict WebSocket accounting enabled.
Record production-browser long-task and navigation measurements as diagnostic evidence, without brittle timing assertions.
Mock-agent completion proves Kandev's test path only; it does not prove a real provider cannot disconnect.

## Related contracts

- [Requirements](../requirements/bounded-user-message-rendering.md)
- [Last-prompt pinning](../requirements/last-prompt-pinning-regressions.md)
- [Transcript navigation settings](../requirements/transcript-navigation-settings.md)

No ADR is needed for this local rendering policy. Existing ownership and transport contracts remain intact.
