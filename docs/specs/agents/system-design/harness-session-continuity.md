---
status: draft
system: agents
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-002
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-003
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-004
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-005
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-006
---

# Harness session continuity system design

## Purpose and boundaries

Kandev owns the task session and saved conversation. Each harness owns its native conversation, private compaction state, and native-state format.

The execution chain remains Kandev, agentctl, then the harness. ACP is a transport contract, not a portable checkpoint format.

This draft extends [agent recovery](agent-resume-runtime-recovery.md).
It preserves the existing token, archive, and explicit branch-replacement contracts.
It uses [durable delivery](../../platform/system-design/durable-agent-delivery.md) for prompt checkpoints.
It extends existing [queue authority](../../tasks/system-design/resume-prompt-queue.md) with a recovery admission condition.
It preserves [queue edit fencing](../../ui/system-design/message-queue-edit.md) and [completed-task conversations](../../tasks/system-design/task-completion.md).
The continuity behavior ships without a feature flag after the complete continuity milestone passes.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001 | Restore policy |
| REQ-AGENTS-HARNESS-SESSION-CONTINUITY-002 | Continuation snapshot |
| REQ-AGENTS-HARNESS-SESSION-CONTINUITY-003 | Restore coordinator |
| REQ-AGENTS-HARNESS-SESSION-CONTINUITY-004 | Workspace identity |
| REQ-AGENTS-HARNESS-SESSION-CONTINUITY-005 | Recovery presentation |
| REQ-AGENTS-HARNESS-SESSION-CONTINUITY-006 | Autonomous consumers |

## Components and responsibilities

- Agent definitions declare tested restore capabilities and native-state requirements.
- ACP dialects translate protocol methods and classify evidence. They do not decide recovery policy.
- Lifecycle owns one restore coordinator for startup, manual recovery, and workspace rebind.
- The task repository owns continuation records and authoritative conversation reads.
- Agentctl owns native calls and durable delivery. It does not choose a replacement conversation.
- The existing recovery service and chat status components expose outcomes.

## Data and contracts

A session has a stable Kandev session ID and an incarnation ID.
A harness generation identifies one native conversation within that incarnation.
A process restart does not create a harness generation.
An explicit context continuation does.

A new additive SQL record stores:

| Record | Fields |
| --- | --- |
| Harness generation | Session/incarnation IDs, generation, predecessor, native ID, agent type, adapter/runtime version, creation reason |
| Workspace identity | Original agent-visible CWD, current target CWD, executor environment reference, native-state reference |
| Restore attempt | Attempt ID, expected generation, action, outcome, classified reason, target CWD, timestamps |
| Continuation snapshot | Attempt ID, source message cutoff, bounded content, byte count, truncation details, content hash |
| Delivery checkpoint | Snapshot ID, target generation, submission ID, prepared/accepted/consumed/uncertain state |
| Recovery block | Block ID, session/incarnation, expected generation, reason, state, consumer/work reference, authorized resolution |

The native-state reference is an opaque executor-owned identity, not a credential or a serialized home directory.
Existing token fields remain compatible projections of the current committed generation.
Historical native IDs remain available for diagnostics and recovery.
SQL writes compare the expected incarnation and generation before replacement.

Outcomes are `native_resumed`, `reattached`, `context_continued`, and `blocked`.
Context delivery status remains separate from the restore outcome.
A created conversation is not proof that its first prompt ran.

Capabilities distinguish native load, native resume, replay behavior, directory binding, and required native state.
Unknown capability values are conservative.
The registry combines negotiated protocol support with tests for the installed adapter version.
An agent name alone does not prove directory portability.

## Restore policy

| Condition | Result |
| --- | --- |
| Live owned process and matching generation | Reattach and reconcile delivery |
| Native ID, compatible workspace, required native state | Prefer native resume without history replay, otherwise native load |
| Transport, authentication, configuration, permission, or unknown error | Block and retain native identity |
| Typed missing native state or unsupported native restore | Offer explicit context continuation |
| Known incompatible directory change | Offer explicit context continuation in the target directory |
| Missing branch | Preserve the existing branch-recovery action and warning |

Opening a session, automatic reconnect, and queue dispatch never authorize context continuation.
The user selects `continue_from_history` after Kandev explains that a new native conversation will start.
The action records the expected generation and target workspace.
A repeated action ID returns the existing attempt.
Concurrent or stale actions cannot replace the current generation.

The existing `resume_new_branch` action remains native-only.
It cannot silently become context continuation.
If native restore then fails, Kandev presents the separate continuation action.
This rule preserves the promise that branch replacement alone retains the original conversation.

A generic ACP error code is not sufficient evidence of missing native state.
Adapter evidence must identify the failure class.
Unknown errors remain blocked.

## Restore coordinator

The coordinator replaces divergent decisions in `createOrLoadSession` and `createReboundACPSession`.
It uses the following ordered steps:

1. Acquire the existing session/environment ownership guard.
2. Capture immutable effective runtime configuration before native lifecycle events can replace cached values.
3. Resolve the original workspace identity and native-state availability.
4. Persist the restore attempt and its authorization.
5. For context continuation, persist the bounded snapshot before native creation.
6. Create or restore the native conversation.
7. Restore model, permission mode, and sorted configuration options.
8. Commit the new current generation with a compare-and-swap update.
9. Publish the persistent recovery outcome.
10. Admit the authorized prompt through existing queue and turn ownership.

A partial configuration error blocks prompt admission.
The coordinator reuses the runtime-configuration contract from the context-reset ADR.
Fresh MCP credentials and trusted Kandev instructions come from the current environment.

A candidate native ID remains attached to the pending attempt until configuration succeeds.
After a crash, recovery can inspect that candidate without losing the previous committed native ID.
It does not blindly create another candidate or send its first prompt.

Before durable transport activation, an interrupted first submission remains uncertain and requires user recovery.
No in-memory boolean can prove that context reached the harness.
Durable transport later supplies the accepted and terminal checkpoints.

## Continuation snapshot

The backend SQL conversation is the source of truth.
The optional lifecycle JSONL history is not a prerequisite.
Codex, Claude Code, OpenCode, and other adapters use the same snapshot builder.

The snapshot contains the current task objective, saved plan reference, workspace identity, and selected conversation history.
It includes recent user messages, assistant messages, and concise tool outcomes.
It excludes debug protocol frames, credential material, sibling conversations, and private child-session history.
It does not read arbitrary workspace files or private harness databases.

The serialized snapshot has a 64 KiB UTF-8 limit.
The builder reserves 16 KiB for task, workspace, and reduced plan context.
Included plan text uses `planinjection.Reduce` with `planinjection.HandoverBudget` (12,000 bytes).
That plan text counts within the 16 KiB reservation and the 64 KiB total, not in addition to either.
The remaining snapshot budget holds recent conversation.
Unused reserved space becomes available to conversation history.

The composer includes the plan once.
It preserves `planinjection.ContainTags` and the trusted system-prompt boundary.
A dedicated composed-context test counts UTF-8 bytes, separators, and truncation markers.
The existing `DynamicBudget` (4,000 bytes) does not change.
The current user prompt and current trusted system instructions remain outside the snapshot cap.
Their existing request-size limits still apply.
Newest complete messages have priority, but their final order remains chronological.
Individual oversized text blocks receive a UTF-8-safe excerpt and an explicit truncation marker.
Binary attachments become metadata references that still require current authorization.

The snapshot records its source cutoff and omitted message count.
The current submitted prompt stays outside this history budget and does not appear twice.
A pending continuation without a prompt remains prepared until the next authorized submission.

History is untrusted context, never a system instruction.
The prompt composer applies the current trusted Kandev instructions separately.
It preserves the saved-prompt trust boundary and the existing plan-injection budget.
The snapshot does not expand old saved-prompt tokens or grant historical permission decisions.

The backend persists the snapshot hash and submission association before delivery.
Retries reuse the same snapshot and final prompt hash.
An uncertain delivery cannot clear the checkpoint or cause another automatic injection.

## Workspace identity

The agent-visible CWD differs from the host source path and worktree display name.
Kandev records it at first native creation rather than reconstructing it from a mutable task title.

Executors retain a stable agent-visible path when their existing isolation or mount contract supports it.
Native host execution uses the real retained directory.
This package does not introduce global symlinks or overwrite shared harness homes.

The coordinator compares both directory identity and native-state availability.
The same directory string on a new empty executor does not prove that native resume can succeed.
A different host path with the same stable container mount can remain compatible.

Harness contract tests cover same CWD, changed CWD, missing state, and missing load/resume capability.
ACP load/resume requests receive the explicit target CWD.
Additional workspace roots do not imply that old tool paths can be rewritten.

A supported directory override retains native resume.
An unsupported override produces a typed blocked result and the explicit continuation action.
Context continuation warns that historical paths refer to the original workspace.

## Recovery presentation

The existing `SessionRecoveryFeedback` and chat status-message surface own the notice.
No new recovery page or competing overlay is necessary.

A completed notice identifies native resume or context continuation.
A context notice states that private harness state did not transfer.
It shows the target CWD and whether history was truncated.
Details show the source generation and classified reason, but not raw prompts, credentials, or raw native-state paths.

A blocked notice exposes the appropriate existing retry action.
For eligible native-state loss, it also exposes `continue_from_history`.
The first submission cannot start until the user selects that action.
Persistent notices survive reload and remain in the canonical conversation.

Desktop retains the current dense inline layout.
Mobile uses the same inline status within the existing chat scroll owner.
Actions wrap without horizontal overflow and use at least 44px touch targets for coarse pointers.
Status uses an accessible live region without repeatedly announcing replayed events.
Keyboard focus remains on the completed action or moves to the resulting notice.

All copy uses locale keys in English, Portuguese, Simplified Chinese, and both Traditional Chinese locales.
The existing mobile session-recovery tests provide the interaction pattern.
Desktop and mobile tests cover native resume, context continuation, blocked restore, reload, and stale double submission.

## Autonomous consumers

The default is park-and-surface, not automatic context continuation.
This rule covers session-backed Office routines, Office configuration-triggered runs, CI automation, and PR/MR automation.
Sessionless utility calls do not acquire artificial conversation recovery state.

The task repository owns a durable `session_recovery_blocks` record.
Its key includes session, incarnation, and expected harness generation.
A block distinguishes missing native state from uncertain prompt delivery.
The first blocked restore commits the record before returning `session_recovery_required`.
Subsequent attempts reuse the same open record.

Every final prompt admission reads this block under the existing ownership boundary.
An unreadable block state fails closed.
Startup, Send Now, queue editing, and automatic queue drain cannot bypass the check.
Queue editing can preserve pending content but cannot release blocked dispatch.

Office owns its run projection, not the session recovery authority.
An additive indexed `session_recovery_id` reference marks the existing run as recovery-blocked.
Before dispatch, the parked run retains queued state.
It remains distinct from `routing_blocked_status`, budget dispositions, and the agent's global paused status.
If no dispatch occurred, the transition releases only the matching run claim and working-owner marker.
If execution can still be live, the run retains its claimed state and working owner until reconciliation or confirmed Stop.
It does not pause unrelated work for the same agent.

Claim selection excludes parked runs.
New wakes for the same blocked session retain their ordinary coalescing identity and inherit the recovery reference.
The final session admission check remains authoritative if the Office projection lags.
Scheduler restart repairs that projection from the canonical block.
Timed provider unpark, stale-claim cleanup, and configuration sync cannot clear it.

CI and PR/MR automation preserve their existing queue row, source event, coalescing key, and dispatch-attempt identity.
The dispatcher records the recovery reference without marking the work delivered or generating another attempt on each poll.
A later source event follows normal coalescing rules but cannot bypass the session block.

Office run detail and the Office inbox show one recovery item per open block.
Automation run detail or the owning task status shows the same actionable condition.
The primary action opens the existing session recovery surface.
Phone notices use direct navigation from the existing inbox row or run header, not another stacked panel.
The destination uses the task layout's current mobile composition and single chat scroll owner.
The notice shows the cause before its recovery action and retains accessible status and touch targets.
Dismissal changes notification visibility only, never dispatch eligibility.
A status-only Office agent recovery and the existing Mark fixed flow do not authorize a new native conversation.

For confirmed native-state loss before dispatch, the operator can select context continuation and authorize the pending work.
The action records the exact pending work reference and current block generation.
After restore succeeds, the owning dispatcher resumes that work through its normal claim path.
Office runs repeat `admitRun` and preserve their original attended/unattended provenance.
No direct `StartTask` shortcut, budget bypass, synthetic approval, or workflow advancement is permitted.

For uncertain delivery, context creation alone does not release the old submission.
The operator must inspect the outcome and explicitly settle or supersede that submission before any new instruction can run.
The settlement is durable, records the actor, and cannot make the old submission dispatchable again.

The [Office status-only recovery](../../office/system-design/agent-recovery.md) contract remains unchanged.
The [stall detector](../../office/requirements/stall-visibility.md) remains detection-only.
It does not create, clear, or act on recovery blocks.
The [budget admission contract](../../office/requirements/budget-admission-integrity.md) remains authoritative for every resumed unattended run.

## Failure and recovery

Native failure never clears the prior token.
Snapshot absence or SQL failure blocks context continuation with a visible reason.
An empty conversation can continue from the task objective, with an explicit empty-history notice.
A configuration failure never releases queued work.

Concurrent rebind, archive, Stop, and continuation retain existing ownership checks.
An archived session cannot restart through the new action.
Read-only session opening still honors prevent-auto-start.

## Persistence

The task repository owns additive continuation tables in SQLite and PostgreSQL.
Its existing schema owner includes these tables in fresh, replay, and previous-stable upgrade tests.
A new schema owner requires registration in both requiredstores and storeconformance.
The Office repository owns its run recovery reference and extends its own schema conformance.
Task, Office, and queue projection writes retain their existing database boundaries.
The canonical block prevents dispatch while a cross-owner projection repair remains incomplete.

There is no runtime feature flag for continuity.
Required schema and historical reads remain available across supported upgrades.
Session deletion removes its continuation records under the existing deletion authority.
A successful delivery can remove snapshot content after seven days.
Generation metadata and hashes remain until session deletion.
Prepared or uncertain snapshots remain until resolution or explicit session deletion.

## Security

Current task access controls apply to restore actions, snapshot reads, and recovery details.
The backend validates the requested workspace against the session's authorized environment.
Logs contain identifiers, counts, and reason codes, not snapshot text.
Context continuation never copies authentication files or rewrites provider-owned history.

## Observability

Lifecycle owns the `agent_restore_*` metric family through one shared metrics package.
Consumers report through that package rather than declaring their own metric names.

| Metric | Unit and labels | Increment owner |
| --- | --- | --- |
| agent_restore_attempts_total | Attempts. outcome, reason, agent_type | Restore coordinator, once per finished attempt |
| agent_restore_context_truncated_total | Snapshots. agent_type | Snapshot builder, once per persisted truncated snapshot |
| agent_restore_recovery_required_total | Blocks. consumer, reason | Recovery service, once per new open block |

Reason, outcome, consumer, and agent_type use bounded enums.
Metric labels exclude session IDs, paths, provider error text, and runtime versions.
Structured logs carry those diagnostic identities after sanitization.
Repeated scheduler polls and replayed outcomes do not count as new attempts or blocks.
Root `AGENTS.md` documents the family when implementation ships.
`CLAUDE.md` remains its symlink.

## Related decisions

- [Durable sessions across harness generations](../../../decisions/2026-09-10-durable-harness-session-boundaries.md)
- [Runtime configuration after context reset](../../../decisions/2026-08-18-context-reset-preserves-runtime-configuration.md)
- [Explicit branch recovery](../../../decisions/2026-08-31-explicit-new-branch-session-recovery.md)
