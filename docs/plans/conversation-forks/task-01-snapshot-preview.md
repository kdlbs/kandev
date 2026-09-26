---
id: "01-snapshot-preview"
title: "Compile and preview immutable snapshots"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CONVERSATION-FORK-001
  - REQ-TASKS-CONVERSATION-FORK-002
  - REQ-TASKS-CONVERSATION-FORK-003
  - REQ-TASKS-CONVERSATION-FORK-005
acceptance_criteria:
  - AC-TASKS-CONVERSATION-FORK-001.2
  - AC-TASKS-CONVERSATION-FORK-001.3
  - AC-TASKS-CONVERSATION-FORK-001.4
  - AC-TASKS-CONVERSATION-FORK-001.5
  - AC-TASKS-CONVERSATION-FORK-002.1
  - AC-TASKS-CONVERSATION-FORK-002.2
  - AC-TASKS-CONVERSATION-FORK-002.3
  - AC-TASKS-CONVERSATION-FORK-002.5
  - AC-TASKS-CONVERSATION-FORK-002.6
  - AC-TASKS-CONVERSATION-FORK-003.3
  - AC-TASKS-CONVERSATION-FORK-003.4
  - AC-TASKS-CONVERSATION-FORK-003.5
  - AC-TASKS-CONVERSATION-FORK-003.6
  - AC-TASKS-CONVERSATION-FORK-005.1
  - AC-TASKS-CONVERSATION-FORK-005.2
  - AC-TASKS-CONVERSATION-FORK-005.3
system_design:
  - ../../specs/tasks/system-design/conversation-forks.md
---

# Task 01: Compile and preview immutable snapshots

## Summary

Build the complete text snapshot service and its authenticated preview endpoints. Keep source reads consistent and token estimates explicitly approximate.

## In scope

- Own the deterministic compiler, range/cutoff validation, clarification projection, optional tool evidence, and omission metadata.
- Own SQLite/PostgreSQL snapshot schema, bounded source reads, expiry, quotas, discard, and draft-creation idempotency.
- Own candidate/content/descriptor/estimate HTTP endpoints and task service authorization.
- Own tokenizer selection and known/unknown model-limit projection. Preserve existing MCP tool estimator behavior. Counts and percentages never gate launch.
- Add rejection of selected attachment IDs until Task 02 supplies the copy path. List attachment candidates and default exclusions now.

## Out of scope

Destination creation, attachment bytes, launch mutation, and rendered browser UI.

## Acceptance

- A consistent snapshot includes the selected cutoff, never later source content, and never silently truncates selected text.
- Authorized draft preview survives repository reopen and exposes correct filtering, limits, expiry, and estimate metadata.
- SQLite and PostgreSQL tests prove concurrent source changes cannot produce mixed snapshots or bypass bounds.

## Verification

Run from the repository root. Add failing behavioral tests before production changes.
All new test names and files are specified in the plan's coverage table.
A missing test file or selector alone is not behavioral RED evidence.

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite -run '^TestConversationFork' -count=1)
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^TestConversationForkPostgres' -count=1)
(cd apps/backend && go test ./internal/task/service -run '^TestConversationSourceService' -count=1)
```

PostgreSQL uses a disposable database and isolated test schemas. A skipped suite leaves this work order incomplete.

## Files likely touched

- `apps/backend/internal/task/models/conversation_fork.go` (new)
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/conversation_fork.go` (new), `base_schema.go`, `base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/conversation_source.go` (reuse ordering and read helpers)
- `apps/backend/internal/task/service/conversation_fork.go` and `conversation_fork_estimate.go` (new)
- `apps/backend/internal/task/handlers/conversation_fork_handlers.go` (new) and task route registration
- `apps/backend/internal/backendapp/` task service wiring
- Matching new `conversation_fork*_test.go` files named in the plan

## Dependencies

None.

## Risks

Source ordering and clarification answer timestamps need one snapshot. Unknown model limits must not become invented window sizes.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/conversation-forks.md) and [system design](../../specs/tasks/system-design/conversation-forks.md).
- Source read and sanitization code, `conversation_source_postgres_test.go`, `service_conversation_source_test.go`, and the design sections Source read through API.

## Results

Implemented the transaction-consistent range reader, deterministic compiler, optional tool-evidence projection, omission metadata, persisted owner-scoped drafts, 24-hour expiry, 20-active-draft quota, idempotent create requests, and authenticated preview/candidate/discard endpoints. Source and draft records retain provenance, compiler version, content hash, selection, estimate, and source revision. Selected external shell payloads are rehydrated inside the same source transaction; missing or corrupt payloads are reported as omissions. Estimates use `o200k_base:conversation-fork-v1`; a known selected-model context limit is populated from models.dev metadata, and unknown limits remain unknown. Estimates do not gate creation.

Behavioral RED was demonstrated for the missing snapshot table, missing compiler projection, and incorrectly escaped compiler terminator. SQLite tests now cover stable inclusive ordering, narrowed ranges, limits, attachment candidates, external payload hydration, draft ownership, quota, expiry, idempotency, and frozen retry contents. Handler tests cover candidate/create/get/discard endpoints and foreign-owner denial. PostgreSQL `TestConversationForkPostgresSnapshotAndDurableDraft` passed against a disposable PostgreSQL 16 container. The targeted fork suites and `TestConversationSourceService` passed.

Command run:

```bash
KANDEV_TEST_POSTGRES_DSN='postgres://postgres:<disposable>@127.0.0.1:55432/postgres?sslmode=disable' \
  go test ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite \
  -run 'ConversationFork|ConversationSourceService' -count=1
```

### Review remediation

Accepted user cutoffs remain eligible while the assistant turn is active; assistant cutoffs still require a completed turn. Initial and refreshed estimates now resolve a selected model's known context limit through the models.dev lookup. Nested forks include an already-admitted fork once. The ordinary prompt retains the frozen compiled bytes, with an independent trusted boundary instruction and without promoting snapshot content into system context. Discard clears compiled text, selection, omission, and estimate payload after the state change; expiry cleanup removes associated private attachment copies.

Validation passed: `go test ./internal/task/service -run '^(TestConversationForkEstimateUsesKnownSelectedModelContextLimit|TestConversationForkDraftServiceCompilesEstimatesAndAuthorizes)$' -count=1`.
