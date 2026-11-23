---
id: "01-replace-screenshot-capture"
title: "Replace a saved screenshot capture"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.6
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-03.md
---

# Task 01: Replace a saved screenshot capture

## Summary

Let `task.preview_feedback.update` replace a saved screenshot item's
`capture_rect` and image in one version-checked transaction, so a resized
region can be recaptured without delete-and-create.

## In scope

- WS handler and DTO: optional `capture` object with `capture_rect` and
  `screenshot_attachment_id` on update. The comment becomes optional when
  `capture` is present, and at least one of them is required.
- Service and both repositories: one transaction that checks the task,
  version, and `kind: "screenshot"`, claims the new staged attachment with the
  create-path checks, replaces the rect and attachment ID, increments the
  version, and releases the previous attachment after the row update.
- Snapshot publish as other mutations do.
- Web API client and slice method that send the new field (no UI yet).

## Out of scope

- UI, bridge, and changes to text or element captures.

## Acceptance conditions

1. Replacing succeeds once, bumps the version, and leaves exactly one claimed
   attachment, with the old one deleted.
2. A stale version, non-screenshot kind, or foreign or already claimed
   attachment fails with no change to the row or either attachment.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/task/service/ -run 'PreviewFeedback' -count=1
go test -tags fts5 ./internal/task/repository/sqlite/ -run 'PreviewFeedback' -count=1
go test -tags fts5 ./internal/task/handlers/ -run 'PreviewFeedback' -count=1
bash ../../scripts/lint-go-changed $(git diff --name-only -- '*.go' | sed 's#^apps/backend/##')
cd ../web && pnpm exec vitest run lib/api hooks/use-preview-capture.test.ts
```

## Likely files

- `apps/backend/internal/task/handlers/task_preview_feedback_handlers.go`, `_test.go`
- `apps/backend/internal/task/service/preview_feedback_service.go`, `_test.go`
- `apps/backend/internal/task/repository/sqlite/preview_feedback.go`, `_test.go`, `_postgres_test.go`
- `apps/backend/internal/task/repository/interface.go`, `previewfeedback/validate.go`
- `apps/web/lib/api/` preview-feedback client and the task preview-feedback store slice

## Risks

- Release the old attachment only after the row update succeeds in the same
  transaction, so rollback keeps the old image.
