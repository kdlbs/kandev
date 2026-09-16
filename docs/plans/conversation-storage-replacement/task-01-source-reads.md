---
id: "01-source-reads"
title: "Add revision-backed source reads"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.4
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.8
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.10
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.11
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.12
system_design:
  - "../../specs/plugins/system-design/conversation-source-reconciliation.md"
---

# Task 01: Add revision-backed source reads

## Summary

Add revision-only schema and transactional source readers behind explicit Host-only v2 requests.
Keep the existing callers operational until their migration. This intermediate work order is not a release.

## In scope

- Own revision schema, both database dialects, message/turn page reads, and service interfaces.
- Capture transient mutation receipts with base/final revision and persisted results inside the source transaction.
- Cover normal creation, streaming append, tool, permission/clarification, and turn writers. Incomplete bulk coverage must return reset.
- Keep keyset cursor identity separate from expected applied revision. Add no content hashes or per-message version storage.
- Add v2 page/revision routes with existing authorization, sanitization, signed query binding, and private reconciliation errors.
- Prove updates, deletion cascades, session moves, rollback, concurrent writes, and absent revision row semantics.

## Out of scope

- Plugin/core caller migration and legacy cleanup.
- New public SDK signatures or a new external plugin.

## Acceptance

- Source-backed pages and revision checks pass privacy, ordering, filter, expiry, and concurrent-mutation tests.
- Schema setup does not read historical payloads. Revision rows remain bounded by session count.
- SQLite and real PostgreSQL tests prove atomic source reads, transaction-bound receipts, concurrent writers, and replayable setup.

## Verification

Run from the repository root. Obtain behavioral RED evidence before production edits.

```bash
(cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/task/service ./internal/plugins -count=1)
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/plugins -run 'ConversationSource|ConversationRevision' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/conversation_source.go (new)`
- `apps/backend/internal/task/repository/sqlite/conversation_source_test.go (new)`
- `apps/backend/internal/task/repository/sqlite/conversation_source_postgres_test.go (new)`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/repository/sqlite/message_payload_replay.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/service/service_messages.go`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/plugins/conversation_handlers.go`
- `apps/backend/internal/plugins/conversation_tokens.go`
- `apps/backend/internal/plugins/conversation_source_handlers_test.go (new)`

## Dependencies

None.

## Risks

- Do not use PostgreSQL READ COMMITTED across separate row and revision queries.
- Sampling the revision after commit can falsely certify an older payload as current. Capture it inside the mutation transaction.
- Avoid JavaScript precision loss and deadlocks when mutations affect several sessions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md) and the frontmatter criteria.
- [System design](../../specs/plugins/system-design/conversation-source-reconciliation.md).
- [ADR](../../decisions/2026-09-16-conversation-source-reconciliation.md).
- Existing conversation handler, journal, Host facade, and recovery tests provide the regression patterns.

## Results

- Added revision-only source metadata and SQLite/PostgreSQL trigger setup with bounded source page readers.
- Added transaction-bound receipts for message creation, updates, deletion, turn creation, and turn completion.
- Added v2 page and revision routes with signed cursors, authorization, sanitization, decimal revision strings, and private mismatch errors.
- Integrated message and turn service publication with receipt data while preserving compatibility callers.
- `GOCACHE=/tmp/kandev-go-cache go test -race ./internal/plugins ./internal/gateway/websocket ./internal/office/testharness -count=1`: passed.
- `GOCACHE=/tmp/kandev-go-cache go test -race ./internal/task/repository/sqlite -run 'Conversation|MessagePayload|PromptIndex' -count=1`: passed.
- `GOCACHE=/tmp/kandev-go-cache go test ./internal/task/service -run 'TestPublishMessageEvent|Conversation' -count=1`: passed.
- `GOCACHE=/tmp/kandev-go-cache go run ./cmd/sqlguard ./internal`: passed.
- Focused source, receipt, route, and gateway tests passed.
- Status remains in progress because the required PostgreSQL source and receipt variants could not run without `KANDEV_TEST_POSTGRES_DSN`.
