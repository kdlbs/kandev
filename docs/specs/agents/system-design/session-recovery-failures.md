---
status: current
system: agents
created: 2026-09-11
updated: 2026-09-20
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
| REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006 | Recovery presentation ownership; post-start recoverable failure detail; responsive amendment |
| REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007 | Attempt isolation; accepted-turn ownership |

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
composer recovery region owns active presentation when messaging is blocked;
chronological rows retain history without mutation controls. See the September 20
amendment. The outer `SessionRecoveryFeedback`
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

### Post-start recoverable failure detail (requirement 006)

`createRecoveryStatusMessage` (`internal/orchestrator/event_handlers_agent.go`)
populates the recovery entry's `error_output` metadata with
`routingerr.Sanitize(data.FailureDetails)` for post-start recoverable failures,
matching what the bootstrap, managed-runtime-npm, and provider-quota paths
already do. Without this, a failure that occurs after agent startup, such as a
model provider rejecting a dispatched prompt with an invalid-tool-definition
`400`, left `error_output` empty, so the recovery card showed only the short
summary line and the collapsed technical-details disclosure never appeared.

The frontend already renders this. `ActionMessageDetails` / `TechnicalDetails`
(`apps/web/components/task/chat/messages/action-message-details.tsx`) render
`error_output` inside an initially collapsed disclosure, so no frontend change
is required. When sanitization yields an empty string, `error_output` is
omitted and the generic recovery card is shown. Sanitization removes URLs,
credentials, and identifiers; raw agent stderr is never added to durable
metadata. The `remediation_url` link stays a separate metadata field and is
never folded into `error_output`.

### Responsive amendment

Use the dedicated phone composition in `task-layout.tsx` and
`mobile/session-mobile-layout.tsx`; the current inline recovery card is the
nearest status exemplar. The September 20 amendment moves blocked-session controls
to the composer region. Desktop has summary, compact action row, then details.
Phone stacks actions below the summary. Historical details use transcript scrolling;
active details use the bounded recovery region defined below. Retain dynamic
viewport sizing and safe-area clearance.
Use 28-pixel fine-pointer buttons and at least 44-pixel phone/coarse-pointer
targets. The semantic disclosure supports Enter/Space and expanded state.


### Chronological recovery entries (September 14 amendment)

This amendment supersedes recovery reveal and prepend placement from the completed
[startup recovery scrolling package](../../../plans/startup-recovery-scroll-timeout/plan.md).
The [error scope package](../../../plans/error-scope-and-history/plan.md) owns implementation.

Render a session failure through the ordinary message pipeline at its persisted occurrence position.
Reuse `ActionMessage`, `RunErrorEntry`, and `SessionBootstrapRecoveryCard` presentation and recovery handlers where applicable.
One correlated failure selects one active control owner. The September 20 amendment
separates its compact historical row from its composer recovery controls.
`TaskChatLaunchError` remains an adapter for compatible callers until those callers migrate.

Remove activity-based deletion from `deduplicateRecoveryMessages` in `processed-message-filtering.ts`.
Deduplicate by session and durable stamp, with message ID as the legacy fallback.
Do not collapse every recovery entry into the latest one. Preserve unrelated errors and provider-specific remediation.
`ActionMessage` must retain a historical body while the session is STARTING, RUNNING, or COMPLETED.
Neither a button click nor any later user message proves successful recovery.
Use the existing durable recovery resolution timestamp and correlated boot evidence for outcome state.
Only the current unresolved stamp owns recovery, even in FAILED. Compare failure
occurrence time before transcript time. Resolved failures yield to ordinary stopped-session
recovery; later failures keep their own actions.
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

Phone historical entries retain inline wrapped details; active controls follow the September 20 composer-region design.
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

## Accepted-turn ownership (requirement 007)

This amendment clarifies the startup boundary in the preceding attempt
isolation design. The [repair package](../../../plans/resumed-turn-cancellation/plan.md)
records implementation and validation of criteria 007.7 through 007.9.

### Startup authority and execution identity

`resumeAttemptRegistry` owns startup cancellation authority. An attempt identity
also identifies valid runtime events after startup. These are separate responsibilities.
The registry must retain event provenance when startup cancellation authority ends.

Add an explicit accepted phase to `resumeAttempt`, protected by the registry
mutex. Transfer authority under the existing session cancellation guard.
The transition checks the current attempt and captured execution before acceptance.
It is idempotent and cannot revive an invalidated attempt.

`preparePromptDispatchCallback` receives provider acceptance through
`PromptWithDispatchCallback`. Record the accepted phase before releasing its
dispatch guard and before publication errors can select startup cleanup.
The callback is evidence of provider acceptance even when a subsequent durable
publication or `afterDispatch` hook fails. Preserve the existing accepted-error path.
Do not wait for the blocking prompt call to return before transferring authority.

`runExplicitCancellationOwned` continues to call `invalidateResumeAttempt`.
Invalidation cancels only attempts that still own startup. An accepted attempt
keeps its event identity while normal runtime cancellation settles the turn.
A cancelled turn does not cancel the accepted attempt context or confer startup
teardown authority. Shutdown and explicit runtime teardown retain their existing owners.

`finishPromptExecutorDispatch` and the compound resume continuation distinguish
accepted work from cancelled startup. Normal cancellation returns the turn outcome.
It does not roll back an accepted claim, decorate it as recovery failure, or
call `cleanupCancelledResumeAttempt`. Apply the same rule to model-switch paths
that bypass the ordinary prompt callback.

Retain normal deferred attempt completion so outer compound calls can finish
without treating the successful transfer as a missing registry owner.
Successful tombstones continue to authorize matching execution callbacks.
Both direct and identity-based startup cleanup reject accepted attempts.
Old cancelled attempts remain fenced, including reused execution IDs and retries.
Do not weaken execution generation checks or accept unknown callback identities.

### Entry points and failure boundaries

Apply the transfer to lazy message resume, `ResumeTaskSessionAndPrompt`, and
the handler retry path. Preserve the original identity across internal retry.
Resume without a prompt relinquishes startup ownership once readiness succeeds.
A later ordinary prompt must not inherit startup cancellation authority.

Before acceptance, cancellation still interrupts startup and prevents dispatch.
After acceptance, provider errors use normal prompt failure handling.
Publication failure does not authorize prompt replay or startup cleanup.
A provider that cannot settle cancellation retains the existing bounded escalation
policy. Archive, workflow parking, explicit stop, crash, and backend restart
remain independent reasons for runtime removal or recovery.

### Compatibility, presentation, and evidence

The change is process-local. It adds no durable state, public API, runtime flag,
locale string, or new interface control. Desktop and phone retain their existing
composer, pause control, cancellation progress, and transcript scroll owner.
Tests use those surfaces and require a successful follow-up on the same execution.
Visible boot-row count alone is insufficient because the UI deduplicates resume entries.

Barrier-controlled service tests prove both acceptance orderings and process
survival. They also prove valid events after cancellation and stale-event rejection.
The existing backend cancellation ADR remains authoritative. This local lifecycle
correction needs no separate ADR. Removing attempt tracking at initial readiness
is insufficient for compound resume because it leaves a pre-dispatch cancellation gap.
Removing all attempt checks would lose stale-callback protection.

## Uniform active recovery presentation (September 20)

Implements requirement 006 criteria .11-.20. The shared recovery presentation is
implemented in
[work order 03](../../../plans/session-error-recovery-ui/task-03-uniform-recovery.md).
[Active recovery ownership](../../../decisions/2026-09-20-active-session-recovery-owner.md)
qualifies the earlier placement choices without changing task error scope.

### Evidence and boundaries

Before this refinement, isolated fixtures on :48540 showed four different layouts. `ActionMessage`
selects `ManagedRuntimeNpmRecovery` and `ProviderQuotaRecovery`, each with its
own markup and action order. Generic `SettledFailureMessage` adds another layout.
`ChatInputContainer` independently mounts `SessionStoppedBanner` for FAILED
sessions. As a result, a transcript recovery entry and composer replacement can
both offer Resume. WAITING_FOR_INPUT with `error_message` disables the input but
keeps the greyed composer rather than using the same recovery presentation.
The original waiting-without-error fixture did not cover these states.

Agents own session eligibility and recovery. Tasks retain durable scope and
shared launch errors. This is a frontend ownership refinement: no new backend
commands, secret repair, migration, provider reset policy or scope inference.
The missing-runtime-secret investigation remains separate. Existing task-owned
launch errors retain their shell owner and typed branch controls.

### Selection and ownership

Extend `lib/session-recovery-presentation.ts` with a pure active-card derivation
using the selected session, its current error stamp, loaded recovery metadata,
existing durable resolution evidence, automatic recovery state and eligibility.
Keep timestamp/message-ID legacy fallbacks explicit; never correlate by text.
The model selects cause/title keys, sanitized diagnostic inputs, allowed operation
kinds, prerequisite/reset information and current pending/result state. Do not
create another durable error store or infer a category from nested raw strings.

In mounted task Chat, `ChatInputArea` owns one recovery region. Use the same
model for detail, preview, simple Chat, Quick Chat and passthrough adapters.
Exactly one adapter mounts mutation handlers for the active failure; retain
`session-recovery-pending.ts` admission and `useSessionRecoveryActions` guards.
Use `SessionRecoveryCard` as presentation over this model and the
existing `RecoveryActions` / `SessionErrorDetails` primitives. Bootstrap and
stopped components become adapters where necessary, rather than alternate skins.

| Selected state | Composer region | Transcript |
| --- | --- | --- |
| FAILED with current unresolved error | Recovery card | Compact dated history, no recovery mutations |
| WAITING_FOR_INPUT plus error_message, or a current correlated unresolved recovery record | Same recovery card | Same compact history |
| Usable WAITING_FOR_INPUT with retained resolved error | Normal composer | Readable historical details |
| STARTING from accepted recovery | Same card with pending feedback until authoritative outcome | No premature success or duplicate controls |
| Recovery failed again | Current attempt cause and controls in same region | Preserve old occurrence; append a new record only for a distinct failure identity |
| Workspace restored, agent stopped | Card reports workspace available; resume still required | Original history retained |
| Task-scoped launch error | Existing task shell owner; no duplicate session owner | Session history remains independent |
| Initial failure without session | Same card structure in existing ensure empty state | No invented session history |

Draft text and attachments remain in the existing session draft store while the
editor is replaced. Do not clear them on card mount, failed retry, or automatic
recovery. Keep normal STARTING draft/queue behavior for healthy startup; the
recovery pending presentation applies only to the tracked recovery attempt.
Mounting the owner cannot depend on its chronological row being loaded. Loaded
matching rows become history-only; pagination/unmount cannot remove the active
owner. On standalone transcript surfaces without a composer, retain one explicit
fallback owner using the same card, not a blanket removal of controls.

Uncorrelated errors keep their distinct historical explanation. They do not each
claim mutation ownership of the currently selected blocked session. If identity
or status is ambiguous, show checking/prerequisite feedback with the existing
status-check operation; do not offer a more permissive resume. Multiple actual
session identities remain separate. Shared task errors stay visible alongside
session recovery without being absorbed into it.

### Cause and action policy

All recovery cards separate a subtly amber-tinted warning header with an amber
icon from an opaque neutral action/details body.
The warning header, border and icon remain amber during recovery. A spinner and
localized status text communicate progress without recoloring the warning. Initial
status checks retain amber and ordinary action labels while keeping recovery
actions disabled until the check completes. All actions use the shared Button outline variant, matching nearby workspace
Retry controls without custom colors, borders or disabled opacity. The recommended
action appears first. Each button
has a decorative operation icon and a stable accessible label. A mounted live status
region expands only while checking or recovering; idle status occupies no space. Render choices from the
current failure data immediately while history loads, disabling them until
loading completes; preserve specialized action policies as metadata arrives. Diagnostics
use an opaque neutral surface. Red is reserved for destructive actions and the
short summary of a failed recovery attempt. The same treatment applies on desktop
and phone, with light/dark theme contrast. Display a localized title and
one brief cause, optional structured provider/model/reset context, then actions,
then initially collapsed technical details. Never expose raw diagnostics in copy,
ARIA labels, tooltips or the summary. Runtime and quota adapters provide data,
not their own DOM hierarchy or alert roles.

| Cause or capability | Primary | Secondary/prerequisite |
| --- | --- | --- |
| Typed runtime credential unavailable | Existing eligible retry only | Explain credential must be restored; no promise retry provisions it |
| Transport interruption | Resume session | Existing eligible fresh-start and workspace restore |
| Managed npm resolution or policy block | Retry setup (`runtime_retry`) | Preserve runtime repair semantics; no generic resume substitution |
| Provider quota | No new mutation invented | Existing provider/model/reset guidance; use supplied eligible operations; absent action metadata retains manual Resume with wait/prerequisite guidance; an explicitly empty action list retains guidance only |
| Missing profile | Existing profile-selection/new-session flow | No resume with deleted profile |
| Branch loss | Existing confirmed continue-on-new-branch flow | Preserve branch guard and confirmation |
| Startup recovery guard | Retry if retryable; otherwise none | No workspace restore; never bypass a nonretryable refusal |
| Unknown status | Existing status check | Do not assume session is safe to resume |
| Resolved historical failure | None | Safe details/copy only |

Current quota metadata exposes archive/delete, not model switching. Do not add
Change model or a deadline-driven auto-retry in this package. Keep archive/delete
reachable in existing task menus on desktop and phone, with confirmations. Move
only their recovery-card presentation. Task launch branch controls remain in their
existing scoped owner. Render every eligible recovery operation as an individual
button, with at most one primary and quieter secondary buttons. Do not use a
More options menu or sheet. Omit ineligible operations rather than exposing
bypasses; busy labels describe the actual operation.

Use existing profile and confirmation handlers. A successful request admission is
not proof of recovery. Authoritative matching resolution retires the owner;
stale completion cannot clear a newer failure. A failed request updates the
card cause/details in place without another toast or stacked warning. Preserve
separate resume and restore diagnostic sections and original chronology.

### Workspace navigation and announcements

Extend existing `TaskLaunchErrorProvider` / session owner linkage rather than
introducing global registration. `WorkspaceUnavailable` uses explicit attempt or
bootstrap-failure identity to show a short unavailable state and View recovery.
The link selects Chat, reveals a collapsed recovery region if necessary and
focuses its heading using existing task navigation, including phone tabs. Do not
reconstruct ownership from a common session ID or equal error text. Independent
workspace failures retain their own explanation and valid workspace retry.

One new-failure announcement per stamp comes from the existing announcement
owner. Specialized cards must not mount their own duplicate `role=alert`.
Historical pagination and reload remain silent. Pending/results use polite
status. Clicking View recovery permits focus movement; background errors do not.
After explicit recovery succeeds, return focus to the draft editor only if the
removed recovery control held focus. Otherwise preserve the user's reading or
navigation focus. Confirmation dismissal restores its initiating button or the surviving card
heading when that button no longer exists.

### Diagnostics, scrolling and mobile composition

Retain `lib/session-error-details.ts`: redact before the 4096-character Unicode-safe
budget and copy exactly the displayed sanitized result. No hidden raw payload,
clipboard fallback or new logging. Keep separate operation labels and omit
unusable details. Backend sanitization remains authoritative; the client protects
legacy/request-local data. Historical details remain readable inline in Chat.

Use `SessionStoppedBanner`'s current composer location as the spatial exemplar,
`RecoveryActions` for 28px fine-pointer / >=44px touch controls. Refactor its
existing menu/sheet presentation into directly visible buttons. The recovery region is
inline at the composer location, not an overlay covering Chat. Phone stacks the
primary and secondary buttons at full width, in the same order as desktop.
Desktop places all eligible actions in a wrapping row.

The compact card fits intrinsically. Expanded details can make the recovery
region scroll with `min-h-0`, `overflow-y-auto` and a maximum block size of 50dvh;
the transcript remains its own sibling scroll region. Do not nest a `pre`
scroller within that region. On short viewports/200% zoom all card controls must
remain reachable by scrolling; no fixed footer overlapping transcript content.
Use the existing safe-area wrapper and dynamic viewport behavior. Card content
uses `min-w-0`, explicit flexible width and word/token wrapping. No recovery
operation requires opening an overflow control. Preserve existing confirmation
dialogs/sheets after selecting actions that require confirmation.
Keep business logic and draft state shared across breakpoints; do not mount a
hidden desktop card on phone.

Localize all titles, cause/prerequisite text, busy states and copy feedback in
existing task/chat catalogs (en, pt-pt, zh-cn, zh-hk, zh-tw plus pseudo). Generate
Traditional Chinese with repository tooling. Verification covers the four seeded
failure classes, healthy/resolved controls, mobile task-menu reachability, details,
pending/result/focus behavior, and draft/attachment survival across recovery.


Quota action presentation preserves explicit backend recovery choices, including
an explicitly empty list. When action metadata is absent, show manual Resume
session only. Keep provider/reset guidance visible; a fresh session does not
reset provider capacity. Resume reuses existing admission and profile checks.
