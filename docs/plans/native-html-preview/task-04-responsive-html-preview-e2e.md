---
id: "04-responsive-html-preview-e2e"
title: "Prove responsive HTML preview flows"
status: done
wave: 3
depends_on:
  - "02-responsive-html-preview"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.6
  - AC-UI-NATIVE-HTML-PREVIEW-001.7
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 04: Prove responsive HTML preview flows

## Summary

Add browser-level evidence that HTML preview works from the current buffer and
keeps untrusted scripts isolated. Prove the same user outcome in the focused
mobile file viewer and verify mobile geometry.

## In scope

- Desktop Chromium coverage for source, unsaved edit, preview, isolation,
  return to source, and same-session refresh restoration.
- Mobile Chrome coverage for focused viewer entry, preview, return to source,
  touch-target size, and document overflow containment.
- Browser assertions that inline scripts run inside the frame but cannot mutate
  the Kandev parent document.
- Rebuild the production Vite bundle before E2E execution.

## Out of scope

- Other browser engines or executor types.
- Review-diff, relative-asset, remote-network, or Browser-panel tests.
- Broad E2E suite execution.

## Acceptance

- Desktop Chromium proves the current unsaved buffer is rendered and source
  state survives the full preview cycle and page refresh.
- Mobile Chrome completes the same preview cycle with a 44-pixel action and no
  document-level horizontal overflow.
- The browser test proves inline script execution remains inside the opaque
  frame and does not alter the parent application.

## Verification

```bash
make build-web
cd apps/web && pnpm e2e:raw --project=chromium tests/chat/html-preview.spec.ts
cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/task/mobile-html-preview.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/chat/html-preview.spec.ts`
- `apps/web/e2e/tests/task/mobile-html-preview.spec.ts`
- `apps/web/e2e/pages/session-page.ts`

## Dependencies

- Task 02 supplies the responsive HTML preview behavior and stable accessible
  selectors.

## Risks

- Happy DOM component tests cannot prove browser sandbox enforcement, so this
  work order must keep the browser-level parent-isolation assertion.
- Preview restoration waits on normal workspace and file-tab hydration and must
  use deterministic visible state rather than fixed sleeps.

## Parallelism

`parallel-safe` with Task 03 after Task 02.

## Inputs

- All acceptance criteria in `REQ-UI-NATIVE-HTML-PREVIEW-001`.
- The responsive contract and verification strategy in the system design.
- Existing Markdown preview desktop E2E and mobile file-viewer E2E patterns.

## Results

Added deterministic Chromium and mobile-Chrome coverage for the desktop and
focused mobile preview flows. Rebuilt the production web bundle and required
E2E backend artifacts before running the managed browser tests. Verification
passed:

```text
make build-web
pnpm e2e:run --host --no-build --project=chromium tests/chat/html-preview.spec.ts
pnpm e2e:run --host --no-build --project=mobile-chrome tests/task/mobile-html-preview.spec.ts
```
