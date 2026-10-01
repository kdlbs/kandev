---
status: draft
system: plugins
created: 2026-09-25
owners:
  - kandev
---

# Plugin managed coordination requirements

## Overview

These requirements define the proposed coordinator extension for the plugins
system. They describe the requested outcome, not currently shipped behavior.
The [implementation plan](../../../plans/plugin-coordinator-platform/plan.md)
stages delivery through public contracts and independent plugin consumers.

## Requirements

### REQ-PLUGINS-MANAGED-COORDINATION-001: Exact authority and capability discovery

**Intent:** Make optional host extensions discoverable and explicitly authorized.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-001.1:** The system shall expose supported exact operations and limits per workspace, installation, and provider, without claiming support for unavailable operations.
- **AC-PLUGINS-MANAGED-COORDINATION-001.2:** Every new exact operation shall use the current workspace approval, host-derived installation identity, resource preconditions, and durable mutation receipts; a revoked or stale authority shall not start a new side effect.
- **AC-PLUGINS-MANAGED-COORDINATION-001.3:** The workspace operator shall be able to inspect, grant, narrow, and revoke declared capabilities on desktop and phone, with the affected installation and workspace visible.

### REQ-PLUGINS-MANAGED-COORDINATION-002: Managed conversation lifetime

**Intent:** Keep coordinator conversations usable across ordinary plugin lifecycle events.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-002.1:** The system shall ensure one managed conversation for an installation, workspace, and plugin-selected instance key; profile, executor, and instruction updates shall use revision checks and an explicit idle boundary.
- **AC-PLUGINS-MANAGED-COORDINATION-002.2:** A managed conversation shall retain its identity and transcript across disable, crash, upgrade, and host restart; disable shall stop new admission and preserve pending work without executing it.
- **AC-PLUGINS-MANAGED-COORDINATION-002.3:** Uninstall shall revoke authority and detach retained transcripts from execution; reinstall shall not acquire old authority or conversations without an explicit human transfer. Legacy conversation APIs shall retain their documented lifecycle.

### REQ-PLUGINS-MANAGED-COORDINATION-003: Durable ordered conversation input

**Intent:** Accept human and automation inputs without losing them while an agent is busy.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-003.1:** A managed conversation shall accept input with a stable occurrence key, preserve FIFO order, and return the same receipt for an identical retry; a different payload with that key shall conflict.
- **AC-PLUGINS-MANAGED-COORDINATION-003.2:** The system shall distinguish accepted, running, completed, failed, cancelled, and uncertain execution outcomes; restart shall reconcile accepted work before another dispatch.
- **AC-PLUGINS-MANAGED-COORDINATION-003.3:** Queue limits, pause, cancellation, and explicit periodic-input coalescing shall be visible; coalescing shall never discard a human message, permission answer, or clarification answer.

### REQ-PLUGINS-MANAGED-COORDINATION-004: Canonical workspace observations

**Intent:** Let plugins make decisions from bounded, authoritative state.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-004.1:** Authorized plugins shall read paginated workspace catalogs, tasks, task relations, pending transitions, sessions, sanitized messages, and pending interactions with resource versions and canonical task status.
- **AC-PLUGINS-MANAGED-COORDINATION-004.2:** Observation results shall include semantic activity, blocking reasons, execution state, change-request evidence, and available usage totals with currency, units, freshness, and explicit unknown values.
- **AC-PLUGINS-MANAGED-COORDINATION-004.3:** Plugins shall be able to reconcile after lost events using authoritative reads; an event subscription shall not be represented as a durable or complete change history.

### REQ-PLUGINS-MANAGED-COORDINATION-005: Exact task coordination commands

**Intent:** Allow plugins to delegate and follow up through shared task services.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-005.1:** Authorized plugins shall create, update, move, label, relate, assign, and archive tasks and send messages or directives through exact commands that preserve workflow, ownership, and pending-transition rules.
- **AC-PLUGINS-MANAGED-COORDINATION-005.2:** A retried task creation shall not create a duplicate; an external source identity shall be checked against active and archived tasks in the same workspace before creating linked work.
- **AC-PLUGINS-MANAGED-COORDINATION-005.3:** Task deletion shall require a current human-confirmed deletion preview; a plugin shall not infer confirmation from chat text or delete unrelated descendants.

### REQ-PLUGINS-MANAGED-COORDINATION-006: Execution and interaction controls

**Intent:** Let plugins safely request execution changes and relay human decisions.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-006.1:** Authorized plugins shall ensure a task run, stop an observed run, request guarded recovery, and cancel an exact pending transition without acting on a replacement execution.
- **AC-PLUGINS-MANAGED-COORDINATION-006.2:** Permission and clarification responses shall target the exact pending request and observed revision; permission approval shall require a human response and shall grant at most once.
- **AC-PLUGINS-MANAGED-COORDINATION-006.3:** Recovery and session-mode changes shall obey supported provider capabilities and host policy; only the exact `plan` mode ID is exposed or accepted for managed sessions, and unknown mode IDs shall be rejected regardless of provider-supplied labels; unsupported operations shall return typed reasons without silently enabling broader tool access.

### REQ-PLUGINS-MANAGED-COORDINATION-007: Workspace administration

**Intent:** Allow optional configuration tools without granting arbitrary database access.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-007.1:** Authorized plugins shall update workspace defaults, create, update, and reorder workflows and steps, and register or update repositories within the approved workspace using typed versioned commands.
- **AC-PLUGINS-MANAGED-COORDINATION-007.2:** Workspace administration shall preserve synchronized workflow, active task, repository, and worktree invariants; destructive changes shall require a current human-confirmed preview.
- **AC-PLUGINS-MANAGED-COORDINATION-007.3:** Workspace administration shall not grant cross-workspace access, credential access, provider login repair, release, deployment, or merge authority.

### REQ-PLUGINS-MANAGED-COORDINATION-008: Linked issue writeback

**Intent:** Support source-aware coordination using host-owned integration credentials.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-008.1:** Authorized plugins shall read supported operations and request a comment or status transition on the task's existing linked Jira or Linear issue in the approved workspace.
- **AC-PLUGINS-MANAGED-COORDINATION-008.2:** The system shall retain writeback receipts across retry and restart and shall expose unknown provider outcomes for reconciliation without automatically repeating an uncertain comment or transition.
- **AC-PLUGINS-MANAGED-COORDINATION-008.3:** Writeback shall record actor, task, source identity, operation, and outcome without exposing integration secrets; absent credentials, stale links, unsupported operations, and rate limits shall have distinct results.

### REQ-PLUGINS-MANAGED-COORDINATION-009: Reusable native conversation UI

**Intent:** Provide shared conversation behavior while leaving the product layout to plugins.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-009.1:** The Host UI shall expose a workspace managed-conversation component with live messages, queue state, composer, pause state, pending interactions, and supported recovery actions.
- **AC-PLUGINS-MANAGED-COORDINATION-009.2:** Desktop and phone shall expose the same task context, conversation, instance selection, outcomes, and settings; phone navigation shall use full-height views and touch-sized selectors without horizontal page scrolling.
- **AC-PLUGINS-MANAGED-COORDINATION-009.3:** Loading, empty, disconnected, revoked, paused, and unsupported-provider states shall be actionable and accessible; all new host copy shall use the existing localization system.

### REQ-PLUGINS-MANAGED-COORDINATION-010: Independent coordinator instances

**Intent:** Make the reference implementation a separately released plugin.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-010.1:** The reference plugin shall define reusable roles and multiple named workspace instances with their own profile, executor, instructions, conversation, task scope, pause state, and namespaced memory.
- **AC-PLUGINS-MANAGED-COORDINATION-010.2:** The reference plugin shall create and follow up delegated tasks through public Host APIs and plugin-owned agent tools, while retaining task links and showing authoritative task and run status.
- **AC-PLUGINS-MANAGED-COORDINATION-010.3:** The reference plugin shall install from a packaged dedicated repository and shall not require coordinator-specific host tables, principals, navigation entries, or private APIs.

### REQ-PLUGINS-MANAGED-COORDINATION-011: Durable coordinator policy

**Intent:** Let plugin authors choose how coordination decisions work.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-011.1:** The reference plugin shall persist proposals, approvals, event reconciliation, memory, and an intent outbox; approving the same proposal twice shall create at most one task.
- **AC-PLUGINS-MANAGED-COORDINATION-011.2:** The reference plugin shall apply configurable event filters, recurring routines, concurrency limits, and usage budgets, preserving durable cursors and suppressing new policy actions while paused.
- **AC-PLUGINS-MANAGED-COORDINATION-011.3:** The reference plugin shall support explicit task adoption, completion-evidence submission, linked-issue writeback, and outcome reports; evidence and estimated costs shall be distinguishable from verified results and measured costs.

### REQ-PLUGINS-MANAGED-COORDINATION-012: Independent consumer compatibility

**Intent:** Prove the surface supports different coordinator policies.

#### Acceptance criteria

- **AC-PLUGINS-MANAGED-COORDINATION-012.1:** A second independently packaged plugin identity shall implement a proposal-first observer policy with different prompts and tools, using the same public contracts without changes to host product logic.
- **AC-PLUGINS-MANAGED-COORDINATION-012.2:** Two installed consumers shall retain separate memory, conversations, approvals, and task claims; unsupported capabilities shall disable only the dependent features with an explanatory state.
- **AC-PLUGINS-MANAGED-COORDINATION-012.3:** Published SDK types, manifest validation, protocol contracts, authoring documentation, and packaged compatibility tests shall agree on the shipped minimum host version and capability requirements.

## Related documents

- [System design](../system-design/managed-coordination.md)
- [Delivery plan](../../../plans/plugin-coordinator-platform/plan.md)
