---
id: "05-deletion-and-orphans"
title: "Retire deleted sessions and purge delivery orphans"
status: complete
wave: 5
depends_on:
  - 04-settled-prompts
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-004
  - REQ-PLATFORM-DELIVERY-RETENTION-003
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-004.1
  - AC-PLATFORM-DELIVERY-RETENTION-004.2
  - AC-PLATFORM-DELIVERY-RETENTION-004.3
  - AC-PLATFORM-DELIVERY-RETENTION-003.2
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 05: Retire deleted sessions and purge delivery orphans

## Summary

Wire explicit deletion into durable retirement jobs, then purge every attributable delivery category in bounded restartable work. Preserve replacement session identity.

## In scope

- Audit release N retirement journaling before session/turn cascades on session, task, workspace, profile-ephemeral and automation-ephemeral deletion. Extend coverage only after updating the N compatibility gate; no N deletion path may lose retirement authority. Add bounded draining.
- Bind attributable workflow receipts before cascades. Retire the entire deleted session identity; block textual ID recreation until its purge finishes, then require a fresh incarnation.
- Purge jobs in indexed bounded phases and resume after lost commit response. All attributable inbox/effect/submission/cursor rows reach zero before completion.
- Add bounded anti-join orphan discovery with fail-closed authoritative reads. Keep unknown/unbound legacy receipts visible.
- Test ordinary deterministic ID creation in `agent_conversations.go` and native managed creation/repair through `managed_conversation_settings.go`/`repairManagedPrimaryTx`, plus fresh UUID executor/Office variants. Retry retirement conflicts, then admit a fresh incarnation after purge.
- Test managed reserved/prepared/released/committed phases and lost final-commit response. Only final deletion atomically schedules delivery purge; preserve owner/revision barriers and external cleanup outside SQL. Cover coordinator conversation deletion through the canonical task path.
- Reject delayed writes and retired-incarnation reuse. Test pending-job recreation refusal, fresh-incarnation recreation after purge, preservation of a pre-existing replacement, concurrent create/delete, archive/Stop and task/workspace cascades.
- Extend current mock/E2E reset quiescence; preserve `orderE2ETasksForDeletion`, coordinator cleanup and existing ownership checks.

## Out of scope

Implementation was authorized by the user on 2026-10-09.
Agentctl journal policy, canonical-history expiry, new UI and feature flags are excluded.
This order does not authorize commit, push, merge or delegation.

## Acceptance

- Retirement and canonical deletion share a transaction; committed retirement blocks old writes immediately, even before physical delivery purge.
- Cancellation/restart resumes batches, and all deletion routes purge the same data without deleting a replacement session or retained archive.
- Orphan classification errors preserve rows and report incomplete progress; no task/session events are omitted by new deletion wiring.

## TDD evidence

Start each changed-logic scenario with a failing test against the pre-change behavior. Record Red, Green and refactor results.
Use existing backend test helpers, fake clocks and deterministic barriers. Do not use sleeps to manufacture race order.
Required new test groups: `TestDeliveryRetentionDeletion`, `TestDeliveryRetentionOrphans`, `TestDeliveryRetentionReplacement`.
Names are planned deliverables, not existing passing tests. Before using a -run selector, list tests and confirm every required group exists.
The real repository and retained journal must prove state transitions where named; fakes do not prove database crash semantics.
Read `.agents/skills/tdd/SKILL.md` and its `references/backend-tests.md` at implementation time.

## Verification

Run from repository root. Commands are implementation-time checks, not run during planning.

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Deletion|Orphans|Replacement)|TestManagedDeletionAdmission' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/service -run 'TestDeliveryRetentionDeletion' -count=1)
```

Add `TestDeliveryRetentionDeletion/ConversationIDReuse` using managed and ordinary conversation service reconciliation with deterministic IDs.
Run both-engine deletion/orphan/replacement tests with a real multi-connection PostgreSQL pool. Include all cascade and ephemeral paths, not just direct DeleteTaskSession.
For every environment-gated test, record executed and skipped subtests. A skip cannot complete the corresponding acceptance.
The package helper checks document links/IDs; it does not prove runtime behavior.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/workspace.go`
- `apps/backend/internal/task/repository/sqlite/managed_deletion.go`
- `apps/backend/internal/task/repository/sqlite/managed_conversation_admission.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention.go` (new)
- `apps/backend/internal/task/service/`
- `apps/backend/internal/backendapp/e2e_reset.go`

Add focused `_test.go` files beside changed logic; split oversized tests rather than extending files past lint limits.
Paths marked new are intended deliverables. Other directory references identify the existing integration boundary.

## Dependencies

Complete `04-settled-prompts` first. Execute sequentially in the primary session.

## Inputs

- [Requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md).
- [Design](../../specs/platform/system-design/backend-agent-delivery-retention.md); sections matching this scope.
- [Plan](plan.md), including baseline, storage thresholds and engine gates.
- Scoped `apps/backend/AGENTS.md`, existing delivery tests and nearby maintenance patterns.

## Risks

The live-session/incarnation lookup runs under the shared lock on every event. Include its indexed-read and lock cost in Task 08 foreground p95 measurements.
FK cascades erase turn ownership before sessionless effect cleanup can infer it. Identity capture and binding must precede deletion. Unknown legacy rows cannot be attributed by parsing an arbitrary key.

## Parallelism

`sequential`. No delegation is authorized.

## Results

Implemented transactional deletion retirement, attributable legacy workflow-receipt binding, bounded phased purge and fail-closed orphan discovery. Session/task/workspace/ephemeral deletion and the final managed deletion commit schedule retirement jobs; archive, Stop, reserved/prepared/released managed attempts and pre-existing replacements preserve delivery evidence. Deterministic ordinary and managed conversation IDs remain fenced until purge, then admit a fresh incarnation. E2E reset quiescence remains owned by Task 06.

The new SQLite repository scenarios were first run while the retirement methods were absent and failed to compile against the new repository contract. After the transaction methods and service admission paths were implemented, `TestDeliveryRetentionDeletion`, `TestDeliveryRetentionOrphans` and `TestDeliveryRetentionReplacement` passed on SQLite and real PostgreSQL. The replacement group covers concurrent create/purge, committed retry, and replacement preservation. Service tests cover deterministic ID reuse, ordinary/managed primary repair, coordinator deletion and managed finalization replay. No deletion event path was replaced; existing task/session event behavior remains in place.

| Exact check | Result |
| --- | --- |
| `go test -trimpath ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Deletion|Orphans|Replacement)' -count=1` | Passed on SQLite, 8.876s. |
| `go test -trimpath ./internal/task/service -run 'TestDeliveryRetentionDeletion|TestEnsureWaitsForDeliveryRetirementBeforePrimaryRepairOverRealRepository|TestManagedDeletionAdmissionOwnershipRecovery/committed_replay' -count=1` | Passed, 2.048s. |
| `KANDEV_TEST_POSTGRES_DSN='postgres://kandev:kandev_test@127.0.0.1:54489/kandev_test?sslmode=disable' go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Deletion|Orphans|Replacement)|TestDeliveryRetentionDeletionRetirement|TestDeliveryRetentionAuthenticatedAdmission' -count=1` | Passed on SQLite and real PostgreSQL, including race-enabled replacement/create/purge coverage; package completed in 56.525s. |
| `go test -trimpath -race ./internal/task/service -run 'TestDeliveryRetentionDeletion|TestEnsureWaitsForDeliveryRetirementBeforePrimaryRepairOverRealRepository|TestManagedDeletionAdmissionOwnershipRecovery/committed_replay' -count=1` | Passed, 1.685s. |
| `gofmt -l $(git diff --name-only -- '*.go'; git ls-files --others --exclude-standard '*.go')` and `git diff --check` | Passed after formatting two existing changed test files. |

The PostgreSQL run executed the environment-gated subtests; none were skipped. No feature flag, staging or commit was added.
