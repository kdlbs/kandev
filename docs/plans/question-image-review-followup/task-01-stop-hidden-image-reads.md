---
id: "01-stop-hidden-image-reads"
title: "Stop image reads while clarification is collapsed"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGES-003
  - REQ-TASKS-QUESTION-IMAGES-005
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGES-003.1
  - AC-TASKS-QUESTION-IMAGES-003.2
  - AC-TASKS-QUESTION-IMAGES-005.2
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
---

# Task 01: Stop hidden image reads

## Summary

Pass panel visibility to question and choice previews independently of keyboard
shortcut enablement. Inspection temporarily disables shortcuts and must not
disable its own image. Keep the overlay mounted so answers and drafts survive.

## Owned files

- `apps/web/components/task/chat/clarification-panel-section.tsx`
- `apps/web/components/task/chat/clarification-input-overlay.tsx`
- `apps/web/components/task/chat/question-images.tsx`
- `apps/web/components/task/chat/clarification-panel-section.test.tsx`
- `apps/web/hooks/domains/session/use-question-images.ts` and its test.
- Existing desktop/mobile question-image E2E specs and their shared helpers.
- The owning question-image system design and this follow-up plan.

## Acceptance

1. Collapse aborts active reads and removes queued question and choice reads.
2. Late completions cannot create preview URLs while collapsed.
3. Expansion reloads previews and retains the same custom answer draft.
4. Collapse does not answer, reject, or resolve the question.
5. Image inspection still works while question keyboard shortcuts are disabled.
6. A reopened four-image group cannot show revoked URLs for queued images.

## UI structure

```text
Expanded: [previews] [choices] [draft] [Collapse]
Collapsed: [Questions] [Expand]  (draft retained, image reads stopped)
Reopened: [fresh previews] [same choices] [same draft]
```

## Validation

First add a component regression through the real panel, overlay, scheduler, and
image hook. Hold two reads with two choice images queued. Collapse, release late
reads, and assert no additional request or blob URL. Reopen and finish the reads.
Assert the draft and unanswered state remain intact.

The remediation review also identified old snapshot reuse when the same group
reopens. Load four images in one hook, collapse, then hold its first two new reads.
Assert that the queued images cannot expose old loaded states or revoked URLs.
Invalidate the snapshot when the effect releases its resources.

Run from `apps/web`:

```sh
pnpm exec vitest run components/task/chat/clarification-panel-section.test.tsx components/task/chat/clarification-input-overlay.test.tsx hooks/domains/session/use-question-images.test.ts
```

Run managed E2E commands separately with one worker and retries disabled:

```sh
pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/chat/question-image-previews.spec.ts --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts --retries=0
```

Use the existing resource-limited local runner and rebuild assets after production
changes. Capture desktop and phone after reopening. Run changed-file lint,
specification lint, documentation catalog, and PR work-order coverage.

## Results

- RED: the panel regression failed because collapse did not abort image reads.
- RED: the four-image hook regression failed because queued images reused loaded
  states with revoked URLs after reopening.
- GREEN: all 53 tests in the three focused unit files passed.
- Desktop: all four image-question E2E tests passed, with retries disabled.
- Phone: all three image-question E2E tests passed, with retries disabled.
- Changed-file ESLint, frontend typecheck, fresh E2E build, specification lint,
  documentation catalog, and PR work-order coverage passed.
- Secondary remediation review found no remaining actionable defect.
- Public docs review: existing image inspection, errors, and reload instructions
  remain correct. This restores the existing loading contract without a new API,
  control, or user procedure. Only internal design and implementation records change.

## PR review refinement

CodeRabbit requested causal synchronization for the new E2E Skip cleanup. Both
viewport tests now arm the clarification-response wait before Skip, require a
successful response, and then assert the idle input with the default timeout.
The updated desktop and phone collapse cases each passed with retries disabled.
