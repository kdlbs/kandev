---
id: "02-session-files"
title: "Deliver files with session comments"
status: done
wave: 2
depends_on: ["01-plan-files"]
plan: plan.md
requirements:
  - REQ-TASKS-COMMENT-FILES-001
acceptance_criteria:
  - AC-TASKS-COMMENT-FILES-001.1
  - AC-TASKS-COMMENT-FILES-001.2
  - AC-TASKS-COMMENT-FILES-001.3
  - AC-TASKS-COMMENT-FILES-001.4
  - AC-TASKS-COMMENT-FILES-001.5
  - AC-TASKS-COMMENT-FILES-001.6
system_design:
  - ../../specs/tasks/system-design/comment-attachment-references.md
---

# Deliver files with session comments

## Scope and acceptance

Connect agent output, walkthrough, editor, diff and whole-file Review drafts to the same upload controls and message mapping. Exercise composer Send and direct Review send. Update public feedback documentation.

Prove that two comments with equal filenames reach the mock agent with different readable bytes. Induce one failed upload and one failed send. Check that retry retains each file with its comment. Cover keyboard and touch submission. Keep clarification routing under its current owner.

## UI preview

```text
Desktop popover                 Phone sheet
[Selected text]                 [Selected text]
[Comment field]                 [Comment field]
[Attach (2)]  [Save]            [Attach (2)]     [Save]

File list open
screen.png   Ready       Remove
notes.txt    Failed      Retry   Remove
```

## Files likely touched

- `apps/web/lib/state/slices/comments/types.ts`
- `apps/web/lib/state/slices/comments/format.ts`
- `apps/web/components/diff/walkthrough-step-feedback.tsx`
- `apps/web/components/task/use-review-dialog.ts`

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/state/slices/comments components/shared/selection-comment)
(cd apps/web && pnpm e2e:run comment-attachment-references.spec.ts)
(cd apps/backend && go test ./internal/mcp/server -run TestExtractQuestionAnswerAttachments)
```

## Results

The mock agent received two identically named files with different bytes under their respective comments. Desktop and phone walkthrough Run, phone agent-output Run, whole-file Review, and both phone file editors passed browser verification. Focused Go tests passed for native delivery, local terminal delivery and SSH transport. The existing clarification text-result test passed. The full lifecycle package passed with an uncached test run after two Linux process fixtures switched to finding sleep on PATH. The checkout fixture also passed.
