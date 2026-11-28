---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
---

# Initial-target retry before route commit

## Boundary and requirement mapping

This design extends
[workflow profile session lifecycle](workflow-profile-session-lifecycle.md)
for `AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.13`. Tasks owns it because the
workflow entry route decides which session may receive the entry. Agents owns
the recovery card. See
[recovery workspace availability](../../agents/system-design/recovery-workspace-availability.md).

## Unrouted initial entries

Task creation obtains its first session through workspace preparation
(`launchPrepare` → `PrepareTaskSession`). No `workflow_session_route` is
written on that path. If workspace preparation fails, the initial-target entry
has no route, and the write-once `workflow_initial_session` snapshot alone
names its recipient.

Without a route, `validateUnroutedWorkflowEntry` accepts an initial target
only when all of these hold:

- The snapshot exists and names a session.
- The bound session, or else the deferral payload session, is empty or equals
  the snapshot session.
- The existing workflow, step, entry-identity, operation-ID, and destination
  checks pass.

A session mismatch returns superseded. A missing snapshot stays permanently
unavailable.

`initialRecoveryWorkflowBinding` clears `DestinationSessionID`, because the
failed session counts as terminal for destination checks. Before deriving the
binding, it requires the recovering session to equal the snapshot session. The
check runs only for an initial target without a route.

Earlier-step targets (`kind: step`) still require their committed route.
`launchPrepare` does not write a route.

## Recovery paths

`runtime_retry` without a resume token is normalized to `fresh_start`. Both
then reach this validation through `RecoverSessionWithOptions`. A retry keeps
the pending `initial_prompt_submission` and composes the current step
instructions through the existing fresh-start workflow prompt.

## Verification

Use service tests with SQLite and `newFreshStartWorkflowFixture`. Cover accepted
`fresh_start` and `runtime_retry`, a foreign session, a changed step or entry,
a missing snapshot, and an earlier-step target without a route.

## Implementation plans

- [Initial-session first-launch retry](../../../plans/initial-session-first-launch-retry/plan.md)
