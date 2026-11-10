---
created: 2026-11-10
status: done
requirements:
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/workflow-cancelled-turn-completion.md
  - ../../specs/tasks/system-design/session-delete-resource-cleanup.md
legacy_specs: []
---

# Fix plan: Cancel/stop client deadlock

## Overview

Remove the nested agentctl read lease that blocks cancellation, Stop, and session deletion in [issue #5230](https://github.com/kdlbs/kandev/issues/5230).
One work order owns the correction, deterministic regressions, and existing workspace compatibility checks.
The package is implemented; its results and verification evidence are recorded below.

## Ownership and requirement conformance

Tasks own this repair because Tasks own turn cancellation, session deletion, and continued access to task environments.
The shared [Platform client lease design](../../specs/platform/system-design/agentctl-client-lease.md) remains a dependency.

The active [cancelled-turn requirement](../../specs/tasks/requirements/workflow-cancelled-turn-completion.md)
already requires bounded escalation, ordered terminal persistence, and protection of successor turns under criterion 001.1.
Criterion 001.2 preserves runtime reconciliation and working-sibling precedence.
The [session deletion requirement](../../specs/tasks/requirements/session-delete-resource-cleanup.md)
already covers preserved workspace ownership and sibling access under criteria 001.1, .2, .3, and .9.
That requirement remains draft. This repair does not promote its broader migration contract.

No requirement changes are necessary.
The two existing task designs now document client propagation and exact-runtime quiescence.
The existing explicit-cancellation, backend-owned-progress, and task-owned-worktree decisions retain their authority.
No new ADR, persistence boundary, or public contract is necessary.

## Evidence and root cause

Investigation used current main `6aa3871a946557e74868e9b389b89ca9294bd329`.
The remote main head matched this checkout on 2026-11-10.
The reported binary used `06d9a5ed070a4783688a6097cbf3574eab20bbe3`.
The issue includes retained logs, a goroutine snapshot, and two earlier temporary diagnostics.
This investigation did not access or mutate the reported live instance.

The current call chain still forms this cycle:

1. `cancelAgentExecution` acquires `agentctlLifecycleMu.RLock` through `AcquireAgentCtlClient` and defers release.
2. `StopAgentWithReason` holds `remoteInstanceLifecycleMu` while `stopExecutionAgentctl` queues for the client write lease.
3. Cancellation reaches an escalation helper that calls `AcquireAgentCtlClient` again.
4. The queued writer blocks the nested reader. The outer reader cannot release until cancellation returns.
5. Deletion waits for runtime teardown and eventually returns an error while the session remains registered.

The ordinary timeout path is not the only unsafe caller.
`cancelDispatchOnlyPrompt`, `waitForCapturedDispatchCompletion`, and `finishDispatchCancelTimeout` also enter escalation without the captured client.
Immediate ordinary-prompt escalation already passes that client.

A temporary Go overlay reproduced the ordinary timeout against the real helper and lease methods.
It held the outer read lease, confirmed a queued writer, and entered the real fallback with a 20 ms wait.
The fallback remained blocked in nested `RLock` after its 200 ms context deadline.
Releasing the diagnostic-owned outer lease drained both operations.
The diagnostic passed by asserting the defect. It did not prove a correction.
The permanent regression must require completion without an external lease release.

## Existing repair of the second reported defect

[PR #5177](https://github.com/kdlbs/kandev/pull/5177) merged as `d3a363def739760482e51d7b6f4428e1f7324fd0`.
It already removed sibling-controller borrowing from the session workspace route.
`ensureWorkspaceExecutionLocked` uses the requesting session's own execution.
`launchRestoreWorkspace` rejects missing execution identity, waits for readiness, and rechecks the binding.

That repair changes the appropriate response to issue #5230's second diagnosis.
Keep session-owned workspace restoration and the frontend's session correlation.
Do not reintroduce sibling reuse or accept arbitrary sibling readiness events.
The existing [continuation recovery package](../session-owned-continuation-recovery/plan.md) remains done.

## Scope

### In scope

- Pass one captured client through every cancellation escalation path.
- Remove implicit client acquisition from `escalateStuckCancel`.
- Preserve prompt identity, durable submission settlement, and asynchronous ready publication.
- Prove concurrent Cancel, Stop, and forced deletion-style quiescence can drain.
- Run existing deletion, workspace restore, desktop, and phone compatibility checks.

### Out of scope

- New timeouts, mutex types, polling, worker goroutines, flags, schemas, or public endpoints.
- Changed cancellation-driven workflow completion or queued-message policy.
- Frontend production changes, layout, touch targets, translation keys, or restoration correlation changes.
- Physical workspace cleanup, repair of an already-deadlocked process, or live-instance operations.
- Renaming the executor's legacy stop-error prefix.

## Technical approach

`cancelAgentExecution` remains the sole client lease owner for its operation.
It releases that lease on every return path through the existing defer.
Keep the lease until escalation and captured durable submission settlement finish.
Early release before settlement can expose the client to Stop or replacement.

Pass an explicit `*agentctlclient.Client` through these helpers:

- `waitForPromptFinishedAfterCancel`
- `cancelDispatchOnlyPrompt`
- `waitForCapturedDispatchCompletion`
- `finishDispatchCancelTimeout`
- `escalateStuckCancel`

Replace the variadic optional argument on `escalateStuckCancel` with a required argument.
Remove its fallback call to `AcquireAgentCtlClient`.
Existing direct helper tests must supply a leased client, or explicit nil for a transport-free fixture.
Production calls must pass the client's existing lease through the whole chain.
The contended-prompt path through `escalateCapturedDispatchWithoutPromptLock` keeps its current generation-scoped local signaling.

Preserve `CancelDeliverySubmission`, its bounded settlement context, and joined errors.
Preserve execution, prompt generation, startup generation, and turn ownership checks.
Keep synthetic prompt completion, buffer ordering, and asynchronous ready publication unchanged.
Leave Stop's writer, runtime serialization, and DeleteSession's quiescence checks intact.

### Compatibility matrix

| Path | Identity and transport | Required result | Evidence |
| --- | --- | --- | --- |
| Ordinary prompt timeout | Captured execution, prompt, client | Escalate with the original lease, then release it | New lifecycle regression |
| Dispatch-only immediate, missing gate, and timeout | Captured generation and client | Same rule, including a transport error beside timeout | Table-driven lifecycle regression |
| Disconnected or unacknowledged cancel | Existing ACP error classification | Preserve immediate settlement and original error behavior | Existing cancellation tests and new queued-writer cases |
| Durable submission | Captured submission and client | Settle before Stop can close the client | New durable-client regression |
| Stop and deletion quiescence | Captured execution ID | Drain serialized stops without targeting a successor | New lifecycle overlap test and existing service tests |
| Shared workspace | Session-owned controller, environment-owned files | Preserve usable sibling data and restoration identity | Existing restore and deletion regressions |
| ACP provider adapters | Shared lifecycle client | Preserve supported cancellation outcomes | Mock transport coverage, independent of provider model |
| Passthrough and remote runtime implementations | Existing stop and recovery boundaries | No new admission, teardown, or survival behavior | Existing contracts, no live provider certification |

The incident used local worktree execution and ACP agents.
The shared tests do not certify live Copilot, Codex, SSH, Docker, or Kubernetes environments.

## Tests

All new lifecycle tests belong in `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_cancel_stop_test.go`.
They use the existing mock agent server, manager fixtures, and channel barriers.
They must exercise the real production cancellation and stop paths.

| Test | Required cases | Criteria |
| --- | --- | --- |
| `TestCancelAgentEscalationUsesPinnedClient` | Ordinary timeout, dispatch immediate rejection, disconnected stream, missing pending gate, dispatch timeout, timeout with transport error | Cancellation 001.1 |
| `TestCancelStopAndDeleteQuiescenceComplete` | Cancel acknowledgement without completion, ordinary `force=false` Stop owning the runtime lock while queued for the client writer, concurrent forced `session deleted` stop queued on that runtime lock, prompt completion and confirmed-absence result | Cancellation 001.1/.2, deletion 001.1/.3 |
| `TestCancelEscalationSettlesCapturedDurableClientBeforeStop` | Submission settlement before client close, successful and failed settlement, queued writer | Cancellation 001.1 |
| Existing `TestManager_CancelAgent_*` and lock-order tests | Late frames, stale generation, competing completion consumer, expired context, ready subscriber re-entry | Cancellation 001.1/.2 |
| Existing `TestDeleteSession_*` | Exact stop ID, failed stop retains row/activity, confirmed absence permits removal, workspace retained | Deletion 001.1/.2/.3 |
| Existing `TestEnsureWorkspaceExecutionForSession_CreatesOwnExecutionInSharedTaskEnvironment` | Requesting session owns its controller while a sibling exists | Deletion 001.9 compatibility |

The lifecycle overlap test queues the same forced-stop primitive used by DeleteSession behind an ordinary Stop that already owns the execution's runtime lifecycle lock.
Existing orchestrator tests separately prove the real session-deletion boundary.
Neither layer alone is evidence of the complete browser incident.
The losing stop must return the confirmed-absence identity error; the test rejects transport or deadline errors.
A separate sequential repeat stop retains confirmed-absence coverage.

Each potentially irrecoverable schedule runs in a bounded helper subprocess with guaranteed termination and captured output.
The parent kills and waits for the child on timeout, so a blocked cancellation branch cannot contaminate the package process or goleak.
The child reports its blocked branch, not a generic suite timeout.
Use barriers to establish the writer wait before escalation.
Do not use arbitrary sleeps or a shorter product timeout as the correction.
The passing test must join cancellation, prompt completion, both stop callers, and event publication.
Verify that an unrelated execution and a successor generation remain unchanged.

## E2E tests and mobile parity

No rendered UI, navigation, copy, or touch behavior changes.
The existing desktop and native phone session surfaces use the same corrected backend lifecycle.
No ASCII UI preview or new mobile composition is necessary.

Run these existing flows through the managed runner after the backend regressions pass:

| Flow | Playwright file and project | Criteria |
| --- | --- | --- |
| Cancel control settles after terminal acknowledgement | `tests/chat/cancel-turn-availability.spec.ts`, `chromium` | Cancellation 001.1 |
| Session deletion retains shared Changes data through sibling hydration | `tests/session/session-tab-management.spec.ts`, `chromium` | Deletion 001.1/.3/.9 |
| Completed workspace restores Files, Changes, and Terminal | `tests/session/completed-workspace-restoration.spec.ts`, `chromium` | Deletion 001.9 compatibility |
| Phone cancel control settles | `tests/chat/mobile-cancel-turn-availability.spec.ts`, `mobile-chrome` | Cancellation 001.1 |
| Phone session sheet deletes the selected session and preserves its sibling | `tests/session/mobile-session-deletion.spec.ts`, `mobile-chrome` | Deletion 001.1/.3 |
| Phone workspace restores after cold restart | `tests/session/mobile-completed-workspace-restoration.spec.ts`, `mobile-chrome` | Deletion 001.9 compatibility |

These flows prove user outcomes. The deterministic Go test proves the lock schedule.
The phone checks reuse the shipped actions sheet and focused Files, Changes, and Terminal navigation.
Their current fixtures remain the mobile exemplar for this repair.

## Work orders

- [x] [Task 01: Carry the pinned client through cancel escalation](task-01-propagate-cancel-client.md)

Execute the work order sequentially in the primary conversation.
No delegation is authorized.

## Verification results

Design-stage evidence on the unmodified production source:

- Temporary `TestIssue5230DiagnosticNestedClientLease`: passed by reproducing the nested-lock defect.
- Eight selected existing lifecycle and orchestrator tests passed across both packages.
- Current remote main and the merged PR #5177 commit were verified.
- Issue #5230 was assigned to authenticated user `carlosflorencio` and read back successfully.
- Catalog validation, the 36 specification-linter tests, full specification lint, documentation coverage preflight, local links, Bash syntax, and whitespace checks passed.

Implementation completed on 2026-11-10.

Production change:

- `cancelAgentExecution` passes its captured client through every helper in the escalation chain.
- `escalateStuckCancel` requires the client argument and no longer acquires the agentctl lease itself.
- Durable settlement keeps using the captured client before the cancellation lease releases; Stop and deletion quiescence semantics are unchanged.

Regression evidence:

- RED phase: the queued-writer cases failed on the unchanged source with `cancellation did not finish with a queued stop writer`; the disconnected immediate branch already passed because it carried the client.
- The isolated harness was rechecked by temporarily restoring the nested acquisition: the helper child failed in `5.01s` with the branch-specific diagnostic while the parent test process stayed clean.
- GREEN phase: all three new regressions passed, including both durable settlement outcomes.
- The full lifecycle package passed under `-race` (`264.302s`), together with the selected workspace, orchestrator, and executor contracts.
- `bash scripts/lint-go-changed` reported `0 issues`.
- Desktop `chromium` passed 12/12 listed flows (`3.8m`); phone `mobile-chrome` passed 4/4 with reused artifacts (`44.7s`).

Browser and documentation checks are recorded in the work order.

## Related delivery records

- [Agentctl lease deadlock](../agentctl-lease-deadlock/plan.md): completed URL-read and recovery-persistence repair, not cancellation escalation.
- [Cancellation completion/delete deadlock](../cancellation-completion-delete-deadlock/plan.md): completed session-guard and coalescer repair, with a different lock cycle.
- [Queued-prompt pause](../queued-prompt-pause-deadlock/plan.md): prior dispatch admission and guard deadline repair.
- [Session-owned continuation recovery](../session-owned-continuation-recovery/plan.md): completed sibling-controller isolation in PR #5177.

Their statuses and historical results remain unchanged.
This package owns the remaining cancellation client lease defect.

## Risks

- Fixing only the ordinary timeout leaves dispatch-only cancellation vulnerable to the same writer queue.
- Releasing the client before durable settlement lets Stop close the transport before cancellation finishes its accepted work.
- A synthetic ready event must not mutate a replacement execution or release a successor prompt.
- Deleting a session after an unconfirmed stop can leave a live process and late frames without an owner.
- A running process already caught in this cycle cannot adopt a source change without an operator-controlled restart.

## Documentation impact

Internal task designs and this delivery package record the repair.
Public documentation needs no change because existing commands, recovery actions, terminology, and ownership rules remain valid.
