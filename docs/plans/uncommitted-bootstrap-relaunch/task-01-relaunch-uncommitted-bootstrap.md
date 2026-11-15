---
id: "01-relaunch-uncommitted-bootstrap"
title: "Relaunch replays an uncommitted first-generation stream"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 01: Relaunch replays an uncommitted first-generation stream

## Summary

Resume or Start fresh of a session whose first launch never committed a native
conversation must pass delivery preflight. It replays the retained g1 setup
events from zero through the existing bootstrap processor, instead of failing
with "agent delivery checkpoint missing". Every existing refusal stays in place.

Managed process-exit errors must also retain their producer manager identity.
The replacement lifecycle compares that identity before it applies the local
process generation, so a failed predecessor's generation-one error cannot fail
the new generation-one child.

## In scope

- Remove the `isResumedSession` condition from `canReplayFreshBootstrapEvents`
  in `apps/backend/internal/agent/runtime/lifecycle/delivery_stream_preflight.go`,
  and update its comment.
- Update `TestDeliveryPreflightRetainedStartupEventBeforeNativeInitialization`.
  Add `TestDeliveryPreflightRelaunchKeepsDurableRefusals`.
- Identify each process-manager lifetime in managed exit events and the
  authenticated delivery descriptor. Reject replayed errors from a different
  manager before lifecycle handles them.
- Add a lifecycle regression that blocks receipt and acknowledgment until the
  replacement generation commits, then checks that the predecessor error is
  ignored even when both managers use process generation one.

## Out of scope

- Orchestrator, executor, generation advancement, journal mutation, UI, schema,
  and metrics.

## Acceptance

1. A launch with `isResumedSession = true` returns cursor 0 with no error and
   leaves the SQL cursor absent. The launch also needs generation 1, bootstrap
   set, no native or required native ID, no committed generation, no cursor, no
   submissions, `FirstRetained` 1, and `Acknowledged` 0.
2. The same relaunch still fails if any of these holds: a native session ID, a
   required native conversation, a submission count, pruned history
   (`ErrAgentDeliveryHistoryGap`), or a committed harness generation.
3. The updated regression assertion fails on the current code before the
   production change.

## Verification

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run 'TestDeliveryPreflight')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle)
(bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/delivery_stream_preflight.go apps/backend/internal/agent/runtime/lifecycle/delivery_stream_bootstrap_preflight_test.go)
(python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/delivery_stream_preflight.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_stream_bootstrap_preflight_test.go`

## Dependencies

None.

## Risks

- Another caller might depend on `canReplayFreshBootstrapEvents` refusing
  resumed launches. Today its only caller is `authenticatedDeliveryReplayCursor`.
  The full lifecycle race suite guards against regressions.

## Parallelism

`sequential`

## Inputs

- `durable-agent-stream-processing.md`, sections "Resumed bootstrap recovery",
  "Explicit fresh start", and "Uncommitted first-generation relaunch".
- Existing test pattern in `delivery_stream_bootstrap_preflight_test.go`.

## Results

- `(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run 'TestDeliveryPreflight')`:
  before the fix, `TestDeliveryPreflightRetainedStartupEventBeforeNativeInitialization`
  failed with "agent delivery checkpoint missing" on the relaunch assertion.
  After the fix, it passed (`ok`).
- `(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle)`: `ok` (214.9s).
- `bash scripts/lint-go-changed <both changed Go files>`: 0 issues.
- `python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all`:
  420 decisions and 1850 specifications validated, and all specification files passed.

Review follow-up:

- Added a per-manager UUID to managed exit events and to the authenticated
  delivery descriptor. Lifecycle compares this identity before the manager-local
  process generation.
- `TestBootstrapReplayIgnoresExitFromFailedPredecessorManager` covers an old
  terminal error replayed after replacement start. It checks that receipt,
  callback and ACK wait for the generation-one commit, and that the old error
  is retired when both manager-local counters equal one. It also checks that a
  legacy unmarked exit is retired when the current descriptor identifies its
  manager.
- `TestProcessManagerIdentityScopesRecoveryDescriptor` checks that each manager
  gets a distinct identity and the descriptor returns its manager's identity.
