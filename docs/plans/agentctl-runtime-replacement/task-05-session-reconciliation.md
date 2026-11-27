---
id: "05-session-reconciliation"
title: "Recover durable session evidence after runtime replacement"
status: done
wave: 5
depends_on: ["04-replacement-coordinator"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003
acceptance_criteria:
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.5
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.1
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.2
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.3
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.4
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.5
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-003.6
system_design:
  - ../../specs/platform/system-design/agent-runtime-availability.md
---

# Task 05: Recover durable session evidence after runtime replacement

## Summary

Recover durable session evidence after runtime replacement. Preserve original session authority and all prior durable-delivery fixes.

## In scope

Compose runtime epoch fencing with existing session admission, durable journal replay, terminal effect, and queue-claim ownership.
Reconstruct original stream, incarnation, harness generation, turn, submission, workspace, and native-state identity.
Allow evidence-only journal reopening after exclusive ownership proof; do not start a harness merely to inspect retained outcomes.
Project retained output before terminal completion. Recover one outcome across duplicate callback/restart schedules.
Keep missing terminal, journal loss, unknown tool effects, and ambiguous child ownership persistently blocked.
Keep idle native sessions resumable through the existing native restore path; no automatic prompt or context continuation.
Allow independent new sessions after runtime publication while old uncertain sessions remain blocked.
Exercise Stop while runtime control is lost; report unconfirmed cancellation truthfully.
Keep queue drain, Send Now, steer, manual resume, Office, and automation guards under their existing authority.
Ensure task stall recovery cannot erase durable uncertainty or release a fenced submission.

## Out of scope

Other work orders, unproven Git repairs, automatic external mutation retries, and unrelated executor changes.

## Acceptance

- The scoped outcome passes every named regression, including stale-owner and failure cases.
- No prompt, tool, or Git mutation is replayed by runtime replacement.
- Partial failure leaves accurate availability and admission fences; cleanup affects only owned resources.

## Tests

TestRuntimeReplacementReplaysTerminalOnce; TestRuntimeReplacementUncertainSubmission; TestRuntimeReplacementAllowsIndependentSession; TestRuntimeReplacementStopUnknownOwner; TestRuntimeReplacementAdmissionEntrypoints.
Use real journal and SQL stores. Cover terminal committed before crash, before/after ACK, active tool ambiguity, idle session, legacy session, and corrupt journal.
Delay an old terminal until a successor exists and assert no successor mutation or workflow advance.
Reopen persisted state after backend interruption during child replacement and verify the same identities and fences.
Proposed tests belong beside their production owners. Verify the command selects them before recording success.

## Verification

Run each command from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/repository/sqlite ./internal/task/service ./internal/office/service ./internal/automation -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

For persisted identity changes, include fresh boot, reopen, prior-schema upgrade, interrupted migration, and PostgreSQL conformance. Record unavailable infrastructure as an incomplete gate.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/durable_adoption.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_recovery_admission.go`
- `apps/backend/internal/orchestrator/agent_delivery_submission.go`
- `apps/backend/internal/task/service`
- `apps/backend/internal/office/service`
- `apps/backend/internal/automation`

New runtime-owner/coordinator and regression files are expected beside these owners.

## Dependencies

Task 04: Coordinate bounded replacement within the healthy backend.

## Risks

Delayed callbacks and partial binding can affect a successor. Prove generation ownership before every state-changing result.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md).
- [Requirements](../../specs/platform/requirements/agent-runtime-availability.md).
- [Design](../../specs/platform/system-design/agent-runtime-availability.md).
- [Decision](../../decisions/2026-09-27-agentctl-runtime-replacement.md).

## Results

Completed on 2026-09-27.

- Runtime loss is scoped to the local standalone executions owned by the lost boot and epoch. Remote, idle, and unrelated sessions remain isolated.
- Retired stream disconnects preserve submission identity and uncertainty. The uncertainty code is persisted before the prompt completion signal, so canonical failure handling cannot race ahead and erase it. No prompt or external action is resent.
- Passed `go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/repository/sqlite ./internal/task/service ./internal/office/service ./internal/automation -count=1`, `go run ./cmd/sqlguard ./internal`, `go test -race ./internal/persistence/storeconformance -count=1`, and `make -C apps/backend lint`.
- SQLite record tests cover fresh schema, reopen, legacy columns, and interrupted migration replay. PostgreSQL conformance is still a release gate because `KANDEV_TEST_POSTGRES_DSN` is not configured in this environment.
- Product OS process-containment tests on native Windows and macOS remain release gates.
