---
status: current
system: platform
created: 2026-10-03
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-TURN-CONTINUITY-003
owners:
  - Kandev
---

# Transient turn runtime continuity system design

## Purpose and boundaries

Separate a provider turn error from terminal execution failure.
The shared runtime owns process and connection lifetime.
Orchestration owns failed-turn settlement and existing retry admission.
Provider dialects translate tested error shapes into bounded evidence.
Task Chat consumes the resulting failure record without owning retry or teardown policy.

The [decision](../../../decisions/2026-10-03-transient-turn-runtime-lifetime.md) records the lifetime boundary.
This design extends [provider error recovery](provider-error-recovery.md) and [interruption continuation](provider-interruption-continuation.md).
The capacity amendment preserves the replay fence and transport-loss rollout defaults.
Its [decision](../../../decisions/2026-10-05-capacity-continuation-after-completed-tools.md) permits a separate live-runtime continuation contract.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-TURN-CONTINUITY-001 | Evidence contract; Runtime settlement; Recovery admission; Ownership and failure |
| REQ-PLATFORM-TURN-CONTINUITY-002 | Durable projection; Desktop and phone; Verification |
| REQ-PLATFORM-TURN-CONTINUITY-003 | Capacity continuation evidence; Capacity recovery admission; Capacity episode verification |

## Evidence and current implementation

The reported execution received Codex capacity notifications at 2026-10-03T14:44:57Z.
The adapter suppressed the explanatory chunk and emitted `EventTypeError` after the prompt RPC returned successfully.
Lifecycle marked the execution failed with synthetic exit code 1.
Orchestration explicitly stopped it for `recoverable agent failure`.
The process then exited during intentional stop.

After rebasing to `a81c68fe838`, these teardown paths remain:

- ACP `adapter_prompt.go` emits capacity and Cursor terminal errors.
- `Manager.finishPromptCompletion` sends every current-generation error through `preparePromptErrorCompletion`.
- `Service.handleAgentFailedLocked` marks the execution failed before recovery admission.
- `handleRecoverableFailureLockedState` schedules runtime cleanup.
- `retryTransientPrompt` tears down the predecessor before replay.
- `retryInterruptedContinuation` stops the predecessor before native restoration.

The newly landed Cursor continuation work supplies native identity, effect evidence, cancellation, and truthful retry dispositions.
Reuse those owners instead of introducing another recovery loop.

## Evidence contract

Add a typed `PromptFailureDisposition` to `streams.AgentEvent` and retained turn outcomes.
The proposed value `retain_runtime` means the adapter observed a current, settled provider failure while ACP remained open.
Omitted or unrecognized values retain terminal failure behavior.
This field is host-generated evidence, not a value copied from provider `_meta` or browser input.

Emit the field only after the prompt RPC settles and the ordered notification queue drains.
Require the same initialized native session, nonzero prompt generation, ownership of completion, and an open adapter lifetime.
Use the existing SDK `ClientSideConnection.Done()` to reject closed transport.
Do not send a probe prompt or create another provider session to establish liveness.

The common extractor requires a high-confidence transient classification from sanitized provider evidence.
It accepts tested application-level `session/prompt` errors and dialect-specific terminal provider-stream diagnostics.
Generic internal errors, Data-only signatures, auth, hard quota, invalid model, and uncertain transport errors receive no retention attestation.
Provider classification alone never establishes ACP liveness.

Initial fixtures cover Codex's ordered systemError/capacity notifications, classifying ACP overload and rate-limit replies, and Cursor's tested terminal stream diagnostic.
Cursor's native restore error contract remains distinct: SDK peer-disconnect errors do not become retention evidence.
Unsupported providers retain existing behavior until their shape has positive and negative fixtures.

Propagate the disposition through process forwarding, `TurnOutcomeRecorder`, agentctl JSON, retained-outcome retrieval, lifecycle snapshots, and watcher conversion.
Remote omission and invalid values fail closed.
The field carries no prompts, tool payloads, paths, or credentials.

## Runtime settlement

At the current completion claim, lifecycle checks the attestation against execution identity, startup attempt, generation, initialization, and live process/stream state.
Apply retention only to concrete-profile interactive tasks.
Office, dynamic attempts, utility operations, passthrough, and automation-owned executions keep their current policy.

For a retained failure, keep the execution tracked and ready for another foreground prompt.
Keep its process owner, agentctl client, native session, workspace, settings, and runtime resources.
Do not set an execution exit code, end its session span, release its execution slot, or publish `AgentFailed` or success `AgentReady`.
Release foreground prompt activity only after the failure owner settles the turn.
Existing background-work authority remains independent of foreground readiness.

Introduce internal `events.AgentTurnFailed` with the current identity, sanitized diagnostic, failure disposition, and immutable prompt-attempt evidence.
Register it in the watcher and route it to a dedicated turn-failure handler.
Keep `AgentFailed` authoritative for terminal runtime failure and existing unsupported errors.
Carry the retained disposition on the complete stream projection so orchestration defers session settlement and prompt-evidence cleanup to the synchronous `AgentTurnFailed` owner.
This avoids making runtime-ready status look like successful task completion.

The prompt waiter must receive an error outcome, not a normal end-turn result.
Carry the retained disposition in the completion signal and a typed caller error.
The blocking `handlePromptError` path recognizes that settlement already has an owner.
It must not close a successor turn, re-run settlement, or reset session state independently.
Accepted queued dispatches retain acceptance ownership even when their turn later fails.

## Recovery admission

The new turn-failure handler uses existing session cancellation and prompt ownership guards.
It validates the captured execution and generation before any durable side effect.
Reuse the existing replay evidence and classifier policy.
Unsafe or uncertain work can prevent automatic action without preventing runtime preservation.

For no automatic operation, mark the observed turn error-terminated and settle its managed input with existing failed/uncertain distinctions.
Persist its sanitized error once, complete that turn as failed, and expose usable `WAITING_FOR_INPUT` with no blocking session error.
Keep the task's existing review reconciliation and explicitly configured error actions.
Do not enter ordinary successful-turn workflow processing or consume queued input as successful completion.
Human queue admission retains Auto-run and exact queue ownership semantics.

For eligible replay, reserve the existing retry owner and budget.
Store the retained execution and disposition in that owner.
At timer fire, check the exact execution, native session, configuration, cancellation, foreground admission, and retry owner again.
Use the ordinary prompt seam on the live execution, without teardown or resume.
An ambiguous accepted retry is never replayed again.

For enabled, eligible Cursor continuation, reuse the existing safety snapshot and instruction.
Add a live-runtime branch before predecessor teardown in `retryInterruptedContinuation`.
It submits the existing internal continuation instruction through ordinary prompt admission on the same native identity.
It retains the current episode budget, permission settings, queue priority, and generation checks.
When the runtime is actually unusable, the existing teardown/restore branch remains authoritative.
The transport-loss branch retains its provider and read-only restrictions.
The capacity branch below permits completed effects only on the same proven usable runtime.

Cancellation or exhaustion of an idle retained retry retires the notice and returns to the usable composer.
Cancellation after dispatch uses ordinary turn cancellation.
It does not imply whole-runtime stop unless existing bounded cancellation escalation requires that stop.

## Capacity continuation evidence

Add an optional typed `CapacityContinuationSnapshot` in `internal/agentctl/types/streams`.
It carries a versioned live-conversation support value, nonzero prompt generation, evidence completeness, and unresolved-work indicators.
It carries no tool payloads, paths, prompts, results, or credentials.
Omission and unknown versions never authorize this branch.
Keep `ContinuationSafetySnapshot.SafeFor` unchanged for the existing read-only restore contract.

The Codex dialect owns support for its tested initialized live-conversation shape.
Orchestration consumes the typed support value without a provider-name branch.
The isolated mock dialect supplies test-only support through construction provenance.
Other dialects cannot inherit support from a generic ACP adapter.

The ordered ACP worker tracks all tool IDs for the active prompt in a bounded ledger.
Every observed tool must have a provider-confirmed completed status before the terminal snapshot permits recovery.
Successful shell, write, and MCP calls can qualify. Their semantic kinds do not establish completion.
Failed, cancelled, pending, unknown, malformed, conflicting, unmatched, or overflowing tool evidence blocks admission.
Background or subagent activity without authoritative completion blocks admission, including Codex start-only collaboration evidence.
Unresolved permission activity also blocks admission.
One completed tool plus one pending or failed tool must block the entire attempt.

Capture the snapshot after ordered notifications drain and before synthetic prompt-end cancellation sweeps.
Synthetic terminal statuses cannot establish completed work.
Response-attempt resets cannot erase unresolved tool evidence.
Native load history cannot populate this active-prompt ledger.
Publish the snapshot only with the same settled prompt and usable-runtime attestation.

Carry it unchanged through stream events, process outcomes, remote JSON, lifecycle prompt evidence, and watcher failure payloads.
Lifecycle copies immutable evidence at the terminal boundary and verifies the same execution and generation.
Never reconstruct eligibility from persisted transcript titles or provider prose.
Older agentctl components keep the current conservative behavior.

## Capacity recovery admission

Keep `handleTransientFailure` as the single scheduler owner.
Original-prompt replay retains `promptAttemptPreResultSafe`.
When replay is unavailable, capacity continuation requires `CodeModelCapacity`, the existing high-confidence short-retry decision, and the new snapshot.
Require retained task ownership, current execution and generation, initialized native identity, and the same configuration fingerprint.
Reject Office, dynamic, automation, passthrough, archived tasks, cancellation, and queued user work.
No requirement to enable `ProviderInterruptionContinuation` applies to this narrower capacity path.

Retain an explicit internal continuation policy in the existing binding and retry entry.
Distinguish live capacity continuation from transport-loss restoration without changing UI recovery mode `continue`.
Shared validation checks policy-specific support and uses the same owner, identity, queue, and cancellation guards.
Do not weaken the transport-loss policy when extracting shared validation.

Settle the failed turn as interrupted before arming the timer.
Reuse the existing 5/10/20/40/60-second ladder and validated short reset hints.
At timer fire, revalidate native identity, execution, generation, runtime readiness, configuration, retry ownership, and queue priority.
Submit one internal instruction through the ordinary prompt seam on the retained runtime.
Use capacity-specific wording: continue unfinished work from existing history, preserve completed actions, and ask the user about uncertain outcomes.
Do not attach the original request or resend attachments.

Reuse `retryRetainedRuntimeContinuation` dispatch and acceptance accounting.
Adapt its policy-specific instruction and validation instead of introducing another timer or prompt loop.
For this policy, runtime loss or an inconclusive probe stops recovery without predecessor teardown or native restoration.
Do not enter the transport-loss restore branch after writes.
Ambiguous dispatch stops without another automatic send.

Every additional capacity failure belongs to the same episode until successful completion or supersession.
Preserve the initial interruption and all completed tool rows.
Do not reset the episode on assistant output, tool completion, or prompt acceptance.
Increment started attempts only at actual dispatch, with at most five additional attempts.
Preserve the existing success and cancellation cleanup owners.
Refusal and exhaustion on a usable runtime retain historical errors and the normal composer.

## Capacity episode verification

Protocol tests prove current-generation snapshots, mixed completed/pending tools, permissions, unknown outcomes, and remote omission.
Service tests prove same-process continuation after completed effects and preservation of the original-prompt replay fence.
Barrier tests cover cancellation, successor generations, queue priority, configuration changes, and runtime loss.
Exhaustion tests prove the five delays, five dispatches, and no sixth attempt after progress.
Desktop and phone E2E tests use the real ACP mock process and assert unchanged native identities and initialization count.
The implementation package must also capture an isolated Codex compatibility trace before enabling its dialect support.
Mock evidence alone does not establish the production Codex capability.
Record the installed CLI version and sanitized frame evidence in the work order.
Use a disposable conversation, never the reported user's task.

The [capacity plan](../../../plans/model-capacity-continuation/plan.md) owns exact tests and commands.
Public guidance changes with implementation in `docs/public/tasks-and-workflows.md`.
The design turn changes internal artifacts only.

## Ownership and failure

Use the existing startup lease, `promptLifecycleMu`, completion claim, and session cancellation guard.
Never perform synchronous lifecycle callbacks while holding a conflicting session guard.
Keep failure identity immutable through asynchronous publication.
Deduplicate by session, execution, and nonzero generation.
One failure cannot mark the entire surviving execution terminal in `markExecutionFailed` or stream activity fences.
Late events from the failed generation cannot revive activity or complete a newer turn.

A concurrent real process exit or ACP disconnect wins over retention.
Only the current live execution can become ready.
A removed, reset, archived, stopped, or replaced execution cannot become ready through a late failure callback.
If durable failure settlement fails, keep the runtime but withhold new foreground admission until settlement succeeds or explicit recovery owns it.
Do not claim input readiness while the turn boundary remains uncertain.
No automatic probe or repair loop is added for that storage failure.

Backend restart does not reconstruct liveness from historical error metadata.
Existing adoption and recovery must establish current runtime ownership.
Stale retry notices retain the existing restart-interrupted retirement policy.

## Durable projection

Use existing task-session messages and turn metadata. No schema migration is needed.
Persist a status/error entry with `failure_scope=turn`, semantic `failure_code`, execution, generation, and turn identity.
Set `runtime_retained=true` only after runtime validation.
These proposed metadata fields are presentation facts and cannot authorize retry or reuse.
Do not write `recovery_actions=true` or an unresolved blocking `last_agent_error` for this path.
Retain historical failed-turn entries after a later successful turn.

During automatic recovery, reuse the existing transient notice and typed recovery disposition.
Count actual started attempts, not classifications or scheduled ordinals.
On exhaustion or cancellation with a usable runtime, replace actionable retry state with a historical turn explanation.
Do not create generic Resume/Start fresh controls.

Correct `action-message-recovery-history.tsx` for existing persisted records too.
An execution ID alone cannot choose the startup view model.
Use explicit bootstrap phase or typed startup causes for startup presentation.
For the reported legacy capacity record, retain its provider explanation instead of the startup fallback.
Existing true bootstrap, quota, runtime failure, and workspace recovery controls remain intact.

## Desktop and phone

Use the existing inline task-chat status row and normal composer.
The nearest phone exemplar is `mobile/session-mobile-layout.tsx` and the existing transient notice in `messages/action-message.tsx`.
The transcript retains its scroll owner. The composer retains existing safe-area and draft behavior.
No new overlay, sheet, model picker, or recovery button is introduced.
The existing model selector remains available when its adapter supports in-session changes.
Use shared failure metadata and view-model rules across task Chat, preview, Quick Chat, and historical rendering.
Phone text wraps. Existing selector and send controls remain touch-accessible.
Expanded details use the existing sanitized disclosure and copy behavior.

Localize new explanatory and retry-result copy in English and all six real locale catalogs.
Generate Traditional Chinese and pseudo catalogs with repository tooling.
Provider diagnostics remain sanitized English evidence.
Desktop and phone previews are in the [plan](../../../plans/transient-turn-runtime-continuity/plan.md).

## Observability

Record failed-turn settlement separately from process exit.
Use bounded fields for disposition, semantic code, invocation phase, and validation result.
Task, session, execution, and generation remain log fields, never metric labels.
Do not log raw provider data or synthetic process exit codes for retained failures.
Runtime resources and execution slots remain visible while the process remains alive.

## Verification

Use barrier-controlled protocol and service tests for error/exit races and duplicate terminal delivery.
Prove subsequent prompt and model selection on the exact same process and connection.
Native process identity and initialization counts are evidence. A deduplicated boot row alone is insufficient.
Mock fixtures exercise the full backend and desktop/phone Chat without touching the reported live task.
Keep actual ACP disconnect, process crash, unknown evidence, dynamic, Office, and utility negative coverage.
Exact commands and test names are assigned by the work orders.
