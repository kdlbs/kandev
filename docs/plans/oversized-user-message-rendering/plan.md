---
created: 2026-09-18
status: complete
requirements:
  - REQ-UI-BOUNDED-USER-MESSAGE-001
system_design:
  - ../../specs/ui/system-design/bounded-user-message-rendering.md
legacy_specs: []
---

# Implementation Plan: Oversized user-message rendering

## Overview

Correct the confirmed unbounded rendering path from [issue 3797](https://github.com/kdlbs/kandev/issues/3797).
One sequential work order delivers bounded previews and desktop/phone regression coverage.
The issue is assigned to `carlosflorencio`.

## Evidence and confidence

Source baseline: `26254fe51f`, inspected on 2026-09-18.

- `renderUserMessageBody` passes complete text segments into `MemoizedMarkdown`.
- `MemoizedMarkdown` normalizes and parses the complete string. Its memo boundary does not avoid first-render cost.
- `remarkBreaks` creates line-break nodes for ordinary pasted logs.
- `AnchoredLastPromptBar` parses the complete prompt even when hidden. Visibility also changes the renderer key.
- `QueuedGhostMessage` sends the complete visible text to `ReactMarkdown` before CSS clipping.
- `message-list-native.tsx` maps mounted transcript items; it does not limit the contents of an individual message.

A temporary Vitest reproduction called the real `renderUserMessageBody` through `renderToStaticMarkup`.
The generated line was `2026-09-18T06:32:25Z INFO worker=N operation completed; duration=125ms status=ok`.
The test joined 3,921 lines with LF, with N from 0 to 3920.

| Input | Source bytes | Break elements | HTML bytes | Render duration |
| --- | ---: | ---: | ---: | ---: |
| 40 lines | 3,269 | 39 | 3,563 | 18 ms |
| 3,921 lines | 328,253 | 3,920 | 347,952 | 345 ms |

A second run with a 200-line assertion failed: `expected 3920 to be less than or equal to 199`.
Its large-message render took 403 ms. These are diagnostic server-render measurements, not browser performance guarantees.
The temporary reproduction was removed after recording the evidence.

Confidence is high for the unbounded-rendering defect and the bounded-preview correction.
The original deleted payload and diagnostic bundle are unavailable in this checkout.
The experiment does not reproduce the reported minute-long RPC delays or prove their full cause.

Backend separation:

- `listTaskSessionMessages` reads history through HTTP. Another history path uses `message.list` RPC.
  Do not attribute all session history traffic to one WebSocket route.
- The gateway read limit is 32 MiB, larger than this generated payload.
  This does not establish the size of the deleted payload or every provider's limit.
- `StreamManager.handleUpdatesDisconnectWithGeneration` wraps an underlying stream error with `agent stream disconnected`.
  That error text does not identify a message-size cause.

## Scope

### In scope

Bound transcript, raw, instruction-disclosure, pinned, and queued previews.
Preserve complete source for storage, agent delivery, copy, editing, and download.
Add localized labels, desktop/phone coverage, and user documentation during implementation.

### Out of scope

Provider disconnect remediation, RPC timeout changes, backend payload projection, composer virtualization,
assistant output, share snapshots, and prompt-history plugin rendering.
Do not close the entire incident as resolved solely from the preview regression.

## Technical approach

Use the [system design](../../specs/ui/system-design/bounded-user-message-rendering.md).
Add `message-preview.ts` and `bounded-message-preview.tsx`; integrate them at the three source-render boundaries.
Retain the existing Markdown, instruction, download, and responsive primitives.
The 200-line/16,000-code-unit budgets are proposed local presentation constants, not transport limits.

## ASCII UI preview

UI-01: Selected user message, before and after. Source confirms complete rendering today.

```text
BEFORE                       AFTER: desktop
+----------------------+     +----------------------------------+
| all 3,921 log lines   |     | first bounded Markdown excerpt   |
| rendered in bubble   |     | Message preview shortened        |
| ...                  |     | [Download full text]             |
+----------------------+     +----------------------------------+

AFTER: phone conversation
+-----------------------------+
| bounded Markdown excerpt    |
| Message preview shortened   |
| [    Download full text   ] |
+-----------------------------+
```

The action follows the notice and stays outside clipped text. Phone targets are at least 44px.
The transcript owns vertical scrolling. Existing message actions remain below the bubble.
Raw mode uses the same structure with a bounded plain-text excerpt.

UI-02: Desktop pinned or queued preview, collapsed and expanded.

```text
COLLAPSED                    EXPANDED
+------------------------+   +----------------------------+
| two clipped text lines |   | bounded Markdown excerpt   |
| shortened [Download]  v|   | shortened [Download]     ^ |
+------------------------+   +----------------------------+
```

Expansion changes height, never the source budget. Pinned height remains capped at 40% of the panel.
Phones retain the existing queue layout and original-message access; no pinned bar is added.
Control order, bounded content, and accessible actions are required. Spacing and labels are illustrative and localized.
These views map to AC-UI-BOUNDED-USER-MESSAGE-001.1 through .5.

## Tests

- `message-preview.test.ts`: exact boundaries, line endings, Unicode, long lines, unchanged input (AC .1, .4).
- `user-message-body.test.tsx`: `bounds oversized user text before Markdown rendering`, raw mode, instruction disclosure, aggregate segment budget, and exact full-message bytes from a shortened segmented preview (AC .1-.4).
- `anchored-last-prompt-bar.test.tsx`: hidden, collapsed, expanded, replaced-source, and ordinary Markdown cases (AC .1-.4).
- `queued-ghost-message.test.tsx`: bounded collapsed/expanded previews and complete edit/send values (AC .1-.4).
- `chat-message.test.tsx`: complete copy source and attachment behavior remain intact (AC .4).

## E2E tests

Add `apps/web/e2e/tests/chat/oversized-user-message.spec.ts` for project `chromium`.
Add `apps/web/e2e/tests/chat/mobile-oversized-user-message.spec.ts` for `mobile-chrome`.

Cover real send, reload, bounded DOM, exact download, session switch, subsequent small-message completion,
queued expansion, desktop pinned expansion, phone touch targets, and horizontal containment (AC .1-.6).
Use `last-prompt-scroll.spec.ts` and its helper as the desktop navigation pattern.
Use `mobile-last-prompt-scroll.spec.ts` as the phone pattern.

## Work orders

- [x] [Task 01: Bound user-message previews](task-01-bound-previews.md) — complete

## Verification results

Diagnostic reproduction passed for full rendering, then failed the proposed bound as expected.
Implementation is complete. The shared preview selector bounds every specified user-message
rendering surface before Markdown parsing or raw DOM creation, while full source remains available
to storage, agent input, copying, editing, and downloads. The implementation adds localized copy,
public documentation, focused unit/component coverage, and desktop/mobile browser coverage.

Verification results:

- `pnpm exec vitest run ...`: 7 files, 140 tests passed.
- `pnpm run typecheck`: passed.
- `pnpm run lint`: passed with zero warnings.
- `pnpm run build:vite`: passed. Vite emitted existing chunk-size and dynamic-import warnings.
- `pnpm run i18n:check`: passed. All five catalogs are complete and no UI em dashes were found.
- Desktop managed E2E: 13 tests passed for oversized and last-prompt flows. The oversized scenario uses the 3,921-line, approximately 328 KB log, checks reload separately, switches away and back through the desktop sidebar, and verifies a subsequent small response reaches `WAITING_FOR_INPUT` and is stored.
- Mobile managed E2E: 2 tests passed for oversized and last-prompt flows. The oversized scenario uses the same incident-sized log, switches through the phone task drawer, verifies the queued preview/download, then completes the `/slow 10s` follow-up after clearing that preview-only queue entry and confirms it is stored.
- Public documentation validators passed.
- `git diff --check`: passed.

`pnpm run i18n:zh-hant` refused to write because the existing workflows catalogs contain the
simplified `openAgentSettings` entry in the Traditional Chinese residual check. The task catalogs
are complete and `i18n:check` passes. Full specification lint passed after the pre-existing
duplicate acceptance criterion in `docs/specs/tasks/requirements/queued-session-ownership.md` was
assigned the unique ID `AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.10`.
Artifact validation on 2026-09-18:

- `python3 scripts/list-docs.py validate`: passed, 290 decisions and 1,017 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed after assigning the previously duplicated
  line 147 criterion in `docs/specs/tasks/requirements/queued-session-ownership.md` the unique
  ID `AC-TASKS-QUEUED-SESSION-OWNERSHIP-003.10`.
  Neither new specification has a reported violation.
- `git diff --check -- docs/specs docs/plans/oversized-user-message-rendering`: passed.
- Catalog discovery lists both new specifications. Git status contains only the four new package documents.
- Issue assignee readback confirms `carlosflorencio`.

Public documentation changes are included in the completed implementation. Browser checks confirm
the bounded DOM, exact complete downloads, source preservation, separate reload coverage, in-app
task switching, queued expansion, pinned expansion, mobile touch targets, and horizontal
containment. The original deleted payload and provider disconnect cause remain unproven, as
recorded above.

## Risks

- Shortened Markdown can end within a construct; retain safe rendering and test malformed fences and links.
- Slicing before instruction parsing can expose markers; preserve existing segmentation before bounding visible content.
- Hidden or expanded copies can bypass the fix; test every listed surface.
- Browser layout can add cost beyond the reproduction measurements.
- Backend disconnections can persist independently. Preserve this uncertainty in the implementation report and PR.
