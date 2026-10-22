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

## Exclusions

Remote sandbox execution, Windows Nono support, universal confinement against host administrators, automatic learning, and automatic upgrades are excluded.
Installation, login, repository setup, user shells, and external MCP servers remain separate execution contracts.
Nono runtime approval support outside the verified Linux capability path remains conditional on dedicated evidence.

## Implementation plans

- [Nono sandbox integration](../../../plans/nono-sandbox-integration/plan.md)
