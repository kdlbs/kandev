---
id: "01-propagate-cancel-client"
title: "Carry the pinned client through cancel escalation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.2
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.1
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.2
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.3
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9
system_design:
  - ../../specs/tasks/system-design/workflow-cancelled-turn-completion.md
  - ../../specs/tasks/system-design/session-delete-resource-cleanup.md
---

# Task 01: Carry the pinned client through cancel escalation

## Summary

Make every escalation branch use the client already leased by `cancelAgentExecution`.
Prove a queued Stop and a concurrent forced deletion-style stop finish after cancellation settles.
Preserve session-owned workspace restoration, exact-runtime deletion fencing, and existing desktop and phone actions.

## In scope

- Add deterministic regression tests before changing production code.
- Pass the captured client through all helper paths listed in the plan.
- Replace the optional client argument on `escalateStuckCancel` with a required argument.
- Remove that helper's implicit `AcquireAgentCtlClient` fallback.
- Adapt direct helper tests to the explicit argument.
- Prove durable settlement retains the original client until its lease releases.
- Run the targeted backend, lint, and existing browser checks.

## Out of scope

- New mutex implementations, goroutines, product timeouts, or runtime flags.
- Frontend production changes or a different restoration identity model.
- Session/workspace deletion policy, filesystem cleanup, or operator recovery of the reported live instance.
- Provider adapter changes or live-provider certification.

## Acceptance

1. Every cancellation branch completes with a queued stop writer, without a nested read lease or test-owned unlock of production state.
2. Escalation preserves captured identities, durable settlement, ordered completion, successor safety, and existing error propagation.
3. Concurrent stop/deletion quiescence drains, failed stop retains the session, and existing sibling workspace and phone flows pass.

## Implementation and TDD

1. Read the linked requirements, design amendment, and the plan's root-cause and test sections.
2. Add `TestCancelAgentEscalationUsesPinnedClient`, `TestCancelStopAndDeleteQuiescenceComplete`, and `TestCancelEscalationSettlesCapturedDurableClientBeforeStop`.
3. Run the new tests against unchanged production source and record the expected nested-lock failure.
4. Keep an irrecoverable RED schedule inside a bounded subprocess that the parent always terminates and joins.
5. Pass the client through `waitForPromptFinishedAfterCancel`, `cancelDispatchOnlyPrompt`, `waitForCapturedDispatchCompletion`, and `finishDispatchCancelTimeout`.
6. Require the explicit client in `escalateStuckCancel` and remove its fallback lease acquisition.
7. Adapt direct callers in existing dispatch-cancellation and lock-order tests.
8. Run all verification commands from the repository root.
9. Record actual results and mark both this work order and the plan complete only after their checks pass.

The regression matrix includes ordinary timeout, dispatch rejection, disconnection, missing dispatch gate, and timeout with a transport error.
The overlap fixture uses the real `CancelAgent` and `StopAgentWithReason` paths with mock transport and runtime dependencies.
It queues an ordinary `force=false` stop behind cancellation's client lease, then queues a force-enabled `session deleted` stop on the same execution's runtime lifecycle lock.
Existing orchestrator tests supply the separate real DeleteSession boundary evidence.

Do not shorten production waits to satisfy the tests.
The fixture can shorten package test durations and must restore them during cleanup.
Use explicit barriers to establish lock ownership and join every test goroutine.
Preserve the existing async-ready regression and late-frame generation checks.

## Verification

Run these commands from the repository root after the RED phase and implementation.
The first command owns the new regressions and every test file that needs a helper-signature update.

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestCancelAgentEscalationUsesPinnedClient|TestCancelStopAndDeleteQuiescenceComplete|TestCancelEscalationSettlesCapturedDurableClientBeforeStop|TestManager_CancelAgent_|TestMarkReadyDoesNotDeadlockWithPromptCompletion|TestMarkReadyAsync_)' -count=1 -timeout=180s)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestEnsureWorkspaceExecutionForSession_CreatesOwnExecutionInSharedTaskEnvironment|TestGetOrEnsureExecutionForEnvironmentDoesNotShareSiblingUnderSessionKey)$' -count=1 -timeout=120s)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator ./internal/orchestrator/executor -run '^(TestDeleteSession_|TestLaunchRestoreWorkspace_|TestStopExecutionLogsOnlyRealFailures|TestStopByTaskID_DoesNotStopReplacementExecution|TestStopByTaskID_UnloadableRecoveryDoesNotStopReplacementExecution)' -count=1 -timeout=180s)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go apps/backend/internal/agent/runtime/lifecycle/manager_interaction_dispatch_cancel.go apps/backend/internal/agent/runtime/lifecycle/manager_interaction_cancel_stop_test.go apps/backend/internal/agent/runtime/lifecycle/manager_interaction_dispatch_cancel_test.go apps/backend/internal/agent/runtime/lifecycle/manager_lock_order_test.go
```

Before the first pnpm command in a fresh worktree, install the workspace dependencies once:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run desktop and phone suites sequentially through the managed runner.
The desktop run builds current artifacts. Reuse them for the phone run only without intervening source changes.

```bash
(cd apps/web && pnpm e2e:run --host --project chromium tests/chat/cancel-turn-availability.spec.ts tests/session/session-tab-management.spec.ts tests/session/completed-workspace-restoration.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-cancel-turn-availability.spec.ts tests/session/mobile-session-deletion.spec.ts tests/session/mobile-completed-workspace-restoration.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/cancel-stop-client-deadlock
```

Run the local documentation coverage preflight:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const prefix = 'docs/plans/cancel-stop-client-deadlock/';
const workOrder = prefix + 'task-01-propagate-cancel-client.md';
const documents = [
  prefix + 'plan.md',
  workOrder,
  'docs/specs/tasks/requirements/workflow-cancelled-turn-completion.md',
  'docs/specs/tasks/requirements/session-delete-resource-cleanup.md',
  'docs/specs/tasks/system-design/workflow-cancelled-turn-completion.md',
  'docs/specs/tasks/system-design/session-delete-resource-cleanup.md',
];
const implementation = [
  'manager_interaction.go',
  'manager_interaction_dispatch_cancel.go',
  'manager_interaction_cancel_stop_test.go',
  'manager_interaction_dispatch_cancel_test.go',
  'manager_lock_order_test.go',
].map(name => 'apps/backend/internal/agent/runtime/lifecycle/' + name);
const result = validateCoverage({
  changedFiles: [...implementation, workOrder].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(documents.map(path => [path, fs.readFileSync(path, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

This preflight uses the repository validator without publishing a GitHub status.
Record its accepted work-order path and errors in Results.

## Files likely touched

Production ownership:

- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_dispatch_cancel.go`

Regression and signature-adaptation ownership:

- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_cancel_stop_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_dispatch_cancel_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lock_order_test.go`

Delivery record ownership:

- `docs/plans/cancel-stop-client-deadlock/plan.md`
- `docs/plans/cancel-stop-client-deadlock/task-01-propagate-cancel-client.md`

The existing orchestrator and browser test files are verification inputs, not planned production changes.
If they expose a separate defect, report its evidence before expanding this work order.

## Dependencies

None within this package.
Current main contains PR #5177's session-owned workspace repair at `d3a363def739760482e51d7b6f4428e1f7324fd0`.
Before implementation, verify that repair remains present and re-read any changed helper or deletion contracts.

## Risks

- One unconverted escalation caller retains the same deadlock.
- Early release exposes durable submission settlement to a closing client.
- A hanging RED test can leak goroutines unless its subprocess always terminates.
- A pre-fix browser test can pass because it does not create the writer schedule.

## Parallelism

`sequential`

## Inputs

- [Cancellation requirement](../../specs/tasks/requirements/workflow-cancelled-turn-completion.md), criteria 001.1 and 001.2.
- [Deletion requirement](../../specs/tasks/requirements/session-delete-resource-cleanup.md), criteria 001.1, .2, .3, and .9.
- [Cancellation client lease design](../../specs/tasks/system-design/workflow-cancelled-turn-completion.md#cancellation-client-lease-and-concurrent-stop).
- [Runtime quiescence design](../../specs/tasks/system-design/session-delete-resource-cleanup.md#runtime-quiescence).
- Existing `manager_interaction_cancel_test.go`, `manager_interaction_dispatch_cancel_test.go`, `manager_lock_order_test.go`, and `completion_cancel_coalescer_test.go`.
- Existing orchestrator `background_work_accounting_test.go`, `session_delete_contract_test.go`, and `session_launch_test.go`.
- Existing desktop and phone session deletion, cancellation, and workspace restoration specs listed in the plan.

## Results

Implemented on 2026-11-10.

Production change:

- `cancelAgentExecution` passes its captured `*agentctl.Client` to `cancelDispatchOnlyPrompt` and `waitForPromptFinishedAfterCancel`.
- Those helpers pass it to `waitForCapturedDispatchCompletion`, `finishDispatchCancelTimeout`, and `escalateStuckCancel`.
- `escalateStuckCancel` now requires the client argument and no longer acquires the lifecycle lease. A nil client remains valid only for transport-free fixtures.
- Durable submission settlement, prompt and startup generation fences, asynchronous ready publication, Stop serialization, and deletion quiescence are unchanged.

New regression file `manager_interaction_cancel_stop_test.go`:

- `TestCancelAgentEscalationUsesPinnedClient` covers ordinary timeout, dispatch immediate rejection, disconnected stream, missing dispatch gate, dispatch timeout, and timeout with a transport error. A prompt-snapshot mutex barrier establishes the queued stop writer after cancellation owns its lease.
- `TestCancelStopAndDeleteQuiescenceComplete` overlaps cancellation, an ordinary `force=false` stop that owns the runtime lifecycle lock while queued for the client writer, and a concurrent forced `session deleted` stop queued on that same runtime lock. It joins cancellation, prompt completion, both stops, and publication; the losing stop must report `ErrExecutionNotFound`, and an unrelated execution stays unchanged.
- `TestCancelEscalationSettlesCapturedDurableClientBeforeStop` blocks settlement, proves Stop cannot finish first, and covers successful and failed settlement.
- Each potentially irrecoverable schedule runs in a bounded helper subprocess. The parent captures child output, kills and waits for the child on timeout, and keeps blocked locks and goroutines out of the package process. The prompt waiter is context-owned and joined on every exit.

RED evidence on unchanged production code:

```text
TestCancelAgentEscalationUsesPinnedClient/ordinary_prompt_timeout: cancellation did not finish with a queued stop writer; escalation likely reacquired the agentctl read lease
TestCancelAgentEscalationUsesPinnedClient/dispatch_immediate_rejection: same failure
TestCancelAgentEscalationUsesPinnedClient/dispatch_missing_completion_gate: same failure
TestCancelAgentEscalationUsesPinnedClient/dispatch_timeout: same failure
TestCancelAgentEscalationUsesPinnedClient/dispatch_timeout_with_transport_error: same failure
TestCancelAgentEscalationUsesPinnedClient/disconnected_stream: passed, because that immediate branch already carried the client
```

Isolated-harness evidence:

- Temporarily restoring the nested acquisition made the helper child fail in `5.01s` with the branch-specific diagnostic above.
- The parent test process remained clean, and the child terminated normally after its test failure.

GREEN evidence:

- `go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestCancelAgentEscalationUsesPinnedClient|TestCancelStopAndDeleteQuiescenceComplete|TestCancelEscalationSettlesCapturedDurableClientBeforeStop|TestManager_CancelAgent_|TestMarkReadyDoesNotDeadlockWithPromptCompletion|TestMarkReadyAsync_)' -count=1 -timeout=180s`: passed.
- Workspace restoration pair with `-race`: passed.
- Orchestrator and executor deletion/stop selection with `-race`: passed.
- Full lifecycle package with `-race`: passed (`264.302s`).
- `bash scripts/lint-go-changed` over the changed lifecycle files: `0 issues`.
- Desktop `chromium` passed 12 tests in `3.8m`: `cancel-turn-availability` (2), `session-tab-management` (9), and `completed-workspace-restoration` (1).
- Phone `mobile-chrome` passed 4 tests in `44.7s` with `--no-build`: `mobile-cancel-turn-availability` (1), `mobile-session-deletion` (2), and `mobile-completed-workspace-restoration` (1).

Documentation checks after implementation:

- `python3 scripts/list-docs.py validate`: 446 decisions and 1963 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The local coverage preflight returned `ok: true` and `status: covered` with no errors.
- Local document links and `git diff --check` passed.

Design-stage evidence retained:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator -run '^(TestEnsureWorkspaceExecutionForSession_CreatesOwnExecutionInSharedTaskEnvironment|TestGetOrEnsureExecutionForEnvironmentDoesNotShareSiblingUnderSessionKey|TestManager_CancelAgent_EscalatesWhenAgentHangs|TestLaunchRestoreWorkspace_Success|TestDeleteSession_(StopsLiveExecutionAndDropsLateFrames|StopFailurePreservesSessionAndActivity|AlreadyMissingRuntimeStillDeletesSession|PreservesTaskWorkspaceAndNeverEnqueuesCleanup))$' -count=1 -timeout=120s)
```

Result: both packages passed, with eight selected tests in total.
The temporary overlay diagnostic independently reproduced the nested-lock defect and drained after its diagnostic-owned lease released.
