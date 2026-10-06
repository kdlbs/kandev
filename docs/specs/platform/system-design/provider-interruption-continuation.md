---
status: current
system: platform
created: 2026-10-02
updated: 2026-10-05
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
owners:
  - Kandev
---

# Provider interruption continuation system design

## Purpose and ownership

Platform owns same-conversation automatic recovery admission, scheduling,
settlement, cancellation, and visible status. Agents owns tested native restoration
and ordered provider tool outcomes; Tasks owns turn and message records.

The [completed-tool continuation decision](../../../decisions/2026-10-05-hidden-completed-tool-continuation.md)
amends the earlier read-only admission boundary. This draft changes the current
implementation: successful foreground tool completion permits a new internal
`continue` turn even after shell, write, or MCP work. It never permits original
prompt replay. The [runtime continuity contract](transient-turn-runtime-continuity.md)
preserves proven usable ACP runtimes. [Provider error recovery](provider-error-recovery.md)
owns truthful diagnostic projection and deterministic error categorization.

The [capacity amendment](transient-turn-runtime-continuity.md#capacity-recovery-admission) uses a separate policy for completed effects on a retained runtime.
Its live-runtime support does not satisfy this document's native restoration capability or read-only safety predicate.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-INTERRUPTION-CONTINUATION-001` | Compatibility and evidence; Admission; Internal dispatch |
| `REQ-PLATFORM-INTERRUPTION-CONTINUATION-002` | Episode ownership; Settlement and restoration; Rollout |
| `REQ-PLATFORM-INTERRUPTION-CONTINUATION-003` | Feedback and mobile composition; Persistence and observability |

## Compatibility and evidence

Real anchors are `ContinuationSupport`, `ContinuationSafetySnapshot`, ACP
`promptTurnState`, `Adapter.observeContinuationSafety`, lifecycle terminal
snapshots, `watcher.AgentEventData`, `promptAttemptEvidence`, and
`continuationBindingForFailure`. Keep provider recognition inside the ACP dialect;
no orchestrator branch inspects Cursor names, tool titles, or shell commands.

Introduce `native_saved_history_completed_tools_v2` alongside
`native_saved_history_v1`. V1 retains its read-only meaning. Do not reinterpret
an older wire snapshot as proof of successful effectful-tool completion. New
Cursor and test-only mock producers attest V2; absent or unknown support fails
closed. A new backend may consume a valid V1 output/read-only snapshot under
its original boundary. An old backend receiving V2 must refuse continuation.

Keep snapshot generation, completeness, unsafe, and pending fields. Add a
bounded successful-foreground-tool count separately from `completed_reads`;
do not report successful shells or writes as reads. Never include raw inputs,
outputs, paths, prompts, or provider identity tokens in the snapshot.

| Producer or shape | Behavior | Evidence and fallback |
| --- | --- | --- |
| Cursor ACP V2 with saved history and all foreground tools completed | New internal `continue` using the same native conversation | Ordered fixtures, isolated native completion probe |
| Cursor ACP V1 output or completed supported reads | Existing limited continuation | Existing native compatibility evidence |
| Pending, failed, cancelled, conflicting, malformed, or incomplete outcomes | Manual recovery | Negative and mixed-completion fixtures |
| Unresolved permission, active background work, or subagent work | Manual recovery | Existing ownership signals and negative fixtures |
| Old remote helper, unknown support, or missing identity | Existing policy only | Remote JSON and version-skew tests |
| Other providers, dynamic profiles, Office, utilities, or passthrough | Existing policy owner | Compatibility regressions |
| Mock ACP with fixture-owned recovery episode | Test-only V2 contract | No production trust in a mock marker |

### Ordered completion ledger

Replace read-kind authorization in `adapter_continuation.go` with a bounded
foreground outcome ledger for V2. Track every unique non-empty ACP tool-call ID
from admission until terminal prompt settlement. A valid successful `completed`
status is sufficient for every foreground tool category, including `execute`,
`edit`, `other`, and ordinary foreground MCP. Categories and command text are
not evidence that the action is repeatable. ACP optional kind omission does not
make an otherwise complete foreground outcome ambiguous.

Pending and in-progress calls remain unresolved until provider-reported
completion. Failed or cancelled outcomes, unmatched updates, conflicting status
transitions, reused IDs, malformed frames, truncation, and overflow permanently
invalidate that attempt. Keep the existing 256-tool limit. Distinguish foreground
MCP from active nested, subagent, or background ownership using existing typed
lifecycle signals. A resolved allowed permission is not an unresolved permission;
a denial or user cancellation cannot be overridden by recovery. Cover the
ordinary permission-resolution path rather than poisoning every resolved request.
Track pending permission counts and their tool IDs on the owning prompt turn.
Only an offered allow option resolves successfully. Every permission's matching
tool must eventually report completion, including when approval precedes the
first tool notification. A delayed response updates its captured turn, and
published terminal snapshots remain immutable.

Admission uses a universal predicate: every observed foreground call must have
successful completion, and no unresolved permission/background owner may remain.
An empty complete ledger permits output-only recovery. Include the mixed case
of one successful call and one pending call; one completion never authorizes
the whole turn. Do not reconstruct this evidence from persisted chat.

Capture the immutable snapshot before prompt-end cancellation sweeps. A synthetic
cancelled status is not success. Historical restore notifications do not enter
the active ledger. Provider response-attempt resets cannot erase prompt outcome
evidence. Preserve unsafe episode evidence across continuation attempts; an
uncertain earlier attempt cannot become eligible because a later ledger is empty.
Successful completed writes need not poison a later continuation episode.

The new native compatibility probe operates only on a disposable conversation.
Run completed shell and file-write work, interrupt the subsequent unfinished
request at the provider boundary, submit exactly `continue`, and assert a new
response in the same native ID. Observe the original dispatch count and workspace
result so original-prompt replay cannot pass the probe. Record Cursor version,
restore advertisement, and sanitized result. Native evidence is required before
V2 support is declared ready; mock success alone does not establish it.

## Admission

`handleTransientFailure` remains the sole concrete-profile retry owner. Preserve
`promptAttemptPreResultSafe` for original replay. For an enabled supported native
conversation, choose continuation after output/tools when:

1. The foreground prompt has settled and notification ordering is complete.
2. `routingerr` supplies a current high-confidence short-retryable provider
   failure, including transport loss, availability, overload, supported capacity,
   and the explicit retryable resource-exhaustion category.
3. Snapshot, local prompt evidence, execution, generation, saved native ID,
   and task-session identities agree under their declared support version.
4. All foreground tool outcomes meet the applicable V1/V2 boundary, and no
   human queue entry, unresolved permission, or other live owner has priority.
5. The same task, workspace, execution profile, model, mode, and permission
   configuration still owns the conversation.

For pre-result attempts, retain existing safe replay admission. A continuation
replacement never falls back to replaying the original prompt. Hard quota,
authentication, resume-corruption, user cancellation, unknown diagnostics,
archive/deletion, and stale or missing evidence retain manual or existing policy.
Use shared semantic codes, not diagnostic prose, to select recovery. The
resource-exhaustion category grants no reset deadline or provider-switch policy.

## Internal dispatch

Set `continuationInstruction` to the exact lowercase ASCII text `continue`.
Use the existing `promptTask` path with `dispatchOnly=true`,
`internalContinuation=true`, `preservePromptContext=true`, and
`disableDispatchRetry=true`. It already bypasses normal prompt augmentation,
original-prompt caching, and human-dispatch retirement of its own episode.
Revalidate those boundaries with tests rather than adding another dispatcher.

No original user text, attachments, task-specific instructions, diagnostic,
plan injection, or context-dependent suffix is added to the recovery payload.
Retain normal turn acceptance and automatic-origin ownership. Do not create a
user-message row, prompt-index entry, client optimistic message, or hidden-message
placeholder. Do not hide arbitrary messages whose text equals `continue`;
user-authored `continue` remains ordinary visible history. The provider-native
history necessarily contains the recovery instruction, which is allowed;
the visibility contract concerns Kandev's chat and user-prompt history.

A proven usable runtime receives the prompt without teardown, initialize,
load, or resume. When runtime loss is real, use existing strict native restoration
with `RequiredNativeConversationID` and reject any fallback to `session/new`.
Restore the same model/mode/permission selection; never silently switch providers.

## Episode ownership and settlement

Reuse `transientRetryEntry`, its notice guard, timer cancellation, and accepted
execution/generation fencing. Keep one episode with at most five additional
attempts and nominal waits of 5, 10, 20, 40, and 60 seconds; validated short timing
hints retain existing handling. This package adds no jitter or new scheduler.

One attempt owns bounded restoration and at most one continuation dispatch.
Pre-dispatch transient restore failure consumes an attempt. Accepted continuation
that later fails consumes the same episode budget. Output never resets the count.
Ambiguous acceptance stops rather than resending. Keep the existing 30-second
teardown and 60-second preparation limits; do not impose a new turn timeout.

Revalidate at timer fire, restoration, and the final prompt admission boundary.
Cancel, stop, archive, delete, start fresh, new user input, queued human work,
profile/model change, and shutdown supersede automatic recovery. Waiting Cancel
leaves an idle usable runtime alive. Cancel after dispatch uses ordinary prompt
cancellation. Stale failures cannot settle or stop a successor turn.

Settle the interrupted turn through error termination, not ordinary successful
completion. Recovery does not advance workflow, drain later queued work, or
report CI success. A successfully completed continuation enters normal completion
processing exactly once. Accepted turns retain durable identity without a
fabricated user message.

## Feedback and mobile composition

Reuse the single inline `TransientRetryNotice`, accepted-turn output, and durable
failure records. Waiting shows provider, actual failure category, attempt/countdown,
and Cancel. Continuing shows backend-owned phase, without claiming a TCP failure
for resource exhaustion. Cancellation/exhaustion reports attempts actually started.
The internal prompt never renders in any message surface or user-prompt index.

Retain task Chat as the only transcript scroll owner. Desktop keeps existing
compact inline controls. Phone reuses the shipped continuation card and stacked
44px Cancel action within the safe-area-aware chat/composer; no new modal,
drawer, navigation, or scroll region is introduced. Technical diagnostics wrap.
Use the same metadata and localization contract in both layouts. New copy goes
through all seven supported language catalogs and generated Traditional Chinese.
ASCII outcome previews and desktop/mobile checks are in the delivery plan.

## Rollout, persistence, and observability

Reuse `features.providerInterruptionContinuation` and
`KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION`, with restart-required,
high-risk experimental metadata. Update its read-only description to match
completion-based admission. Keep prod/dev/e2e defaults and frontend defaults off.
Environment > SQLite override > profile precedence and all disabled entry gates
remain intact. Error-categorization correction applies regardless of this toggle.
This package neither promotes the flag nor changes the live installation.

There are no new tables, migrations, or durable retry jobs. Notices remain
projections. Backend restart retires process-local recovery without redispatch;
live adopted turns retain existing reconciliation protection. Diagnostics are
bounded and sanitized before persistence, display, generic logs, or stream events.
Log semantic code, outcome/refusal reason, mode, phase, attempt count, support
version, and execution/generation correlation, without prompt or tool contents.
Identifiers never become metric labels.

## Validation and delivery

[The delivery plan](../../../plans/cursor-hidden-continuation/plan.md) owns exact
commands and regression mappings. Retain the completed October 2/3 plans as
historical results; their V1 read-only and long-instruction assertions are
superseded only where this draft explicitly changes the contract. Test actual
provider category, V1/V2 omission/skew, universal completion admission, exact hidden
payload, same native identity, shared budget, cancellation, human supersession,
reload/pagination/second viewer, and ordinary visible user `continue`. Keep the
data-only generic ACP error projection, dynamic/Office behavior, and original
replay fence under their existing specifications.

ACP permission requests and late tool notifications do not identify their
originating prompt generation across a handoff. A successor that inherits the
predecessor gate therefore cannot establish exclusive foreground ownership and
is ineligible for automatic continuation. A later serialized turn may establish
fresh evidence. Permission resolutions that began before the handoff retain
their captured turn owner.

Confirmed continuation cancellation requires WAITING_FOR_INPUT independently
of workflow completion eligibility, under the existing captured-turn guard.
Disabling workflow advancement must not skip the session settlement.
