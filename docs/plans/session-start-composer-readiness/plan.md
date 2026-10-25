---
created: 2026-09-02
status: done
requirements:
  - REQ-UI-SESSION-START-COMPOSER-READINESS-001
system_design:
  - ../../specs/ui/system-design/session-start-composer-readiness.md
legacy_specs: []
---

# Implementation Plan: Session Start Composer Readiness

## Overview

Separate editor readiness from submission readiness in the shared chat
composer. Implement the state split before the focused session-recovery E2E
change because the E2E flow depends on the new editor behavior.

## Scope

### In scope

- Keep the shared text editor editable during session startup and resume.
- Keep all regular submit paths blocked until the session is ready.
- Preserve the draft when the session becomes ready or enters recovery.
- Add focused hook and session-recovery coverage.

### Out of scope

- Backend, WebSocket, lifecycle, queue, or persistence changes.
- New composer copy or layout changes.
- Changes to non-startup editor gates.

## Technical approach

Update `computeDerivedState` in
`apps/web/components/task/chat/use-chat-input-container.ts`. Derive the editor
gate without `isStarting`. Add startup to `submitDisabled` as a separate gate.

Keep the clarification exception on the startup submission gate. Keep the
environment-prepare reason on the disabled send action.

`ChatInputEditorArea` already sends `isDisabled` to `TipTapInput` and uses
`submitDisabled` for native and plugin submission. No component API change is
necessary.

Update the hook test to prove the two gates have different values during
startup. Add a deterministic slow-preparation E2E flow that enters text before
readiness, proves that submission is disabled, and verifies the preserved draft
after readiness. Keep the manual resume flow focused on post-recovery
usability because its backend request resolves only after the resumed agent is
prompt-ready.

## Tests

- `AC-UI-SESSION-START-COMPOSER-READINESS-001.1` and
  `AC-UI-SESSION-START-COMPOSER-READINESS-001.2` map to
  `apps/web/components/task/chat/use-chat-input-container.test.ts`.
- `AC-UI-SESSION-START-COMPOSER-READINESS-001.3` through
  `AC-UI-SESSION-START-COMPOSER-READINESS-001.5` map to the same hook suite and
  the existing state tests.
- `AC-UI-SESSION-START-COMPOSER-READINESS-001.6` uses the shared composer
  architecture. No viewport-specific logic changes.

## E2E tests

- Update
  `apps/web/e2e/tests/session/session-recovery.spec.ts` for
  `AC-UI-SESSION-START-COMPOSER-READINESS-001.1`,
  `AC-UI-SESSION-START-COMPOSER-READINESS-001.2`, and
  `AC-UI-SESSION-START-COMPOSER-READINESS-001.3`.
- The slow-preparation E2E flow holds a new session in STARTING, enters a
  draft, and verifies that the draft remains after the submit gate clears.
- The manual resume flow separately verifies that the recovery card clears and
  the resumed composer remains usable.
- A separate mobile E2E change is not required. The shared state logic does not
  alter mobile layout, navigation, scrolling, or touch behavior.

## Work orders

- [x] [Task 01: Separate composer readiness gates](task-01-separate-composer-readiness-gates.md)

## Verification results

- Installed the locked frontend dependencies successfully.
- Focused Vitest coverage passed: 2 files and 20 tests.
- The production-build Chromium startup and resume flows passed: 2 tests.
- Targeted ESLint, specification lint, and `git diff --check` passed.

## Risks

- A submission shortcut can bypass a disabled button if it does not use the
  shared guarded submit handler.
- A startup state change can clear the draft if a caller replaces the composer
  identity.
