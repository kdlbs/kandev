---
id: "01-submission-replay"
title: "Recover the original submission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-002
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-002.1
  - AC-TASKS-PROMPT-ATTACHMENTS-002.2
  - AC-TASKS-PROMPT-ATTACHMENTS-002.3
  - AC-TASKS-PROMPT-ATTACHMENTS-002.4
  - AC-TASKS-PROMPT-ATTACHMENTS-002.5
  - AC-TASKS-PROMPT-ATTACHMENTS-002.6
  - AC-TASKS-PROMPT-ATTACHMENTS-002.7
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 01: Recover the original submission

## Summary

Persist the original file-backed submission before launch and recover it after
failed initial startup. Use an owned prompt-free startup followed by attachment
replay through the normal prompt path.

## In scope

- Add the bounded proposed submission model and persist it after claim, before launch.
- Mark acceptance through the owned initial dispatch callback. Preserve restart state.
- Add a strict recovery loader for pending input and the documented legacy case.
- Validate canonical attachment claims and byte availability before provider reset.
- Carry a narrow internal no-automatic-prompt setting through `ResumeOptions`.
- Use the owned continuation and initial-prompt hold; preserve cancellation fences.
- Record/backfill one original user message with its descriptors.

## Out of scope

Frontend layout, warning retirement corrections, and the initial runtime timeout.
Do not replay accepted conversation history or persist legacy inline bytes.

## Acceptance

1. PNG plus resource and attachment-only submissions survive startup failure and
   restart, reach the agent in order, and produce one original user record.
2. Invalid or uncertain input never produces partial replay. Ordinary resume,
   read-only restore, siblings, and post-conversation fresh policy remain unchanged.
3. Concurrent or cancelled retries preserve owned dispatch. Queue drain cannot
   precede replay, and late acceptance cannot update a successor attempt.

## Verification

From the repository root, run:

```bash
(cd apps/backend && go test -trimpath -timeout=90s ./internal/orchestrator ./internal/orchestrator/executor ./internal/task/handlers ./internal/task/models ./internal/task/service ./internal/task/repository/sqlite ./cmd/mock-agent -run 'TestFreshStart|Test(New|Load)InitialPromptSubmission|TestInitialPromptPreview|TestTaskCreateInitialPreview|TestInitialTaskSubmissionReceiptSurvivesRequestCancellation|TestInitialPromptSubmissionRecordSurvivesSQLiteReopen|TestSetSessionMetadataKeyIfJSONValue|TestInitializeCanFailBeforeSessionCreationForE2E|TestTraceACP|TestSummarizeACPPromptBlocks' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestFreshStartSubmissionConcurrentRetry|TestFreshStartSubmissionCancelledAttempt|TestFreshStartRecoveryPreservesSuccessorFailure|TestFreshStartRecoveryCancelledBoot|TestInitialTaskSubmissionReceiptSurvivesRequestCancellation)$' -count=1)
```

Write `TestFreshStartSubmissionReplay` first. Assert captured image/resource
input and bytes after `RecoverSessionWithOptions`, not synthetic preview metadata.
The current path must fail because it sends text without attachments.
Record RED and GREEN results. Use real claimed attachment records.

## Files likely touched

- `apps/backend/internal/task/models/initial_prompt_submission.go` (new)
- `apps/backend/internal/task/models/initial_prompt_submission_test.go` (new)
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_initial_preview_test.go`
- `apps/backend/internal/task/service/attachment_service.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/initial_submission_recovery.go` (new)
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/fresh_start_submission_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/metadata_initial_prompt_submission_cas_test.go` (new)

## Dependencies

None. Reuse the existing attachment registry and recovery attempt mechanism.

## Risks

Receipt uncertainty, legacy provenance, queue ordering, and full-row metadata overwrites.
Do not weaken claims or infer acceptance from preview visibility.

## Parallelism

`sequential`

## Inputs

Read the initial submission recovery amendment in the paired task design.
Use `initial_prompt_preview_test.go`, attachment service tests, and
`workflow_start_prompt_effective_input_test.go` as current patterns.
Read the backend and agentctl scoped guides before changing runtime code.

## Results

Done. The implementation stores a bounded submission receipt before launch.
Fresh recovery validates all claimed attachments before it clears provider identity.
An owned, prompt-free resume waits for readiness, then sends the original input once.
The transcript stores one original user message with its attachment descriptors.

- Focused Go tests passed for replay from a new receipt and a legacy preview,
  attachment bytes, invalid claims, request cancellation, receipt compare-and-set,
  and bounded model records.
- A SQLite reopen test passed for durable receipt storage. Handler and mock-agent tests
  passed, and the selected command compiled related executor and service packages.
- `TestInitialTaskSubmissionReceiptSurvivesRequestCancellation` exposed a cancelled
  request context in the dispatch receipt callback. The detached-context fix passed.
- Race-enabled tests passed for concurrent retry, cancelled attempts, successor errors,
  cancelled boot, and request-context cancellation.

The initial runtime timeout remains unexplained and out of scope. An uncertain
provider acceptance still blocks automatic replay.
