---
id: "01-rendering-engine-marker"
title: "Classify the rendering engine"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/webkit-task-dialog-rendering.md"
---

# Task 01: Classify the Rendering Engine

## Acceptance

- Safari, WKWebView/WebKitGTK-shaped user agents, iOS branded browsers, and iPadOS desktop-mode
  Safari UAs classify as `webkit`, while Blink, WebView2, Firefox, unknown user agents, and
  iPadOS desktop-mode Chrome/Edge UAs carrying Blink tokens classify as `other`.
- The document root receives `data-rendering-engine="webkit|other"` before boot-payload loading and
  React rendering, without persistence or a user-visible setting.
- Classification failure falls back to `other` and does not block application startup.

## TDD sequence

1. Add table-driven classifier and marker-helper tests, then run the focused Vitest command and
   confirm RED because the helper does not exist.
2. Implement the smallest pure classifier and marker helper that satisfies the cases.
3. Call the marker helper from `src/main.tsx` before `loadBootPayload()`.
4. Rerun the focused test and refactor only if naming or compatibility-token handling is unclear.

## Files likely touched

- `apps/web/lib/browser/rendering-engine.ts`
- `apps/web/lib/browser/rendering-engine.test.ts`
- `apps/web/src/main.tsx`
- `docs/plans/webkit-task-dialog-rendering/plan.md`
- `docs/plans/webkit-task-dialog-rendering/task-01-rendering-engine-marker.md`

## Verification

Run from `apps/web`:

```bash
pnpm test -- lib/browser/rendering-engine.test.ts
```

## Dependencies

None.

## Parallelism

`sequential`. Task 02 consumes the exact root marker contract established here.

## Inputs

- Classification and failure-mode requirements in
  `docs/specs/ui/requirements/webkit-task-dialog-rendering.md`.
- Existing early application bootstrap in `apps/web/src/main.tsx`.
- Existing navigator-mocking pattern in `apps/web/lib/keyboard/utils.test.ts`.
- Tauri's current macOS/Linux system-WebView boundary and Chromium/WebView2 exclusion requirement.

## Risks

- Desktop Chromium includes the `AppleWebKit` compatibility token; never use that token alone.
- iOS Chrome/Edge/Firefox brand tokens do not imply Blink/Gecko on iOS and must remain WebKit.
- Keep the helper DOM-injectable so unit tests do not need module-level global mutation.

## Output contract

Report the observed RED failure, classifier cases added, files changed, final focused test result,
remaining UA-classification risks, and updated task/plan statuses in this conversation.
