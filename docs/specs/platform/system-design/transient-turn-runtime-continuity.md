---
status: current
system: platform
created: 2026-10-03
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
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
It does not change their replay fence or rollout defaults.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-TURN-CONTINUITY-001 | Evidence contract; Runtime settlement; Recovery admission; Ownership and failure |
| REQ-PLATFORM-TURN-CONTINUITY-002 | Durable projection; Desktop and phone; Verification |

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
Do not broaden continuation to Codex or allow write recovery.

Cancellation or exhaustion of an idle retained retry retires the notice and returns to the usable composer.
Cancellation after dispatch uses ordinary turn cancellation.
It does not imply whole-runtime stop unless existing bounded cancellation escalation requires that stop.

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
