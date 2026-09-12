---
created: 2026-09-10
status: done
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
legacy_specs: []
---

# Implementation Plan: Preparation attachment previews

## Overview

Show submitted screenshots and file labels while a new task prepares its workspace.
One sequential work order carries a session preview from task creation to chat.
The task system owns this change because it owns submitted attachments and session identity.

## Evidence and root cause

The reported task `7a8da962-d5ba-4b6c-b9ee-92691f21e3b4` has two PNG descriptors
in its first stored user message. The supplied screenshot shows its text-only
row above worktree preparation. A read-only source trace confirms:

- `prepareStartAgentSession` creates the session without passing attachment display data.
- `postLaunchCreated` / `postLaunchStart` call `recordInitialMessage` after launch.
- `buildTaskDescriptionMessage` creates a synthetic row without attachment metadata.
- `chat-message.tsx` renders images and file labels from that missing metadata.

Reproduce with a new task containing text and two images while workspace setup
is held pending. The fallback has text, but no attachments. No live instance
was mutated, and no runtime reproduction or permanent test has been run yet.

## Scope

In scope: file-backed task-create attachments during session preparation,
prepare-only creation, reload, failure, stored-message replacement, and phone parity.

Out of scope: upload limits, new delivery modes, queue changes, additional-session
prompt redesign, legacy inline preview backfill, and changing when the backend
records a delivered initial message. A queued task without a session is outside
this workspace-preparation view.

## Technical approach

Follow the existing [attachment design](../../specs/tasks/system-design/prompt-attachments.md#initial-preview-during-workspace-preparation).
Extend the internal preparation options in `session_launch.go` and
`task_operations.go`; forward display data from both task-create prepare branches
in `task_http_handlers.go`. Persist bounded `initial_prompt_preview` metadata
after attachment admission and before session publication/workspace work.

Read the resolved session through `useSessionState` / `useSessionData`, then
normalize its preview in `useProcessedMessages`. Render with the existing
attachment components. Preserve all current history-exhaustion guards and
stored-message precedence. Keep snapshot data out of agent dispatch and turn accounting.

## ASCII UI preview

UI-01: Task Chat during workspace preparation, shared desktop/phone composition.

```text
Before                         After
+-------------------------+    +-------------------------+
| Submitted text          |    | [image 1] [image 2]      |
+-------------------------+    | [notes.txt]             |
Preparing environment...       | Submitted text          |
  Create worktree              +-------------------------+
                               Preparing environment...
                                 Create worktree
```

The user row stays above progress. Phone attachments wrap within the existing
single-column Chat view; Chat owns vertical scrolling. Image taps open the
existing viewer. Files retain their existing compact labels. Grouping and
order are required; spacing is illustrative. Reuse localized controls.

Attachment-only: omit the text area and keep the attachment row. No attachments:
keep the current text row. Failure: keep the preview above the existing error.
After launch: show only the stored user message with its attachments.
These states cover AC-TASKS-PROMPT-ATTACHMENTS-001.8 through .11.

## Tests

- `.8`, `.9`: new `task_http_handlers_initial_preview_test.go` and
  `initial_prompt_preview_test.go` verify the snapshot exists before background
  preparation, survives a fresh session read, and cannot start an agent itself.
- `.8`, `.10`: `use-processed-messages-fallback.test.ts` covers descriptor-only
  images/files, empty text, late metadata, stored-row replacement, unknown
  history, older history, malformed entries, and session switches.
- `.11`: rendered checks use existing image/file controls and verify unavailable
  content does not remove other content.
- Preserve claim-denial and initial-message dedup tests; a preview is not a turn.

## E2E tests

Add `e2e/tests/task/preparation-attachments.spec.ts` (`chromium`) and
`e2e/tests/task/mobile-preparation-attachments.spec.ts` (`mobile-chrome`).
Use shared fixtures and a controlled preparation barrier, not a fixed sleep.
On a disposable repository, a setup script can wait for a test-owned release
file; always release the barrier and remove owned fixture data in `finally`.
Verify the selected executor actually runs the barrier before asserting UI.

Create a task with two uploaded images and a resource file through the UI.
While preparation is pending, open an image, close it, inspect the file label,
reload, and verify the same attachments. Release preparation and assert exactly
one stored initial message. Add preparation-failure and inaccessible-content
cases. Phone checks use taps and assert zero document horizontal overflow.
These scenarios cover `.8` through `.11`; capture the preparation view for comparison to UI-01.

## Work orders

- [x] [Task 01: Show initial attachment previews](task-01-initial-attachment-previews.md)

## Verification results

- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `git diff --check`: passed.
- Worktree status confirms the requirement/design edits and new plan/work order.
- Implementation, runtime reproduction, and browser checks: not run in this design turn.

Implementation verification: targeted backend suites passed; hook tests passed
(12); TypeScript, native builds, and the E2E web build passed. Managed desktop
tests passed (2), and managed phone tests passed (1), with retries disabled.
Both captured views show the images and file label during preparation. See the
work order for commands, fixture corrections, and detailed results.

## Risks

- Preview data could accidentally dispatch twice if reused as launch input.
- Session reuse or stale hydration could show another session's attachments.
- Relaxing fallback history guards could invent an initial message in old history.
- Metadata persistence must preserve unrelated keys and precede background work.

The public task attachment guide now explains preparation previews and distinguishes
them from successful agent delivery. See the work order for implementation results.
