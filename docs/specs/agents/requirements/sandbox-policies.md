---
status: draft
system: agents
created: 2026-10-22
owners:
  - kandev
---

# Sandbox policy requirements

## Overview

Operators can confine Local and Worktree agent executions with an optional sandbox provider.
Agents owns effective launch policy and its operator authority.
[Generic wrapping](generic-launch-wrappers.md) remains available without plugins.
[Plugin capability](../../plugins/requirements/sandbox-provider-capability.md) owns extension compatibility.
Tasks retains launch failure, recovery, queue, and error-history ownership.

## Terms

- **Policy revision:** An immutable, operator-approved set of filesystem, network, and runtime permissions.
- **Ceiling:** The maximum access an executor permits, including its mandatory restrictions.
- **Selection:** A reference to an approved revision, not provider flags or profile text.
- **Attempt:** One process launch generation within a Kandev session.
- **Learning:** An operator reviews denied operations and explicitly approves a new policy revision.
- **Execution details:** A session view of its captured launch configuration, current enforcement evidence, and protection scope.

## Requirements

### REQ-AGENTS-SANDBOX-001: Optional enforcement

**Intent:** Enable provider services without making basic command wrapping depend on them.

- **AC-AGENTS-SANDBOX-001.1:** With every sandbox plugin disabled, valid generic prefixes shall retain their supported launch behavior.
- **AC-AGENTS-SANDBOX-001.2:** A selected sandbox shall fail before agent startup when its provider, version, host, or required capability is unavailable.
- **AC-AGENTS-SANDBOX-001.3:** Failure shall never trigger an unwrapped retry, broader permissions, or silent provider substitution.
- **AC-AGENTS-SANDBOX-001.4:** Availability, launch success, and verified confinement shall appear as distinct outcomes for each tested host and transport.

### REQ-AGENTS-SANDBOX-002: Policy selection and authority

**Intent:** Operators control defaults and restrictions across agent execution contexts.

- **AC-AGENTS-SANDBOX-002.1:** Executor profiles shall define the provider, ceiling, eligible revisions, and default selection. Agent profiles can define an eligible default.
- **AC-AGENTS-SANDBOX-002.2:** An explicit task selection shall precede its workflow-step selection, then its agent default, then its executor default.
- **AC-AGENTS-SANDBOX-002.3:** Every selection shall satisfy the executor ceiling. An invalid, deleted, ambiguous, or incompatible selection shall stop launch without fallback.
- **AC-AGENTS-SANDBOX-002.4:** Only an authenticated human operator with configuration authority can change defaults, ceilings, selections, decisions, or learned policies.
  Agent tools, task text, repository files, agent credentials, and synthetic administrators shall not confer that authority.
- **AC-AGENTS-SANDBOX-002.5:** Policy-required executions shall retain their selected revision across continuation, recovery, and automatic replacement.
  An explicit restart can select current approved defaults, but shall display and record any policy change before launch.
- **AC-AGENTS-SANDBOX-002.6:** A task or workflow cannot disable required confinement or weaken mandatory restrictions through profile selection, environment variables, wrapper flags, or imports.

### REQ-AGENTS-SANDBOX-003: Lifecycle and approvals

**Intent:** A sandbox preserves agent transport and offers bounded operator decisions.

- **AC-AGENTS-SANDBOX-003.1:** Supported discovery shall remain prompt-free. ACP chat shall preserve framing, while native terminal mode shall preserve input and resize behavior.
- **AC-AGENTS-SANDBOX-003.2:** Initial launch, restart, cancellation, timeout, failed startup, and cleanup shall preserve attempt identity and terminate only owned resources.
- **AC-AGENTS-SANDBOX-003.3:** A verified approval request shall display its exact operation, policy revision, session, deadline, and permitted decisions to authorized operators.
- **AC-AGENTS-SANDBOX-003.4:** Denial, timeout, malformed replies, lost channels, stale requests, and unverified callers shall never grant access.
  Approval shall apply only once to the identified attempt and operation.
- **AC-AGENTS-SANDBOX-003.5:** Ordinary agent auto-approval shall not approve sandbox requests. Unsupported approval types shall remain denied with an explicit reason.
- **AC-AGENTS-SANDBOX-003.6:** A browser disconnect shall not detach or stop the supervisor. Core cancellation shall remain available if the plugin crashes or is disabled.

### REQ-AGENTS-SANDBOX-004: Denial reporting and learning

**Intent:** Operators can improve policy without converting agent requests into authority.

- **AC-AGENTS-SANDBOX-004.1:** Reports shall distinguish filesystem, network, and command denials from launch, authentication, and transport errors.
  Missing, truncated, or unverified evidence shall appear explicitly.
- **AC-AGENTS-SANDBOX-004.2:** A learning proposal shall show the exact access difference, source revision, inherited profiles, and intended persistence scope.
  It shall grant nothing before explicit operator approval.
- **AC-AGENTS-SANDBOX-004.3:** Applying a proposal shall create an immutable revision with operator, time, source digests, and decision provenance.
  It shall reject stale revisions and changes beyond the ceiling.
- **AC-AGENTS-SANDBOX-004.4:** Suppressing a repeated denial notice shall not grant access. A one-operation approval shall not change a persisted policy.
- **AC-AGENTS-SANDBOX-004.5:** Existing attempts shall retain their snapshot. New attempts shall use a learned revision only after an authorized selection change.

### REQ-AGENTS-SANDBOX-005: Explicit runtime connectivity

**Intent:** Required Kandev tools work through narrowly approved routes.

- **AC-AGENTS-SANDBOX-005.1:** Policy can explicitly allow the current session's Kandev MCP service and required runtime artifacts.
  It shall use execution-host identities rather than fixed ports or machine-specific paths.
- **AC-AGENTS-SANDBOX-005.2:** A permitted MCP route shall not grant access to settings APIs, other sessions, unrelated loopback services, or provider approval channels.
- **AC-AGENTS-SANDBOX-005.3:** Endpoint rotation shall revoke the old route and resolve the new route through the same approved policy intent.
  Unavailable endpoints shall stop launch or report an explicit connection failure without broadening policy.
- **AC-AGENTS-SANDBOX-005.4:** No integration default shall use unrestricted network access, arbitrary localhost ports, or the whole temporary directory.

### REQ-AGENTS-SANDBOX-006: Operator surfaces and acceptance

**Intent:** Operators can select, inspect, and change policy on desktop and phone.

- **AC-AGENTS-SANDBOX-006.1:** Settings and task surfaces shall show the effective policy, its source, ceiling restrictions, and capability health.
  Loading, empty, invalid, unavailable, and read-only states shall preserve saved selections.
- **AC-AGENTS-SANDBOX-006.2:** Desktop and phone shall provide equivalent selection, approval, denial inspection, and learning outcomes with localized copy and keyboard access.
  Phone controls shall have at least 44px targets and avoid horizontal page overflow.
- **AC-AGENTS-SANDBOX-006.3:** Nono acceptance shall prove discovery, ACP tools, native terminal tools, restart, cancellation, and cleanup with the pinned real runtime.
- **AC-AGENTS-SANDBOX-006.4:** Each supported configuration shall prove permitted work and blocked filesystem and network operations.
  Generic fixtures and successful replacement sessions shall not substitute for these checks.

### REQ-AGENTS-SANDBOX-007: Discoverable execution configuration

**Intent:** Operators configure ordinary wrappers and optional sandboxes without constructing provider commands.

- **AC-AGENTS-SANDBOX-007.1:** Agent profile settings shall expose an Execution section with separate Command wrapper and Sandbox groups.
  Existing command prefixes shall retain their value and behavior. Editing a wrapper shall not require a plugin.
- **AC-AGENTS-SANDBOX-007.2:** Command wrapper shall provide an editable prefix, validation, and a safe command preview.
  Runtime placeholders shall remain explicit until execution context exists. Viewing settings shall not start an agent or install a provider.
- **AC-AGENTS-SANDBOX-007.3:** Sandbox shall show the executor-selected provider, agent policy default, inherited source, availability, and supported capabilities.
  When executor context is absent, settings shall request that context before claiming an effective policy or host support.
- **AC-AGENTS-SANDBOX-007.4:** The policy summary shall show readable locations, writable locations, network permissions, Kandev MCP access, and mandatory restrictions.
  Unsupported or unknown permissions shall remain explicit. Details shall distinguish unavailable capabilities from disabled capabilities.
- **AC-AGENTS-SANDBOX-007.5:** A custom wrapper and a sandbox selection shall coexist. The custom wrapper shall run inside the selected confinement.
  Provider names and command text shall not select special UI behavior or prove enforcement.
- **AC-AGENTS-SANDBOX-007.6:** Workflow-step and task selectors shall show inherited defaults and effective selection sources before launch.
  Invalid selections shall preserve the saved reference and explain the launch blocker. Read-only users shall not receive mutation controls.

### REQ-AGENTS-SANDBOX-008: Session execution visibility

**Intent:** Users can inspect actual session execution without confusing configuration with active confinement.

- **AC-AGENTS-SANDBOX-008.1:** Execution details shall show the captured wrapper, provider, policy revision, selection source, and current attempt status.
  Settings for the next launch shall appear separately. Editing defaults shall not change the displayed history of an existing attempt.
- **AC-AGENTS-SANDBOX-008.2:** The sandbox status shall distinguish configured, starting, active, failed, unknown, stopped, and not configured.
  Only current runtime evidence bound to the attempt and policy shall permit active status.
  A launch receipt, installed executable, provider declaration, or successful agent reply shall not suffice.
- **AC-AGENTS-SANDBOX-008.3:** Each Dockview session tab shall expose its own execution indicator and Execution details action.
  A confirmed active sandbox shall use a shield. An ordinary wrapper shall use a neutral wrapper indicator.
  A failed required sandbox shall use a warning. Configured, starting, unknown, and stopped states shall not show an active shield.
- **AC-AGENTS-SANDBOX-008.4:** Status from a sibling session or previous attempt shall not change the selected session's indicator.
  Connection loss or uncertain runtime ownership shall remove the active claim until current evidence restores it.
  A completed or stopped attempt shall retain historical evidence without claiming current protection.
- **AC-AGENTS-SANDBOX-008.5:** Execution details shall state which agent process tree the policy covers.
  It shall not imply protection for user terminals, repository setup, external MCP servers, or the whole task.
  Kandev MCP route access and external service protection shall appear as separate facts.
- **AC-AGENTS-SANDBOX-008.6:** Execution details shall expose supported approval and denial-report views with their completeness and service health.
  Provider loss shall preserve the captured configuration. It shall not imply that the kernel removed existing confinement.
- **AC-AGENTS-SANDBOX-008.7:** Desktop users shall open details with pointer or keyboard activation without closing, renaming, or dragging the session tab.
  Phone users shall have a labelled, tappable equivalent. Tooltips, color, and hover shall not be the only status or navigation cues.
- **AC-AGENTS-SANDBOX-008.8:** Previews, indicators, and detail responses shall omit credentials, callback secrets, and unfiltered provider output.
  Legacy attempts without recorded execution evidence shall show unknown history. Current profile settings shall not reconstruct that history.

### REQ-AGENTS-SANDBOX-009: Server-enforced MCP surface for guarded sessions

**Intent:** An agent under a task guard cannot widen its authority through Kandev's own MCP tools. The catalogue agentctl serves is not the only gate.

- **Guarded session:** a Kanban or Office task session whose task holds a task guard in the session's workspace. Coordinator, automation, configuration, and managed-conversation principals are not guarded sessions and keep their own guards.
- **Direct child:** a task in the caller's workspace whose parent is the caller's task.

- **AC-AGENTS-SANDBOX-009.1:** The MCP dispatcher shall derive guarded status from the server-resolved caller task, never from payload fields. It reads the task guard once per call. A guard that cannot be read, is malformed, or belongs to another workspace shall refuse the call as an unavailable principal, never treat the session as unguarded.
- **AC-AGENTS-SANDBOX-009.2:** A guarded session shall reach only actions the classification table lists as `read`, `self`, `child_write`, or `internal`. Any other action, including an unlisted one, is refused as an unknown action even when a catalogue lists it. The table is keyed by bare tool name, so shared actions follow their table class.
- **AC-AGENTS-SANDBOX-009.3:** Class `denied` shall contain configuration and settings mutations (agents, agent profiles, executors, executor profiles, workflows and steps, settings, MCP config, prompts, automations), `transfer_task`, `resolve_agent_permission`, `answer_question`, plugin tool listing and invocation, canvas publication, and `archive_task` and `delete_task`, which cascade beyond the writable set.
- **AC-AGENTS-SANDBOX-009.4:** A `self` action shall name no task or the caller's task. A `child_write` action shall name only the caller's task or its direct children. Referenced fields are `task_id`, the task of `session_id`, `parent_id`, `depends_on_task_id`, and `blocked_by` in array order. A missing, null, or empty field references nothing. A `read` action may reference only a task, session, workflow, or workspace of the caller's workspace; the refusals of AC-AGENTS-SANDBOX-009.5 apply. A `read` action that lists across workspaces returns only the caller's workspace, and tools whose data is install-wide rather than workspace-owned (executor profiles, agents and their profiles, diagnostic log bundles) are denied.
- **AC-AGENTS-SANDBOX-009.5:** A missing task, a foreign-workspace task, an unrelated task, a missing session, and a malformed or wrongly typed reference shall all return the same Forbidden refusal with reason `guarded_session_scope`. A repository read failure shall return Unavailable with reason `guard_lookup_failed`.
- **AC-AGENTS-SANDBOX-009.6:** Payload keys shall match reference and attribution fields case-insensitively with Unicode simple folding. Two keys folding to one field, including duplicate identical keys, shall refuse the call. The handler shall receive only canonical key names. A null or empty payload is `{}`; any other non-object is refused.
- **AC-AGENTS-SANDBOX-009.7:** Attribution fields (`source_task_id`, `sender_task_id`, `caller_task_id`, caller or source session) shall be overwritten with the principal's values.
- **AC-AGENTS-SANDBOX-009.8:** For `create_task` the workspace is the caller's (a different non-empty string or a wrong type is refused). An empty, `self`, or own-task `parent_id` resolves to the caller's task, and any other `parent_id` is refused. Guard inheritance and executor admission remain owned by task creation admission.
- **AC-AGENTS-SANDBOX-009.9:** A `message_task` to the caller's parent shall be refused. Parent-reporting routes (`step_complete_kandev`, `ask_parent_question_kandev`, `ask_user_question_kandev`) are unchanged.
- **AC-AGENTS-SANDBOX-009.10:** A tool in the task-session catalogue without a table entry shall fail a parity test enumerating the assembled catalogue for the Kanban and Office surfaces. Plugin tools are reached only through `invoke_plugin_tool`, class `denied`.
- **AC-AGENTS-SANDBOX-009.11:** Coordinator and automation principals are evaluated only by their existing guards. Sessions without a task guard are unchanged; extending this surface to them is a follow-up.
- **AC-AGENTS-SANDBOX-009.12:** Every criterion shall hold with `KANDEV_FEATURES_AUTH=false` and with authentication enabled, because it depends on no user identity.

### REQ-AGENTS-SANDBOX-010: No silent approval of mutating Kandev tools

**Intent:** A human sees a mutating Kandev tool call before a sandboxed session runs it.

- **AC-AGENTS-SANDBOX-010.1:** agentctl shall treat an instance as guarded when its launch carries a sandbox (`Sandbox` set or `SandboxRequired`), a value the backend supplies, never the agent.
- **AC-AGENTS-SANDBOX-010.2:** In a guarded instance a permission request resolving to the injected Kandev server shall be approved only for tools classed `read` or `self`. Neither the blanket flag nor the generic injected-server approval approves any other tool of that server.
- **AC-AGENTS-SANDBOX-010.3:** The lookup shall parse the qualified name, require the `_kandev` suffix, strip it, and compare the whole action to table keys by exact equality. A name with no suffix, an unlisted key, or a non-terminal suffix is not approved and uses the normal prompt.
- **AC-AGENTS-SANDBOX-010.4:** A request that does not resolve to the injected Kandev server keeps its existing behavior, including the blanket flag. The adapter-level auto-approve flag shall be off for a guarded instance.
- **AC-AGENTS-SANDBOX-010.5:** Coordinator, managed-conversation, and unguarded-session approval are unchanged.
- **AC-AGENTS-SANDBOX-010.6:** The classification table is the single source for REQ-AGENTS-SANDBOX-009 and this requirement.

## Exclusions

Remote sandbox execution, Windows Nono support, universal confinement against host administrators, automatic learning, and automatic upgrades are excluded.
Out of scope for the guarded surface: the table is not applied to sessions without a task guard; own-task launch policy, REST and external MCP paths, and mixed-version agentctl keep their existing contracts.
Installation, login, repository setup, user shells, and external MCP servers remain separate execution contracts.
Nono runtime approval support outside the verified Linux capability path remains conditional on dedicated evidence.

## Implementation plans

- [Nono sandbox integration](../../../plans/nono-sandbox-integration/plan.md)
