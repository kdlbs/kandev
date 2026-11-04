---
created: 2026-10-30
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
legacy_specs: []
---

# Fix plan: relaunch after an uncommitted first launch

## Overview

A task session whose first ACP launch fails before any native conversation
commits can never start again. Every Resume and Start fresh fails with
`failed to initialize ACP: agent delivery checkpoint missing`. This fix lets
such a relaunch replay the retained setup events through the existing
fresh-bootstrap path. One work order covers the change, because the defect is
a single preflight predicate.

## Evidence and root cause

On 2026-10-30, task "Temos que converter tudo para a" (session `933bd6fa`)
failed and stayed failed.

1. 09:15:16: the first launch failed. `atomica-code` rejected ACP `session/new`
   with "Atomica Experience could not be reached". Before exiting, the child
   appended seven events to `<incarnation>:g1`: one `agent_capabilities` and
   six `mcp_attachment`.
2. 09:34:48 and 09:35:52: the user tried recovery (`fresh_start`, seam
   `resumeTaskSession`). Both attempts failed in
   `StreamManager.connectUpdatesStream` with "failed to load durable delivery
   replay cursor ... agent delivery checkpoint missing". The session returned
   to `FAILED`.

Persisted state (read-only):

| Record | State |
|---|---|
| `harness_session_generations` | No rows |
| `agent_delivery_cursors`, `agent_delivery_inbox`, `agent_delivery_submissions` | No rows |
| agentctl journal stream g1 | `high_water` 7, `first_retained` 1, `acknowledged` 0, no submissions, unsealed |
| Stream g1 event types | `agent_capabilities`, `mcp_attachment` ×6. No turn content, no terminal event |

Root cause. `initialDeliveryReplayCursor` calls
`authenticatedDeliveryReplayCursor` (`lifecycle/agent_delivery_retention.go`).
That function finds no SQL cursor and a non-empty stream, so only two escapes
remain:

- `canReplayFreshBootstrapEvents` (`lifecycle/delivery_stream_preflight.go`)
  rejects the launch because `execution.isResumedSession` is true. The flag is
  set from `PreviousExecutionID` (`manager_launch.go`), and every recovery has
  a previous execution.
- `receiveResumedBootstrapCheckpoint` rejects the launch because it needs a
  committed native session ID, and there is none.

Start fresh also stays on g1. The
[fresh-start generation fix](../fresh-start-delivery-generation/plan.md)
advances the generation only when one is already committed.

The `isResumedSession` guard is wider than its intent. PR #4425 meant to keep
refusing restored sessions, retained prompt work, and pruned history. The
predicate's other conditions already cover all three: no native identity, no
committed SQL generation, no cursor, no submissions, and a full unacknowledged
prefix. The guard additionally blocks relaunches of sessions that never had
anything durable.

This violates AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2: Start fresh can
never create the new provider-native session. It also violates
AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8: recovery can never succeed after
the required relaunch. No requirement changes. The stream-processing design now
states the uncommitted first-generation relaunch rule.

## Scope

### In scope

- Remove the `isResumedSession` condition from `canReplayFreshBootstrapEvents`.
  Keep every other condition.
- Update the existing preflight regression test so it asserts the corrected
  contract. Add negative cases that still prove refusal for a relaunch with
  native identity, retained prompt work, a committed generation, or pruned
  history.

### Out of scope

- Generation advancement for Start fresh without a committed generation.
  Generation one stays correct, because nothing earlier exists to supersede.
- Retiring, sealing, or discarding the orphaned g1 journal stream.
- Changes to the orchestrator, UI, schema, metrics, or the resume-token contract.
- Mutating the affected user's live session, journal, or database.

## Technical approach

`apps/backend/internal/agent/runtime/lifecycle/delivery_stream_preflight.go`:

- Delete `execution.isResumedSession ||` from the guard in
  `canReplayFreshBootstrapEvents`. These conditions remain:
  `deliveryBootstrap`, empty `ACPSessionID` and `RequiredNativeConversationID`,
  `DeliveryHarnessGeneration == 1`, `FirstRetained == 1`, `Acknowledged == 0`,
  no unresolved or recorded submissions, no SQL cursor, and no committed
  harness generation.
- Update the function comment to state the invariant: nothing durable has
  committed for this incarnation, whichever execution produced the retained
  events.

Unchanged behavior that the fix relies on:

- `session.go` sets `deliveryBootstrap` for every generation-one launch.
- `deliveryHarnessGeneration` (`durable_delivery_stream.go`) holds event
  admission until generation one commits. If the commit never arrives, startup
  fails as it does today.

Rejected alternative: discarding or sealing the orphaned stream before the
relaunch. That needs new journal-mutation authority across the agentctl
boundary, and it changes retained evidence. Replay through the existing
bootstrap path is smaller and already proven for in-execution child retries.

## Tests

| Criterion | Evidence |
|---|---|
| AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2, AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8 | `delivery_stream_bootstrap_preflight_test.go`: `TestDeliveryPreflightRetainedStartupEventBeforeNativeInitialization`. A relaunch (`isResumedSession = true`) with no native identity now returns cursor 0 and creates no SQL cursor. |
| Fail-closed boundaries kept | The same file: `TestDeliveryPreflightRelaunchKeepsDurableRefusals`, a new test. A relaunch still errors with a native session ID, a required native conversation, a submission, pruned history, or a committed generation. |

## E2E tests

None. The failure needs a provider that rejects `session/new` after agentctl
has created a durable stream. The mock agent has no such scenario. The UI and
recovery actions are unchanged, and the defect sits entirely in the backend
preflight predicate, which the unit tests above cover.

## Work orders

- [x] [Task 01: Relaunch replays an uncommitted first-generation stream](task-01-relaunch-uncommitted-bootstrap.md)

## Verification results

- `(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run 'TestDeliveryPreflight')`:
  before the fix, `TestDeliveryPreflightRetainedStartupEventBeforeNativeInitialization`
  failed with "agent delivery checkpoint missing" on the relaunch assertion.
  After the fix, it passed (`ok`).
- `(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle)`: `ok` (214.9s).
- `bash scripts/lint-go-changed <both changed Go files>`: 0 issues.
- `python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all`:
  420 decisions and 1850 specifications validated, and all specification files passed.

## Risks

- Replay projects the failed child's setup events (`agent_capabilities`,
  `mcp_attachment`) before the replacement's own events. The in-execution
  child-retry path already does this. Attachment events are scoped by attempt
  ID, and later capabilities supersede earlier ones.
- This fix does not unblock the affected session until the build ships. Until
  then, the user can open a new session or move the session's journal
  directory aside while Kandev is stopped.
