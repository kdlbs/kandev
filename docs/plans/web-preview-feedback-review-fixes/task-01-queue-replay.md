---
id: "01-queue-replay"
title: "Reconcile accepted queue retries"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-003
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-003.6
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
---

# Task 01: Reconcile accepted queue retries

## Summary

Make an identical queue request return its accepted result after feedback
consumption. Keep one durable admission and one delivery across lost responses,
concurrent requests, and removal of the visible queue row.

## In scope

- Original-request fingerprints before server expansion.
- Authorized replay lookup before mutable feedback resolution.
- Transactional replay, feedback consumption, attachment claims, and receipts.
- Text, element, screenshot, and mixed plan/preview requests.
- Preservation of direct-message and ordinary queue behavior.

## Out of scope

- New tables, changed queue capacity, auto-merge policy, or delivery semantics.
- General plan-comment refactoring or a second admission transaction.
- Automatic resend after the external-I/O marker.

## Root cause and reproduction

`wsQueueMessage` resolves `PreviewFeedbackAttachments` before replay.
After first admission, those pending rows no longer exist.
The resolver returns `preview_feedback_changed` before repository replay.
The service also fingerprints the attachment list after screenshot enrichment.

Reproduce through the actual handler, service, and database. Accept one request
and discard its response. Repeat the unchanged request with the same
`client_queue_id`. The current handler rejects the retry.

## Implementation approach

1. Add the behavioral regression before production changes.
2. Keep authenticated task/session scope checks before any receipt disclosure.
3. Compute a stable identity from the original caller fields and exact references.
4. Exclude server-expanded prompt content and screenshot descriptors from that identity.
5. Look up an accepted result before pending-item reads, capacity checks, or new claims.
6. Repeat the lookup inside the final task/session admission transaction.
7. On a miss, resolve exact feedback and perform the existing atomic acceptance.
8. Persist the original identity and accepted response in `queue_admission_receipts` within that transaction.
9. Keep the queue receipt and external-I/O marker behavior unchanged.

Reuse the ordinary receipt helpers in `repository_admission.go`. Extend their
internal use for feedback rather than nesting `AdmitQueueMessage` around
`InsertWithTaskFeedback`. The accepted response contains expanded content and
attachments, but its fingerprint represents the original request.

The identity includes task, session incarnation, caller queue ID, content,
model, plan mode, caller attachments, metadata, feedback references, and
primary-session intent. Preserve authorization and existing user provenance.
Changed content or versions under the same ID remain a conflict.

Do not invalidate ordinary or plan-only receipt formats. For legacy preview
entries without the new original-request identity, preserve fail-closed
behavior. Never invent a receipt or re-enqueue consumed feedback from a legacy
fingerprint that cannot establish an exact match. Record that compatibility
limit in the result. This feature is still an unmerged contribution.

## Acceptance

1. Identical authorized retries return one accepted result before and after
   queue drain, including real screenshot descriptors and mixed feedback.
2. Altered payloads, foreign task/session identities, and competing new sends
   fail without another admission or attachment transfer.
3. New feedback added after acceptance remains pending. Existing direct-message,
   plan-only, capacity, and ordinary queue tests retain their behavior.

## Tests

- Add `TestWsQueuePreviewFeedbackLostResponseReplay` in the proposed
  `queue_handlers_preview_feedback_test.go`. Use real services and stores,
  not the current no-op attachment-preparer stub.
- Add `TestPreviewFeedbackQueueReplayAfterDrain` and
  `TestPreviewFeedbackQueueReplayConflict` in `repository_preview_feedback_test.go`.
- Cover text, element, screenshot, ordinary attachments plus screenshot, and
  mixed plan comments. Assert response content and attachment IDs, not only success.
- Add a deterministic two-request race. Delay the first admission at a barrier,
  then release it before the second resolves consumed rows.
- Cover a new pending item between commit and retry. Assert its version remains.
- Use the existing PostgreSQL fixture when `KANDEV_TEST_POSTGRES_DSN` is set.
  A skipped database case is not a passing cross-database result.

## Verification

Run from the repository root. Use an isolated test database for the optional
PostgreSQL fixture. The caller supplies its DSN through the existing environment
variable. Do not use a production database.

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/orchestrator/handlers ./internal/orchestrator/messagequeue ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite -run 'PreviewFeedback|TaskFeedback|PlanComment|QueueAdmission')
git diff --check
```

Record the failing regression first. Repeat this exact block after the fix.
Repeat it with the isolated PostgreSQL fixture enabled before claiming database
parity. If the known screenshot-query defect blocks it, record the failure
without expanding this work order silently.

## Files likely touched

- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_plan_comments_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_preview_feedback_test.go` (new)
- `apps/backend/internal/orchestrator/messagequeue/service_plan_comment.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_preview_feedback.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_preview_feedback_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_admission.go`
- `apps/backend/internal/task/service/service_attachments.go`

## Dependencies

None. Run first for a clear delivery baseline.

## Risks

- Comparing a fingerprint after screenshot enrichment repeats the defect.
- A receipt written after commit permits duplicate admission during a crash.
- A lookup without task/session authorization exposes another user's accepted prompt.
- Receipt retention and visible queue retention are different lifecycles.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), including scope exclusions and baseline.
- [Design: Replay before pending resolution](../../specs/tasks/system-design/web-preview-feedback.md#replay-before-pending-resolution).
- `apps/backend/internal/task/handlers/message_handlers.go`: `addMessageReplayResponse`.
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`: `admitIdentifiedOrdinaryQueuedMessage`.
- `apps/backend/internal/orchestrator/messagequeue/repository_admission_test.go`.
- `apps/backend/internal/orchestrator/messagequeue/repository_plan_comment_test.go`.

## Results

Implemented. The handler now performs authenticated replay lookup before
mutable preview-feedback and attachment resolution. The service retains the
original request fingerprint, and the repository records the accepted response
in `queue_admission_receipts` in the same transaction as queue insertion,
attachment transfer, and feedback consumption.

The RED handler regression reproduced `preview_feedback_changed` after queue
drain. GREEN coverage now passes through the real handler and service path for
text and screenshot feedback, including response-content and attachment-ID
checks. Repository coverage proves replay after visible queue drain, conflict
on a changed request, and preservation of feedback added after the original
Send snapshot.

The focused backend command passed with race detection. No PostgreSQL run was
available because `KANDEV_TEST_POSTGRES_DSN` was not set.
