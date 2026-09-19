---
created: 2026-08-29
status: done
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
legacy_specs: []
---

# Implementation Plan: Keep WebKit Transcripts at the Bottom

## Overview

Replace the unsafe bottom-scroll offset with the largest signed 32-bit integer.
Keep the write-only message path and all current scroll-owner guards.

## Scope

### In scope

- Keep enabled transcripts at the bottom in Safari, Orion, Firefox, and
  Chromium browsers.
- Preserve the write-only scroll command for live message commits.
- Add a regression test for the WebKit-safe offset.
- Keep the existing desktop and mobile scroll behavior.

### Out of scope

- Transcript controls, copy, layout, pagination, or navigation.
- Disabled auto-scroll behavior and saved scroll positions.
- A new Playwright browser project or a change to the CI browser matrix.
- Browser-specific user-agent branches.

## Confirmed root cause

[PR #3093](https://github.com/kdlbs/kandev/pull/3093) changed the native bottom
write from `scrollTop = scrollHeight` to `scrollTop = Number.MAX_SAFE_INTEGER`.
The change removed a synchronous layout read.

A Playwright WebKit 26.5 probe used a container with a 900 px scroll range.
The old `scrollHeight` write produced 900. The new `Number.MAX_SAFE_INTEGER`
write produced 0. A `2_147_483_647` write produced 900.

The live-message effect calls this helper after each streamed update. WebKit
therefore moves the transcript to the top after each update. Chromium and
Firefox clamp the same unsafe value to the bottom.

## Technical approach

### Bottom-scroll helper

- Change `NATIVE_BOTTOM_SCROLL_TOP` in
  `apps/web/components/task/chat/message-list-native-scroll.ts` to
  `2_147_483_647`.
- Keep `scrollNativeToBottom` as one write-only operation.
- Keep the work-start, live-append, and catch-up call sites unchanged.

### Regression test

- Update `apps/web/components/task/chat/message-list-native.test.tsx`.
- Name the failing regression `uses a WebKit-safe maximum offset for pinned appends`.
- Make the `scrollHeight` getter throw to protect the write-only requirement.
- Record the setter value and require `2_147_483_647`.

## Tests

| Acceptance criterion | Test evidence |
| --- | --- |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.5` | The native-scroll regression requires a WebKit-safe bottom offset after an append. |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.7` | The same regression fails on any synchronous `scrollHeight` read. |

## E2E tests

- Desktop: run the enabled live-message scenario in
  `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts` with the `chromium` project.
- Mobile: run the matching scenario in
  `apps/web/e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts` with the
  `mobile-chrome` project.
- WebKit: the direct browser probe proves the browser-specific offset behavior.
  The unit regression applies that safe value to the product helper.

## Mobile design contract

- Desktop outcome: live messages keep the transcript at the bottom.
- Mobile entry point: the existing Chat tab in the full-height task layout.
- Nearest exemplar: `apps/web/components/task/task-layout.tsx` and the current
  mobile auto-scroll test.
- Hierarchy and surface: the transcript remains the only vertical scroll owner.
- Shared logic: desktop and mobile use the same bottom-scroll helper.
- Mobile proof: the current mobile live-message scenario covers the shared path.

## Work orders

- [x] [Task 01: Use a WebKit-safe bottom offset](task-01-use-webkit-safe-bottom-offset.md)

## Verification results

- RED: the regression received `9_007_199_254_740_991` and expected
  `2_147_483_647`. The other 26 focused tests passed.
- GREEN: all 27 native-scroll tests passed after the one-line correction.
- TypeScript typecheck and targeted ESLint passed.
- All specification files passed lint. The specification linter's 20 tests
  passed.
- Desktop Chromium E2E passed one live-message scenario with retries disabled.
- Mobile Chrome E2E passed the matching Pixel 5 scenario with retries disabled.
- Direct probes showed that `2_147_483_647` reaches the bottom in Chromium 151,
  Firefox 152, and WebKit 26.5.

## Risks

- A smaller sentinel can fail if a transcript exceeds that scroll range.
  The signed 32-bit maximum is larger than a practical browser scroll range.
- A direct WebKit E2E project expands the CI browser matrix.
  This fix keeps that expansion out of scope.
