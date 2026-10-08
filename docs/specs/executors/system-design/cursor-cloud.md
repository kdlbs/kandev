---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-CURSOR-CLOUD-001
  - REQ-EXECUTORS-CURSOR-CLOUD-002
  - REQ-EXECUTORS-CURSOR-CLOUD-003
  - REQ-EXECUTORS-CURSOR-CLOUD-004
  - REQ-EXECUTORS-CURSOR-CLOUD-005
  - REQ-EXECUTORS-CURSOR-CLOUD-006
---

# Cursor Cloud execution system design

## Purpose and boundaries

The executor system owns the cloud binding, admission, lifecycle, and recovery.
This is a draft implementation target. Proposed symbols and tables below do not exist yet.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-CURSOR-CLOUD-001 | Configuration, runtime routing, rollout |
| REQ-EXECUTORS-CURSOR-CLOUD-002 | Launch and follow-ups, cancellation |
| REQ-EXECUTORS-CURSOR-CLOUD-003 | Persistence, observation and recovery |
| REQ-EXECUTORS-CURSOR-CLOUD-004 | Repository contract, capabilities |
| REQ-EXECUTORS-CURSOR-CLOUD-005 | Scoped callback |
| REQ-EXECUTORS-CURSOR-CLOUD-006 | Desktop and phone surfaces |

## Verified baseline

- `internal/agent/runtime/runtime.go` defines `Runtime`, `LaunchSpec`, and `ExecutionRef`.
- `facade.go` delegates to lifecycle. Its `SubscribeEvents` returns `ErrUnsupported` today.
- `internal/agent/runtime/lifecycle/executor_backend.go` defines `ExecutorBackend.CreateInstance` and an `ExecutorInstance.Client` of type `*agentctl.Client`.
- `internal/orchestrator/executor` uses `AgentManagerClient` directly for launch, prompts, stop, and lookup.
- `internal/secrets/store.go` defines `SecretStore` and `ValidateGlobalReference` for shared profile references.


## External contract evidence

Checked on 2026-09-25 against the [Cursor API reference](https://cursor.com/docs/cloud-agent/api/endpoints).
The v1 API provides durable agents, per-prompt runs, SSE observation, cancellation, and repository results.
Git results describe the conversation snapshot. General terminal/filesystem APIs and v1 webhooks are unavailable.
This design sends no envVars. The required create and follow-up contracts are specified below.

No live account was tested. Work order 01 must validate schemas with fixtures.
A live smoke test remains a rollout prerequisite, particularly for MCP and account entitlement.

### Contract gates owned by work order 01

The [Create A Run contract](https://cursor.com/docs/cloud-agent/api/endpoints#create-a-run) documents replacement `mcpServers` definitions on follow-ups.
The [Create An Agent contract](https://cursor.com/docs/cloud-agent/api/endpoints#create-an-agent) documents caller-supplied `agentId` and conflict behavior.
Work order 01 must pin both schemas and exercise unsupported responses in fixtures. Fixtures verify our implementation, not provider compliance.
The live rollout check must prove a second turn uses a new MCP header and the previous grant is rejected.
It must also prove duplicate creation with the same ID cannot create a second agent.

If either contract is absent or contradicted, fail closed and leave launch disabled for that installation.
Do not fall back to a broad or binding-lifetime bearer. Old requests cannot safely prove their operation generation with that bearer alone.
Do not retry a create without its stable ID. A possibly accepted request remains submission unknown and requires explicit resolution.
Changing these boundaries requires a design revision; unsupported-provider behavior is a tested error path, not implicit implementation discretion.

## Runtime routing

Introduce a proposed `RuntimeRouter` in `internal/agent/runtime`.
It selects the existing facade for existing execution kinds and a new `cursorcloud` implementation for `cursor_cloud`.
Selection uses a persisted binding, never the current profile value alone during resume or recovery.
Reject an unknown execution kind; never route `cursor_cloud` through the standalone default.

Keep `ExecutorBackend` unchanged. Do not invent an agentctl URL or client for a cloud execution.
Add typed managed-launch data to `LaunchSpec` for repository identity, starting ref, model selection, and secret reference.
The router must reject unsupported attachments and runtime modes before external calls.

Adapt `AgentManagerClient` integration at the orchestration boundary so launch, prompt, stop, lookup, and liveness reach the router.
Keep existing facade behavior through a compatibility adapter. Cloud events enter the existing normalized event handlers and persistence pipeline.
Do not build a second message history or workflow engine.
The migration must cover reconciliation, deletion cleanup, cancellation tracking, and task-detail capability projection, not only Start.

`Launch` reserves a local execution and binding without contacting Cursor.
`StartExecution` rechecks authorization and admission, then performs the initial create.
`Start` composes those operations. This preserves persistence before paid work starts.
`Resume` submits one journaled follow-up after the prior run is terminal.
`SetMcpMode` applies the server-side capability policy immediately and supplies updated MCP definitions on the next run.
Unsupported transitions return `ErrUnsupported`; they never broaden an existing grant.

## AgentManagerClient compatibility mapping

Work order 05 owns this mapping from `internal/orchestrator/executor/executor.go`.
No method may inherit standalone fallback.

| Existing method | Managed cloud behavior |
| --- | --- |
| `LaunchAgent` | Reserve binding and execution without network submission; return real local identity and no agentctl URL. |
| `StartAgentProcess` | Call journaled StartExecution; repeated start reconciles the same operation. |
| `IsAgentCommandConfigured` | True only for a validated managed launch snapshot; no command or workspace-promotion test. |
| `StopAgent` | Terminate local execution after confirmed remote run cancellation; retain detached conversation binding for explicit later resume. |
| `StopAgentWithReason` | Same termination contract; preserve reason and ownership generation. Force cannot erase unknown remote state. |
| `PromptAgent` | Submit through the shared operation journal and queue. Honor dispatchOnly through acceptance semantics; reject attachments. |
| `CancelAgent` | Cancel the active remote run, keep the local conversation execution resumable, and confirm terminal state before clearing pending. |
| `RespondToPermissionBySessionID` | Return unsupported without provider calls; native provider permission prompts are unavailable. |
| `ListPendingPermissionsBySessionID` | Empty typed list; Kandev user questions remain a separate supported tool flow. |
| `ResolvePermissionBySessionID` | Return unsupported without side effects. |
| `CancelPermissionBySessionID` | Return unsupported without side effects. |
| `ProbeBackgroundWorkloads` | Return ProbeResultUnknown; remote status supplies liveness, not a fabricated process-workload sample. |
| `IsAgentRunningForSession` | Compatibility bool: true while remote work is live or unknown; false only after confirmed terminal/absent state. Recovery uses the richer classification below. |
| `IsAgentReadyForPrompt` | True only when the bound conversation is usable, the prior operation is terminal, and no submission/cancellation is unresolved. |
| `ResolveAgentProfile` | Resolve the managed family and catalog model without requiring a local executable. |
| `SetExecutionDescription` | Update local prompt context for an unsubmitted operation; never mutate an in-flight request digest or silently submit work. |
| `SetExecutionEnv` | Accept an empty map; reject any environment forwarding before dispatch. |
| `SetMcpMode` | Narrow server-side grant policy immediately; reject unsupported surface changes; refresh supported definitions at the next turn. |
| `RestartAgentProcess` | Unsupported. No silent new conversation or context loss. |
| `ResetAgentContext` | Unsupported. A new task session is required for a fresh conversation. |
| `SetSessionModelBySessionID` | Unsupported after binding creation; frozen model remains authoritative. |
| `SetSessionModeBySessionID` | Unsupported native permission-mode switch; no provider mutation. |
| `WasSessionInitialized` | True after remote create/run identity is committed, not merely after reservation. |
| `GetSessionAuthMethods` | Empty list; recovery directs users to the configured secret reference, not interactive CLI login. |
| `IsPassthroughSession` | False. |
| `WritePassthroughStdin` | Unsupported. |
| `ResolvePassthroughConfig` | Unsupported. |
| `MarkPassthroughRunning` | Unsupported. |
| `GetRemoteRuntimeStatusBySession` | Project binding/run state, observation time, and sanitized connection error. |
| `PollRemoteStatusForRecords` | Observe only locally bound identities; never create or submit work. |
| `CleanupStaleExecutionBySessionID` | Remove only confirmed-terminal in-memory tracking. Preserve binding, history, and unresolved work. |
| `EnsureWorkspaceExecutionForSession` | Unsupported; never create local or agentctl workspace resources. |
| `GetExecutionIDForSession` | Resolve/recover the managed execution from its durable session binding. |
| `ListExecutionsForTask` | Include managed executions, including cancellation-pending and unknown operations. |
| `GetGitLog` | Unsupported; archive callers skip workspace snapshot capture by capability. |
| `GetCumulativeDiff` | Unsupported; preserve remote result links instead of fabricating an empty diff. |
| `GetGitStatus` | Unsupported; capability-gated callers must not request local workspace status. |
| `GetGitStatusFresh` | Unsupported, with the same caller guard. |
| `WaitForAgentctlReady` | Unsupported; cloud readiness is not agentctl readiness. |

Implement the optional `PromptTurnIDSetter` for exact cloud turn correlation.
Audit direct manager consumers as well as the interface. Add a typed remote-liveness result: live, terminal, absent, or unknown.
Never use the compatibility bool alone for destructive recovery. A provider outage is unknown, not dead.

The chat Stop action uses `agent.cancel`, through `Service.CancelAgent` to `CancelAgent`.
Explicit session termination and task/session cleanup use `Executor.StopSessionDetailed`, `StopSessionSynchronously`, or `StopByTaskID`, then `StopAgentWithReason`.
Retain those entry-point distinctions. Cloud termination detaches local execution after confirmed cancellation and revokes grants, without deleting the provider agent.
An explicit resume may bind a new local execution generation to the same retained remote conversation, after admission.

## Configuration and capabilities

Add executor type `cursor_cloud` and a distinct managed agent identity with the same name.
Keep `cursor-agent` discovery and local CLI settings unchanged.
Agents-page discovery includes cursor_cloud only when the feature is enabled and the user can use a saved cloud executor profile.
Configured means required fields and secret reference pass local validation; it does not require a successful live probe.
Unsaved/incomplete profiles do not qualify. Temporary provider failures retain visibility with an error state.
Recompute the server projection after executor save/delete or access changes; hide the type when the last qualifying profile disappears.
Preserve stored agent profiles and history. Executor setup must remain accessible independently through executor settings, before agent discovery.
Only the cloud agent and cloud executor pair is compatible in this release.
Agent-profile model values come from the server-side Cursor model catalog; stale selections fail admission.
Existing provider credentials and CLI configuration bundles are not forwarded.

The executor profile stores a global secret reference, consistent with shared executor-profile rules.
That reference shares one Cursor account's access, billing, and attribution across authorized users of the profile.
Recheck task/workspace access and profile-use authority on every dispatch; visibility of a secret or profile is not launch authority.
Do not add per-user keys or a new ACL system in this release. Retain existing profile-use policy and disclose shared billing in configuration.
It also stores a reachable HTTPS callback base URL. Reuse the secret picker and profile save transaction.
Server-only discovery and connection-test handlers validate the secret reference before revealing its value.
Use `https://api.cursor.com` as the production API origin. Unit tests inject a constructor dependency. Real-binary E2E uses the separately gated fixture transport below.
Do not forward Authorization through redirects. Bound responses and timeouts; sanitize errors.

Publish a server-authoritative capability projection for the selected execution.
The proposed fields describe chat, stop, follow-up, remote results, workspace files, terminal, Git mutation, LSP, and preview availability.
Cloud enables the first four and disables workspace capabilities.
Also project model switching, agent-profile switching, native permission prompts, permission-mode switching, plan-mode switching, and context reset as false.
Cloud creation uses agent mode. Hide those chat controls and reject direct mutations server-side before changing persisted selection.
Profile edits apply only to future bindings; they cannot silently change a running conversation's displayed model.
Existing executor values are derived from their current behavior; missing legacy projections retain that behavior.
Unknown managed kinds fail closed. Backend handlers enforce the projection even if a client sends a hidden action directly.

## Persistence

Add an additive migration through `internal/task/repository/sqlite` and the existing database abstraction.
Support SQLite and PostgreSQL using the repository's shared migration conventions.
Do not rename existing execution or message tables.

Proposed tables:

| Table | Identity and fields |
| --- | --- |
| `managed_agent_bindings` | Unique session ID; task/workspace/user IDs; execution ID; provider kind; executor/profile IDs; credential reference; generated remote agent ID; lifecycle; revision |
| `managed_agent_operations` | Unique prompt-turn ID; binding ID; operation kind; request digest; submission state; remote run ID; dispatch generation; timestamps; sanitized error |
| `managed_agent_streams` | Binding and remote run ID; last committed SSE cursor; terminal projection marker; history-gap state |
| `managed_agent_stream_events` | Unique binding, remote run, event ID, and event type receipt; committed with message projection and stream checkpoint |
| `managed_agent_tool_grants` | Grant ID; binding and operation IDs; token hash; scope snapshot; expiry; revocation and generation |

A binding freezes repository identity, starting ref, and model for the conversation.
Profile edits affect future conversations. Secret rotation uses the same reference; verify the remote binding before resuming with changed credentials.
No plaintext API key, callback bearer, provider response body, or signed download URL enters execution metadata or logs.
Short-lived recoverable grant material, if needed for a create retry, uses encrypted secret storage and is deleted after the operation settles.

Compare-and-swap revisions protect concurrent commands and stale background workers.
A per-binding dispatch lease permits one writer; a unique active operation enforces one outstanding submission.
Expired leases allow observation first, never automatic prompt resubmission.
Restart recovery scans local bindings, not the account-wide Cursor agent list.

## Repository contract

Accept one authorized GitHub repository already attached to the task.
Resolve and validate the published starting ref before remote creation. A local-only branch or dirty working tree is never uploaded implicitly.
Show the remote-content rule in the start surface. Local modifications remain untouched.
Use a new Cursor output branch and set `workOnCurrentBranch` false.
Expose `autoCreatePR` as an explicit launch choice, default false, and freeze it in the binding.
Existing PR takeover and writes to the starting branch are excluded.

Normalize provider repository URLs for comparison without changing repository authority.
Result branches and PRs must match the attached GitHub repository before association.
Treat returned Git data as the latest conversation snapshot. Do not attribute the entire snapshot to each completed turn.
Persist the observation time and supplying run ID without claiming a per-run diff.
Render branch and PR links through existing task change-request association contracts where possible.
Unknown or mismatched links produce a diagnostic, not a cross-repository association.
Do not fetch or merge into a local worktree. Do not fabricate workspace or environment filesystem paths.

## Launch and follow-ups

1. Resolve the feature gate, user authority, task ownership, compatible profiles, repository, and remote callback configuration.
2. Reject Office, autopilot, automation, configuration-chat, passthrough, dynamic routing, and unsupported attachments.
3. Persist binding, prompt turn, request digest, and a generated `bc-<uuid>` before create.
4. Issue a scoped callback grant and freeze the create payload. Reuse existing prompt and plan composition with cloud-specific capability limits.
5. Create the remote agent once. Persist its returned initial run before consuming events.
6. Feed normalized activity and terminal outcomes into the existing task event pipeline.

A successful remote create with a failed local commit is recovered using the preallocated agent ID.
On a timeout, query that ID first. If creation remains unconfirmed, retry only the identical create payload and ID after reconciliation.
A conflict requires identity reconciliation; it is not proof that an unrelated agent belongs to this task.
An unrecoverable or contradictory response leaves submission unknown and blocks further dispatch.

Follow-up requests have no assumed provider idempotency key.
Persist dispatch intent before sending. A timeout or process crash during submission leaves the operation unknown.
Never blindly repeat that request. Record the pre-submit latest-run identity and inspect remote runs for evidence.
If exact attribution cannot be established, keep dispatch blocked and show an Open in Cursor recovery action.
Resolve submission permits verified candidate binding or an explicit retry with duplication-risk acknowledgment.
The action must show the candidate ID and state; retries remain a deliberate user action, never a timer effect.
Externally submitted runs cannot be silently attributed to a Kandev prompt.

Use the existing message queue for messages submitted while busy.
`agent_busy` retains queued work and triggers observation; it does not create a new conversation.
Do not support live steering until Cursor provides a verified contract for it.

Normal workflow `on_enter`/`auto_start` prompts, post-completion step prompts, and queued-message drains use this same journaled dispatcher.
Use the existing workflow-operation or message identity as the deduplication source for one prompt turn.
Recheck current step and pending question state immediately before submission. Stale workflow prompts cannot cross a step transition.
An accepted completion signal cannot trigger the next run until the prior remote run is terminal and existing workflow guards permit advancement.
Workflow-generated normal task prompts are supported; excluded automation/Office task origins remain excluded.

## Observation and recovery

Use one SSE observer per active run, independent of whether a browser is connected.
Choose simplified events; do not also render equivalent `interaction_update` events.
Convert text and tool activity into existing stream event shapes, retaining tool-call identity and truncation markers.
Unknown event kinds are ignored with bounded diagnostics. Unknown lifecycle statuses are treated as observation-unknown, never successful completion.

Commit message changes, event receipts, and the checkpoint together in the task repository transaction boundary.
Use `(binding, run, event ID, event type)` as the deduplication identity: different terminal event types may share an SSE ID.
No-ID status framing updates state idempotently and does not advance the cursor.
Guard every update with binding generation and prompt-turn identity.
Terminal result text must reconcile with the saved assistant message, not append a duplicate final response.
Emit terminal side effects once, even if SSE, polling, and restart reconciliation observe the same outcome.

On disconnect, reconnect with bounded exponential backoff and jitter.
Use capped status polling while SSE is unavailable; honor rate-limit retry timing and stop hot loops after authentication failure.
If retention expires, preserve stored messages, mark the gap, and retrieve terminal state.
If the run is still live, continue status observation without fabricating missing activity.
A stream error alone does not mean that the remote run failed.

Map CREATING/RUNNING to active execution; FINISHED settles the turn to waiting for input.
ERROR and EXPIRED expose failed outcomes. CANCELLED settles only after remote confirmation.
No provider outcome alone emits `step_complete_kandev` or bypasses a pending user question.
Wire managed liveness into startup and periodic task reconciliation so no-process heuristics cannot settle a live cloud run.

### Watchdog and restart consumers

Work order 06 covers `internal/task/service` reconciliation plus `internal/orchestrator/service.go` startup reconciliation, `reconcile_liveness.go`,
`event_handlers_stall.go`, `stuck_signal_watchdog.go`, and the lifecycle stall source in `internal/agent/runtime/lifecycle/session.go`.
Check the current source split before editing; `reconcile_restart_test.go` supplies restart regression patterns.
All consumers consult remote-liveness classification at the mutation boundary, with execution and turn-generation guards.
A quiet stream, heartbeat-only stream, long remote tool, or Kandev downtime cannot prove that work never started or became execution-less.
Live or unknown remote state blocks never-started failure, forced turn settlement, and stuck-signal reclamation, even after an accepted completion signal.
It can produce an advisory connection notice. Do not synthesize fake agent activity to satisfy local timers.
A confirmed terminal run goes through once-only reconciliation before pending completion signals advance a workflow.
Test watchdog thresholds with live, unknown, and terminal states, including pending signals.

## Cancellation and cleanup

Persist cancellation intent before calling Cursor. Show the existing cancellation-pending projection until a terminal read confirms the result.
If the cancel response is ambiguous or returns a terminal conflict, read the run state before settling locally.
Late completion may win a cancellation race; preserve the actual provider outcome without duplicate completion effects.
A new follow-up requires confirmed terminal state and resolved submission uncertainty.
Backend shutdown stops observers but leaves remote work running for recovery.

Task/session deletion must stop known active work or report a blocker before removing its binding.
Unknown submission state also blocks destructive local cleanup until the user resolves it.
Kandev task archive retains the native archived task transition and durably schedules session termination for each active cloud binding.
It does not archive or delete the Cursor agent. Archive blocks new prompts immediately and revokes tool grants.
Known active runs enter cancellation pending; unknown submissions retain their journal and a visible unresolved-cleanup warning.
The archived-session reconciler retries observation/cancellation only. It never discards bindings or settles an unknown run as stopped.
Unarchive alone does not relaunch work; an explicit start/resume must pass admission after cleanup is confirmed.
Document this retention boundary and link to Cursor for provider-side cleanup.

## Scoped callback

Add a dedicated managed-execution MCP transport at `/api/v1/managed-agent-mcp/{grant_id}`; never expose unrestricted external MCP with a personal access token.

Mint a random bearer per operation (maximum 24 hours), persist only its hash, and revoke it at settlement. Each follow-up gets a new grant.
Resolve identity and tool scope from trusted binding records. Apply `SurfaceManagedTask` (questions, plans, rich output, completion, optional one-shot title); reject mismatched task/session IDs and omit task creation, workspace, executor, repository-admin, and shell tools.
Reuse `internal/mcp/profile`, `internal/mcp/scope`, and existing handlers. Enforce live ownership, user permission, title, and question/completion guards on every call; recheck grant generation after a long question before returning its result.
Expired grants reject calls with a tool-connection error. Recovery requires stopping the old run before a new turn.

Operators configure reachable HTTPS; only gated E2E permits loopback HTTP. No public tunnel or listen-exposure change is included.
The credential-safe connection test checks URL syntax, routing, and scoped handshake. It cannot prove Cursor reachability; rollout must verify a callback from a real cloud agent.
Only authorized profile configuration selects the callback destination. Never fetch task-supplied URLs or put bearer grants in prompts or query parameters; send them in MCP Authorization headers.
No generic shell or workspace tools are added through this transport.

## Desktop and phone surfaces

Reuse profile editors and the secret selector in `profile-edit/sprites-api-key-card.tsx`.
Add a Cursor Cloud configuration section, model discovery, callback status, and connection test.
Persist secret references only. Configuration errors identify the failing field without exposing values.
The start surface shows repository/ref, cloud-content limits, and an optional PR checkbox.
Unsupported profile fields are unavailable and rejected server-side, rather than silently discarded.

Reuse `components/task/task-layout.tsx` to branch before mounting workspace panels.
Desktop shows conversation activity with a compact remote-results section and Open in Cursor action.
Phone uses the existing `SessionMobileLayout` structure and `MobilePickerSheet` pattern for temporary choices.
Settings remains a direct page; a dense profile form does not become stacked modal sheets.
The conversation has one scrolling body and a fixed composer with safe-area clearance.
Secondary result actions open an inset drawer. Returning focus restores the opener.

Required states: unconfigured, checking, ready, starting, running, reconnecting, cancellation pending, finished, failed, history gap, and submission unknown.
The last state presents Open in Cursor and Resolve submission; normal Send remains disabled.
The resolution surface shows candidate runs and an explicit retry acknowledgment.
All new copy uses localization catalogs, including Portuguese, Japanese, and both Traditional Chinese variants generated through the repository command.
Controls use 28px desktop sizing and at least 44px phone/coarse-pointer hit areas.
Use `100dvh`, safe-area padding, and no document horizontal overflow.
Mobile fallback never overwrites saved desktop panel preferences.

## Real-binary E2E isolation

Add `KANDEV_MOCK_CURSOR_CLOUD` under mocks in root `profiles.yaml`: prod empty, dev empty, e2e true.
This mock selector does not enable the feature flag. Feature defaults remain false in all profiles.
Add a fixture-only `KANDEV_MOCK_CURSOR_CLOUD_BASE_URL` read at backend composition, populated with the worker's allocated local server URL.
Honor it only when the resolved profile is e2e and the mock selector is true.
Production and dev ignore both variables and retain the fixed HTTPS provider origin, even if an override is present in the process environment.
Test those negative paths without setting the E2E profile selector; setting `KANDEV_E2E_MOCK=true` selects e2e by definition.

The override accepts only loopback hosts on the fixture's allocated port, without credentials, queries, fragments, or redirects.
Under the same e2e/mock gate only, callback admission accepts the worker backend's loopback HTTP origin.
Do not add a general insecure-TLS switch or relax callback validation in production.
The mock server exercises actual outbound requests and scoped callbacks through the real backend routes.
Start it before restarting the backend; stop it after restoring the backend baseline during fixture teardown.

Add two dedicated projects to `apps/web/e2e/playwright.config.ts`: `cursor-cloud` (Desktop Chrome) and `cursor-cloud-mobile` (Pixel 5).
They match `tests/session/cursor-cloud*.spec.ts` and `tests/session/mobile-cursor-cloud*.spec.ts` respectively.
Add explicit ignores for these patterns to ordinary chromium and mobile-chrome projects so specs cannot run twice or share unrelated workers.
The Cursor fixture restarts its isolated backend with `KANDEV_FEATURES_CURSOR_CLOUD=true`, the mock selector, and allocated mock URL.
Use `backend.restart(overrides)` from a baseline snapshot; do not mutate runtime overrides in ordinary projects.
After each spec, restore the dedicated fixture baseline; final teardown uses `backend.restart()` before stopping the mock.
Disabled-feature tests restart with the flag false and restore their baseline afterwards.

Work order 07 owns fixture creation, project registration, runner/manifests discovery, and ordinary UI scenarios.
Work order 08 owns failure injection and end-to-end unknown-submission recovery.
Ensure CI shard manifests enumerate both new projects and the guarded runner accepts their names.
Use one worker per shard and retain the memory-aware runner. No feature-enabled mock origin may leak into ordinary projects.

## Rollout and observability

Introduce `features.cursorCloud` / `KANDEV_FEATURES_CURSOR_CLOUD`, restart-required, disabled in prod/dev/e2e profiles.
Register it through the typed runtime flag registry and matching frontend defaults.
Gate configuration, catalogs, launch, follow-up, tool callbacks, and automatic dispatch at backend composition and request boundaries.
When disabled, retain read-only history and narrow observation/cancel access for existing bindings so paid work is not orphaned.
This drain exception cannot submit prompts or invoke agent tools and requires the existing binding and normal user authority.
Fixture tests opt in explicitly; mocks must be unreachable in production.

Use structured logs for admission, dispatch uncertainty, reconnects, cancellation, and terminal reconciliation.
Expose these expvar counter families and matching structured log events. Label sets are closed:

| Metric | Labels and increment boundary |
| --- | --- |
| `cursor_cloud_dispatch_total` | operation: create, followup; outcome: accepted, rejected, unknown. Once when a journaled submission attempt obtains that classification. |
| `cursor_cloud_reconnect_total` | reason: disconnect, backend_restart; outcome: connected, retryable_error, auth_error, history_expired. Once per observer reconnect attempt. |
| `cursor_cloud_submission_unknown_total` | operation: create, followup; reason: timeout, transport, crash_recovery, persistence. Once per operation's transition into unknown. |
| `cursor_cloud_cancel_total` | outcome: confirmed, pending, rejected. Once per cancellation attempt after response/readback classification. |

Counter updates do not submit work, transition workflows, or imply billing precision.
Do not increment unknown repeatedly while polling the same operation. Diagnostics never weaken journal idempotency.
IDs belong in logs, not metric labels.
Work order 06 implements and tests the counters; work order 08 documents these families in root `AGENTS.md` under Observability.
`CLAUDE.md` is its shared entry point; preserve the symlink rather than creating divergent content.
Never log prompts, API keys, bearer grants, raw provider bodies, or MCP headers.
Public documentation ships with implementation in executors, agents/profiles, and remote-access guidance.
Keep this planning turn limited to internal documents.

## Related decisions and implementation

- [Managed remote-agent runtime boundary](../../../decisions/2026-09-25-managed-remote-agent-runtime.md).
- [Task model unification](../../../decisions/0004-task-model-unification.md).
- [Implementation plan and work orders](../../../plans/cursor-cloud/plan.md).
