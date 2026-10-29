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

## Exclusions

Remote sandbox execution, Windows Nono support, universal confinement against host administrators, automatic learning, and automatic upgrades are excluded.
Installation, login, repository setup, user shells, and external MCP servers remain separate execution contracts.
Nono runtime approval support outside the verified Linux capability path remains conditional on dedicated evidence.

## Implementation plans

- [Nono sandbox integration](../../../plans/nono-sandbox-integration/plan.md)
