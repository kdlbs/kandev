---
id: "02-attachment-copies"
title: "Preserve selected attachments"
status: done
wave: 2
depends_on: ['01-snapshot-preview']
plan: "plan.md"
requirements:
  - REQ-TASKS-CONVERSATION-FORK-002
  - REQ-TASKS-CONVERSATION-FORK-003
  - REQ-TASKS-CONVERSATION-FORK-005
acceptance_criteria:
  - AC-TASKS-CONVERSATION-FORK-002.4
  - AC-TASKS-CONVERSATION-FORK-002.5
  - AC-TASKS-CONVERSATION-FORK-003.4
  - AC-TASKS-CONVERSATION-FORK-003.5
  - AC-TASKS-CONVERSATION-FORK-005.2
  - AC-TASKS-CONVERSATION-FORK-005.4
system_design:
  - ../../specs/tasks/system-design/conversation-forks.md
---

# Task 02: Preserve selected attachments

## Summary

Make explicitly selected source attachments independent of source retention. Integrate physical copies into draft readiness and cleanup.

## In scope

- Own authorized streaming copies through AttachmentService and draft-owned staging descriptors.
- Extend snapshot creation to copy selected files before readiness. Preserve the previous ready draft on replacement failure.
- Enforce aggregate limits across copied files and later new uploads. Keep attachment token costs labeled unmeasured.
- Add expiry/discard cleanup and restart reconciliation for failed or interrupted copies without touching source claims.

## Out of scope

New upload storage architecture, source ownership transfers, destination launch wiring, and browser markup.

## Acceptance

- Selected files have independent private storage and survive source retention after destination admission becomes available.
- Missing bytes, denied reads, failed copies, and aggregate-limit errors never produce a ready partial draft.
- Discard and expiry remove only draft copies and never remove source files or another draft’s claims.

## Verification

Run from the repository root. Add failing behavioral tests before production changes.
All new test names and files are specified in the plan's coverage table.
A missing test file or selector alone is not behavioral RED evidence.

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/repository/sqlite -run '^(TestConversationFork|Test.*Attachment)' -count=1)
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^TestConversationForkPostgres' -count=1)
```

PostgreSQL uses a disposable database and isolated test schemas. A skipped suite leaves this work order incomplete.

## Files likely touched

- `apps/backend/internal/task/service/conversation_fork_attachments.go` (new)
- `apps/backend/internal/task/service/attachment_service.go`, `service_attachments.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/conversation_fork.go` and attachment claim helpers
- `apps/backend/internal/task/service/conversation_fork_attachments_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/conversation_fork_postgres_test.go` (extend)

## Dependencies

Task 01 must pass before this work starts.

## Risks

Source deletion during copying and restart cleanup can race. The registry must own every copy before cleanup can classify it.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/conversation-forks.md) and [system design](../../specs/tasks/system-design/conversation-forks.md).
- `attachment_service.go`, `attachment_service_lifecycle_test.go`, `attachment_test.go`, and design sections Attachments and Persistence.

## Results

Behavioral RED was observed when a selected source attachment was rejected before copy; the service then copied selected files through the authorized attachment service into draft-owned staging storage. Candidate availability, explicit selection, range validation, aggregate count/size limits, partial-copy cleanup, prior-ready-draft preservation, retry identity, discard cleanup, and source-deletion independence are covered by service tests. The copied descriptor remains private and separate from the source claim, and fork estimates label attachment bytes as unmeasured.

Verification passed:

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite -run 'ConversationFork|ConversationSourceService' -count=1)
(cd apps/backend && go test ./internal/task/service ./internal/task/repository/sqlite -run '^(TestConversationFork|Test.*Attachment)' -count=1)
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='postgres://postgres:cxtestonly@127.0.0.1:55432/postgres?sslmode=disable' go test ./internal/task/repository/sqlite -run '^TestConversationForkPostgres' -count=1)
```

The PostgreSQL test ran against a disposable PostgreSQL 16 container and passed without a skip. Draft-row expiry cleanup and staged-attachment TTL use their existing maintenance paths. No source attachment or source claim is changed during copy or discard.

### Review remediation

The retained attachment descriptors are now merged into the first destination prompt alongside new uploads and passed through the ordinary attachment materialization path. The combined attachment count and byte limits are checked at task admission and before launch. Expired and discarded drafts remove their physical staged copies; attached copies remain destination-owned after source deletion. Task 03 owns launch delivery verification.
