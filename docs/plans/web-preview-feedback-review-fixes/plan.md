---
created: 2026-09-16
status: complete
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-001
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-002
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-003
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
legacy_specs: []
---

# Implementation plan: Web preview feedback review fixes

## Overview

Repair three confirmed defects from PR #3708 without changing its feature
direction. Keep task-owned feedback, ordinary Send, attachment storage, and
atomic admission. This package is now being implemented in the primary
session. The original planning handoff did not authorize production changes or
a commit; the current user request supersedes that handoff constraint.

The source baseline is `d8816fcf0bb5d6da954d35b95d14b08410f8d7c0` on
`feat/web-preview-feedback`. Its base is
`60aef37cbd8a043786393a56b603111eb3bd1f63`. Before implementation, compare the
current branch with this baseline. Preserve contributor and user changes.

The [existing requirements](../../specs/tasks/requirements/web-preview-feedback.md)
already define the intended behavior. This package reuses them without new IDs.
The [current design](../../specs/tasks/system-design/web-preview-feedback.md)
contains the implementation clarifications. The task system remains the owner
because the collection and accepted delivery belong to the task.

## Scope

### In scope

- Exact queue replay after feedback consumption, including screenshot metadata.
- Retention of the screenshot comment across upload and failed creation.
- A shared collection editor that does not require the source preview.
- Access to that editor when no agent session can accept a prompt.
- Focused regression tests, desktop and touch behavior, and artifact results.

### Out of scope

- A new execution path, per-item Send, or automatic delivery.
- New database tables, preview origins, screenshot libraries, or dependencies.
- General extraction of `PlanService` or broad DTO cleanup.
- Other PR comments, failing CI, and unrelated fixture changes.
- Completing the unfinished whole-PR audit or declaring the PR ready to merge.
- Commits, pushes, GitHub comments, thread resolution, and agent delegation.

The user selected the three new findings as the planning context. Existing
review comments remain separate work. Do not infer that this package resolves
the URL-secret, PostgreSQL, PNG, capture accessibility, or screenshot-preview
findings. A regression that hits one of those defects needs a recorded blocker,
not a weaker assertion or a silent expansion of scope.

## Evidence and root causes

| Defect | Source evidence | Regression owner |
| --- | --- | --- |
| Queue retry fails after commit | `wsQueueMessage` calls `PreviewFeedbackAttachments` before repository replay. Consumed rows fail its exact-reference check. | Task 01 |
| Screenshot comment disappears | `DraftEditor` resets on draft object identity. Upload replaces that object before create settles. | Task 02 |
| Collection controls require preview chrome | `buildContextItems` omits `onOpen`. `PreviewFeedbackItem` provides only a read-only preview. | Task 03 |

These are static source traces, not executed reproductions. Each work order
requires a failing behavioral test before production changes.

## Technical approach

### Queue replay

Separate the original request identity from server-expanded content and image
descriptors. Keep authorization before replay. Move mutable pending-item
resolution behind the replay decision. Repeat the replay decision inside the
final transaction to handle competing identical requests.

Reuse `queue_admission_receipts` for an accepted response that outlives the
visible queue row. Write its receipt in the same transaction as feedback
consumption and queue insertion. Do not call ordinary queue admission as a
second transaction. Preserve task-before-session lock order, capacity checks,
attachment claims, mixed plan comments, and the external-I/O marker.

### Capture draft

Give each logical capture stable local identity and controller-owned comment
state. Upload enrichment must not replace the identity or reset the comment.
The editor reads and updates that state instead of resetting on object changes.
Only successful creation or explicit discard settles that draft. A late result
must not clear a newer draft. UI closure alone does not discard a mounted draft.

### Collection access

Extract the saved collection view from the capture controller. Give it a task
ID and the existing `usePreviewFeedback` operations. Keep capture mode, iframe,
and upload state outside this view. Reuse it from preview controls and the
composer context item.

Mount collection access outside `ChatInputContainer` branches that replace or
hide the input for stopped sessions and launch errors. `ChatInputArea` already
receives a task ID independently of `resolvedSessionId`. Keep a task-level
fallback entry visible when the ordinary context chip is unavailable. Preserve
archived-task restrictions. Opening feedback must not start or recover an agent.

Use a controlled Popover on desktop and a Drawer for touch. Do not place
interactive edit forms inside the read-only HoverCard. Reuse the Drawer
composition in `mobile/mobile-picker-sheet.tsx`: fixed header, one scroll body,
and safe-area padding. Keep domain state and mutation logic shared.

## ASCII UI preview

Structure and action availability are required. Labels and spacing are
illustrative. All shipped copy uses localization keys.

### UI-R1: Capture draft after create failure

Entry: saved PNG upload followed by a rejected create. The same form appears
in a desktop Popover or touch Drawer. Covers `AC-TASKS-WEB-PREVIEW-FEEDBACK-002.4`
and `AC-TASKS-WEB-PREVIEW-FEEDBACK-004.4`.

```text
Before: [PNG preview] [empty comment]        [Save disabled]
After:  [PNG preview]
        Comment: [Keep this typed feedback              ]
        [Localized create error. Comment remains.       ]
                                    [Discard] [Retry]
```

The controller retains the draft while its form closes and reopens. Upload
and create disable duplicate saves. Error status does not replace input.

### UI-R2: Task collection from the composer

Entry: the context chip, or the task-level fallback when the input is hidden.
Covers `AC-TASKS-WEB-PREVIEW-FEEDBACK-001.4`, `003.1`, `004.1`, and `004.2`
under the common `AC-TASKS-WEB-PREVIEW-FEEDBACK-` prefix.

```text
Desktop
[Preview feedback (2)] -> +----------------------------------+
                         | Pending feedback (2)      [Close] |
                         | /checkout - captured evidence     |
                         | Saved comment       [Edit][Delete]|
                         | [Comment editor]    [Cancel][Save]|
                         | /account - captured evidence      |
                         | Saved comment       [Edit][Delete]|
                         |                      [Clear all]  |
                         +----------------------------------+

Phone / coarse pointer
[Preview feedback (2)] -> bottom Drawer
                         +----------------------------------+
                         | Pending feedback (2)      [Close] | fixed
                         | /checkout                         |
                         | Captured evidence                 |
                         | Saved comment                     | scroll
                         | [Edit]                  [Delete]  |
                         | /account ...                      |
                         | [Clear all]                       |
                         +----------------------------------+
                           safe-area clearance

No available session: [Preview feedback (2)] [existing recovery UI]
Empty after delete:   [No pending feedback]              [Close]
Load failure:         [Localized load error]             [Retry]
Edit conflict:        [Typed edit retained] [Current item available]
```

This is a temporary review surface, not a new page or stacked preview pane.
The header stays fixed. One body owns vertical scrolling within dynamic
viewport bounds. Touch actions measure at least 44px. Ordinary desktop actions
retain 28px sizing. Escape and dismissal return focus to the actual opener.
Clear uses the existing confirmation pattern without nested mobile drawers.

## Tests

The named regressions are proposed tests, not existing results.

| Criteria | Proposed evidence |
| --- | --- |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-003.5`, `.6` | `TestWsQueuePreviewFeedbackLostResponseReplay` in `queue_handlers_preview_feedback_test.go`; real service and repository |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-003.6` | `TestPreviewFeedbackQueueReplayAfterDrain` and `TestPreviewFeedbackQueueReplayConflict` in `repository_preview_feedback_test.go` |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-002.4`, `004.4` | Real-controller component regression in `preview-feedback-controls.test.tsx`; hook regression in `use-preview-capture.test.ts` |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-001.4`, `.5`, `003.1` | New `preview-feedback-collection.test.tsx`; existing context-item and input-area suites |
| `AC-TASKS-WEB-PREVIEW-FEEDBACK-004.1`, `.2` | Component focus tests plus desktop and mobile collection E2E |

Mappings identify repaired scenarios, not proof of every clause in an entire
requirement. Task 03 also preserves failed edit input while it extracts that
editor. This is necessary to avoid carrying the known edit-loss defect into
the new shared surface.

## E2E tests

- Extend `apps/web/e2e/tests/preview/preview-feedback.spec.ts` in `chromium`.
  Restore saved feedback with the preview closed. Open the chip, edit, delete,
  reload, and prove that no prompt was sent. Retain the existing capture and
  direct/queued Send scenarios.
- Extend `apps/web/e2e/tests/task/mobile-html-preview.spec.ts` in `mobile-chrome`.
  Leave the focused preview, tap the collection entry, and manage the same
  pending items. Cover a stopped source session, Drawer geometry, focus return,
  safe-area clearance, and no document overflow.
- Add the upload-success/create-failure regression to both existing flows.
  Use deterministic test-controlled failure, not network timing or sleeps.
  The component regression remains the primary RED proof for this defect.

## Work orders

- [x] [Task 01: Reconcile accepted queue retries](task-01-queue-replay.md)
- [x] [Task 02: Preserve screenshot draft comments](task-02-draft-preservation.md)
- [x] [Task 03: Expose the independent collection editor](task-03-collection-access.md)

Task 01 has no dependency on Task 02. Execute sequentially in the listed order.
Task 03 depends on Task 02 because both change the capture controls and editor
ownership. This package does not authorize spawning agents.

## Companion package

The [original package](../web-preview-feedback/plan.md) records the contributor's
implementation and reported results. Those counts remain historical.
This package owns new regression evidence and the three open corrections.
Original Tasks 02, 03, 04, and 05 intersect this work. Their recorded completion
does not establish that these new scenarios pass.

## Verification results

Implementation verification completed on 2026-09-16. The initial regressions
were reproduced before the fixes: queue replay returned `preview_feedback_changed`
after consumption, and screenshot create failure cleared the typed comment.

- Backend focused tests passed with race detection across handlers, messagequeue,
  task service, task handlers, and SQLite repository packages.
- Frontend focused tests passed: 10 files and 62 tests.
- Full web ESLint passed with `--max-warnings 0`; backend `golangci-lint` passed
  with 0 issues.
- Web typecheck, localization check, localization ratchet, and whitespace checks
  passed. Documentation catalog validation passed with 279 decisions and 954
  specifications, and specification lint passed.
- Desktop Chromium preview feedback E2E passed: 1 test.
- Mobile Chrome HTML preview E2E passed: 3 tests, including failed-create retry,
  completed-session collection access, edit/delete, reload restoration, Drawer
  bounds, safe-area padding, focus return, and overflow checks.
- `KANDEV_TEST_POSTGRES_DSN` was not set, so the optional PostgreSQL fixture was
  not run. PostgreSQL parity is not claimed. Existing PostgreSQL screenshot-read
  review coverage remains outside this package.

The final source commit and PR head are recorded by the delivery workflow. Do
not treat this package's completion as whole-PR audit completion or merge approval.

The documentation handoff uses these checks from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/plans/web-preview-feedback-review-fixes docs/plans/web-preview-feedback/plan.md docs/specs/tasks/system-design/web-preview-feedback.md
git status --short -- docs/plans/web-preview-feedback-review-fixes docs/plans/web-preview-feedback/plan.md docs/specs/tasks/system-design/web-preview-feedback.md
```

Planning results (2026-09-16) were superseded by the implementation results
above. The package is ready for the authorized commit, push, wait, and PR-fixup
workflow. Existing review findings and the unfinished whole-PR audit remain
separate.

## Risks

- Queue fingerprints currently include server-added screenshot descriptors.
  Moving lookup alone does not correct replay identity.
- A receipt lookup outside the transaction does not close a concurrent-send race.
- PostgreSQL screenshot reads have a separately reported defect. Do not hide it
  by omitting the PostgreSQL regression result.
- A stopped-session banner can hide a correctly implemented collection chip.
- Draft cleanup must not remove an image after the server accepted its claim.
- The remaining review and existing security comments still block merge clearance.

## Handoff constraints

Implement only after the user assigns this package to the next agent.
Use TDD for each work order. Keep results pending until its commands pass.
Do not commit or push under this package. Do not treat a finished package as
permission to close GitHub review threads or advance an incomplete PR review.
