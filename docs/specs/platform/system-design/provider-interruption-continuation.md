---
status: current
system: platform
created: 2026-10-02
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
owners:
  - Kandev
---

# Provider interruption continuation system design

## Purpose and boundaries

Extend concrete-profile interactive recovery with a separate post-output
continuation mode. Platform owns admission, the recovery episode, and its
projection. Reuse Tasks' prompt admission, native-session identity, error
settlement, and cancellation. Provider dialects remain the only place that
recognizes supported wire capabilities and tools.

The [decision](../../../decisions/2026-10-02-safe-interrupted-conversation-continuation.md)
extends [Provider Error Recovery](provider-error-recovery.md). Its replay fence
and classifier remain intact; criteria `.8` and `.15` cross-reference this
separate continuation lifecycle. Completed companion plans remain historical
delivery evidence.

## Requirement mapping

| Requirement                                  | Design sections                                                      |
| -------------------------------------------- | -------------------------------------------------------------------- |
| `REQ-PLATFORM-INTERRUPTION-CONTINUATION-001` | Compatibility and evidence; Recovery admission; Restore and dispatch |
| `REQ-PLATFORM-INTERRUPTION-CONTINUATION-002` | Episode ownership; Settlement; Rollout and restart                   |
| `REQ-PLATFORM-INTERRUPTION-CONTINUATION-003` | Feedback contract; Desktop and phone composition; Observability      |

## Evidence from the investigation

On 2026-10-02 the reported task's lifecycle classified the Cursor failure as
high-confidence `agent_transport_lost`, `auto_retryable=true`. Orchestration
then logged `refusing automatic transient retry without safe prompt-attempt
evidence`. Ordered frames contained output and tools before the terminal
diagnostic. Existing tests intentionally require that refusal.

The reported version's `createRecoveryStatusMessage` chose
`transientFailureExhaustedMessage` whenever classification permits short retry,
even if no timer was armed. This explained the false exhaustion wording. The
correction is independent of the new release toggle.

## Compatibility and evidence

Existing code anchors are `CursorACP.Runtime` and its native-session setting,
ACP `Adapter.LoadSession`, `promptTurnState`, `observeCursorRetriableEvidence`,
`sendPrompt`'s notification barrier, lifecycle `AgentEventData`, and
orchestrator `promptAttemptEvidence` in `dynamic_evidence.go`.

Introduce internal typed `ContinuationSupport` and
`ContinuationSafetySnapshot` fields on the streamed terminal failure contract.
Propagate through agentctl, runtime/lifecycle, and watcher conversion, including
remote JSON round trips. An omitted field is unsupported. Support identifies a
versioned restore contract, never arbitrary provider-controlled policy text.
Set support in the provider dialect after negotiated native load/resume support
and tested constructor policy agree. There is no new universal ACP extension
advertised to clients and no provider-name branch in orchestration.

| Provider/transport shape                                                         | Intended behavior                                                                                 | Evidence and unsupported fallback                                                        |
| -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| Cursor ACP with tested native restore, current generation, saved conversation    | Eligible output-only or completed-read continuation                                               | Captured/sanitized fixtures plus isolated native restore check; failed restore is manual |
| Cursor ACP with only `kind=read` and completed status                            | Eligible only after the native compatibility check proves this wire shape is a read-only contract | Positive and conflicting-frame tests; unrecognized identity/kind is unknown              |
| Cursor ACP search or a title such as `Read File` without positive typed evidence | Unsupported tool for continuation                                                                 | No title, payload-normalizer, shell-command, or heuristic inference                      |
| Cursor ACP pending reads, writes, execute, MCP, permissions, monitors, subagents | Manual recovery                                                                                   | Negative matrix, including writes from previous recovery attempts                        |
| Other production adapters, old remote agentctl omitting the snapshot             | Existing recovery only                                                                            | Compatibility-negative tests, no implicit support inheritance                            |
| Mock ACP in isolated dev/E2E                                                     | Test-only version of the same typed contract                                                      | Constructor provenance and mock-only marker; no production marker acceptance             |
| Dynamic profiles, Office, utility, passthrough                                   | Existing policy owners                                                                            | Regression tests; no legacy-loop interception                                            |

The native compatibility check is a delivery prerequisite: a disposable
workspace/conversation must show that restore returns the same provider session
and retains the saved request/history before accepting a continuation. The
probe must finish the interrupted read request in that same native ID, allowing
a safe re-read if Cursor did not persist its interrupted result. Kandev retains
its own transcript and completed tool records; native interrupted-result
retention is not required and must not be claimed. Record CLI version, advertised capabilities, sanitized
frame shapes, and results in the plan's compatibility evidence. Do not copy a
real user transcript or operate on the user's failed session. If an installed
version cannot establish the contract, do not enable automatic support for that
shape; report Task 01 incomplete rather than claiming mock proof is native proof.

Primary contracts are [Cursor ACP](https://prod.cursor.com/docs/cli/acp) for
`session/load` and the [ACP v1 tool schema](https://github.com/agentclientprotocol/agent-client-protocol/blob/main/schema/v1/schema.json)
for tool categories and terminal statuses. These describe restoration and tool
reporting, not exactly-once invocation recovery. Native compatibility evidence
must establish the additional supported Cursor behavior rather than infer it
from those documents.

The ordered ACP worker maintains prompt-local tool IDs and terminal status.
The only initial positive tool classification is the tested Cursor read shape.
All other classifications are `unknown` or `state_changing`. Conflicting
updates are sticky unknown; reused IDs, unmatched updates, nesting, missing
status, truncation, and overflow fail closed. A maximum of 256 observed tool
identities bounds the ledger; overflow makes the episode ineligible. Cancelled
statuses synthesized during prompt-end sweeping are not successful outcomes.
Capture the immutable safety snapshot before those sweeps. Do not relax the
existing `EffectObserved` boolean for read-only tools.

Snapshot identity includes session, execution, and non-zero prompt generation;
fields include support version, evidence completeness, unsafe activity observed,
and pending/unknown outcome presence. No raw inputs, outputs, paths, prompts,
or tool descriptions cross this new policy channel. Lifecycle captures the
snapshot before terminal bookkeeping and carries it with `agent.failed`.
Orchestration retains its local identity fence and never reconstructs safety
by scanning persisted chat. The episode combines snapshots monotonically so a
write in any earlier continuation attempt remains unsafe.

History emitted during native restore is historical context, not active-turn
tool evidence. Open a new ledger only at prompt admission for the replacement
generation. Retain the episode's prior snapshot separately; never let historical
replay, a response-attempt reset, or a new empty ledger clear episode safety.

## Recovery admission

Keep the existing classification boundary and `handleTransientFailure` owner.
Select recovery mode `replay` or `continue`:

1. Reject stale identity, cancellation/shutdown, task archive/deletion, dynamic
   or Office ownership, passthrough, incomplete evidence, and pending manual or
   user-owned work.
2. Require the existing high-confidence, auto-retryable classification and
   a known current terminal foreground prompt. For the first supported
   continuation contract, the admission cause is `agent_transport_lost`.
   Do not broaden to all `RetriableError` prose or data-only errors.
3. If `promptAttemptPreResultSafe` succeeds, use existing replay mode.
4. Otherwise require the enabled toggle, positive support, native saved
   conversation identity, fully known safety snapshot, no state-changing or
   unknown tools, no pending tools/permissions/background work, and no queued
   prompt waiting to own the session. Admit continuation.
5. If neither mode is eligible, publish an explicit manual disposition and
   preserve the interrupted history and saved identity.

Unsupported or unknown states never become permission for a fresh launch.
`FallbackAllowed` stays false for transport loss. Provider-progress clearing
and the post-prompt notification barrier remain unchanged.

## Episode ownership

Extend `transientRetryEntry`, the session notice guard, and existing retry
cancellation, rather than adding a second scheduler. The current entry's
identity and cancellation context own each attempt. Retain mode, started and
scheduled counts, saved identity fingerprint, restored execution, and accepted
execution/generation. Resume-attempt identity and teardown claims fence late
work. Continuation stays sticky and each replacement needs fresh safe evidence.
Reserve entries under the existing per-session guard and arm after settlement.
No database I/O or RPC runs while the global runtime-state mutex is held.

Use `transientMaxAttempts=5` and `transientRetryDelayFor`'s existing 5/10/20/40/60
ladder. Preserve existing validated short timing hints. One attempt contains
bounded teardown, restore, and at most one continuation dispatch. Reuse the
30-second teardown bound and bound restore/acceptance preparation to 60 seconds;
normal provider turn execution is not subject to a new recovery timeout.
Transient, definitely pre-dispatch restore failures consume the same budget.
An acknowledged continuation that later fails also consumes the same episode
budget. No output/chunk resets it; reset only on ordinary successful completion
or terminal retirement. Never retry ambiguous prompt acceptance.

At timer fire, before launch, after restore, and immediately before prompt
dispatch, revalidate ownership, cancellation, task/session eligibility, queued
human work, configuration identities, and exact provider conversation. Use
existing prompt/foreground admission as the final serialization boundary.
Admit new user work by retiring automatic work first; queued work gets priority
over a waiting automatic continuation and does not run behind a stale timer.
Late failed/stopped events for the torn-down execution cannot terminate the
replacement. Complete the old execution's cleanup claim before replacement;
if teardown cannot confirm absence of live work, stop for manual recovery.

Cancelling a timer and cancelling an admitted prompt are separate operations.
If continuation has crossed prompt admission, cancel its current execution
through the ordinary session cancellation path. Do not merely erase its entry
while letting the provider continue. Multiple viewers call existing authorized
session recovery/cancel operations; they never own scheduling.

## Restore and dispatch

Factor the existing `retryTransientPrompt` restore/cleanup sequence so mode
chooses either the original cached prompt or a new internal continuation prompt.
Use advertised `session/resume` through the current runtime/executor path,
falling back to advertised `session/load` with existing replay suppression. Both
restore the existing native session ID. Add a new internal
`RequiredNativeConversationID` option. When set, session-load failure, token loss,
identity mismatch, history uncertainty, or fallback-to-new-session must return
a typed refusal before prompt dispatch. Do not change ordinary manual resume's
fallback semantics. Use existing strict model/mode reconciliation; no silent
settings restoration or profile switching.

Native-only resume waits for ACP initialization at the shared executor startup
seam. The continuation owner receives the actual typed failure and exact failed
execution; generic startup failure projection does not run for that owned path.
Confirm exact-execution teardown and park the session in `WAITING_FOR_INPUT`
before scheduling another preparation attempt; `STARTING` would retain the
executor's boot grace. A removed
runtime row does not remove the required saved conversation identity. Ordinary
resume keeps its asynchronous startup and existing failure handling.

The continuation prompt is bounded static English engineering text:

```text
Your previous turn was interrupted by a temporary connection failure.
Continue the unfinished request using this conversation's existing history
and available completed tool results. Re-read files when needed to inspect
current state or recover missing read results. Do not repeat completed actions
or restart the original request. If the next action or an earlier
outcome is uncertain, stop and ask the user.
```

Send with dispatch-only semantics through the ordinary prompt acceptance boundary with automatic launch
origin and an internal recovery origin marker. Do not append original text,
attachments, plans, diagnostics, or replayed chat. Keep accepted-turn identity
and normal auditability, but never present this as a user-authored message or
overwrite the original cached user prompt. Continuation guidance is not an
idempotency guarantee; safety depends on conservative admission. Acceptance
finishes the startup resume attempt; normal runtime lifetime owns the turn.

## Settlement

Use the existing error-termination path to settle the interrupted turn without
successful completion signals. Do not call the normal success path merely to
free admission. Park recovery through existing `WAITING_FOR_INPUT` plus the
owned notice; queue admission must still know that recovery owns the slot.
Ensure CI auto-fix reconciliation records interruption, not success. The
new continuation is a separately admitted turn in the same conversation;
successful completion enters the existing workflow path once. A subsequent
failure rechecks the episode's combined effect evidence before rescheduling.

## Feedback contract

Introduce typed recovery disposition metadata on the existing status/error
records, not a second frontend state owner. Proposed fields:

| Field                                   | Values or meaning                                                                                                            |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `recovery_mode`                         | `replay`, `continue`                                                                                                         |
| `recovery_phase`                        | `waiting`, `reconnecting`, `continuing`, `manual`                                                                            |
| `recovery_disposition`                  | `manual`, `cancelled`, `exhausted`, `restart_interrupted`; omitted while automatic recovery is active                        |
| `recovery_reason`                       | Manual refusal: `disabled`, `unsafe_work`, `unsupported_restore`, `missing_evidence`; omitted for cancellation or exhaustion |
| `attempt` / `max_attempts` / `retry_at` | Existing scheduled ordinal, bound, absolute UTC deadline                                                                     |
| `attempts_started`                      | Actual launches started, kept distinct from scheduled ordinal                                                                |

Carry disposition from the actual policy result into
`createRecoveryStatusMessage`. Do not choose exhaustion from `routingerr.Decide`.
Capture the exhaustion outcome before `resetTransientRetry` destroys counters.
No active episode and absent legacy metadata renders neutral connection-loss
copy, never guesses exhaustion. Existing status records remain readable.
Manual refusal derives from the backend admission evidence, including saved
identity and invocation freshness. Localized feedback distinguishes its closed
reason codes; unknown or absent codes retain the neutral legacy fallback.
Retry notice persistence remains through `TransientRetryMessageService` and
task-service update/delete with existing event publishing and duplicate repair.

`TransientRetryNotice` in `messages/action-message.tsx`, `ActionMeta`,
`action-message-details.tsx`, and `session-recovery-model.ts` consume the same
metadata. Count down only in waiting phase; render reconnecting or continuing
when the deadline has passed and the backend advances phase. While continuation
is RUNNING, render its single owned notice with Cancel; audit the current
RUNNING warning suppression rather than mounting a second banner. Retire it on
successful completion and every terminal/supersession path. Do not announce
every timer tick. Add keys in English, pt-pt, zh-cn, ja, generated zh-tw/zh-hk,
and pseudo; engineering diagnostics remain sanitized English.

## Desktop and phone composition

Reuse inline task-chat recovery and the shipped `TransientRetryNotice` plus
`mobile-transient-retry.spec.ts`. The curated mobile language's dense-chat
surface remains the full task Chat route; this shallow status requires neither
a modal nor a new navigation destination. Keep transcript as the only scroll
owner, existing composer/safe-area handling, and the existing manual-recovery
card's stacked phone actions. Desktop displays text and Cancel in one row;
phone puts the full-width action below status. Desktop ordinary controls remain
28px; phone/coarse-pointer targets are at least 44px. Technical details wrap.
Shared metadata/view-model and handlers remain viewport independent. ASCII
structure and state previews are in the plan and Task 03.

## Rollout and restart

Runtime identity: `features.providerInterruptionContinuation`,
`KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION`,
`FeaturesConfig.ProviderInterruptionContinuation`, JSON
`providerInterruptionContinuation`. Register once through `runtimeflags` with
restart-required metadata and high-risk experimental copy. Keep all-off frontend
default and all-off `prod`, `dev`, and `e2e` profile values. Use the existing
environment > SQLite override > profile precedence.

Gate evidence collection at adapter construction/configuration and automatic
admission at the backend composition boundary. Carry effective state through
the existing typed managed-agentctl startup contract and instance/adapter
configuration; absent state is false on old remote
helpers. No new manual continuation endpoint exists, and HTTP/WS/MCP callers
cannot bypass the automatic-admission gate. Explicit manual resume remains
available. Truthful failure copy is an unconditional correction.

There is no new persistence table, migration, or durable dispatcher. Persisted
notices remain projections, not jobs. Backend stop disarms unaccepted timers and
drops local ownership, retaining notices for startup. Accepted turn contexts
remain under runtime shutdown policy. Startup retires stale notices for sessions
with no current retry owner and exposes `restart_interrupted` manual recovery;
do not invent retained safety evidence or re-arm from a message. If an agent
survived backend restart, existing adoption/reconciliation must prove its live
ownership before settlement; never stop a live adopted turn merely because an
old notice exists. Flag promotion and retirement are later work.

## Security and observability

Keep current task/session binding and authorization before recovery state reads
or cancellation. No trusted support flag comes from browser request metadata.
Do not log the continuation prompt, raw diagnostic, tool arguments, or provider
resume token. Structured decision logs record mode, disposition, evidence
completeness, bounded tool-category counts, attempts, and correlated task,
session, execution, generation, and episode identifiers. IDs are log fields,
never metric labels. Use the existing safe classifier detail for UI disclosures.

## Validation strategy

Adapter fixtures and native compatibility evidence prove restore support and
positive read identity separately. Race-enabled lifecycle/orchestrator tests
exercise immutable snapshot propagation, remote omission, write poisoning
across attempts, single-owner scheduling, cancellation at each boundary,
transient restore failures, acceptance ambiguity, shutdown/adoption, and
workflow/queue settlement. Controlled mock fixtures prove output/read recovery
and manual unsafe recovery in desktop and phone Chat, including reload and
persisted state. Preserve existing data-only `/transport-lost` and dynamic/Office
manual-recovery expectations. Exact tests and commands are assigned by the
[implementation plan](../../../plans/provider-interruption-continuation/plan.md).
