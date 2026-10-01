---
id: "03-outcome-settlement"
title: "Resolve only delivery blocks proved settled by durable evidence"
status: done
wave: 3
depends_on: ["02-repeatable-reconciliation"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 03: Resolve only delivery blocks proved settled by durable evidence

## Summary

Resolve only delivery blocks proved settled by durable evidence. Preserve original ownership and the no-resend contract.

## In scope

Add typed persisted binding from a delivery recovery block to its original submission and stream/turn identity.
Audit single-open-block coalescing. Preserve independent causes instead of letting automatic delivery settlement erase them.
Add registered additive migration and safe legacy-record handling. Unknown old bindings remain blocked.
Reconcile completed, failed, and cancelled terminal outcomes after ordered projection through existing repository/queue authority.
Resolve exactly the matching block by compare-and-set; never call the broad manual resolver solely because a socket connected.
Persist/reuse a settlement intent so a crash after projection but before block/queue settlement is repairable.
Cover already-projected terminals, duplicate callbacks, and terminal-before-error-block races.
Recheck queue, Office, automation, Send Now, and steer guards after settlement without replaying the original prompt.

## Out of scope

Other work orders, contributor-owned executor redial, long-horizon scheduling, and detached MCP policy.

## Acceptance

- All named regressions exercise the actual owning boundary and pass.
- No stale owner, unrelated recovery cause, or automatic resend crosses the repaired path.
- Partial failure remains visible, bounded, and restart-reconcilable without deleting retained evidence.

## Tests

TestDeliveryTerminalSettlesMatchingBlock uses real journal and SQL stores, not interface fakes.
TestDeliveryTerminalPreservesOtherRecoveryCause includes mixed delivery and operator/native recovery causes.
TestDeliverySettlementCrashBoundaries crashes after each durable stage and repeats reconciliation, including cursor already beyond terminal.
TestDeliverySettlementRejectsOldSubmission delivers an old terminal after a new block/generation exists.
TestDeliveryBlockUpgradeUnboundRecord keeps ambiguous historical blocks open; test real queue claim release exactly once.

## Verification

Run from repository root after implementation. All commands are independently rooted.

```bash
(cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/office/service ./internal/automation -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Persistence evidence must include fresh boot, replay/reopen, pre-change upgrade, interrupted settlement, and PostgreSQL conformance. Record unavailable PostgreSQL explicitly; do not count skipped tests as passed.

## Files likely touched

- `apps/backend/internal/task/models/session_continuity.go`
- `apps/backend/internal/task/repository/sqlite/session_continuity.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/orchestrator/agent_delivery_submission.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_adoption.go`

Add the named regression files beside the owning source packages.

## Dependencies

Task 02: Unify disconnect and later retry under one reconciler.

## Risks

Concurrent old events and new ownership can race settlement or cleanup. Use immutable identity and compare-and-set at the mutation boundary.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md).
- [Requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Design](../../specs/platform/system-design/durable-agent-reattachment.md).

## Results

Completed 2026-09-28.

- Delivery recovery records bind the original submission to its session, execution, incarnation, harness generation, stream, and turn identity. Terminal settlement uses durable intent plus repository compare-and-set, clears only the exact matching delivery block, and preserves unrelated recovery causes. Unbound legacy records remain blocked.
- Ordered terminal projection is restart-repairable, including when the terminal is already past the stream cursor. Queue cleanup is idempotent. Replayed workflow effects settle the session only when no active successor exists; a regression test proved the replay path had left a completed session RUNNING and now protects both cases.
- Passed terminal, duplicate, old-submission, legacy-upgrade, crash-boundary, and unrelated-cause regressions with `go test -race ./internal/task/repository/sqlite -run '^(TestDeliveryTerminal.*|TestDeliverySettlement.*|TestDeliveryBlockUpgradeUnboundRecord)$' -count=1`.
- Race coverage passed for SQLite, orchestrator, Office, and automation in the five-package run. The complete lifecycle package passed separately with `go test -race ./internal/agent/runtime/lifecycle -count=1` using a private `/tmp` mount. The runtime-loss regression now waits for the identity-bound recovery notice and reads mutable execution state under its store lock.
- `go run ./cmd/sqlguard ./internal`, `go test -race ./internal/persistence/storeconformance -count=1`, backend lint, document catalog validation, full spec lint, and whitespace checks passed. Store conformance used SQLite; PostgreSQL coverage was skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured.
- Native Windows/macOS containment tests remain release gates. No test or implementation changes were committed.

### CI remediation, 2026-10-01

- Event projection now owns `agent_delivery.event:<stream>:<sequence>`; the orchestrator exclusively owns workflow transition deduplication. The dynamic workflow completion browser regression passes.
- Explicit cancellation commits its terminal record and cancelled submission in one journal transaction. Repeated cancellation preserves one terminal. A capacity failure rolls back both, including after reopening the journal.
- Terminal projection maps wire `complete`/`error` and `data.stop_reason=cancelled` to submission outcomes before lifecycle callback suppression. SQL settlement uses the same cancellation meaning.
- Passed `go test -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/task/repository/sqlite ./internal/backendapp -run 'TestProjectionSettlesTerminal|TestCancel|Test.*Delivery|TestPrepareExecutionCreateRequest' -count=1`.
- Passed all five desktop/mobile cancellation browser regressions with `pnpm e2e:raw --project=chromium --project=mobile-chrome e2e/tests/session/pause-resume-recovery.spec.ts e2e/tests/session/mobile-pause-resume-recovery.spec.ts --retries=0 --reporter=line`. Per-spec retry overrides were removed.
- Passed the complete five affected backend packages with `go test -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/task/repository/sqlite ./internal/backendapp -count=1`. Backend lint, web typecheck, affected web lint/helper tests, catalog/spec lint, and whitespace passed before the subsequent Docker stop-policy fix. Current-head CI remains pending publication.

- Follow-up CI found `TestDeliveryAPICancelsCurrentSubmission` seeded an unbound stream directly in the journal. The fixture now uses production `AdmitDeliverySubmission` and asserts a replayable terminal for the exact submission. The unchanged production identity fence remains enforced. Reproduced the failure, then passed `go test -race -tags fts5 ./internal/agentctl/server/api -count=1` (64 seconds).
