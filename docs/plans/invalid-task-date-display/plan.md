---
created: 2026-09-05
status: done
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
legacy_specs: []
---

# Implementation Plan: Suppress invalid task timestamps

## Overview

Prevent the task transcript from displaying the browser's `Invalid Date` text
when it renders the frontend-only task-description fallback. The fallback has
content but no persisted message timestamp, so the timestamp action is omitted;
the shared legacy relative formatter also returns an empty label for malformed
input. The implementation is one sequential frontend slice with focused unit
regressions.

## Root cause

`useProcessedMessages` creates `TASK_DESCRIPTION_SYNTHETIC_ID` with
`created_at: ""` when history is exhausted without a stored user prompt.
`MessageActions` always renders `MessageTimestamp`, and the legacy
`formatRelativeTime` helper treats the invalid `Date` as though its numeric
comparisons were valid before falling through to `date.toLocaleDateString()`.
The browser therefore supplies the visible string `Invalid Date`.

## Scope

### In scope

- Validate a message timestamp before rendering the timestamp action and its
  absolute-time disclosure.
- Make the shared legacy relative formatter return an empty string for empty or
  malformed input.
- Add unit coverage for the observed synthetic fallback path and the shared
  formatter contract.

### Out of scope

- Changing task-description fallback eligibility, message persistence, or API
  timestamp fields.
- Adding a new mobile composition or changing action-row sizing, touch targets,
  navigation, or scrolling.
- Reworking the separate locale-aware formatter in `lib/i18n/formats.ts`.

## Technical approach

- `apps/web/lib/utils.ts`: return `""` immediately when the parsed date is
  invalid in `formatRelativeTime`.
- `apps/web/components/task/chat/messages/message-actions.tsx`: validate
  `createdAt` in `MessageTimestamp` before deriving the absolute label or
  mounting the `<time>`/Drawer surface. Invalid timestamps keep content actions
  visible but omit only the timestamp affordance.
- Keep the existing `formatRelativeTime` import and action-row presentation so
  valid message timestamps and desktop/mobile interaction behavior are
  unchanged.

## Tests

- `apps/web/lib/utils.test.ts`: add an invalid and empty input regression for
  `formatRelativeTime`, covering the shared helper's empty-label contract.
- `apps/web/components/task/chat/messages/message-actions.test.tsx`: add a
  synthetic-fallback timestamp case asserting no `<time>` element or
  `Invalid Date` text/disclosure is rendered. This covers
  `AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.15`.

## E2E tests

No new Playwright scenario is required. This repair changes only timestamp
normalization and conditionally removes an affordance from an existing inline
message row; it does not change mobile composition, touch behavior, navigation,
scrolling, or viewport geometry. The focused message-action test covers the
shared desktop/mobile rendering path, and existing task-chat mobile flows remain
the rendered transcript smoke coverage.

## Work orders

- [x] [Task 01: Suppress invalid message timestamps](task-01-suppress-invalid-message-timestamps.md)

## Verification results

Passed:

- `pnpm --filter @kandev/web test -- --run lib/utils.test.ts components/task/chat/messages/message-actions.test.tsx`: 47 tests passed.
- ESLint passed for the four changed frontend source and test files.
- `pnpm run typecheck` passed from `apps/web`.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
- Managed Chromium and Pixel 5 capture specs passed with synthetic task data; both screenshots contain no `Invalid Date` text. Temporary capture specs were removed after capture.

## Risks

- A malformed timestamp supplied by a persisted message will lose its timestamp
  affordance instead of showing a misleading value; message content and other
  actions remain available.
- The focused test command may regenerate frontend release-note/changelog
  artifacts as part of the package pretest hook; unrelated generated changes
  must not be included.
