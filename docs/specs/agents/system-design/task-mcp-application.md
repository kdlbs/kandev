---
status: current
system: agents
created: 2026-11-14
requirements:
  - REQ-AGENTS-MCP-APPLY-001
  - REQ-AGENTS-MCP-APPLY-002
  - REQ-AGENTS-MCP-APPLY-003
owners:
  - kandev
---

# Task MCP application design

## Evidence and decision

The user reported revision 3 with Configuration enabled on 2026-11-13.
Session `668f7b96-5a3f-413a-9a81-0cb21d1fac0c` retained revision 0, accepted at 15:26:31 UTC.
Its packages were Core, Diagnostics, Rich output, and Walkthroughs. Configuration appeared later.
The user supplied this report. This investigation did not inspect that runtime or establish what caused the later availability.
The separate originating task is `3ca9fad4-bd5b-4eda-ba73-47c918cf1b55`.

Verified source behavior:

- `lifecycle.prepareBuiltinWorkspaceExecution` verifies an existing binding instead of replacing it.
- `orchestrator.ReadBuiltinSessionMCP` reports desired state separately from accepted execution evidence.
- `BuiltinPolicyService.UpdateTask` authorizes a human and saves preferences with task and policy revision checks.
- `BuiltinPendingPackages` renders `settings:builtinMcp.pending`. Current copy promises the next normal process start.
- `runtime.Runtime.SuspendIdle` retains conversation inventory and has activity and prompt generation checks.
  It can delete the standalone helper and stop its terminal or preview resources. It is not the replacement path for this feature.

The observed delay is consistent with the current contract. A message, reconnect, or backend adoption does not change that contract.
The proposed change creates a deliberate replacement boundary. It never edits a retained binding in place.
Immediate cancellation risks work loss. Hot mutation cannot invalidate every provider's catalog.
A next-start-only approach preserves safety but leaves idle users waiting indefinitely.
The revised design restarts only the provider process inside the retained workspace runtime.
The [provider lifetime decision](../../../decisions/2026-11-15-provider-refresh-workspace-lifetime.md) records this boundary and its alternatives.
This design replaces the prior proposal to wait for an empty helper and then delete it. Open workspace resources must not block application.

## Contract mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-MCP-APPLY-001 | Coordinator, Safe replacement, Provider admission |
| REQ-AGENTS-MCP-APPLY-002 | Saved intent, Input ownership, Acceptance |
| REQ-AGENTS-MCP-APPLY-003 | Recovery, UI, Verification |

The [existing package design](builtin-mcp-management.md), [profile continuity](mcp-profile-continuity.md),
and [resume identity](agent-resume-runtime-recovery.md) remain dependencies.
This draft changes automatic application timing only for human task preference saves.

## Saved intent and persistence

Add an additive task-owned application record for each affected session in the task SQLite repository.
Proposed table: `task_builtin_mcp_applications`, keyed by session, with task and workspace ownership.
Store target preference revision, policy revision, effective selection hash, actor ID, request ID, and attempt generation.
Also store source execution ID, stage, reason, timestamps, and optimistic row version.
The existing implementation has source/replacement execution fields. Provider refresh retains the execution ID.
Add source and target process generation, source and target binding ID, helper incarnation, and expected configuration revision.
Store the unchanged conversation delivery owner separately from each provider process generation.
Do not use a different execution ID to represent this provider refresh. The existing replacement-execution field stays empty for this operation.
Keep immutable accepted binding snapshots keyed by execution and binding ID, including process generation and the resolved manifest digest.
A current-binding pointer advances by compare-and-swap only after acceptance. Retired snapshots remain historical evidence.
The SQL application row is the desired target. Its attempt fields are the frozen target of an operation already in progress.
Keep provider tokens in their existing protected inventory. Never duplicate them into this record or logs.

Extend the preference-save transaction in `agent/settings/store/builtin_mcp_policy_write.go` to persist application intent for eligible nonterminal existing sessions.
The task repository owns the schema. The settings store owns the atomic preference write.
A commit succeeds only when preferences and intent persist together. Publish a wake event after commit.
A lost wake is recoverable from stored pending rows. Browser lifetime does not own the operation.
A session created later uses the latest selection through normal launch preparation.
No-op effective selection changes can advance observed preference metadata without replacing the execution.
Never rewrite the historical accepted manifest merely to match a preference revision.

Retain the current human-only authentication and revision checks in `BuiltinPolicyService.UpdateTask`.
The request cannot supply a manifest, execution identity, grant, or runtime command.
Resolve the effective manifest from stored choices. Revalidate workspace ownership, actor access, surface eligibility,
and policy at the replacement boundary. A policy change invalidates prepared work and requires fresh resolution.
It cannot grant Configuration without its saved explicit opt-in.
Workspace-only policy saves do not trigger this coordinator.

## Coordinator and state transitions

A new orchestrator coordinator owns pending rows and calls the optional `runtime.ProviderMCPReplacement` seam.
It serializes with existing session resume, cancellation, workflow launch, and prompt admission.
Use one lock order: session admission, then runtime execution lifecycle admission.
Never hold a database transaction across runtime I/O. Use conditional writes for stage transitions.

Stages are `pending`, `preparing`, `retiring`, `starting`, `verifying`, `accepting`, `applied`, `failed`, and `superseded`.
Pending reasons distinguish running, question, permission, background work, unknown delivery, unsupported resume,
no live execution, and capacity. Applying covers preparing through verifying.
One durable attempt generation owns one source provider generation and at most one successor in the same execution.
Status reads do not launch or retry work.

On save, coalesce pending work to the latest saved revision. Re-read it before stop and before launch.
If a save arrives during launch, record the older attempt's factual result against its own generation.
Do not admit new input until the coordinator resolves the newer target.
If the replacement is still idle, replace it once for the latest target. Otherwise defer at its safe boundary.
A revert to the currently accepted effective selection cancels pending work before stop.
After provider stop, start a new provider generation using the latest target, even if the user reverted the selection.
Duplicate save requests use revision checks and an idempotent request identity.

## Safe replacement and resource ownership

An idle label alone is insufficient. Require a live, settled provider and durable native resume identity.
Exclude active submissions, delivery uncertainty, unresolved questions, permission requests, provider tool calls, and provider-owned background work.
Use provider activity, prompt generation, and durable delivery evidence. Unknown provider evidence means pending.
An unanswered question can report WAITING_FOR_INPUT while retaining a live tool call. It is not idle for this operation.
An ordinary terminal command or preview is not provider activity merely because it uses the same instance.

Keep the same helper instance, runtime execution ID, container or remote connection, port leases, task environment, and workspace attachments.
Restart only the provider child and its adapter. Keep the native conversation ID, CWD, and transcript.
Do not call `SuspendIdle`, `stopAgentViaBackend`, `DeleteInstance`, `CloseAdmission`, `StopForTeardown`, or the current broad `Manager.Stop`.
Do not rerun workspace setup/cleanup hooks, release workspace leases, reconnect terminals, or reload previews as a substitute for preservation.
Unsupported retained helpers remain pending. There is no fallback to deleting their instance.
Reuse the existing execution capacity reservation. If a provider change requires fresh capacity admission, obtain it before retirement or remain pending.
Do not release workspace leases or count the retained execution and its successor provider as two live sessions.

The existing `process.Manager` contains both lifetimes. Introduce explicit internal provider-only stop/start operations within that owner.
Do not introduce a second workspace owner or move resources between instances.
Separate provider cancellation, adapter workers, stdin/stdout, generation-specific channels, and process-group ownership from instance lifetime.
Keep `shell`, `shellMgr`, `processRunner`, `workspacePreview`, VS Code, workspace trackers, LSP owners, and instance cleanup hooks alive.
Keep instance HTTP/WebSocket services, backend control credentials, and the delivery journal open.
Provider cleanup closes only that provider's transports, sidecars, and temporary delivery claims.
Existing explicit full-stop and teardown callers retain their complete cleanup semantics.

`startWithGeneration` currently renews workspace trackers and uses shared configuration.
The provider-only start must avoid tracker renewal, workspace setup, and shared environment mutation.
Build a frozen provider configuration from the saved profile and retained workspace facts.
Do not call `setTrackerGitEnvironment` or change the environment used by new terminal/workspace process starts during this refresh.
Provider MCP credentials and generation metadata belong in this provider configuration, not the shared terminal environment.
Audit wait groups, exit handlers, event forwarding, and cleanup hooks so an old provider exit cannot close workspace owners or a successor.

Provider processes already use owned process groups. Preserve group termination proof for the old provider before starting a successor.
Independent terminal and workspace process groups must remain untouched, including their descendants.
A provider-spawned job still in its group cannot be preserved by skipping shell cleanup. Defer while that job is active.
An explicitly transferred job may survive only when an existing workspace owner proves the transfer and containment.
Unknown or escaped provider descendants block automatic application. Do not invent a transfer during refresh.

## Provider replacement protocol

Proposed helper capability: `provider-mcp-replacement-v1`. Negotiate it before reserving a replacement.
The public runtime seam calls an authenticated internal `/api/v1/agent/provider-replacement` operation.
Use POST actions `prepare`, `retire`, `activate`, `accept`, and `release`, plus GET status for one operation ID.
The request identifies the backend operation, helper incarnation, execution, source process generation, source binding, and expected configuration revision.
Only the authenticated backend supplies a resolved target profile and native-resume configuration.
The API must reject caller-supplied authority from provider MCP routes. It does not modify settings.

`prepare` validates the tuple, capability, provider idleness, and delivery state and returns a fenced reservation with a 30-second expiry.
It persists an idempotent receipt in retained helper storage before acknowledgement. Repeating the same operation returns its receipt.
A different payload under the same ID or an overlapping operation returns conflict.
Preparation blocks only provider prompt/start/configure/resume/MCP-effect admission. It must not close instance admission.
Workspace terminal, preview, LSP, and independent process requests continue normally, including new resource starts.
Provider requests admitted before reservation must settle before preparation succeeds. Otherwise release the reservation and defer.
Native provider commands carry the helper-owned `KANDEV_PROVIDER_WORKLOAD_ID` activity marker.
Workspace commands do not inherit it. Provider orphan checks use this marker instead of the shared task session ID.
The marker grants no authority. Unsupported platform evidence defers replacement; the first implementation uses Linux.
No network wait holds a manager mutex. Lock order follows session admission, provider operation serialization, then short state/configuration locks.

Persist the source tuple and intended target in SQL before `retire`. Recheck saved revision, policy, actor, and task ownership at this boundary.
The same conditional SQL transition closes backend effect admission for the source scope before sending retire.
The historical accepted-binding pointer does not override a retiring or retired scope's admission state.
Any request queued before that transition must finish under source ownership before retire can succeed. No unresolved effect crosses the boundary.
`retire` validates the receipt and source generation, closes only old provider MCP effect admission, and stops only that provider group.
It persists the termination result. The coordinator resolves a lost response through status, without another stop or a fresh operation ID.
An uncommitted reservation can expire or release without cancelling work. A retired operation cannot expire back into old authority.

After confirmed termination, reread the latest saved selection and permission limits.
`activate` installs a new immutable MCP scope and provider configuration and starts one native-resumed generation without a prompt.
Its request is bound to the current saved target, operation receipt, and unchanged conversation delivery owner.
Preparation for a different target requires a new target digest within the same retired operation before any start.
After start, a newer save becomes a separate pending target. It cannot rewrite the running generation's manifest.
Keep provider mutation and user-prompt admission closed until backend acceptance commits.
A changed target before acceptance supersedes that candidate and converges to the latest saved target without dispatching a user turn.
A duplicate activate returns the recorded result and never launches again.
After matching catalog/native identity evidence, recheck human access and current policy. Commit SQL acceptance with target revision and row-version checks.
`accept` opens provider admission only for that committed current binding and exact operation/generation.
The coordinator reconciles a lost accept response idempotently. Until both sides agree, input remains queued.
A newer save that wins before SQL acceptance keeps the candidate unaccepted and mutation admission closed.
A save after accepted activation creates a new pending target under the ordinary running/idle rules.

The helper records operation stages, source/target generations, and binding digests durably before effects and after their proof.
A compatible control stream stays on the same conversation journal and cursor.
The old provider must drain and its output must be acknowledged before the successor starts. No cursor resets.
A scoped provider requires the replacement protocol header. An older open agent control stream closes at scoped activation.
Workspace transports do not require this provider header and remain open.
The backend owns desired intent and accepted state. Recovery compares both records with actual process liveness.
Do not infer a new authority snapshot from process presence alone.
User Stop and full instance teardown invalidate the reservation through the existing stop generation and win over later activation.
A settings request cannot suppress an explicitly authorized stop.

## Generation-scoped MCP authority

Current `PrepareBuiltinProfile` and `BindInitialProfile` permit only an unused helper.
Current MCP package URLs lack generation scope and bypass control authentication.
Those paths cannot safely install a new authority snapshot into a retained running helper.
Keep their current immutability rules. Add a separate capability-gated generation path.

One provider generation receives one immutable scope: execution ID, helper incarnation, process generation, binding ID, profile digest, and selected packages.
The instance owns a dispatcher for these scopes. Construct a new MCP server/registry/transport for each successor, rather than mutating the old server's profile.
Use proposed scoped routes `/mcp/generations/{binding_id}` and their package routes, with equivalent SSE/message routes only where tested.
Require a fresh provider-only bearer secret bound to that scope. Never reuse the backend control secret or pass the new secret to workspace shells.
Support URL/header or bridge delivery only when that provider transport preserves the scope credential. Do not place secrets in query strings or logs.

Fence calls twice: at the helper scope dispatcher and before backend effects.
Add trusted scope identity to the MCP bridge envelope and `streams.MCPExecutionContext`.
The backend validates it against the current provider owner and accepted immutable binding, not merely the still-live execution ID.
Tool arguments, historical qualified names, old SSE sessions, and cached catalogs cannot supply or replace this authority.
During retirement reject new old-scope effects and prove previously admitted effects settled before granting successor authority.
The new scope permits initialization/catalog reads while verifying. Mutations stay closed until backend acceptance commits.
Retired scopes return a typed stale-generation error before effects, even for a tool still present in the successor selection.
Unscoped legacy routes must never alias to the successor. For a helper upgraded into this flow, retire them before activating the first successor.
Replays and old connections cannot discover a new secret through an old scope.

The retained execution's original binding metadata remains historical initial-binding evidence.
Teach recovery and session views to resolve the current generation snapshot through an explicit pointer.
Do not overwrite or delete the original metadata to trick `prepareBuiltinWorkspaceExecution` into accepting changed authority.
Legacy helper adoption still verifies its original binding. Mixed versions without the new capability remain unsupported.

## Delivery and event identity

Retain the authenticated helper owner, journal file, stream ID, incarnation, SQL harness generation, canonical transcript, queue IDs, and native conversation identity.
A package refresh changes the provider process and tool scope. It does not create a native conversation or delivery owner.
Process generation, harness generation, binding ID, and application attempt generation remain separate fields.
Persist the new process/binding mapping before activation. Its harness generation and stream must equal the source owner.
Do not allocate a harness generation or roll the delivery stream for this maintenance operation.
This preserves the existing queue, cursor, replay, and projection boundaries. Ordinary helper-loss recovery keeps its existing continuity rules.

Before retirement, drain the old provider's terminal events through the existing durable inbox/projection acknowledgement boundary.
An unsettled or uncertain submission blocks refresh. Do not retire it as a side effect of changing MCP settings.
Keep historical streams replayable and old acknowledgements bound to their original stream.
Late events remain associated with the old generation. They cannot settle a new turn, overwrite current binding evidence, or trigger restart loops.
Provider replacement emits maintenance status only, never task completion or workflow advancement.
A queue item received during refresh keeps its original identity and becomes dispatchable only after successor acceptance.
No save, resume, catalog query, or recovery action inserts a synthetic user turn.

## Provider experiment (2026-11-15)

A disposable native probe tested installed Codex 0.162.1 and managed Codex 0.166.0.
Each probe created one read-only initial turn, closed the provider process, and resumed the same thread in a replacement process.
The replacement used a different sentinel MCP tool. Its provider catalog contained the new name and omitted the old name.
Both responses retained one total turn after resume. The probes archived their completed disposable threads.
This proves a native-provider mechanism, not Kandev orchestration, helper resource preservation, or adapter conformance.
The scripts are temporary files under `/tmp/kandev-mcp-tools`. They are not permanent test coverage.
The managed probe used an isolated npm cache after the shared cache returned ENOENT.
An early probe with an empty conversation could not resume because no initial turn established durable history.
A second early probe exposed buffered-output handling in the probe itself. The successful probe used a reader queue.

## Input ownership and cancellation

Application never owns a user turn. Existing message/submission IDs remain authoritative.
Queued prompts retain their IDs, attachments, composed context, and order.
Accepted or uncertain submissions must settle before replacement. Never replay them as a new submission.
Drain queued input only after the latest target passes acceptance, through normal single-owner dispatch.
If a provider cannot establish a resumed idle conversation without a prompt, mark automatic application unsupported.
An ordinary user-authorized resume can later apply the selection through its existing delivery contract.

The coordinator runs under service lifetime, not the save request context.
Closing the settings panel cancels only its waiter. It does not cancel committed work.
User Stop, task archival, deletion, or a workflow-owned terminal transition cancels further automatic application.
Their admission wins over a maintenance retry. A later explicit resume uses the current saved selection.
Test save-request cancellation before commit and after commit separately.

## Provider admission and catalog compatibility

Existing `SupportsSessionResume` or `NativeSessionResume` flags are necessary evidence, not proof of this operation.
Add an explicit adapter capability for idle native resume with fresh MCP attachment and observable catalog completion.
Enable it only after provider-specific conformance tests pass. Unknown adapter versions remain unsupported.

| Provider path | Current repository evidence | Required application proof | Fallback |
| --- | --- | --- | --- |
| Claude ACP | Native resume declaration and session resume flag | Exact conversation retained, fresh deferred tool discovery, no turn | Pending unsupported |
| Codex app-server 0.166.0, Linux | Permanent native helper conformance passes | Same thread, fresh catalog, no turn, surviving workspace connections | Automatic application |
| Codex ACP and other app-server versions | Resume declarations only | Each transport/version requires independent conformance | Pending unsupported |
| Pi ACP | Native resume and prepared project delivery | Rewrite owned project attachment and observe its catalog after resume | Pending unsupported |
| OpenCode / Cursor ACP | Resume declarations | Exact identity and authoritative new attachment for each adapter | Pending unsupported |
| Gemini, custom TUI, plugins, legacy helpers | Generic resume or unknown semantics | No support inferred from a CLI resume flag | Next authorized start |

The app-server row has native conformance evidence. The other rows remain unsupported. Remote executor launch and outage behavior are not claimed as browser coverage. The provider operation stays inside a compatible retained helper.
All executor types must preserve the same invariants. An executor without a retained-runtime provider replacement capability remains pending unsupported.

## Acceptance and stale observations

Acceptance requires matching task/session, retained execution, helper incarnation, attempt, process/harness generations, binding ID, selection, and binding acknowledgement.
The acceptance pointer updates only when the latest saved effective selection still matches the attempt.
Also require the provider to resume the expected conversation and confirm a fresh attachment catalog for each selected package.
Use adapter-specific catalog evidence. A successful helper `tools/list` alone cannot prove the model's catalog changed.
Partial or truncated observations leave the attempt verifying until a bounded timeout, then failed.
A failed candidate remains unable to dispatch provider effects. Cleanup targets only its provider group and scope.
Removed packages must be absent from attachment and registered routes. Old provider-scope credentials must no longer dispatch. Backend control credentials remain valid for workspace services.
Historical qualified tool names are not authority. Calls through stale routes fail before effects.

Generate startup instructions from the same accepted profile. Preserve the system marker, questions, title ownership,
completion gates, autopilot, and delegation boundaries. Do not inject a conversational user turn to refresh instructions.
If an adapter cannot refresh these instructions during native resume, do not advertise automatic application support.
Browser state ignores observations from old generations and rehydrates status from authoritative reads after reconnect.
Applied does not mean every tool is healthy. Keep per-server health and observed tool details separate.

## Recovery and bounded retries

Use one automatic attempt per session and effective target. An explicit Retry allocates one new attempt generation.
A materially new saved target can allocate a new attempt. Repeated identical saves cannot reset a failed attempt.
A retry authenticates the human, checks current row version, and re-resolves the latest saved selection.
It cannot carry an old manifest or grant.

A preparation failure before the retirement transition preserves the old live process and active scope.
After the retirement transition, an uncertain or failed stop leaves provider effect admission closed, even if the old process remains live.
Do not advertise that old tools remain usable in this state. Workspace services remain available.
After retirement failure, preserve workspace services, conversation, and queue. Show recovery controls and do not restart the old permission snapshot.
An ambiguous stop blocks launch until the coordinator establishes liveness. An ambiguous start adopts only its matching persisted replacement identity.
Otherwise fail closed. Never allocate a second process to resolve uncertainty.

Backend startup reconciles nonterminal records against runtime inventory and durable delivery state.
It resumes a known stage or records failure. It does not reset attempt counters or repeat a confirmed stop/start.
Reuse existing bounded startup deadlines. Add a persisted 60-second verification deadline after provider readiness. Backend restart does not extend that deadline.
Timeouts stop automatic retries, not queued input retention. Retry remains available after a safe-state recheck.
Older helpers without the capability retain the next-start behavior.
Do not permit an older backend to adopt an execution after a generation scope transition unless it understands that ownership record.
Such adoption fails closed for provider dispatch. Do not silently restore initial-binding metadata or delete the live workspace runtime.
A normal supported restart or an explicit compatible recovery resolves this state.
Rollout must not synthesize intents for historical saves during migration.

## UI and accessibility

Use Task Customize > MCP servers > Built-in packages and the existing Connections evidence panel.
Keep switches as desired selection. Add an inline application status directly below save feedback.
Show a per-session list when results differ. A single successful session must not certify the task.

Copy examples: “Saved. Waiting for the current turn to finish.”, “Saved. Applying settings.”,
“Applied to this session.”, and “Saved. Settings could not be applied. Retry”.
For pending removal, show “Previous packages remain active until settings apply.”
For unsupported resume, show “Saved. Applies at the next supported session start.”
A provider-start failure may say “Saved. Agent restart failed. Terminals and previews remain available.”
Use the old-active-packages notice only while that old provider is live and its scope still permits effects. After retirement show that the agent is unavailable, not that old tools remain active.
Save failure retains the dirty draft. Application failure keeps the clean saved selection and exposes a separate Retry action.
Do not make a spinner or color the sole status indicator. Use a polite live region without moving focus.

Reuse `BuiltinPackageSelector`, `BuiltinPackageSave`, `BuiltinSessionEvidence`, and shared control sizing.
Phone uses the existing one-level package editor with visible Back/Close, one internal scroller, and fixed safe-area actions.
Keep desktop panel composition. Status content wraps into a single column on phones.
Keep the current modal, scroll position, and notification behavior.
Use 28px desktop controls and at least 44px phone/coarse-pointer targets.
Keep existing tokens and icon library. Routine state changes need no decorative motion.
Any optional transition must respect reduced motion and retain static text.
Localize all copy in seven shipped languages and generate Traditional Chinese with the existing script.
Implementation includes browser and visual verification. The current review inspected source only.

## Observability and verification

Emit structured stage transitions with request, session, attempt, execution, target revision, and bounded failure reason.
Do not log tokens, prompt content, or secrets. IDs belong in logs, never metric labels.
Add bounded outcome counters for applied, deferred, failed, and superseded with stage/reason dimensions.
Expose the persisted stage, process generation, binding ID, and timestamps in session evidence for support diagnostics.

The [plan](../../../plans/task-mcp-application/plan.md) defines exact checks, provider gates, race tests, and phone coverage.

### Context reset and fresh launch compatibility

`scope.harness_generation` records provenance at acceptance. A later authorized context reset can advance the conversation journal within that provider process. Dispatch still requires the exact immutable scope and established current delivery ownership. Recovery compares the helper's current journal owner with SQL, separately from scope provenance. Fresh launches strip provider authority from task/executor metadata. Retained peers restore it only from the matching SQL execution owner. Ordinary starts cannot reuse scoped credentials for another process generation.

Explicit Retry resolves the latest saved task choices under current workspace policy. The SQL request/version, task preference revision, and policy revision must still match before admission. The request cannot supply package choices.

### Permanent native conformance result

`TestProviderReplacementManagedCodexConformance` uses the actual managed 0.166.0 binary with an isolated provider home and local model fixture. It creates one completed turn. The real helper then replaces the provider, resumes the same native thread, enables Configuration, removes Diagnostics, and verifies the live catalog. Model calls remain one. The same PTY, preview URL/content, and editor subprocess connection survive. The test requires `KANDEV_CODEX_MCP_CONFORMANCE_BIN`. No user account, conversation, or external model endpoint is used.
