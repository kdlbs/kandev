---
status: current
system: platform
created: 2026-10-02
updated: 2026-10-26
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
amends the earlier read-only admission boundary. Successful foreground tool
completion permits a new internal
`continue` turn even after shell, write, or MCP work. It never permits original
prompt replay. The [runtime continuity contract](transient-turn-runtime-continuity.md)
preserves proven usable ACP runtimes. [Provider error recovery](provider-error-recovery.md)
owns truthful diagnostic projection and deterministic error categorization.

The [graduation decision](../../../decisions/2026-10-08-unconditional-interruption-continuation.md)
makes supported continuation normal recovery in every profile. Availability no
longer depends on an installation toggle; provider capability and prompt safety
remain the admission boundary. Its implementation is tracked separately from
the completed continuation packages.

The [capacity amendment](transient-turn-runtime-continuity.md#capacity-recovery-admission) uses a separate policy for completed effects on a retained runtime.
Its live-runtime support does not satisfy this document's native restoration capability or versioned foreground-outcome predicate.

The [pending-tool policy decision](../../../decisions/2026-10-26-cursor-pending-tool-continuation.md)
amends foreground admission for retained Cursor transport failures. This is an
agent-managed recovery policy, not an attestation that pending tools are safe
or stopped. The existing capacity policy remains separate.

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
an older wire snapshot as proof of successful effectful-tool completion. Legacy
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
| Cursor V3, retained runtime, transport loss, valid pending foreground tools | Internal `continue`; Cursor manages unfinished work | Mixed-tool fixture and external probe limitations |
| V1/V2 pending tools; failed, cancelled, conflicting, malformed, or incomplete evidence | Manual recovery | Negative and version-skew fixtures |
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
Only an offered allow option resolves successfully. Under V1/V2 every permission's
matching tool must report completion. Under V3 it must have a valid observed
foreground ledger entry; it can remain pending after approval. A delayed response updates its captured turn, and
published terminal snapshots remain immutable.

V1/V2 admission uses a universal predicate: every observed foreground call must
have successful completion, and no unresolved permission/background owner may remain.
V3 permits each call to be completed, pending, or in progress under the retained
transport policy below. One failed or invalid call rejects the whole attempt.
An empty complete ledger permits output-only recovery. Cover one completed call plus one pending call: V1/V2 refuse; the retained V3
transport policy admits it. Also cover completed plus pending plus failed calls,
which must refuse under every version. Do not reconstruct this evidence from persisted chat.

Capture the immutable snapshot before prompt-end cancellation sweeps. A synthetic
cancelled status is not success. Historical restore notifications do not enter
the active ledger. Provider response-attempt resets cannot erase prompt outcome
evidence. Preserve unsafe episode evidence across continuation attempts; an
invalid earlier attempt cannot become eligible because a later ledger is empty.
Valid V3 pending foreground work is not invalid evidence under the new policy.
Successful completed writes need not poison a later continuation episode.

The new native compatibility probe operates only on a disposable conversation.
Run completed shell and file-write work, interrupt the subsequent unfinished
request at the provider boundary, submit exactly `continue`, and assert a new
response in the same native ID. Observe the original dispatch count and workspace
result so original-prompt replay cannot pass the probe. Record Cursor version,
restore advertisement, and sanitized result. Native evidence is required before
V2 support is declared ready; mock success alone does not establish it.

### Agent-managed pending-tool policy

Add `ContinuationNativeHistoryAgentManagedToolsV3` with wire value
`native_history_agent_managed_tools_v3` in `types/streams/continuation.go`.
Keep V1/V2 meanings unchanged. Cursor's dialect emits V3; other production
providers do not acquire it through shared ACP code. The mock producer supports
it only for explicit fixtures. Old consumers reject the unknown support value.

Add `PermissionsResolved bool` (`permissions_resolved`) to
`ContinuationSafetySnapshot`. V3 requires explicit true; absence fails closed.
Keep `Pending` true whenever foreground tools remain unfinished. Do not clear it
to gain admission. V1/V2 continue to reject `Pending`; V3's `SafeFor` validates
known evidence, generation, `Unsafe == false`, and resolved permissions.
The method denotes policy eligibility, not proof of tool completion.

In `adapter_continuation.go`, separate permission eligibility from unfinished
foreground work. Set `PermissionsResolved` only when the captured turn has zero
outstanding permission requests and every granted permission names an observed
valid ledger entry. Missing entries, denial, cancellation, invalid status,
background ownership, overflow, and malformed evidence still fail closed.
Update `beginContinuationPermission` to accept V3 as well as V2. Preserve the
captured-turn callback owner and all permission option validation.

In `continuationBindingForFailure` / `continuationFailureHasEvidence`, admit a
V3 pending snapshot only for `agent_transport_lost` with
`PromptFailureDispositionRetainRuntime`. Keep classification confidence and
all existing task, native-ID, execution, generation, queue, and profile checks.
A V3 snapshot without pending work preserves the existing completed-tool policy.
Do not extend `CapacityContinuationSnapshot` or its separate timing contract.

Bind a pending-tool episode to the retained execution. Revalidate runtime
continuity at timer fire and final dispatch, not only at failure receipt.
If preparation would replace, tear down, load, or resume that runtime, stop the
pending-tool episode with an accurate refusal. Do not silently convert it to
saved-history restoration. Keep this restriction for the episode after an
accepted continuation, even if a later snapshot contains no pending tools.

Send exact `continue` through the existing dispatcher and five-attempt schedule.
No process polling, command/title heuristics, additional reconciliation prompt,
or mandatory delay beyond existing backoff is introduced. Kandev does not join,
kill, or replay native tools. Preserve original prompt replay refusal after effects.

Capture evidence before synthetic prompt-end cancellation as today. That UI
cleanup never changes `Pending`, establishes termination, or poisons the earlier
snapshot as a genuine user Stop. Do not fabricate missing native results.
Late notifications must not mutate published snapshots or authorize successor
work; retain captured-turn and ambiguous-handoff guards. A future live event
without reliable generation attribution cannot prove a pending tool completed.

The [external probe](https://github.com/kdlbs/kandev/issues/4801#issuecomment-6418179442)
shows a surviving command, absent terminal ACP results, and a qualified repeat.
It does not establish generic reconciliation or no-overlap guarantees. This
policy intentionally does not require that proof. Deterministic integration
fixtures must establish host dispatch, identity, budget, and exclusion behavior.

## Admission

`handleTransientFailure` remains the sole concrete-profile retry owner. Preserve
`promptAttemptPreResultSafe` for original replay. For a supported native
conversation, choose continuation after output/tools when:

1. The foreground prompt has settled and notification ordering is complete.
2. `routingerr` supplies a current high-confidence short-retryable provider
   failure, including transport loss, availability, overload, supported capacity,
   and the explicit retryable resource-exhaustion category.
3. Snapshot, local prompt evidence, execution, generation, saved native ID,
   and task-session identities agree under their declared support version.
4. All foreground tool outcomes meet the applicable versioned boundary, and no
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
Eligible model-capacity recovery on a retained runtime instead uses
[capacity episode timing](transient-turn-runtime-continuity.md#capacity-episode-timing).
This exception does not extend transport-loss restoration or other error policies.

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

Retained-runtime refusals must carry `recovery_reason` from the same admission
checks used for dispatch. Keep the closed existing reasons (`unsafe_work`,
`unsupported_restore`, `missing_evidence`) and do not derive them from prose.
For zero started attempts, `retainedTurnRecoveryKey` selects localized reason
copy prefixed with “Automatic continuation did not start.” An absent historical
reason uses a neutral fallback; it must not invent a cause or claim exhaustion.
Positive attempt counts use the actual count. Pending V3 tools alone produce
no refusal. Reuse existing progress and Cancel controls; do not add a warning
or confirmation dialog.

Retain task Chat as the only transcript scroll owner. Desktop keeps existing
compact inline controls. Phone reuses the shipped continuation card and stacked
44px Cancel action within the safe-area-aware chat/composer; no new modal,
drawer, navigation, or scroll region is introduced. Technical diagnostics wrap.
Use the same metadata and localization contract in both layouts. New copy goes
through all seven supported language catalogs and generated Traditional Chinese.
ASCII outcome previews and desktop/mobile checks are in the delivery plan.

## Availability, persistence, and observability

Remove `features.providerInterruptionContinuation` and
`KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION` from live profile,
`FeaturesConfig`, runtime-registry, boot feature-state, and frontend-default
contracts. Append their exact identities to `retiredRuntimeFlagIdentities`;
retain existing override rows as inert data. Old YAML, environment, or override
values must not affect continuation. Other runtime flags retain their existing
precedence and restart semantics.

Remove the corresponding boolean from `orchestrator.Config`, backend composition,
`AgentctlStartupConfig`, agentctl server/instance configuration, process adapter
configuration, and shared transport configuration. Default-on behavior must not
depend on a propagated true value or a replacement opt-in. Managed and standalone
agentctl collect bounded safety evidence whenever the dialect, native capability,
session identity, and active prompt support it. Keep historical-load suppression,
captured permission ownership, notification ordering, and generation fencing.

Remove only the availability conditions from continuation admission, permission
tracking, safety poisoning, and final dispatch-owner validation. Keep all safety,
task ownership, profile, queue, cancellation, and provider-version conditions.
New consumers continue to reject missing or unknown evidence from old remote
helpers; removal of the toggle does not invent support for those helpers.
Old JSON fields received by new components have no semantic effect.

There is no active `disabled` continuation refusal. Existing persisted records
may still describe a historical disabled installation; preserve their display
compatibility without keeping a live flag or mutating old messages. Upgrade does
not reconstruct or restart an abandoned process-local recovery episode. This
delivery does not mutate or restart the user's live instance.

There are no new tables, migrations, or durable retry jobs. Notices remain
projections. Backend restart retires process-local recovery without redispatch;
live adopted turns retain existing reconciliation protection. Diagnostics are
bounded and sanitized before persistence, display, generic logs, or stream events.
Log semantic code, outcome/refusal reason, mode, phase, attempt count, support
version, and execution/generation correlation, without prompt or tool contents.
Identifiers never become metric labels.

## Validation and delivery

[The graduation plan](../../../plans/provider-interruption-continuation-graduation/plan.md)
owns flag removal, default-behavior checks, and public guidance updates.
[The completed continuation plan](../../../plans/cursor-hidden-continuation/plan.md)
retains its native compatibility evidence and historical results. Retain the completed October 2/3 plans as
historical results; their V1 read-only and long-instruction assertions are
superseded only where the completed-tool contract explicitly changes them. Test actual
provider category, V1/V2/V3 omission/skew, versioned foreground admission, exact hidden
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

The [pending-tool package](../../../plans/cursor-pending-tool-continuation/plan.md)
owns V3 admission and refusal feedback. Earlier completed plans remain historical.
