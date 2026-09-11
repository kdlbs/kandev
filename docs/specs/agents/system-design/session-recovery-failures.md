---
status: current
system: agents
created: 2026-09-11
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
owners:
  - Kandev
---

# Session Recovery Failures System Design

## Context and mapping

This amendment extends [agent recovery](agent-resume-runtime-recovery.md).
It owns workspace-only eligibility and recovery presentation. Tasks continue
to own contribution admission and durable bootstrap failure projection.

| Requirement | Design section |
| --- | --- |
| REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005 | Workspace-only registration |
| REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006 | Recovery presentation ownership; responsive amendment |

The following amendments are implemented in the
[contribution resume recovery package](../../../plans/contribution-resume-recovery/plan.md).
They qualify the older recovery-surface descriptions below.

### Workspace-only registration (requirement 005)

`launchRestoreWorkspace` authorizes the task/session pair and rejects archive
before calling `EnsureWorkspaceExecutionForSession`. Preserve those guards and
the existing session-keyed singleflight and task-environment reuse paths.

`registerAndPublishExecution` currently calls `ensureLaunchSessionStillActive`
before and after registration. Distinguish agent launch from authorized
workspace-only registration at both checks. Use an explicit internal purpose
from the workspace creation path; never derive permission from empty command
text or client-controlled metadata. Agent launch keeps its terminal rejection.
Workspace-only registration may retain `FAILED`, `COMPLETED`, or `CANCELLED`
state for a live, unarchived task with valid retained environment ownership.

Recheck task existence, archive, session binding, cleanup intent, and environment
ownership at both registration boundaries, including reused-execution paths.
Preserve durable registration so cleanup can inventory created resources.
A failed check rolls back only this creation and does not revive the session.
Do not transition a terminal session to `STARTING` merely to browse files,
request an agent credential lease, or weaken the credential broker.
Workspace-only access retains its existing authorized tool capabilities; the
UI's read-only recovery label describes the stopped agent, not a new filesystem
sandbox. No autonomous Git or prompt operation runs as part of restoration.

### Recovery presentation ownership (requirement 006)

Extend the existing task/session launch-error ownership boundary instead of
creating a second error store. The
[task launch projection](../../tasks/system-design/task-launch-failure-recovery.md)
owns durable bootstrap failures. Recovery request state contributes pending
actions and the separately labeled resume/restore results.

Correlate by task, session, execution/attempt identity, and durable error stamp.
Carry an optional error stamp in recovery error details so the client can match
the request failure to its durable record. Do not deduplicate by message text
or by session alone. Without correlation, retain a distinct historical error.

One shared recovery view model selects the active record and fallback request
state. Task detail, preview, and Quick Chat consume it. In a mounted chat, the
inline recovery card owns presentation. The outer `SessionRecoveryFeedback`
renders only when there is no matching chat owner; initial session creation
keeps its current ensure-error surface. Do not mount duplicate action hooks
that can issue equivalent requests from separate renderers.

Reuse `TaskLaunchErrorEntry`, `SessionStoppedBanner`, and their existing
handlers behind this ownership decision. Do not route session Resume through
fresh launch or discard provider identity. Keep confirmed fresh-start and typed
branch-loss controls available as secondary choices, without suggesting that
they resolve a Git permission or history problem.

Automatic resume and manual recovery share the same presentation and busy
state. Automatic fallback remains allowed; manual restore remains explicit.
Success clears only its matching attempt. A stale callback cannot clear a
newer failure. Retain the archive/navigation generation guards from requirement
004 and the provider-specific runtime recovery policies.

### Responsive amendment

Use the dedicated phone composition in `task-layout.tsx` and
`mobile/session-mobile-layout.tsx`; the current inline recovery card is the
nearest status exemplar. This short, task-local decision stays inline in Chat.
Desktop has summary, compact action row, then details. Phone stacks actions
below the summary; the transcript owns vertical scrolling. Details wrap inside
the same scroll owner. Retain dynamic viewport sizing and safe-area clearance.
Use 28-pixel fine-pointer buttons and at least 44-pixel phone/coarse-pointer
targets. The semantic disclosure supports Enter/Space and expanded state.


## Persistence and compatibility

Use the existing error records and optional fields specified by the
[task launch projection](../../tasks/system-design/task-launch-failure-recovery.md).
No new table or parallel frontend error store is introduced. Older records use
safe generic summaries and keep unrelated historical errors visible.

## Verification

The [package](../../../plans/contribution-resume-recovery/plan.md) maps each
criterion to admission races, projection tests, component tests, and desktop/
mobile recovery scenarios. Preserve existing archive and branch-loss tests.
