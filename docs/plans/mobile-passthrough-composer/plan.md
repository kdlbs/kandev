---
spec: docs/specs/cli/requirements/mobile-passthrough-composer.md
design: docs/specs/cli/system-design/mobile-passthrough-composer.md
created: 2026-09-02
status: completed
---

# Implementation Plan: Mobile Passthrough Composer

## Overview

Make the existing inline passthrough composer safe to operate by touch, then
extend its mobile end-to-end coverage across the flows requested in GitHub
issue `#2809`. Keep passthrough submission, literal slash input, and raw PTY
input as separate contracts.

## Implementation

- Add an explicit mobile touch presentation for shared composer controls so
  context, attachment, plan, cancel, send, and split Implement targets are at
  least 44-by-44 CSS pixels. Retain current desktop geometry.
- Apply the same mobile-only touch geometry to the owned passthrough Chat,
  Comments, Proceed, and Send to Agent controls in the status row. Integration
  status chips remain governed by their own component contracts.
- Keep the composer inline and preserve the existing `100dvh`, safe-area,
  visual-viewport overlay, and single flexible terminal layout.
- Add focused tests for touch geometry and mobile passthrough flows. Do not add
  an unsupported slash-command menu or a raw-terminal keybar.

No backend, WebSocket, persistence, or localization change is expected.

## Implementation waves

Wave 1:

- [x] [Task 01: Touch-safe mobile controls](task-01-touch-safe-mobile-controls.md)

Wave 2:

- [x] [Task 02: Passthrough mobile regression coverage](task-02-passthrough-mobile-regression-coverage.md), depends on Task 01.

## Risks

- Increasing controls can crowd narrow toolbars. Keep secondary actions in the
  existing internal horizontal scroller and assert no document overflow.
- Shared composer primitives also serve ACP chat. Gate geometry by mobile
  presentation and run the focused ACP composer tests.
- Mobile Chromium cannot prove iOS software-keyboard behavior. Record iPhone
  Safari and Android Chrome smoke results before closing `#2809`.
- One-tap raw PTY keys require routing to the active agent xterm. Track that as
  a separate issue instead of coupling it to semantic composer submission.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/passthrough-toolbar.test.tsx components/task/chat/chat-input-toolbar.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/cli-mode/mobile-passthrough-composer.spec.ts)
python3 scripts/lint-spec-files.py --all
```

Complete the issue's manual smoke matrix on iPhone Safari and Android Chrome.
Verify composer focus with the software keyboard open, `@` selection, operating
system file selection, task-switch draft restoration, and explicit send.

Automated coverage and implementation are complete. Physical-device smoke
remains an explicit environment blocker because this workspace has no iPhone
Safari or Android Chrome device runner. Issue `#2809` remains open for that
manual closure gate.
