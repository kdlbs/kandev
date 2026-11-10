---
id: "01-retain-feedback"
title: "Retain and match walkthrough feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WALKTHROUGH-STEP-FEEDBACK-001
acceptance_criteria:
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.1
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.2
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.3
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.4
  - AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.5
system_design:
  - ../../specs/ui/system-design/walkthrough-step-feedback.md
---

# Task 01: Retain and Match Walkthrough Feedback

## Summary

Retain submitted walkthrough feedback in the existing comment store. Match it
to its exact step, and keep it outside subsequent pending submissions.

## In scope

- Pure step selector and optional revision snapshot on new comments.
- Source-specific acknowledgement, persistence, hydration, and cleanup.
- Exact-ID acknowledgement in normal and passthrough chat success paths.
- Pending selector guards and stale local Run protection.

## Out of scope

Rendered comment list, composer disclosure, server storage, and other comment
types' delivery semantics. No new UI preview is needed for this state-only task.

## Acceptance

1. Matching rejects other sessions/tasks/revisions and ambiguous legacy records.
2. All accepted walkthrough delivery paths retain history and exclude it from
   pending context. Other comment sources retain their existing cleanup behavior.
3. Rejection preserves one pending record. Late success only acknowledges its
   captured IDs. Hydration and cleanup preserve the specified lifecycle.

## Verification

Use TDD. Run these commands from the repository root. Install dependencies once
in this worktree before the first pnpm command if they are absent.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/walkthrough-comments.test.ts lib/state/slices/comments/walkthrough-history.test.ts lib/state/slices/comments/persistence.test.ts lib/state/slices/comments/format.test.ts hooks/domains/comments/use-pending-comments.test.ts hooks/domains/comments/use-run-comment.test.ts components/task/chat/chat-input-area-context-submission.test.tsx components/task/passthrough-chat-composer.test.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

## Files likely touched

- `apps/web/lib/state/slices/comments/{types,comments-store}.ts`
- Proposed `apps/web/lib/walkthrough-comments.ts` and `.test.ts`.
- Proposed `apps/web/lib/state/slices/comments/walkthrough-history.test.ts`.
- `apps/web/hooks/domains/comments/{use-pending-comments,use-run-comment}.ts` and tests.
- `apps/web/components/task/chat/chat-input-area.tsx` and its context-submission test.
- `apps/web/components/task/passthrough-chat-composer.tsx` and test.
- `apps/web/components/diff/walkthrough-step-card.tsx`, only comment creation metadata.

The primary conversation owns these changes. Keep the task's edits intact when
Task 02 changes the shared card file.

## Dependencies

None.

## Risks

Normal chat and passthrough chat currently remove walkthrough rows after success.
Do not change explicit user removal into acknowledgement. Use the submitted
snapshot when a new pending comment arrives during an outstanding request.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/walkthrough-step-feedback.md), requirement 001.
- [Design](../../specs/ui/system-design/walkthrough-step-feedback.md), identity through delivery.
- Existing comment store, persistence tests, pending hooks, and delivery tests.

## Results

Completed retention, revision matching, pending guards, and exact-ID chat acknowledgement.
The prescribed eight Vitest files passed: 97 tests. RED showed deletion of sent
history, stale pending inclusion, and destructive chat cleanup before the fixes.
The first typecheck exhausted Node's 4 GiB heap. The 8 GiB run reached only new
Task 02 test typing errors, now corrected. The final package typecheck passed with the 8 GiB heap.
`git diff --check` passed. The dependency install completed successfully.
