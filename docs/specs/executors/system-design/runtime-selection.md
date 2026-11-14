---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-RUNTIME-SELECTION-001
  - REQ-EXECUTORS-RUNTIME-SELECTION-002
  - REQ-EXECUTORS-RUNTIME-SELECTION-003
---

# Executor runtime selection design

## Boundary and mapping

Executors owns selection enforcement, retained runtime compatibility, and physical identity.
Task orchestration supplies the selected executor after its existing precedence rules.
Auth supplies caller and durable actor authority. Runtime selection never grants either authority.

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-RUNTIME-SELECTION-001 | Selection and admission; Failure channels |
| REQ-EXECUTORS-RUNTIME-SELECTION-002 | Retained execution; Recovery and Stop |
| REQ-EXECUTORS-RUNTIME-SELECTION-003 | Identity projection; Verification |

The [required-runtime decision](../../../decisions/2026-11-08-required-executor-runtime.md) records the compatibility choice.
The [ownership design](per-user-ownership.md) remains authoritative for actor checks.
The [workspace reuse design](../../tasks/system-design/additional-session-workspace-reuse.md) remains authoritative for environment transitions and source continuity.

## Selection and admission

Keep `models.ExecutorType.Runtime()` and `executor.ExecutorTypeToBackend` unchanged.
Nonempty effective selections require the mapped backend. An empty selection uses `getDefaultExecutorBackend`.
Defaults resolved to a concrete executor are required selections, even without a picker action in this request.
Do not infer an unspecified selection from missing container metadata or a provider error.
`resolveExecutorConfig` can retain an executor ID with an empty type after a failed lookup.
Preserve that selection intent and propagate the existing authorization/lookup error before runtime defaulting.
An unresolved concrete executor/profile binding must never become the empty-selection compatibility path.
An authenticated retained binding is also selection evidence, even when an older session omits its executor type.
Resolve it through current ownership and provider rules. Do not replace it with the host default.
Forward an empty resolved type to lifecycle retained validation so the binding's own type is still compared.

`Manager.getExecutorBackend` must return a typed unavailable error for every missing nonempty backend.
Its result must not depend on `ExecutorFallbackAllow` or `ExecutorFallbackWarn` for a required selection.
Keep the constructor parameter and constants for internal source compatibility during this repair. Mark their restricted semantics explicitly.
Set production wiring to `ExecutorFallbackDeny` to express intent. That wiring change alone cannot protect cached paths.
No public fallback setting or new per-request bypass is introduced.

Use lifecycle-owned read-only admission through the existing narrow orchestrator capability seam, with required production wiring.
Proposed registration operation: `ValidateExecutorRuntime(executorType string) error`.
Proposed reuse operation: `ValidateExecutionRuntime(ctx, sessionID, taskEnvironmentID, executorType) error`.
The first resolves required registration. The second compares the selected reuse candidate's cached and durable evidence against its target environment binding.
`orchestrator.NewService` currently installs capabilities through an optional type assertion in `service.go`.
Make production runtime admission wiring required and cover the real adapter path. A missing validator must not silently permit concrete selection.
It does not provision compute, probe credentials, reconcile a checkout, acquire a journal, or stop an execution.
Existing capability booleans such as `RequiresCloneURL` cannot replace this error-bearing admission.

Order admission as follows:

1. Resolve the effective executor and authorize the caller or durable actor through existing admission.
2. Validate registration before credential resolution, scripts, workspace mutation, environment claims, or runtime cleanup for this attempt.
   Classify any existing policy-authorized environment transition through read-only authoritative state.
   For a reuse candidate, validate actual compatibility before credentials or cleanup.
3. Retain provider-specific validation and creation on the selected backend. Daemon and creation errors propagate without host retry.
4. Validate returned runtime and required Docker identity before registration, persistence, readiness, or agent startup.

An invalid returned identity uses existing exact-resource rollback only when this attempt proves ownership of a newly created resource.
Retained adoption and unproven resources remain recorded for authorized observation and cleanup. Never guess a destructive target.

Orchestrator integration sites include `prepareSessionAttempt`, `buildLaunchAgentRequest`, and resume preflight before `cleanupStaleExecutionBeforeResume`.
The credential boundary in `buildResumeRequestAtCredentialBoundaryWithOptions` must also retain the check.
If PR #5175 is present, registration denial precedes `claimInheritedEnvironmentTransition`.
Preserve existing authorization before dependencies. A rejected caller must not learn private runtime details.

Lifecycle repeats the check at `Launch`, before `launchInternal` profile/environment preparation, and in workspace-only creation.
Move registration validation ahead of `reconcileExecutionWorkspace` and credential-bearing `prepareExecutionCreateRequest`.
Revalidate the selected result after session coalescing and under the existing serialization boundary before promotion.
An admitted provider can fail later. That failure never selects another backend.

## Retained execution

Compare normalized runtimes, not raw executor strings: Local, `local_pc`, and Worktree all use standalone.
The known `mock_remote` mapping remains a test-only standalone control.
Compare current selected configuration against actual `AgentExecution.RuntimeName` and the matching `executors_running.Runtime` record.
For Local Docker, require a nonempty container ID and the Docker reconnect path's authenticated resource evidence.
Metadata saying `executor_type=local_docker` or `is_remote=true` cannot override standalone evidence.

Apply this predicate at all return/admission sites:

- Session cache fast paths in `GetOrEnsureExecution` and `EnsureWorkspaceExecutionForSession`.
- Environment cache paths in `GetOrEnsureExecutionForEnvironment` and `ensureWorkspaceExecutionLocked`.
  Compare against the requesting session's resolved selection, not just the execution's original session.
- Both owners and waiters of the coalesced ensure/launch operation, then `promoteWorkspaceExecution` before MCP or agent profile mutation.
- Resume request construction before credential leases and destructive stale-runtime cleanup.
- `StartAgentProcess` and `PromptAgent` before new authenticated effects on an existing execution.
- `EnsurePassthroughExecution`, `ResumePassthroughSession`, and the shared passthrough start path before terminal process reuse or startup.
  The existing ready-process fast return must not bypass compatibility.

An existing required-runtime mismatch returns a typed mismatch error even when the requested backend is now healthy.
Do not remove the cached worker, rotate its execution ID, relabel it, or create a replacement in that branch.
Any later deliberate runtime transition follows existing workspace policy and requires its normal authorization.
Unknown retained runtime or conflicting container evidence blocks new work. It does not imply standalone compatibility.

### Registration must settle before comparison

`registerAndPublishCreatedExecution` adds an execution to the in-memory store before `persistExecutorRunningResult` rewrites `executors_running.agent_execution_id`.
Callers that compare outside the session's coalesced slot (the orchestrator's resume admission through `admitRetainedExecutionRuntime`) can otherwise observe the new execution next to the predecessor's row and refuse it as an identity mismatch (`AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.10`).

Each newly created `AgentExecution` carries a registration-settled signal that closes exactly once, after its inventory row is persisted or after its registration is rolled back.
`ValidateExecutionRuntime` waits for that signal, bounded by the caller's context, before it reads the retained row, then re-reads the store.
A rolled-back or rotated execution falls through to the durable-row path; it is never compared against its own unsettled registration.
Executions adopted by recovery or built without the creation path have no pending registration and compare immediately.
The wait never weakens the predicate: a settled registration whose row still names another execution remains refused.
Do not take `remoteInstanceLifecycleMu` in the validator. Rollback holds it across provider teardown, and the lock is not context-aware.

### Authorized transitions are separate from reuse

PR #5175's refreshed transition design can claim a child environment and retire that session's exact inherited predecessor.
Preserve that path when its authoritative old/new environment identities and existing policy prove a deliberate transition.
The old worker is not a reuse candidate for the new environment.
Perform registration admission first, then preserve the existing transition claim, generation, active-agent refusal, and identity-aware cleanup rules.
Final lifecycle admission still refuses any predecessor returned from cache instead of the selected new runtime.
There is no client-supplied transition bypass or new cleanup permission.

A same-binding environment labelled `local_docker` with actual standalone is the defect, not an eligible executor transition.
Do not manufacture detachment from that mismatch. Unknown predecessor identity retains the existing refusal.
When available, use the execution's persisted `task_environment_id` to distinguish predecessor inventory from selected-environment inventory.
Classify that predecessor before runtime comparison so an authorized transition can replace it; same-binding rows stay refused.
Do not infer that distinction from runtime names alone.

## Recovery and Stop

`ExecutorRegistry` and recovery keep using actual recorded runtime for transport and resource ownership.
Passive authenticated adoption can preserve observation, delivery reconciliation, and Stop for an already admitted process.
A recovered mismatch must not emit an availability result that authorizes a new prompt or marks the required Docker boundary satisfied.
Perform the compatibility decision before availability publication in `manager_lifecycle.go` and `manager_recovery_turn_outcome.go`.
Retain the existing authenticated descriptor, SQL generation, cursor, submission, and source-root checks.
No extra ACP initialize, load, resume, new, or journal writer is permitted to diagnose the mismatch.

Stop, cancellation, read-only transcript access, and cleanup use actual resource identity.
They must not call required-selection admission as a prerequisite for operating on an existing mismatched worker.
Keep existing ownership checks on those surfaces. Stop targets only the authorized exact execution.
An unreachable actual provider can still make Stop fail. Report that provider failure without substituting a local stop or hiding the action.
An ownership change or selection mismatch never terminates unrelated work automatically.

## Identity projection

ADR [0003](../../../decisions/0003-executors-running-as-execution-id-source-of-truth.md) remains the persistence rule.
Lifecycle owns `executors_running.runtime`, `container_id`, status, and execution ID.
`buildRunningFromExecution` already writes actual runtime and container identity. Do not add parallel session columns or a second writer.

Add a read-only `execution_runtime` field to session detail and summary DTOs.
Use JSON `null` for unknown, and a runtime string when actual inventory is known.
Read it from the same `executors_running` join as execution ID and container ID in `session.go` and `session_summary.go`.
Carry it through `TaskSession`, summary observation conversion, DTO conversion, and direct session event maps.
Update the handwritten web session type to accept the optional nullable field for older servers.
Do not change the existing optional `container_id` wire semantics or infer runtime from it.
No SQL schema migration is needed because the inventory runtime column already exists.

Existing executor/profile IDs and snapshots describe configuration. `execution_runtime` describes the recorded execution.
Existing runtime event `runtime` and remote-status `runtime_name` retain their names and actual-runtime meaning.
The execution ID correlates these surfaces. A retained stopped row remains historical inventory and does not prove liveness.
When no inventory exists, the new field stays null even if configuration selects Docker.
Do not turn `ResolveSessionRuntime`'s cold configuration prediction into observed runtime evidence.

Successful Docker creation must agree on selected mapping, backend name, returned instance runtime, and container ID.
The Docker backend establishes resource ownership through its daemon and existing authenticated reconnect checks.
Tests independently inspect the returned ID in the daemon inventory and compare backend and worker container membership.
The hostname and filesystem markers are never identity authorities.

## Failure channels

Define typed sentinels and a bounded safe error payload for unavailable runtime, runtime mismatch, and invalid resource identity.
Proposed reason codes: `executor_runtime_unavailable`, `executor_runtime_mismatch`, and `executor_identity_unverified`.
Keep raw provider errors in protected logs. Persist only sanitized reasons through the existing `LastAgentError` cause/details path.
Use the existing generic launch-failure presentation and recovery actions. Retry must preserve selection and repeat admission.
Do not introduce a new launch category that silently falls through frontend normalization.

The safe diagnostic names the selected runtime, observed runtime when known, and the permitted next action.
It contains no Docker endpoint, host path, secret reference, raw environment, or private profile details.
A missing Docker backend explains that it is disabled or unavailable, without claiming which configuration source caused it.
Ownership denial retains its existing not-found/forbidden precedence.

No new rendered controls, composition, or translation keys are required for this package.
Server diagnostics remain diagnostic data within existing translated failure framing.
If implementation requires authored browser copy, externalize it through `t()` in all seven languages and generate Traditional Chinese and pseudo catalogs.
That change requires updating this package's UI scope and exact rendered checks first.

## Verification

Use table-driven registration tests for all internal fallback policies and runtime controls.
Use counting fakes for credentials, scripts, create, process-start, prompt, and stop operations.
Registration denial requires zero such effects. Provider errors require zero standalone effects.
Use real temporary SQLite for retained inventory and projection tests, including a mixed inventory with a healthy sibling.
Exercise authorized caller, unauthorized caller, durable actor, deleted actor, and auth-disabled controls.
Use channel barriers for concurrent ensure/promotion tests, with cancellation and execution-rotation cases.

The [delivery plan](../../../plans/explicit-executor-runtime-isolation/plan.md) defines exact future test names, commands, and the disposable container recipe.
Only document validation and disposable unit diagnosis run during planning.
