---
status: draft
system: agents
created: 2026-09-11
updated: 2026-09-14
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
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
| REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007 | Proposed attempt isolation |

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


### Chronological recovery entries (September 14 amendment)

This amendment supersedes recovery reveal and prepend placement from the completed
[startup recovery scrolling package](../../../plans/startup-recovery-scroll-timeout/plan.md).
The [error scope package](../../../plans/error-scope-and-history/plan.md) owns implementation.

Render a session failure through the ordinary message pipeline at its persisted occurrence position.
Reuse `ActionMessage`, `RunErrorEntry`, and `SessionBootstrapRecoveryCard` presentation and recovery handlers where applicable.
One correlated failure selects one renderer, not a footer plus a prepended card.
`TaskChatLaunchError` remains an adapter for compatible callers until those callers migrate.

Remove activity-based deletion from `deduplicateRecoveryMessages` in `processed-message-filtering.ts`.
Deduplicate by session and durable stamp, with message ID as the legacy fallback.
Do not collapse every recovery entry into the latest one. Preserve unrelated errors and provider-specific remediation.
`ActionMessage` must retain a historical body while the session is STARTING, RUNNING, or COMPLETED.
Neither a button click nor any later user message proves successful recovery.
Use the existing durable recovery resolution timestamp and correlated boot evidence for outcome state.
Only the matching current unresolved stamp can mount active recovery controls.
A newer failed attempt creates a new chronological entry. The previous entry remains historical with no stale actions.
An update within the same failure identity updates that entry without moving it.

Remove recovery-specific `prependContent`, `recoveryRevealKey`, and top-placement branches from mounted chat paths.
Remove only error-specific scroll behavior, preserving search, ordinary history anchors, bottom follow, and environment-switch placement.
Update detail, preview, simple Chat, and Quick Chat. Keep the composer outside the single transcript scroll owner.
A following reader sees new errors through normal append behavior. A reader in history stays at the same anchor.
An active error outside the loaded page must not be fabricated at the tail.
The existing session metadata can supply one provisional entry at its occurrence time until the persisted marker arrives.
Merge that entry by stamp and replace it with the persisted message identity without duplication.
After recovery, never reconstruct absent historical errors from current state alone.

Phone entries stack actions with 44-pixel targets. Their details expand inline and wrap in the transcript.
Shared task errors use the task-owned shell surface described in the
[task design](../../tasks/system-design/task-launch-failure-recovery.md).
The [scope decision](../../../decisions/2026-09-14-error-scope-and-history.md) records the tradeoffs.

## Persistence and compatibility

Use the existing error records and optional fields specified by the
[task launch projection](../../tasks/system-design/task-launch-failure-recovery.md).
No new table or parallel frontend error store is introduced. Older records use
safe generic summaries and keep unrelated historical errors visible.

## Verification

The [package](../../../plans/contribution-resume-recovery/plan.md) maps each
criterion to admission races, projection tests, component tests, and desktop/
mobile recovery scenarios. Preserve existing archive and branch-loss tests.

## Proposed attempt isolation (requirement 007)

The prior workspace and presentation amendments remain implemented. This
section is draft and maps to the [resume cancellation package](../../../plans/resume-cancellation/plan.md).
Agents owns provider continuity and attempt outcomes. Task admission and queue
policy retain their existing ownership.

### Inconclusive load failures

`SessionManager.createOrLoadSession` currently falls back after any error that
`isTransportDeadErr` does not recognize. The agentctl boundary can serialize a
deadline as an ACP internal error, so `errors.Is` cannot recognize it.

Use explicit positive classification for existing supported fallback cases:
method unsupported, advertised load capability absent, or confirmed unknown
session. An unclassified internal error, timeout, cancellation, authentication
failure, or transport failure returns the load error without `session/new`.
Preserve structured error codes when available. Keep compatibility matching
narrow at the existing transport boundary and cover the recorded nested JSON
error. Unknown messages never authorize fallback.

This change preserves established fallback behavior for confirmed unsupported
or missing sessions. A broader change to those cases is outside this package.
The new checks apply before fallback and before token publication. A successful
response from an invalidated attempt cannot replace the stored token.

### Attempt ownership

The orchestrator owns a cancellable startup attempt for each task session.
Register it before asynchronous launch or readiness work. Retain an opaque
attempt identity, its cancel function, and the originating prompt identity.
Reuse existing lifecycle and cancellation guards instead of a parallel dispatch
queue. Keep provider execution generation distinct from startup attempt identity:
one execution can be reused across multiple attempts.

The operation context survives request disconnects but remains cancellable by
explicit cancellation and service shutdown. Derive it from the service lifetime
and carry required request values. Do not use an uncancellable context for the
whole operation. Bounded detached contexts remain valid for owned cleanup.

`CancelAgent` invalidates the captured attempt under the existing cancellation
guard before runtime cancellation. It cancels startup and readiness waits, then
uses existing bounded runtime cancellation and escalation. It never waits for
the lifecycle lock while holding a guard needed by the startup completion path.
The cancellation projection remains pending until owned cleanup settles.
A cleanup failure retains truthful failure state and recovery controls.

Every continuation checks attempt ownership after a blocking operation and
before dispatch admission, token persistence, state publication, or fallback.
Identity validation and dispatch admission share the existing cancellation
guard through provider acceptance. A late callback from the old attempt cannot
write an error, complete a new turn, or stop a replacement execution.
Cleanup uses captured execution identity and generation, never only session ID.

Apply this ownership to `ResumeTaskSessionWithOptions`, lazy resume through
`ensureSessionRunning`, and `handlePromptWithResume`. The handler retry must
retain the original attempt identity. It cannot create a new operation after
explicit cancellation. A distinct user retry obtains a new identity only after
cancellation admission allows it.

Keep existing queue reservation, incarnation checks, and Auto-run behavior.
A cancelled direct prompt is never inserted into the queue as a recovery step.
Unrelated queued prompts remain governed by the
[resume queue design](../../tasks/system-design/resume-prompt-queue.md).
No provider-level exactly-once guarantee is introduced.

### Failure projection and presentation

Pre-dispatch resume failures use the existing durable launch-error projection
and shared recovery owner. Preserve task, session, attempt, and error-stamp
correlation. Suppress the generic synthetic send error only when that same
failure has a recovery owner. Unrelated historical errors remain visible.

Retain the actual resume cause when an internal retry fails before dispatch.
The old readiness error must not replace a later, more specific load failure.
Once dispatch is accepted, normal prompt error handling remains authoritative.
Explicit cancellation is a cancellation outcome, not a resume failure card.

Reuse the existing `TaskLaunchErrorEntry` and recovery view model. The summary
identifies recovery failure. Details identify the load timeout. Existing Retry
and confirmed Start fresh actions keep their semantics. No new setting, public retry endpoint, or background retry loop is necessary.
The chronological presentation amendment governs the layout.

The nearest phone exemplar is the existing inline launch recovery card in
`mobile/session-mobile-layout.tsx`. Phone actions stack below the summary and
retain 44-pixel hit areas. Desktop retains compact actions. Both use the same
recovery state, a single transcript scroll owner, wrapped details, and existing
safe-area behavior. The package includes a compact preview and rendered checks.
Any new cause label uses the existing locale catalogs.

### Persistence and verification

No schema change is required. Attempt ownership is process-local. Backend
restart uses existing recovery reconciliation and never replays a cancelled
prompt from the old process. Existing resume-token fields remain authoritative.

Use barrier-controlled tests for timeout, cancellation before readiness, late
success, late failure, and retry during cleanup. Include browser disconnects,
shutdown, and an unrelated queued message with Auto-run disabled. Trace accepted
prompt counts, stored token, active turn, final state, and execution ownership.
Desktop and phone tests exercise the actual backend resume path and reload.

This applies the accepted [backend cancellation ownership decision](../../../decisions/2026-08-03-backend-owned-cancellation-progress.md).
It extends that implementation to startup attempts without a new durable state
or an alternative cancellation owner. No new ADR is required.
