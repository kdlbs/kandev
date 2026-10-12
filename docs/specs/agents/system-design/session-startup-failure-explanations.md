---
status: current
system: agents
created: 2026-09-30
updated: 2026-10-06
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
owners:
  - Kandev
---

# Session Startup Failure Explanations

## Ownership and requirement mapping

This design extends the [recovery presentation contract](session-recovery-failures.md)
for criterion group 006.21-.34. Agents owns selection evidence and recovery
semantics. Tasks retains durable error ownership, atomic failure settlement,
and contribution admission. There is one recovery system and one active owner.

| Criteria | Design section |
| --- | --- |
| .21-.23 | Typed selection evidence |
| .24-.26 | Safe persistence and transport; technical details |
| .27-.29 | Correlation and visible explanations |
| .30-.31 | Empty-turn feedback |
| .32-.33 | Recovery success and history |
| .34 | Desktop and phone composition |

The [implementation package](../../../plans/session-startup-failure-explanations/plan.md)
records the implementation sequence and verification results. This design
documents the delivered behavior and remains the authority for its boundaries.

## Verified baseline and failure path

The planning checkout starts at `ff917a370ac9127fee6f8d7cee80b6b7b9b6175b`.
`git merge-base --is-ancestor fa729f2d7653f4480c107c6a6d50a5452eb3dd0e HEAD`
succeeds. PR #4065's explicit Auggie settings recovery is present.
Its [requirements](../requirements/explicit-resume-settings.md),
[design](explicit-resume-settings.md), and
[decision](../../../decisions/2026-09-29-explicit-resume-settings.md) remain authoritative.

Source inspection establishes these losses:

1. `lifecycle/start_model.go` produces `ModelSelectionDecision`, but strict
   `unavailableStartModel` returns a formatted error. The SetModel failure branch
   also wraps an arbitrary provider error without typed selection evidence.
2. `SessionManager.applyExplicitSessionMode` distinguishes RPC failure,
   unconfirmed result, and confirmed mismatch in formatted errors only.
3. `manager_startup.go` calls `wrapBootstrapFailure`. `bootstrapFailureFor`
   preserves an existing typed failure and recognizes deadlines; other errors
   become `unknown` with generic safe detail. The original diagnostic survives
   `Error`/`Unwrap` for backend consumers but is absent from safe projections.
4. `handleAgentStartFailed` and executor `bootstrapFailureCause` project only
   safe code/detail. Initial startup has an empty operation; current operation
   validation accepts only resume/workspace restore. Fresh-start causes can be
   dropped even if a new code is added.
5. `NormalizeAgentErrorCauses` has no model/mode codes. The frontend parser
   `readAgentErrorCauses` retains only operation, code, and detail.
6. `buildRecoveryCardModel` concatenates durable, automatic, and manual causes.
   `RecoveryCardContent` repeats labels, details, and legacy `error.details`.
   Read-only notice selection disables the main summary.
7. The mounted `session-recovery-model.ts` adapter creates a bootstrap error
   with an empty occurrence time and without outer attempt/execution fields.
   Its details builder repeats causes and request errors;
   `SessionRecoveryCard.recoveryHeaderCopy` can replace the cause with a notice.
8. `computeEmptyTurnNotice` checks `hadOutput`, ephemeral surfaces, existing
   notice IDs, and subagent output. It does not inspect `lifecycle_only` or
   correlated startup failure evidence. This is a verified guard gap; source
   inspection alone does not prove which event produced the reported warning.

The two real failures are supplied incident evidence from another computer.
Do not access their task IDs, databases, or localhost endpoints for validation.
Use scripted agents and disposable fixtures.

## Typed selection evidence

Extend the existing `BootstrapFailure` and `models.AgentErrorCause` contract.
Do not add a parallel error registry, recovery store, or raw diagnostic field.
The outer category remains `generic_launch_failure`, preserving action routing;
`phase=bootstrap` remains a phase. Specific cause codes carry the explanation.

| Code | Allowlisted reason | Evidence boundary |
| --- | --- | --- |
| `model_unavailable` | `requested_not_advertised` | Populated usable catalog omits the strict requested model |
| `model_selection_failed` | `catalog_empty` | No usable model catalog |
| `model_selection_failed` | `selection_unsupported` | Existing method-not-found detection at model application |
| `model_selection_failed` | `application_failed` | Applying an advertised selection fails |
| `model_selection_failed` | `selection_missing` | Exact selection required but no model supplied |
| `permission_mode_failed` | `client_unavailable`, `application_failed` | SetMode cannot run or returns an error |
| `permission_mode_unconfirmed` | `confirmation_missing` | Result unconfirmed or effective mode absent |
| `permission_mode_mismatch` | `effective_mismatch` | Confirmed nonempty effective mode differs from requested mode |

The fifth code, `permission_mode_failed`, is needed because an RPC failure is
not the same as a successful RPC with missing confirmation. Reasons distinguish
cases within that code. Existing timeout, authentication, and cancellation
handling retains its precedence. Context cancellation and terminal-session
sentinels remain outside bootstrap error conversion; a deadline must retain
its timeout meaning instead of being relabelled an application rejection.

Produce typed evidence at the decision branch. Keep `Cause` for `errors.Is` and
`errors.As`; no persistence consumer uses its `Error()` as user detail.
Model selection can return an error containing a typed BootstrapFailure; startup
fills its operation at the execution boundary without replacing evidence.
Wrap source evidence at mode application in the same way.
Do not match complete diagnostic strings. Do not change fallback behavior in
successful compatible-policy paths, context reset, routing, or other providers.
Audit strict error returns in shared model helpers without broadening policy.

Add `start` to the operation allowlist and use it for initial launches.
Keep `resume` and `restore_workspace`. Bootstrap is never the operation cause.
An older record with no operation remains legacy and does not acquire an
invented start/resume distinction.

The optional cause fields use snake_case on the wire:

```text
operation: start | resume | restore_workspace
code: existing code | one of the five codes above
reason?: closed set constrained by code
requested_model?: safe selector ID
effective_model?: safe provider-reported selector ID
attempted_model?: safe model ID passed to a failed application call
requested_mode?: safe selector ID
effective_mode?: safe provider-reported selector ID
prompt_not_sent?: boolean
```

`prompt_not_sent` is optional, not a default false/true inference. Set it true
only at selection checks that precede `dispatchInitialPrompt` and readiness.
Omission means unknown. Effective values describe observed provider state at
failure; they do not mean Kandev accepted the requested setting or admitted a
prompt. For `application_failed`, retain the configured value in
`requested_model` and the exact safe value passed to `SetModel` in
`attempted_model`. The visible cause uses the attempted value so a rejected
advertised fallback or model variation is not described as a failed primary
selection. Omit irrelevant model fields on mode causes and vice versa.

Reuse outer `LastAgentError` fields for occurrence time, phase, stamp, attempt,
execution, and session ownership. Do not copy identity into each durable cause.
Transient UI projections must retain their existing request/attempt identity;
when a response is associated with a durable error, propagate that association
through existing request-local recovery state rather than infer it from text.

## Safe persistence and transport

Extend both lifecycle safe-code validation and task-model normalization.
Normalization validates code/reason combinations, operations, optional booleans,
and bounded selector values. Preserve the existing two-cause bound. Count new
fields inside a bounded evidence budget, rather than treating them as free
metadata. Each selector is at most 256 UTF-8 bytes; reject oversize or unsafe
values rather than cut secrets into apparently harmless fragments. Reason and
operation values are closed enums. Keep the complete diagnostic text within
the existing 4096-byte backend budget and 4096-character client display budget.

Use established `routingerr` credential/full sanitizers before truncation.
Selector validation rejects control characters, credentials, URLs, environment
assignments, filesystem paths, and opaque secret-like values. If a value cannot
be safely retained, omit it and use a localized cause-only fallback. A legitimate
namespaced model ID need not be converted into a guessed friendly name. Provider
catalog labels use the full sanitizer before persistence and a 256-byte bound;
ordinary spaces are allowed, but a label is stored only when sanitization leaves
it unchanged. Frontend legacy-notice rendering applies the same defense. A field
name alone never makes arbitrary provider input safe.

Safe detail is host-authored fallback text or a bounded projection of the
allowlisted evidence. It cannot include raw SetModel/SetMode error messages,
stderr, stack traces, or full catalogs. `SafeDetail` must not pass arbitrary
caller-supplied detail unchecked. Secret-injection tests cover every new field,
legacy details, malformed reasons, and provider display names.

Carry the same normalized cause through both asynchronous failure projections:
`handleAgentStartFailed` and executor `buildBootstrapLastAgentError`.
Retain authentication branches, `fromResume`, cancellation guards, stale-attempt
checks, and exact-execution cleanup. Avoid changes to generic repository and
workspace error classification in `buildLastAgentError`.

`transitionBootstrapFailure` continues to commit error and FAILED state through
its execution/attempt-fenced repository boundary before publication.
`persistBootstrapFailureMessage` and `watcher.AgentEventData.Causes` retain the
same evidence and error stamp. Repair after an accepted state commit remains
idempotent. Persist selection fields in existing JSON metadata; no SQL schema
migration or retroactive enrichment is required.

Audit `LastAgentError`, bounded task-status active-error projection, DTO
converters, session responses, boot payload, and WebSocket events. Typed copies
must retain all optional fields. Extend frontend `AgentErrorCause`,
`readAgentErrorCauses`, active-error adapters, history metadata parsing, and
hydration/store merges. A partial event omitting cause fields cannot erase a
newer accepted snapshot. Unknown fields are ignored, not trusted.

## Correlation and visible explanations

Keep `lib/session-recovery-presentation.ts` as the ownership authority. Update
both the mounted `SessionRecoveryCard` / `useRecoveryPresentation` path and
legacy bootstrap adapters. Pass the complete normalized error identity/time
through `buildBootstrapRecoveryModel`; changing only legacy card content leaves
the mounted recovery header and details unchanged.
Use session, attempt/execution, stamp, and operation from existing error/request
state to associate projections. Prefer the canonical durable stamp. Both backend
projection paths currently compute stamps independently; equal trusted execution
and attempt ownership for the same failed operation can associate those
projections without changing either stamp. A newer admitted attempt never
merges merely because its cause code or text matches. Prefer normalized durable typed evidence for
that same failure over automatic/manual generic wrappers. The card shows one
primary cause. Details serialize that cause once; do not append redundant
legacy `agent_bootstrap; cause=...` text when structured evidence exists.

Never suppress a different attempt merely because code/detail matches.
A failed workspace restore remains a separate operation section. An unrelated
manual request failure without trustworthy correlation remains distinct.
Do not combine later generic recovery failure with an earlier model failure as
if they were one attempt. The current failure owns controls; previous attempts
retain their original dated entries and technical fields.

Derive read-only state from the explicit workspace recovery outcome, not from
any nonempty notice. Read-only success adds a workspace status line below the
session explanation. It cannot replace title/body or clear session failure.
Conversation availability uses retained native identity and authoritative
recovery state. Do not promise resumability when identity is missing or invalid.
The existing read-only label describes stopped-agent workspace access, not a
new filesystem sandbox or proof that the agent is running.

### Proposed English copy

Use `task`/`chat` namespace keys with interpolation, resolved at render time.
The following table specifies visible text for safe known values.

| State | Title | Body |
| --- | --- | --- |
| Saved model absent | Saved model unavailable | Auggie did not list "claude-opus-4-8" among its available models. |
| Catalog empty | Model selection could not be verified | Auggie did not report an available model catalog. The session could not become ready. |
| Selection unsupported | Model selection is not supported | Auggie does not support applying the selected model. The session could not become ready. |
| Advertised model apply failure | Model selection failed | Auggie could not apply model "claude-opus-4-8". The session could not become ready. |
| Required model absent | Model selection is required | No model was selected for this session. |
| Mode unconfirmed | Permission mode could not be confirmed | Auggie did not confirm permission mode "default". The session could not become ready. |
| Mode mismatch | Permission mode did not match | Auggie reported permission mode "plan" instead of "default". The session could not become ready. |
| Mode apply failure | Permission mode could not be applied | Auggie could not apply permission mode "default". The session could not become ready. |
| Mode client unavailable | Permission mode could not be applied | The agent connection was unavailable while applying permission mode "default". |

Replace Auggie only with a registered, trusted agent display name from the
owning session's provider identity. Never convert an arbitrary provider slug
into a guessed brand. With missing provider identity use localized "The agent".
Use a reliable persisted/catalog model label for that exact ID or the ID itself.
For absent/unsafe selectors use a cause-specific sentence without interpolation.
Do not display a current successor model label as the historical requested model.

Eligible provider-restored recovery guidance:
"Resume this conversation using Auggie's restored settings. Saved selections
remain unchanged." Keep the existing disclosure that restored permission mode
can differ and may be more permissive. This guidance does not enable recovery.
For other providers retain their current eligible action guidance.

Workspace status after successful restore:
"The workspace is available in read-only mode. The agent has not resumed."
For retained identity: "Your previous conversation is preserved." This does not
guarantee a provider can restore it. When lifecycle proves dispatch did not
occur, an optional line says "No prompt was sent." Never infer this from FAILED
state alone.

Keep Resume, Start fresh session, Restore read-only workspace, branch recovery,
and relocation safeguards. Add a short selection-failure disclosure to fresh
start: "A fresh session uses your saved selections and may encounter the same
problem." Do not invent a model-switch button or recommend repeated strict
retry as a guaranteed repair.

## Technical details

Use the existing `SessionErrorDetails` display/copy primitive. Format an explicit
allowlist: operation, phase, cause code, reason, requested/effective selectors,
known no-prompt evidence, occurrence time, and the existing host attempt and
execution references. Existing phase remains bootstrap. Labels are localized;
machine values retain their original meaning.

Display and clipboard use one safe formatter. Preserve host correlation UUIDs
only in separately validated identity fields. Accept the host-generated
`resume-<uint64>` attempt format (and legacy UUID attempts) separately from UUID
execution references; do not disable blanket UUID redaction for arbitrary prose.
Apply the legacy free-text sanitizer before assembling safe labelled fields,
then enforce the final display budget.
No raw fallback exists in ARIA, tooltips, hidden DOM, or failed clipboard paths.
Clipboard failure keeps the safe text selectable and announces localized failure.

## Empty-turn feedback

`registerTurnsHandlers` calls `maybeEmitEmptyTurnNotice` for completed events.
Pass existing turn metadata through that path and explicitly exclude
`models.TurnMetaKeyLifecycleOnly` / `metadata.lifecycle_only`.
`task/service.createCompletedTurn` already marks synthetic lifecycle history.
Reuse that contract rather than redefining `had_output=true` for a failed turn.

For non-lifecycle turns settled during pre-readiness failure, correlate the
turn's owning execution/attempt with bootstrap evidence. Audit
`task/service.CompleteTurn`, `publishTurnEvent`, and failure settlement producers.
If the existing turn metadata does not carry trustworthy correlation, add only
the bounded host-owned failure stamp/execution association and no-prompt marker
to that affected turn's existing metadata before completion publication. Keep
its error outcome; do not synthesize normal inference completion.

A completed event may arrive before the failure record. Its own lifecycle or
pre-dispatch evidence must therefore suffice to prevent a notice. A delayed
matching startup record may retract only the deterministic synthetic notice for
that exact turn. Historical bootstrap errors cannot suppress successor turns.
Retain existing explicit `had_output=false`, ephemeral-surface, deterministic
notice-ID, subagent, and slash-command checks for real completed empty turns.
Test both event orders, reload, and an actual empty turn after recovery.

## Recovery success and history

Reuse `handleAgentBootReady` settlement and `persistProviderRestoredResumeNotice`.
Only matching authoritative readiness resolves the current failure. Request
admission, workspace restore, and manual dismissal do not prove agent recovery.
Retain the current execution/attempt fences and original error stamp.

Success metadata already contains the attempt ID, settings policy, skipped
sources, and effective-selector known flags/IDs. Render from this notice's
persisted snapshot, not mutable current store selectors. If needed, retain a
bounded trusted model display name for that exact model in the same snapshot.
Absence of a trustworthy label uses the safe ID; absence of confirmed model uses
no model claim. A later selector update must not rewrite historical success.

Known model notice: "Session resumed with Gemini 3.7 Flash. Your previous
conversation was preserved."
Unknown model notice: "Session resumed. Your previous conversation was preserved."
Both retain the existing saved-selection/permission disclosure. Do not infer
Gemini's friendly name from `gemini-3-7-flash` without reliable catalog evidence.

Before or independently of the best-effort transcript notice, persist an
authoritative bounded record for each successful owned resume that captured an
active failure stamp. The record contains the exact error stamp, host resume
attempt ID, and resolution time; keep at most 16 records and deduplicate by
stamp under the session metadata row lock. The resume attempt's captured stamp
and boot-ready execution/attempt fences establish ownership. Dismissal of the
current active error remains protected by the existing stamp CAS. Preserve
`recovery_resolved_at` for legacy unstamped history, but never use that global
timestamp to settle a stamped failure. A transcript notice can improve the
timeline but is not required to reconstruct success after reload or pagination.

The frontend history matches the bounded resolution record by exact error
stamp and validates the host attempt format. Manual dismissal remains distinct.
Use each historical message's own immutable typed cause, phase, attempt,
execution, and timestamp for its localized explanation and safe copied details;
do not infer old selections from current selectors. When the active card owns
the same occurrence, its history row becomes a compact marker pointing to the
card above rather than another generic error. Distinct attempts and unknown
legacy rows remain separate.
A later failed recovery still displays its own failure; earlier success remains
a dated historical notice. No broad rewrite or deletion of conversation history.

## Desktop and phone composition

Entry point: Chat's blocked composer, with compact dated transcript history.
Reuse the current `SessionRecoveryCard` and `RecoveryActions` and the curated
phone exemplar `mobile/session-mobile-layout.tsx`, including its safe-area and
focused chat layout. No new modal, picker, navigation route, or overflow menu.
This short recovery belongs inline where the user would send a message.

Hierarchy: cause title/body, conversation/prompt evidence, separate workspace
outcome, recovery disclosure/actions, collapsed technical details. Desktop
actions wrap; phone actions stack full width. Use the same domain model/hooks.
Resume is recommended only when already eligible; busy state disables equivalent
controls across consumers. Ordinary fine-pointer controls remain 28px and
phone/coarse-pointer controls measure at least 44px.

Keep the existing recovery region's bounded vertical scroll owner for expanded
content, sibling transcript scrolling, safe-area clearance, and dynamic viewport
limits. No nested diagnostic scroller or document horizontal overflow.
Long selector IDs and translated copy wrap. Technical disclosure and Copy details
have keyboard and touch controls. New failures announce once without stealing
focus; user-initiated success restores composer focus only under existing rules.

Translate in en, pt-pt, zh-cn, ja; generate zh-hk, zh-tw, and pseudo through repo
scripts. No module-scope translations or Unicode em dash. Verify selected locale
changes, pseudo expansion, narrow fine-pointer phone sizes, and short/zoomed panes.

## Compatibility and decisions

| Consumer | Evidence availability | Behavior |
| --- | --- | --- |
| Auggie ACP strict start/ordinary resume | Source model/mode decisions | Specific failure, no silent fallback |
| Auggie explicit recovery | Existing validated provider-restored policy | Same native conversation; saved inputs untouched |
| Other shared ACP providers | Only when their current policy actually fails | Typed explanation without a new recovery exception |
| Non-ACP/passthrough or pre-selection failures | No selection evidence | Existing honest generic/specialized handling |
| Older persisted errors/client | New fields absent/ignored | Safe legacy text and existing action eligibility |
| Workspace/branch/auth/cancel failures | Existing contracts | Existing distinct causes and safeguards |

No new feature flag, provider policy, database, API operation, or architectural
owner is required. The local additive evidence contract and its rationale fit
this design; no new ADR is needed. Existing decisions on
[active recovery ownership](../../../decisions/2026-09-20-active-session-recovery-owner.md)
and explicit Auggie recovery remain in force.

## Fresh-start readiness repair

This implementation correction applies existing criteria 006.4, .8, .9, .16,
.17, .19, .32, and .33. Task-owned history remains governed by
[task error ownership](../../tasks/requirements/task-launch-failure-recovery.md).
Delivery: [Fresh-start recovery](../../../plans/fresh-start-recovery/plan.md).

A failed-start fresh retry keeps a nonempty `TaskDescription` in the resume request.
`SessionManager.dispatchInitialPrompt` dispatches that prompt without its no-prompt
`markReady` branch. Thus `handleAgentBootReady` does not run the stamp-specific
resolution path. A normal completed turn proves agent output, but does not retire
the persisted bootstrap error. Do not fix this by clearing every error on RUNNING,
a user message, or turn completion.

Separate fresh startup from initial submission dispatch through the task system's
[owned recovery flow](../../tasks/system-design/prompt-attachments.md#owned-recovery-dispatch).
The owned resume attempt captures the unresolved stamp before provider reset.
Its provider-confirmed boot-ready event records a `SessionRecoveryResolution`
through `markRecoveryResolvedForAttempt`. Reuse `RecordSessionRecoveryResolution`
and stamp-CAS dismissal through `dismissRecoveredAgentErrorForAttempt`.
No new error store or generic resolution timestamp is authoritative.

Propagate the same attempt identity through no-prompt launch, readiness, and
continuation. A readiness event without an owned matching attempt cannot retire
an error. A successor failure wins its stamp comparison. Failed or cancelled
startup retains the current controls; workspace-only restoration cannot resolve it.
After readiness, a replay delivery failure owns a new correlated error.

Publish the matching inactive error event and session metadata after durable
resolution. Readiness, error updates, HTTP hydration, and reconnect must converge
without a later user message. Preserve the original dated error and details.
Frontend consumers use the bounded stamp-specific proof and existing dismissal
state, never preview text or a global timestamp, to retire recovery actions.
The composer retains its draft and selected attachments. Return focus only from
the disappearing user-initiated recovery controls.

Use `SessionRecoveryCard`, `useRecoveryPresentation`, and
`lib/session-recovery-presentation.ts` for both desktop and phone. Successful
recovery restores the existing composer. Historical entries remain in the
transcript without mutation controls. A later failure owns its own active card.
The dedicated phone layout retains stacked 44px targets, safe-area handling,
and one transcript scroll region. No new layout or user-facing label is needed
for successful recovery. Test both WS event orders, missing optional success
notice, reload, sibling failure, and stale prior completion.
