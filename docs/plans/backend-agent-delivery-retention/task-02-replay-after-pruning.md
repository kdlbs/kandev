---
id: "02-replay-after-pruning"
title: "Handle compact and expired replay without resending"
status: completed
wave: 2
depends_on:
  - 01-schema-and-format
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-003
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-002.1
  - AC-PLATFORM-DELIVERY-RETENTION-002.2
  - AC-PLATFORM-DELIVERY-RETENTION-002.3
  - AC-PLATFORM-DELIVERY-RETENTION-002.4
  - AC-PLATFORM-DELIVERY-RETENTION-003.2
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 02: Handle compact and expired replay without resending

## Summary

Make receipt-aware receive/project and authenticated state-only ACK recovery work before enabling any collector. Provide the behavior that makes later deletion safe.

## In scope

- Define hash version 1 using exact existing payload normalization, length framing and tagged raw-byte fallback. Add compact/pruned models and typed local expiry errors.
- Prevent absent old rows/cursors from becoming new input; enforce owner, session, incarnation, harness generation, stream and submission identity before receipt checks.
- Add metadata-only paginated submission reads for adoption; full-payload readers return typed pruning results. Test bounded pagination across retained history and active/uncertain identities.
- Make release N read/write format-1 evidence, preserve settlement metadata/jobs and deny retired-ID recreation in every creation variant. Journal exact retirement atomically before canonical deletion on all cascade paths, without purging rows. N ships no collector or destructive maintenance entry point; Task 05 adds draining/orphan discovery and audits coverage.
- Cover native managed primary creation through `repairManagedPrimaryTx`; journal retirement only in final managed delete/committed-marker transaction, never reserved/prepared admission.
- Preserve terminal-first RPC completion, current structural metadata CAS, accepted initial-prompt identity and provider continuation with distinct fresh submission IDs. Pruning cannot trigger a new send or hide uncertain tools/permissions. Hash persisted source diagnostics even when canonical notifications omit them.
- Update lifecycle duplicate/project paths and ACK recovery: identical compact event is a no-op; expired event reconnects through exact SQL P and authenticated peer state.
- Test altered text/tool/terminal/identity versus permitted reconstructed lifecycle fields, malformed JSON equality, unknown hashes, missing checkpoint and stale owner transfer.
- Exercise lost ACK request, lost success response, expired local replay window, remote journal expiration and quiet terminal recovery. Count harness sends, ACP initialization, canonical appends and workflow transitions.

## Out of scope

Implementation was authorized by the user on 2026-10-09.
Agentctl journal policy, canonical-history expiry, new UI and feature flags are excluded.
This order does not authorize commit, push, merge or delegation.

## Acceptance

- Matching compact replay produces no canonical/effect writes; conflicting content fails. Expired replay performs no insertion or event-derived ACK advancement.
- Lost ACK recovery uses the retained exact projected cursor and authenticated owner, with no prompt/tool resend or conversation replacement.
- Release N deletion journals retirement without collection; rollback-shaped jobs and compact evidence survive new writes and forward upgrade.
- Missing/inconsistent evidence remains blocked and leaves independent causes intact; completed prompt IDs preserve no-resend/hash-conflict behavior.

## TDD evidence

Start each changed-logic scenario with a failing test against the pre-change behavior. Record Red, Green and refactor results.
Use existing backend test helpers, fake clocks and deterministic barriers. Do not use sleeps to manufacture race order.
Required new test groups: `TestDeliveryRetentionReplay`, `TestDeliveryRetentionHash`, `TestDeliveryRetentionLostAck`, `TestDeliveryRetentionNoResend`, `TestDeliveryRetentionMetadataPagination`, `TestDeliveryRetentionRetirementAdmission`, `TestDeliveryRetentionCompatibilityReader`.
Names are planned deliverables, not existing passing tests. Before using a -run selector, list tests and confirm every required group exists.
The real repository and retained journal must prove state transitions where named; fakes do not prove database crash semantics.
Read `.agents/skills/tdd/SKILL.md` and its `references/backend-tests.md` at implementation time.

## Verification

Run from repository root. Commands are implementation-time checks, not run during planning.

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Hash|Replay|MetadataPagination|RetirementAdmission|CompatibilityReader)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'TestDeliveryRetention(LostAck|Replay|NoResend)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'TestDeliveryRetentionNoResend|TestDeliveryCompletionAfterTerminalProjection|TestInitialTaskSubmissionReceiptSurvivesRequestCancellation' -count=1)
```

The SQL hash/replay, bounded metadata pagination, creation/deletion retirement admission and format-1 compatible reader/writer tests run SQLite and PostgreSQL subtests using the real repository. Require populated N+1-shaped evidence fixtures (not just DDL), new writes, pruned prompt IDs and pending jobs; freeze a real N build artifact before Task 03. The lifecycle lost-ACK tests reopen the database and retained journal, not only fake interface responses.
For every environment-gated test, record executed and skipped subtests. A skip cannot complete the corresponding acceptance.
The package helper checks document links/IDs; it does not prove runtime behavior.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/agent_delivery.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_state.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/task/repository/repoerrors/`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_adoption.go`
- `apps/backend/internal/orchestrator/agent_delivery_submission.go`
- `apps/backend/internal/task/repository/sqlite/managed_conversation_admission.go`
- `apps/backend/internal/task/repository/sqlite/managed_deletion.go`

Add focused `_test.go` files beside changed logic; split oversized tests rather than extending files past lint limits.
Paths marked new are intended deliverables. Other directory references identify the existing integration boundary.

## Dependencies

Complete `01-schema-and-format` first. Execute sequentially in the primary session.

## Inputs

- [Requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md).
- [Design](../../specs/platform/system-design/backend-agent-delivery-retention.md); sections matching this scope.
- [Plan](plan.md), including baseline, storage thresholds and engine gates.
- Scoped `apps/backend/AGENTS.md`, existing delivery tests and nearby maintenance patterns.

## Risks

Current duplicate paths can retry terminal projection. Every caller must recognize compact receipts before reconstructing an effect. The indexed live-session/incarnation read and shared lock occur on every streamed event; Task 08 measures their foreground cost against the pre-retention baseline.

## Parallelism

`sequential`. No delegation is authorized.

## Results

Implemented receipt-aware replay, hash-v1 identity, metadata-only submission pagination, authenticated lifecycle admission and state-only lost-ACK recovery. Durable event admission now checks the live task/session incarnation, execution owner, current harness generation, exact stream and associated submission under the shared session/stream locks before reading or advancing its cursor. Executor owner replacement uses the same turn/delivery lock order. All session/task/workspace/ephemeral/quick-chat deletion paths now write retirement jobs transactionally before canonical deletion; managed reservation/preparation remains non-retiring and finalization journals only in its committed delete transaction. Compact evidence remains untouched in Release N.

TDD evidence: the authenticated owner test failed on both SQLite and PostgreSQL because a replaced execution was accepted; after the SQL owner guard, wrong execution, unknown submission, wrong stream and post-transfer events reject before cursor mutation, while the current owner succeeds. The deletion matrix initially failed on five deletion routes due missing jobs; all routes and rollback cases now pass. The lifecycle no-resend case initially had no live SQL owner and failed closed; after adding a real task/session/execution/generation fixture it recovers only the projected ACK cursor, with zero callbacks/effects/resends. The plugin inventory/resume race initially deadlocked on PostgreSQL; moving recovery/task admission before turn and delivery locks restored the required lock order.

Executed checks, all passing:

```bash
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='postgres://kandev:kandev_test@127.0.0.1:54489/kandev_test?sslmode=disable' go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Hash|Replay|MetadataPagination|RetirementAdmission|CompatibilityReader)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'TestDeliveryRetention(LostAck|Replay|NoResend)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'TestDeliveryRetentionNoResend|TestDeliveryCompletionAfterTerminalProjection|TestInitialTaskSubmissionReceiptSurvivesRequestCancellation' -count=1)
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='postgres://kandev:kandev_test@127.0.0.1:54489/kandev_test?sslmode=disable' go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention|TestDeleteTaskSessionDoesNotDeleteReplacementIncarnation|TestTaskSessionQueueIncarnationIsGeneratedProjectedAndRenewed|TestPluginExecutorPostgresInventoryCAS' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -count=1)
(cd apps/backend && make build-kandev)
```

Both SQLite and real PostgreSQL subtests executed for the retention repository matrix. The full SQLite package race run hit its 10-minute test timeout while running `TestSidebarQueryPoolMemoryPlateau`; it exposed the plugin lock-order and session-ID-reuse fixture issues above, which now pass their targeted regression checks. No cleanup worker or destructive maintenance entry point is included in Release N.

Frozen Release N binary (Linux/amd64): `/tmp/kandev-delivery-retention-release-n-20261009/kandev`, version `v0.97.0-143-gd7a44e96ed-dirty`, commit `d7a44e96ede93d50c6277773f4174de716272f02`, SHA-256 `3df1e310220f0a7588453c062f2fa90aa0a611dd7277cc77c76dd4f8bc3a0517`. A separate pre-lock-fix candidate remains at `/tmp/kandev-delivery-retention-release-n-20261009/kandev-before-lock-fix`; the frozen artifact includes the final WO02 lock ordering and must remain unchanged through rollback validation.
