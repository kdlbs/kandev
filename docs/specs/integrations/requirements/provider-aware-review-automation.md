---
status: active
system: integrations
created: 2026-08-03
updated: 2026-08-03
owners:
  - kandev
---
# Provider-Aware Review Automation Runtime Requirements

## Overview

Task agents currently discover both GitHub pull-request automation tools and GitLab merge-request automation tools even when none of the task's repositories use one of those providers. The extra tools consume context, invite invalid calls, and make the MCP surface describe capabilities the task cannot use.

## Requirements

### REQ-INTEGRATIONS-PROVIDER-AWARE-REVIEW-AUTOMATION-001: Provider-Aware Review Automation Runtime

**Intent:** Task agents currently discover both GitHub pull-request automation tools and GitLab merge-request automation tools even when none of the task's repositories use one of those providers. The extra tools consume context, invite invalid calls, and make the MCP surface describe capabilities the task cannot use.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROVIDER-AWARE-REVIEW-AUTOMATION-001.1:** The backend derives a normalized set of review-automation providers from all repositories attached to the task. The primary repository has no special precedence.
- **AC-INTEGRATIONS-PROVIDER-AWARE-REVIEW-AUTOMATION-001.2:** A GitHub-only task exposes `get_task_pr_automation_kandev` and `update_task_pr_automation_kandev`, but not the corresponding MR tools.
- **AC-INTEGRATIONS-PROVIDER-AWARE-REVIEW-AUTOMATION-001.3:** A GitLab-only task exposes `get_task_mr_automation_kandev` and `update_task_mr_automation_kandev`, but not the corresponding PR tools.
- **AC-INTEGRATIONS-PROVIDER-AWARE-REVIEW-AUTOMATION-001.4:** A mixed GitHub/GitLab task exposes both pairs.
- **AC-INTEGRATIONS-PROVIDER-AWARE-REVIEW-AUTOMATION-001.5:** A task with only local, empty, unknown, or currently unsupported providers exposes neither pair. Provider names are normalized and matched against an explicit allowlist; unknown values fail closed.
- **AC-INTEGRATIONS-PROVIDER-AWARE-REVIEW-AUTOMATION-001.6:** All provider-neutral task tools remain available according to the existing MCP mode.
- **AC-INTEGRATIONS-PROVIDER-AWARE-REVIEW-AUTOMATION-001.7:** Tool visibility is capability discovery, not authorization. Backend handlers remain registered and enforce task, workspace, provider, and credential rules for direct or stale calls.
- **AC-INTEGRATIONS-PROVIDER-AWARE-REVIEW-AUTOMATION-001.8:** Initial launch and resume carry the normalized provider set from the orchestrator through lifecycle into agentctl. Agentctl enforces the supplied set and does not infer provider identity from filesystem remotes.

## Migrated source detail

## Why

Task agents currently discover both GitHub pull-request automation tools and
GitLab merge-request automation tools even when none of the task's repositories
use one of those providers. The extra tools consume context, invite invalid
calls, and make the MCP surface describe capabilities the task cannot use.

The GitLab merge-request lifecycle path also repeats provider and database work
after every subscribed poll: it can fetch the same merge request twice, resolve
the authenticated user again, and load every lifecycle checkpoint for the task
when evaluation needs only one. Detached follow-up writes additionally discard
their parent timeout, so a stalled store or event bus can retain the automation
singleflight indefinitely.

Provider-specific discovery and lifecycle evaluation need an explicit, bounded
runtime contract without changing the public automation payloads or merging the
different GitHub and GitLab feature sets.

## What

### Provider-scoped MCP discovery

- The backend derives a normalized set of review-automation providers from all
  repositories attached to the task. The primary repository has no special
  precedence.
- A GitHub-only task exposes `get_task_pr_automation_kandev` and
  `update_task_pr_automation_kandev`, but not the corresponding MR tools.
- A GitLab-only task exposes `get_task_mr_automation_kandev` and
  `update_task_mr_automation_kandev`, but not the corresponding PR tools.
- A mixed GitHub/GitLab task exposes both pairs.
- A task with only local, empty, unknown, or currently unsupported providers
  exposes neither pair. Provider names are normalized and matched against an
  explicit allowlist; unknown values fail closed.
- All provider-neutral task tools remain available according to the existing MCP
  mode.
- Tool visibility is capability discovery, not authorization. Backend handlers
  remain registered and enforce task, workspace, provider, and credential rules
  for direct or stale calls.

### Runtime propagation and refresh

- Initial launch and resume carry the normalized provider set from the
  orchestrator through lifecycle into agentctl. Agentctl enforces the supplied
  set and does not infer provider identity from filesystem remotes.
- The internal agentctl instance-create/config contract accepts
  `mcp_providers` as a normalized array. A live
  `PUT /api/v1/mcp/providers` operation replaces the current set.
- Replacing the set rebuilds the tool registry while preserving the active MCP
  mode. When the effective list changes, the MCP server emits the protocol's
  tool-list-changed notification.
- After a repository source or legacy branch is attached successfully, the
  backend recomputes the authoritative union once for the completed batch and
  refreshes active sessions for the task. This best-effort live refresh is
  cancellation-independent but bounded by a short total deadline, so a stalled
  database or agentctl dependency cannot delay the attachment event
  indefinitely.
- A live refresh failure does not roll back a successfully attached source. The
  agent keeps its previous set, which is a fail-closed subset for addition-only
  mutations; an actionable warning is logged, and the next launch or resume
  restores the authoritative set.

### Lean GitLab lifecycle evaluation

- A normal subscribed poll performs one merge-request status fetch. The reviewer
  observation from that response is carried with the internal MR-updated event
  and reused during review-request evaluation.
- The event distinguishes an observed empty reviewer list from a missing
  observation. Events that do not contain a reviewer observation retain a
  strict provider-read fallback.
- Reviewer observations are ephemeral evaluation input. They survive internal
  event serialization but are not added to the public or persistent `TaskMR`
  model.
- The evaluator reads only the task's automation options and the exact lifecycle
  checkpoint for the target repository/project/IID. It does not load every MR
  lifecycle state for the task.
- Reviewer rebinding uses the authenticated username already persisted in the
  workspace GitLab configuration. Configuration save and health flows remain
  responsible for refreshing that identity. Missing identity fails closed; the
  evaluator does not call the authenticated-user endpoint for every MR.
- Existing public HTTP and MCP automation request and response shapes remain
  unchanged.

### Bounded automation follow-up work

- Dispatch keeps its existing detached automation deadline and singleflight
  behavior.
- Checkpoint persistence, error recording, response loading, and event
  publication performed after a side effect use fresh, short timeouts derived
  from a cancellation-independent context.
- Those follow-up operations may outlive cancellation of the initiating poll,
  but they cannot run without a deadline or hold the singleflight indefinitely.
- Existing dispatch-before-checkpoint ordering and at-least-once behavior remain
  unchanged.

### Handler inventory observability

- MCP WebSocket handler registration reports the actual change in dispatcher
  handler count rather than maintaining manual arithmetic alongside registration
  calls.
- The dispatcher exposes a concurrency-safe count operation for this diagnostic
  purpose. Handler registration behavior and public protocol remain unchanged.

## API surface

The new surface is internal to the managed runtime:

| Surface | Contract |
|---|---|
| Agentctl instance create/config | Optional `mcp_providers: string[]`; normalized supported provider identifiers only |
| Agentctl live update | `PUT /api/v1/mcp/providers` replaces the provider set for the running instance |
| MCP server | `SetProviders` replaces the effective provider set and rebuilds tools without changing mode |
| Launch/lifecycle requests | Carry the derived provider union explicitly through every executor backend |
| Internal GitLab MR update event | May carry a reviewer observation plus an explicit validity marker |
| Internal GitLab evaluation query | Returns task options and the exact target lifecycle checkpoint only |

No public GitHub/GitLab automation endpoint, MCP tool argument, or automation
response payload changes.

## Permissions

- Provider derivation uses repositories already authorized and attached to the
  task; it does not grant access to another repository or workspace.
- Agentctl treats `mcp_providers` only as a discovery allowlist. It cannot bypass
  the backend's authorization, provider ownership, or credential checks.
- Live refresh targets only executions belonging to the affected task/session.
- Persisted GitLab usernames remain workspace-scoped integration data and are
  resolved through the existing task-to-workspace ownership path.

## Failure modes

- Missing, malformed, or unsupported provider values expose no provider-specific
  review automation tools.
- If an initial agentctl launch cannot accept its provider set, launch fails
  through the existing instance-creation error path rather than exposing all
  tools.
- If a live provider refresh fails after source attachment, attachment remains
  committed, the old tool set remains active, and the failure is logged. The next
  launch or resume recomputes the correct set.
- A GitLab event without a valid reviewer observation performs the existing
  strict MR read; a valid empty observation performs no fallback read.
- A missing persisted GitLab username prevents reviewer-targeted dispatch and is
  reported through existing automation error handling; it does not trigger a
  per-MR authenticated-user lookup.
- Timed-out checkpoint, error, response, or publish follow-up work is logged and
  releases the automation singleflight. Existing retry/re-evaluation semantics
  apply.
- A handler count mismatch cannot affect registration; the reported value is
  derived after registration completes.

## Persistence guarantees

- Repository provider identity remains owned by the existing repository records;
  no duplicate provider configuration is persisted for agentctl.
- Every launch and resume derives the provider set from current durable task
  repositories. A live agentctl value is replaceable runtime state.
- Reviewer observations remain transient event data. Existing `TaskMR` and
  lifecycle checkpoint persistence schemas do not gain reviewer arrays.
- GitLab lifecycle checkpoints retain their current keys and at-least-once
  dispatch semantics. The lean query changes read scope, not checkpoint meaning.
- Existing event decoding remains compatible with events that predate the
  reviewer observation fields.

## Scenarios

### GitHub-only task

**Given** every attached task repository is GitHub, **when** the task agent lists
tools, **then** it sees the PR automation pair and does not see the MR automation
pair.

### GitLab-only and mixed tasks

**Given** a GitLab-only task, **when** the task agent lists tools, **then** it sees
only the MR pair. **Given** at least one GitHub and one GitLab repository, **then**
it sees both pairs regardless of repository ordering.

### Unsupported providers fail closed

**Given** a task containing only local, blank, Azure DevOps, or unknown provider
values, **when** the task agent lists tools, **then** it sees neither provider-
specific pair while provider-neutral tools remain available.

### Live source addition

**Given** a running GitHub-only task, **when** a GitLab repository source is
successfully attached, **then** active agentctl instances receive the new union,
the MCP tool list changes to include MR automation, and clients receive a tool-
list-changed notification without a restart.

### Failed live refresh

**Given** a successful repository attachment and an unavailable agentctl refresh
endpoint, **when** refresh is attempted, **then** the repository remains attached,
the agent retains its previous subset, and a later launch or resume exposes the
authoritative union.

### Defense in depth

**Given** a stale or direct call to a hidden provider-specific handler, **when**
the backend receives it, **then** the existing task/provider/access checks still
accept or reject the operation independently of discovery.

### Subscribed GitLab poll

**Given** a poll response containing an observed reviewer list, **when** lifecycle
automation evaluates the update, **then** it reuses that observation, reads only
the relevant options/checkpoint, uses the persisted integration username, and
does not fetch the MR or authenticated user again.

### Event without reviewer observation

**Given** an options-change, legacy, or manually published MR event without a
reviewer observation, **when** review-request automation evaluates it, **then** it
performs the strict MR fallback read. A serialized valid empty observation
survives round-trip and does not trigger that fallback.

### Stalled follow-up dependency

**Given** a checkpoint store or event bus that blocks after dispatch, **when** the
follow-up deadline expires, **then** evaluation returns, logs the timeout, and
releases the per-MR singleflight for later work.

## Out of scope

- Unifying GitHub PR and GitLab MR automation into generic MCP tools.
- Adding GitLab auto-fix or auto-merge behavior.
- Exactly-once notification dispatch or broader automation idempotency changes.
- UI menu, accessibility, zero-MR access, or shared-component refactors.
- Concurrent GitLab poller fan-out.
- Live provider refresh after repository removal.
- Persisting reviewer snapshots or changing public automation data models.
