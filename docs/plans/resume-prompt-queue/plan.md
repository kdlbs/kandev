---
created: 2026-09-09
status: complete
requirements:
  - REQ-TASKS-RESUME-PROMPT-QUEUE-001
system_design:
  - ../../specs/tasks/system-design/resume-prompt-queue.md
legacy_specs: []
---

# Implementation Plan: Resume Prompt Queue

## Overview

Enable Send during resume through the existing durable queue. First close the
readiness race at admission. Then enable the composer and verify the user flow.

Requirement: [REQ-TASKS-RESUME-PROMPT-QUEUE-001](../../specs/tasks/requirements/resume-prompt-queue.md).
Design: [Resume prompt queue](../../specs/tasks/system-design/resume-prompt-queue.md).

## Scope

### In scope

- Existing queue-capable sessions during resume or initial startup.
- Task chat and Quick Chat on desktop and mobile.
- Persistence before draft clearing, automatic dispatch, and current queue policy.
- Focused regression coverage and a public documentation update during implementation.

### Out of scope

- New session creation without queue identity or terminal input emulation.
- New queue storage, timers, schedulers, settings, or public API actions.
- Changed Auto-run, merge, recovery, workflow, or cancellation policy.

## Technical approach

### Admission and dispatch

Before this change, `QueueHandlers.wsQueueMessage` persisted and published
without a readiness recheck. Add an internal automatic-dispatch collaborator and wire the
orchestrator through the existing queue-handler registration.

Reuse guarded identity-aware reservation and task admission. Preserve Auto-run
OFF. Do not use the manual drain endpoint, which enables Auto-run.
The post-admission check and `handleAgentBootReady` cover either event order.
An accepted admission remains successful if immediate dispatch is deferred.

### Composer

`deriveSessionInputMode` already maps `STARTING` to `queue`. Reuse that selected
session mode and complete queue identity for startup eligibility.
`useChatInputContainer` permits submission only when that eligibility exists.
`isPreparingEnvironment` alone remains insufficient.

`useMessageHandler` retains its submission-time state read and payload builder.
`useQueueAdmissionAction` must not silently report success without identity or
an operation token. Failed attempts preserve the draft and attachments.

### Documentation

During implementation, update the queue guidance in
`docs/public/sessions-and-review.md`. This is a how-to section: explain Send
during resume, the queued indicator, Auto-run OFF, and failed-resume recovery.
The public documentation update is included with the implementation.

## Tests

All suffixes below refer to `AC-TASKS-RESUME-PROMPT-QUEUE-001`.

| Criteria | Evidence |
| --- | --- |
| `.3`, `.4`, `.5`, `.7`, `.8` | New `resume_prompt_queue_test.go`: admission before/after boot readiness, concurrent checks, paused queue, failed resume, stale identity |
| `.3`, `.4`, `.5` | `handlers/queue_handlers_test.go`: actual `wsQueueMessage` calls the automatic check only after successful admission |
| `.1`, `.6`, `.9` | `use-chat-input-container.test.ts`: button/capability gate matrix, identity unavailable, preparation-only, existing blockers |
| `.1`, `.2`, `.8` | `use-message-handler.test.ts`: current-session routing and complete queued payload |
| `.2`, `.6`, `.8` | `use-queue.test.ts` and `use-chat-input-state.test.ts`: rejected/no-op admission preserves content, accepted admission clears it |

The backend tests use channel barriers for ordering. They count actual prompt
dispatches and retain existing boot-ready and queue-policy regression tests.

## E2E tests

Add `e2e/tests/session/session-resume-prompt-queue.spec.ts` for `chromium`.
Add `mobile-session-resume-prompt-queue.spec.ts` for `mobile-chrome`.
Shared scenarios belong in `e2e/helpers/session-resume-prompt-queue.ts`.

Cover a real delayed resume, Send, visible queue admission, then one resulting
turn after readiness (`.1` through `.4`). Cover reload after admission (`.8`).
Cover Auto-run OFF and failure recovery (`.5`, `.7`). Use task chat for the
full lifecycle matrix and Quick Chat for shared composer admission and delivery.
The mobile case uses tap, confirms the target is reachable, and asserts no
horizontal overflow (`.9`).

Reuse session recovery and queue fixtures. The current git-delay shim can hold
workspace preparation. If resume reuses its workspace, use a deterministic
mock-agent readiness barrier. Delaying only a browser response cannot prove
that the backend agent is still resuming.

Update the initial-start test in `session-recovery.spec.ts` where queue
eligibility replaces its previous disabled-Send assertion. Keep coverage for
preparation without queue identity. Disable inherited retries for regressions.

## Work orders

- [x] [Task 01: Close queue admission readiness race](task-01-admission-readiness.md)
- [x] [Task 02: Enable Send during resume](task-02-resume-composer.md)

Order: Task 01, then Task 02. Both tasks are complete. Work was sequential in
this session.

## Verification results

Task 01 and Task 02 implementation and focused verification are complete.

Task 02 verification on 2026-09-09:

- Frontend unit suite: passed, 150 tests across 10 files.
- Frontend typecheck and i18n check: passed.
- Desktop resume, Quick Chat, and layout E2E: passed, 3 tests.
- Mobile resume, touch target, and overflow E2E: passed, 1 test.
- Existing session-recovery startup E2E: passed, 1 test.
- Backend focused suites: passed, including 16 handler tests, 23 orchestrator
  tests, 6 readiness tests with `-race`, 1 handler race test, and 12 mock-agent
  tests.
- Public documentation validation: passed, 61 tests and 46 published pages.
- `rtk git diff --check`: passed.

Design validation on 2026-09-09:

- `rtk python3 scripts/lint-spec-files.test.py`: passed, 30 tests.
- `rtk python3 scripts/lint-spec-files.py --all`: passed.
- `rtk git diff --check`: passed.

Task 01 verification on 2026-09-09:

- Focused handler suite: passed, 16 tests.
- Focused orchestrator suite: passed, 23 tests.
- New readiness and identity cases with `-race`: passed, 6 tests.
- Handler readiness callback with `-race`: passed, 1 test.

## Risks

- The one-shot boot-ready event can precede queue insertion.
- A manual drain would silently enable a paused queue.
- Presentation startup can precede queue identity hydration.
- A silent admission no-op can clear an unsaved draft.
- Browser-only delays can conceal a backend readiness race in E2E.
- Queue admission must preserve initial-prompt order during normal startup.
