---
id: "01-answer-file-admission"
title: "Admit and retain answer files atomically"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ANSWER-ATTACHMENTS-001
  - REQ-TASKS-ANSWER-ATTACHMENTS-002
acceptance_criteria:
  - AC-TASKS-ANSWER-ATTACHMENTS-001.5
  - AC-TASKS-ANSWER-ATTACHMENTS-001.6
  - AC-TASKS-ANSWER-ATTACHMENTS-002.1
  - AC-TASKS-ANSWER-ATTACHMENTS-002.2
  - AC-TASKS-ANSWER-ATTACHMENTS-002.4
  - AC-TASKS-ANSWER-ATTACHMENTS-002.5
system_design:
  - ../../specs/tasks/system-design/question-answer-attachments.md
---

# Task 01: Admit and retain answer files atomically

## Summary

Add safe file references to clarification answers and persist their complete set with the winning response transaction.
Keep attachment ownership recoverable after delivery restoration and compatible with current reference-aware cleanup.

## In scope

- Add optional file-backed descriptors to protocol answers and their durable serializer.
- Canonicalize descriptive fields from registry rows. Reject inline data, caller paths, unsupported delivery modes, and duplicate IDs.
- Validate shared count and byte limits across all new files in the bundle.
- Add a task-model claim type and attachment-aware completion operation through the task service, repository, and backend adapter.
- Derive ownership from the authenticated caller and stored source task/session.
- Adopt files and persist question-scoped manifests in the current-turn claim transaction.
- Preserve no-file encoding and existing file-free empty-entry semantics.
- Retain the owned manifest during restoration. Permit its reuse only for the same question.
- Replace or clear retry manifests transactionally when the user changes files or skips. Release removed files after commit.
- Keep the existing queue, task, and turn lock order on SQLite and PostgreSQL.

## Out of scope

Runtime file streaming, agent result projection, web controls, and new database tables are excluded.

## Acceptance

1. Valid image/resource descriptors survive response serialization and atomic admission with canonical metadata and exact question/message ownership.
2. Mixed invalid files, wrong scope, duplicate IDs, byte/count excess, stale turns, and competing responders produce no partial adoption or duplicate winning set.
3. Restored claims, exact retries, file removal, Skip, mixed terminal/pending siblings, and cleanup preserve every still-referenced file.

## TDD and verification

First add `TestAnswerAttachmentResponseRoundTrip` in the new `internal/clarification/answer_attachments_test.go`.
Its RED assertion must show that the actual durable response loses attachment metadata.
Add repository tests in a new `message_clarification_attachments_test.go` to avoid growing the existing large response file.

Required cases include exact and excess limits, duplicate IDs, canonical sizes, mixed valid/foreign files, claim races, restart reads, and retry-manifest cleanup.
Use a real SQLite repository and the existing environment-gated PostgreSQL fixture.
The mixed-sibling test must include a terminal row and a pending row with different file sets.

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/clarification/... ./internal/task/service ./internal/task/repository/sqlite -run 'AnswerAttachment|SerializeResponse|NormalizeAnswered|Clarification.*Claim|CompleteActiveClarificationBundleWithAttachments' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
```

PostgreSQL coverage gate: no disposable `KANDEV_TEST_POSTGRES_DSN` was available. The attachment-aware
claim tests run on SQLite; a PostgreSQL lock-order/concurrency regression remains open and is recorded
in the plan's coverage gates. A skipped test is not PostgreSQL evidence.
Run `scripts/lint-go-changed` against the final owned Go file list before completion.

## Files likely touched

- `apps/backend/internal/clarification/protocol/types.go`
- `apps/backend/internal/clarification/response_serialization.go`
- `apps/backend/internal/clarification/outcome_validation.go`
- `apps/backend/internal/clarification/resolver.go`
- `apps/backend/internal/clarification/answer_attachments_test.go` (new)
- `apps/backend/internal/task/models/clarification_attachments.go` (new)
- `apps/backend/internal/task/service/service_clarification_attachments.go` (new)
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/message_clarification_response.go`
- `apps/backend/internal/task/repository/sqlite/message_clarification_attachment_lock.go` (new)
- `apps/backend/internal/task/repository/sqlite/message_clarification_attachments.go` (new)
- `apps/backend/internal/task/repository/sqlite/message_clarification_attachments_test.go` (new)
- `apps/backend/internal/backendapp/clarification_attachments.go` (new)

New production helpers need adjacent tests. Update interface stubs only where the new completion operation requires them.

## Dependencies

None.

## Risks

Separate attachment and response transactions can strand claims. Lock-order drift can deadlock PostgreSQL cleanup.
Restoration must retain its own files without modifying terminal siblings.

## Parallelism

`sequential`

## Inputs

- [Requirements 001 and 002](../../specs/tasks/requirements/question-answer-attachments.md).
- [Answer contract, Atomic admission, and Recovery and retention](../../specs/tasks/system-design/question-answer-attachments.md).
- Existing `claimCreationAttachmentsTx`, attachment claim predicates, `CompleteActiveClarificationBundle`, and response restoration tests.
- `apps/backend/AGENTS.md` and the backend testing reference from `/tdd`.

## Results

Implemented the canonical answer-attachment contract, structural admission, durable serialization,
attachment-aware task-model claim, atomic SQLite transaction, per-question manifests, retry reuse,
removal release, and reference-aware cleanup compatibility.

Verification passed: `go build ./...`; clarification, SQLite repository, task service, and
storeconformance tests; `go run ./cmd/sqlguard ./internal`; and changed-file Go lint with 0 issues.
New tests cover response round-trip, structural rejection, canonical adoption, mixed valid/foreign
rollback, retry manifest retention, removal, excess count, losing-caller non-adoption, and mixed
terminal/pending siblings through the attachment-aware production path. Terminal sibling files and
responses are preserved; their incoming descriptors are ignored. PostgreSQL was not available, so
the new repository tests ran on SQLite only.
