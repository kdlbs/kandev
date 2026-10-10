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

### Completion admission remediation, 2026-10-06

- `TestPromptTaskWithoutExecutorLeavesSessionUnchanged` reproduced a nil-executor panic before the fix. Prompt admission now rejects the missing runtime before claiming a turn or changing session state. Its race regression passed.
- `TestCompleteStreamGitSnapshotDoesNotHoldPromptAdmission` reproduced completion holding the session guard during a blocked Git read. Ordinary chats now finish guarded settlement and release admission before the synchronous snapshot. The test passed and checks that the snapshot finishes before handler return and cannot overwrite a successor's RUNNING state. Office and automation retain capture before runtime teardown.
- Passed `go test -race -tags sqlite_fts5 ./internal/orchestrator -run '^TestCompleteStreamGitSnapshotDoesNotHoldPromptAdmission$|^TestPromptTaskWithoutExecutorLeavesSessionUnchanged$' -count=1` and `go test -race -tags sqlite_fts5 ./internal/orchestrator -run 'TestPromptTask|Test.*RecoveryBlock|TestQueueAndInterruptForPeerMessage' -count=1`.
- The Office terminal fixture waits for its asynchronous stop before checking the exact count. The boot simulator's one-shot readiness signal tolerates repeated start calls while preserving launch and prompt assertions. Both focused race cases passed ten repetitions each.
- Passed the full `go test -race -tags sqlite_fts5 ./internal/orchestrator -count=1` suite (292.829 seconds). Hosted checks remain pending. PostgreSQL and native Windows/macOS containment evidence remain separate release requirements.
- Passed `go test -race -tags sqlite_fts5 ./internal/orchestrator -run '^TestCompleteStreamGitSnapshot|^TestPromptTaskWithoutExecutorLeavesSessionUnchanged$' -count=1`, including snapshot persistence before Office stop. `golangci-lint run ./internal/orchestrator/... --allow-serial-runners --timeout=5m` passed with zero issues. Backend and plugin-package builds passed.
- Passed `pnpm e2e:run --host --no-build --project mobile-chrome -- tests/chat/mobile-agent-goal.spec.ts --retries=0` (two cases, 52.9 seconds) and `pnpm e2e:run --host --no-build --project chromium -- tests/chat/quick-chat-cancel-palette.spec.ts tests/task/file-tree-download.spec.ts --retries=0` (five cases, two minutes). No retry was exercised. A deterministic WebSocket barrier preserves cancellation-pending UI assertions when native acknowledgement clears pending within milliseconds, releases all buffered frames, and then verifies final settlement. No cancellation behavior, deadline, or assertion was relaxed.
- Integration with main `3908fca85f23093ad7f8b34a361dde13151d4761` preserves its capacity continuation's native/live-execution/authorization fences alongside this branch's fresh durable submission IDs and delivery identity. Passed `go test -race -tags sqlite_fts5 ./internal/orchestrator/... ./internal/agentctl/journal/... ./internal/agentctl/server/process/... ./internal/agent/runtime/lifecycle/... -count=1` (orchestrator 220.541 seconds, process 280.490 seconds, lifecycle 176.328 seconds). Passed `golangci-lint run ./... --allow-serial-runners --new-from-rev=3908fca85f23093ad7f8b34a361dde13151d4761 --timeout=10m` with zero issues, all eight changed frontend test files (76 tests), catalog/full spec lint, web typecheck, and backend/plugin/web E2E builds. Hosted checks on the final head remain pending.
- Current-main browser validation passed all nine cases without retries: the mobile goal file (two cases, 45.4 seconds); `pnpm e2e:run --host --no-build --project mobile-chrome -- tests/session/mobile-transient-turn-runtime-continuity.spec.ts --grep 'completed tools continue' --retries=0` (one case, 11.2 seconds); the desktop cancellation/download command above (five cases, 1.9 minutes); and `pnpm e2e:run --host --no-build --project chromium -- tests/session/transient-turn-runtime-continuity.spec.ts --grep 'completed tools continue' --retries=0` (one case, 13.0 seconds). The continuation checks assert the same native runtime across reload and a second viewer, preserving completed tools.

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

### CI remediation, 2026-10-02

- Unnumbered cancellation terminals complete only the exact active submission. Generation-bound event ownership takes precedence over a concurrent successor dispatch. Late predecessor terminals preserve the successor status and completion signal.
- Workspace-only recovery now projects the persisted incarnation, current harness generation, and stream into executor creation. Generation storage failures reject recovery; SQL identity fences remain unchanged.
- The missing-owner regression failed before the fix. Passed `GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/task/service -count=1`, covering current-generation restore and failed generation reads.
- Passed desktop cancellation with `pnpm e2e:raw --project=chromium e2e/tests/session/session-recovery.spec.ts --grep 'pausing an accepted lazy resume' --retries=0 --reporter=line` and phone cancellation with `pnpm e2e:raw --project=mobile-chrome e2e/tests/session/mobile-session-resume-recovery.spec.ts --grep 'pausing an accepted lazy resume' --retries=0 --reporter=line`. Both preserve runtime identity across two pauses and a follow-up prompt.
- Backend lint and complete locale validation passed after rebasing onto current main. Published-head CI and review verification remain pending.

### CI remediation, 2026-10-06

- Local reproduction of the Quick Chat cancellation CI failure exposed a terminal-first completion race: ordered SQL projection could settle a submission before the prompt RPC returned. Its return handler then incorrectly reported `backend_delivery_completion_failed`. The handler now accepts the matching submission's persisted terminal state without overwriting its outcome. Unresolved and mismatched submissions remain errors.
- `TestDeliveryCompletionAfterTerminalProjection` failed before the fix for completed, failed, and cancelled outcomes. After the fix it also verifies preserved outcomes, rejection of unresolved states, and rejection of another session's terminal outcome.
- Passed `go test -race ./internal/orchestrator -run 'TestDeliveryCompletionAfterTerminalProjection|TestPrepareAgentDeliverySubmission|TestDeliveryTerminal|TestDeliverySettlement' -count=1` and the complete `go test -race ./internal/orchestrator -count=1` suite.
- Passed `make -C apps/backend lint` with zero issues, document catalog validation, full specification lint, and whitespace checks.
- Separate test fixture corrections preserve conditional request validators, await actual emitted transcript content, retry Files activation with its click, and seed PR-author policy checks in their required Done state. Final focused checks passed 15 desktop repetitions and six mobile conditional-refresh/stale-response repetitions; no test retry was exercised.
- Passed `go test -race ./internal/task/repository/sqlite -run 'TestDeliveryTerminal|TestDeliverySettlement|TestCancelledCompleteEvent' -count=1`, web typecheck, and focused ESLint/Prettier checks.
- Passed all three Quick Chat cancellation cases and both PR-author visibility cases with `pnpm e2e:run --host --no-build --project chromium -- tests/chat/quick-chat-cancel-palette.spec.ts tests/pr/pr-approve-button.spec.ts --retries=0` (five passed, no retry exercised).
- Hosted checks must be rerun on the pushed revision. Native Windows/macOS containment and live harness relocation validation remain separate release gates.

### Journal admission remediation, 2026-10-06

- `TestSubmissionCompletionReadyBeforeDispatchReturns` reproduced admission remaining blocked after a replayable completion while the native dispatch callback had not returned. A definitive completion now commits the matching dispatching submission's completed state with its terminal event. Event ownership is fenced by session, incarnation, harness generation, and bound stream. Other recorded outcomes remain unchanged.
- Passed `go test -race ./internal/agentctl/journal -count=1`, including reopen-before-dispatch-return, wrong-owner rejection, atomic batch rollback, cancellation, and uncertain-outcome preservation.
- Passed `go test -race ./internal/agentctl/server/process/... ./internal/agentctl/server/api/...` (process 354 seconds; API 258 seconds), backend lint with zero issues, and rebuilt backend, agentctl, mock agent, and plugin fixture.
- The mobile PR-only commit CI failure remains unconfirmed: three isolated repetitions and all nine mobile changes cases passed without retries. Failure attachments now preserve the backend log for both mobile changes and Quick Chat cancellation. Assertions, budgets, and retry policy remain unchanged.
- Fresh hosted checks and review evidence remain pending. The cancelled Windows process job was retriggered on the previous published revision. This journal fix does not replace the native Windows/macOS containment or live harness release gates.
- `TestSubmissionCompletionSurvivesDispatchError` additionally failed for both a late transport error and a deterministic native request error after saved completion. The dispatch error handler now preserves that definitive completed record; without terminal evidence, existing failure and uncertainty handling remains authoritative. All `TestSubmission` race regressions passed after the fix.
- Final return-handler validation passed `go test -race ./internal/agentctl/server/process/... ./internal/agentctl/server/api/...` (process 245 seconds; API 156 seconds), backend lint with zero issues, the rebuilt backend/plugin fixture, and all three Quick Chat cancellation cases without retries. The preceding atomic-journal revision passed the same complete Quick Chat sequence twice (six cases). Catalog validation, full specification lint, affected browser-test ESLint/Prettier checks, and whitespace checks passed.
