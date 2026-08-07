---
spec: docs/specs/secure-context-browser-fallbacks/spec.md
created: 2026-08-02
status: implemented
---

# Implementation Plan: Secure-context browser fallbacks

## Overview

Route every client-only UUID through the existing guarded generator, then
extract the existing clipboard fallback into a shared utility and migrate
direct Clipboard API call sites to it. The audit confirms that file hashing,
voice input, notifications, and sound already check capability boundaries; the
focused tests will preserve those fallbacks while covering the newly repaired
workflow, layout, and copy paths.

## Root cause

`useWorkflowCreation`, workflow step creation, and layout profile creation call
`crypto.randomUUID()` directly. On a non-localhost HTTP origin the browser
exposes `crypto` but not `randomUUID`, so Add Workflow throws before its draft
state is updated. The shared `generateUUID()` helper already handles that
condition, but those call sites bypass it. Several copy actions similarly call
`navigator.clipboard.writeText()` directly even though the existing copy hook
has a DOM fallback for insecure contexts.

## Frontend — client-only UUIDs

- `apps/web/app/settings/workspace/use-workflow-creation.ts`: use
  `generateUUID()` for temporary workflow IDs.
- `apps/web/components/settings/workflow-card-actions.ts`: use
  `generateUUID()` for temporary step IDs.
- `apps/web/lib/layout/layout-profiles.ts`: use `generateUUID()` for layout
  profile IDs.
- `apps/web/app/settings/workspace/use-workflow-creation.test.ts`,
  `apps/web/components/settings/workflow-card-actions.test.ts`, and
  `apps/web/lib/layout/layout-profiles.test.ts`: add insecure-context coverage
  with `crypto` missing `randomUUID`.
- Keep `apps/web/lib/utils/file-diff.ts` unchanged unless its existing guarded
  fallback needs a test adjustment; it already degrades to `djb2Hash` when
  `crypto.subtle` is unavailable.

## Frontend — clipboard fallback

- Add `apps/web/lib/utils/copy-to-clipboard.ts` with the existing dialog-aware
  DOM fallback and a promise-returning `copyToClipboard(text)` function.
- Refactor `apps/web/hooks/use-copy-to-clipboard.ts` to use the shared utility,
  preserving its copied state and timing.
- Migrate direct Clipboard API calls in the settings, onboarding, task,
  diff/editor, review, and share surfaces to the shared utility. Preserve each
  caller's current success state, toast, and error behavior.
- The audited call sites are `apps/web/app/settings/agents/page.tsx`,
  `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.tsx`,
  `apps/web/components/settings/external-mcp-settings.tsx`,
  `apps/web/components/automations/trigger-configs/webhook-config.tsx`,
  `apps/web/components/task/executor-environment-info.tsx`,
  `apps/web/components/task/port-forward-dialog.tsx`,
  `apps/web/components/task/simple/OfficeSimplePane.tsx`,
  `apps/web/components/task/inspector/annotations-panel.tsx`,
  `apps/web/components/task/share/share-dialog.tsx`,
  `apps/web/components/task/chat/messages/auth-methods-panel.tsx`,
  `apps/web/components/task/chat/messages/tool-edit-message.tsx`,
  `apps/web/components/editors/file-actions-dropdown.tsx`,
  `apps/web/components/editors/monaco/use-diff-viewer-comments.ts`,
  `apps/web/components/editors/monaco/diff-viewer-context-menu.tsx`,
  `apps/web/components/editors/monaco/monaco-diff-viewer.tsx`,
  `apps/web/components/diff/diff-header-toolbar.tsx`,
  `apps/web/components/review/review-diff-toolbar.tsx`,
  `apps/web/components/onboarding/step-agents.tsx`,
  `apps/web/components/settings/account/api-tokens.tsx`,
  `apps/web/components/settings/system/log-viewer.tsx`,
  `apps/web/components/settings/system/invite-dialog.tsx`, and
  `apps/web/components/settings/ssh-agent-readiness-card.tsx`.
- Add `apps/web/lib/utils/copy-to-clipboard.test.ts` covering modern success,
  missing API, rejected API, and dialog focus containment. Update the hook test
  only where its implementation seam changes.

## Existing capability audit

- `apps/web/lib/utils/file-diff.ts` already guards `crypto.subtle` and falls
  back to `djb2Hash`.
- Voice input gates its UI on secure-context/capability checks and catches
  capture failures; notification permission actions check `Notification`
  before use; sound creation checks `AudioContext` before construction.
- No behavior change is planned for those already guarded paths; their
  existing tests remain part of focused frontend verification.

## Tests

- **UUID regression:** no `randomUUID` must not prevent workflow creation,
  step addition, or layout profile creation. Files listed above; Vitest hook and
  pure-function tests.
- **Clipboard regression:** missing/rejected Clipboard API uses the DOM
  fallback without an uncaught rejection. New utility test plus existing hook
  tests.
- **Existing fallbacks:** run the file-diff, voice, notification, and sound
  tests selected by the focused test command where applicable.

## E2E and mobile parity

This repair changes shared state/utility behavior, not layout, navigation,
touch targets, scrolling, or mobile composition. The existing workflow settings
desktop/mobile E2E coverage remains the rendered regression path; no new mobile
scenario is required for this data-normalization change. A browser-level
insecure-origin smoke run is optional if an isolated HTTP instance is already
available.

## Risks and out of scope

- The DOM copy fallback depends on `document.execCommand("copy")`, which is
  deprecated but is the compatibility path already used by Kandev.
- Fallback UUIDs are not cryptographically secure and must stay limited to
  client-only identifiers.
- Backend/API contracts, TLS setup, and security-token generation are out of
  scope.

## Implementation waves

Wave 1 (sequential):

- [x] [Task 01: UUID fallback audit](task-01-uuid-fallback.md)

Wave 2 (sequential; depends on Task 01):

- [x] [Task 02: Clipboard fallback migration](task-02-clipboard-fallback.md)

## Verification Results

### Task 01 — UUID fallback audit

- Focused Vitest: 4 files, 78 tests passed.
- Web typecheck passed.
- Static search found only the guarded `generateUUID()` implementation.

### Task 02 — Clipboard fallback migration

- Focused utility/hook Vitest: 3 files, 21 tests passed.
- Broader task-defined Vitest: 360 files, 2,647 tests passed, 4 skipped.
- Web typecheck, targeted ESLint, Prettier, and `i18n:ratchet` passed.
- Static search found no direct application Clipboard API calls outside the
  shared guarded utility.
