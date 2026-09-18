---
status: draft
system: orchestration
created: 2026-09-17
owners:
  - Kandev
---

# Orchestrator central-assistant capabilities

## Overview

Complete the central Orchestrator capabilities without confusing workspace task
observation, agent-turn completion and fulfilled user outcomes. This document
replaces the legacy personal-assistant specification. Work-order status, not
this document, records what is built. Existing owner/intake/objective behavior
must survive the remaining implementation.

The Orchestrator is an owner-selected existing coordinator conversation. An
objective records the current intent and acceptance conditions; an attention
record points to a canonical actionable source. A credential descriptor contains
resolver/reference/scope knowledge, never a secret. A workspace grant is explicit
permission to observe/coordinate/export context within the owner's existing access.

## Requirements

### REQ-ORCHESTRATION-ASSISTANT-001: Durable ownership and intent

**Intent:** Keep accepted requests, their authority and completion evidence intact.

#### Acceptance criteria

- **AC-ORCHESTRATION-ASSISTANT-001.1:** Selecting, switching or deleting a default
  binding shall not transfer or expose old conversation history, memory or
  configuration. The retained owner may read and reselect it; an inactive private
  conversation shall reject new turns, stale credentials and foreign claims.
- **AC-ORCHESTRATION-ASSISTANT-001.2:** Retrying an accepted message with the same
  client identity/content shall return one durable comment/run receipt. Reusing
  the identity for different content shall conflict; an accepted message shall
  survive queue unavailability, browser disconnect and process restart.
- **AC-ORCHESTRATION-ASSISTANT-001.3:** Follow-ups shall preserve exact source
  identity and advance the objective's intent revision. Superseded intent shall
  not authorize later mutations, retries or completion; already-started effects
  shall remain honestly reported rather than promised undone.
- **AC-ORCHESTRATION-ASSISTANT-001.4:** Questions/inspection shall not require
  delivery cards. Execution/design shall use validated existing workflows and
  profiles. A finished turn, merged change or REVIEW state shall not complete an
  objective without evidence against its current acceptance revision.

### REQ-ORCHESTRATION-ASSISTANT-002: Scoped context and credential knowledge

**Intent:** Let relevant confirmed context follow work without exporting secrets.

#### Acceptance criteria

- **AC-ORCHESTRATION-ASSISTANT-002.1:** Owners shall list, inspect, correct and
  forget scoped memories with provenance, confirmation, revision and expiry.
  Pagination shall not expose another owner's or scope's entries; invalid scope
  references shall fail without modifying memory.
- **AC-ORCHESTRATION-ASSISTANT-002.2:** Relevant confirmed preferences and required
  objective/account constraints shall survive newer activity within a bounded
  context packet. Required overflow shall be explicit, never silently truncated.
- **AC-ORCHESTRATION-ASSISTANT-002.3:** Two eligible workers shall obtain a scoped
  credential descriptor and current validation/availability metadata without
  values, vault inventories or authentication tokens. Locked/unavailable resolvers
  shall identify a specific unblock action and shall not substitute accounts.
- **AC-ORCHESTRATION-ASSISTANT-002.4:** Corrected, forgotten, expired or narrowed
  context shall invalidate future and queued dispatches, including restart/resume,
  steering and explicit send-now paths. Previously delivered text is not recalled.

### REQ-ORCHESTRATION-ASSISTANT-003: Live capability directory

**Intent:** Make usable native/integration/plugin/MCP capabilities discoverable.

#### Acceptance criteria

- **AC-ORCHESTRATION-ASSISTANT-003.1:** Scoped paginated entries shall identify
  origin, effect, schema, applicability, account/workspace scope, health and
  availability reason with generation/revision evidence.
- **AC-ORCHESTRATION-ASSISTANT-003.2:** Discovery shall disclose no secret
  configuration or inaccessible resource; a directory entry shall not constitute
  authorization or a guarantee that an attachment is healthy.
- **AC-ORCHESTRATION-ASSISTANT-003.3:** Conversation applicability shall be explicit.
  Existing task-only plugin declarations shall not gain conversation access;
  plugin tools shall retain their individual typed native dispatch identities.
- **AC-ORCHESTRATION-ASSISTANT-003.4:** Disabling/disconnecting/deauthorizing a
  capability shall block its next invocation despite a stale directory response.

### REQ-ORCHESTRATION-ASSISTANT-004: Enforced inspection authority

**Intent:** Make inspect mode an execution guarantee instead of prompt advice.

#### Acceptance criteria

- **AC-ORCHESTRATION-ASSISTANT-004.1:** Inspect shall permit only reads whose effect
  restrictions are enforced across native, MCP, plugin and provider-native tools.
  Unknown effects and unsupported provider/executor restriction paths shall be
  rejected before launch/invocation with an actionable limitation.
- **AC-ORCHESTRATION-ASSISTANT-004.2:** Every invocation shall recheck current
  intent, owner access, grant, profile, mode and native gates. Tool output,
  retrieved text, memory and metadata hints shall not enlarge authority.
- **AC-ORCHESTRATION-ASSISTANT-004.3:** Inspect may write internal conversation
  receipts/audit bookkeeping but shall not mutate delivery tasks, repositories,
  external services, plugin settings or credentials.
- **AC-ORCHESTRATION-ASSISTANT-004.4:** Mutations outside inspect shall use stable
  operation identities; lost acknowledgements shall remain unknown until native
  evidence resolves them. Uncertain external effects shall not be blindly retried.

### REQ-ORCHESTRATION-ASSISTANT-005: Actionable attention and recovery

**Intent:** Notice pending work without requiring a board move or a wake loop.

#### Acceptance criteria

- **AC-ORCHESTRATION-ASSISTANT-005.1:** Questions, permissions, authentication
  failures, errors, required review and results across every relevant session of
  managed tasks shall produce correctly identified attention records. Idle work
  with no request shall not produce a question.
- **AC-ORCHESTRATION-ASSISTANT-005.2:** Duplicate/out-of-order events and restarts
  shall converge on one current record per source/revision, with no duplicate
  notifications or replayed external actions.
- **AC-ORCHESTRATION-ASSISTANT-005.3:** In a healthy local instance, actionable
  source events shall surface within five seconds and missed managed-work events
  shall reconcile within 60 seconds. Model response latency is separate. An
  unchanged reconciliation shall spend no agent turn.
- **AC-ORCHESTRATION-ASSISTANT-005.4:** Paused/disabled assistants shall retain
  pending state without launching. Budget/rate limits shall give one useful
  status and shall not silently change accounts or create a repeated wake loop.

### REQ-ORCHESTRATION-ASSISTANT-006: Native input resolution

**Intent:** Resolve the actual pending request and keep native task UI consistent.

#### Acceptance criteria

- **AC-ORCHESTRATION-ASSISTANT-006.1:** A human central action shall resolve only
  its current task/session/request/revision using native options and authority.
  Native task and assistant UI shall converge on that same source result.
- **AC-ORCHESTRATION-ASSISTANT-006.2:** Automatic answers shall be limited to
  explicitly delegable questions supported by confirmed scoped context, with
  provenance and one durable operation. Runtime shall not impersonate a human
  or approve permissions.
- **AC-ORCHESTRATION-ASSISTANT-006.3:** Duplicate/stale/expired responses shall
  return an accurate receipt or conflict without approving another request or
  starting duplicate turns. Restart shall not resurrect expired provider handles.
- **AC-ORCHESTRATION-ASSISTANT-006.4:** Pause and stop-managed-work shall remain
  separate controls. Stopping shall identify the affected workers and report each
  result; pausing the assistant shall not silently stop or restart workers.

### REQ-ORCHESTRATION-ASSISTANT-007: Personal assistant interface

**Intent:** Expose the durable assistant outcomes through one understandable UI.

#### Acceptance criteria

- **AC-ORCHESTRATION-ASSISTANT-007.1:** App-level Assistant navigation shall
  resume the selected existing conversation with its identity on desktop/mobile,
  without creating a new workspace, workflow or delivery task on visits.
- **AC-ORCHESTRATION-ASSISTANT-007.2:** Users shall inspect objectives/acceptance
  evidence, activity/delivery state, pending decisions, capabilities and memory
  provenance/correction/forget controls without visiting every worker task.
- **AC-ORCHESTRATION-ASSISTANT-007.3:** Send/reconnect shall preserve message
  identities, delivery receipts and history cursors. Workspace/owner changes
  shall discard stale content; transport acceptance shall not display as completion.
- **AC-ORCHESTRATION-ASSISTANT-007.4:** Mobile shall expose the same essential
  actions at 390 pixels with keyboard/assistive labels, stable focus and no clipped
  controls. Loading, empty, unavailable, paused and partial states shall be distinct.

### REQ-ORCHESTRATION-ASSISTANT-008: Supervised workflow improvements

**Intent:** Make repeatable friction actionable without weakening safeguards.

#### Acceptance criteria

- **AC-ORCHESTRATION-ASSISTANT-008.1:** Redacted incidents shall distinguish
  denial, pending approval, missing capability, expired authentication, transport
  failure and suspected classifier error. Three matching incidents across at
  least two tasks in seven days shall create one scoped candidate; frequency
  shall not prove the safeguard wrong.
- **AC-ORCHESTRATION-ASSISTANT-008.2:** Without a workflow-maintenance grant,
  candidates shall remain proposals. With a grant, one isolated normal task may
  investigate, propose a narrow fix, run positive/negative checks and record a
  local commit with evidence.
- **AC-ORCHESTRATION-ASSISTANT-008.3:** The loop shall not approve its own requests,
  expand broad allowlists, suppress denials, change provider-global policy or
  publish/restart/deploy without separate authority. Unknown classifier internals
  shall remain unknown.

### REQ-ORCHESTRATION-ASSISTANT-009: Explicit linked-workspace scope

**Intent:** Support multiple workspaces without flattening access/account boundaries.

#### Acceptance criteria

- **AC-ORCHESTRATION-ASSISTANT-009.1:** Initial scope shall be the home workspace.
  An authorized owner shall explicitly link additional workspaces with observation,
  coordination and context-export scope and the receiving execution profile.
  Installation scanning shall not automatically grant or adopt workspaces.
- **AC-ORCHESTRATION-ASSISTANT-009.2:** Every read/wake/write shall resolve one
  target and recheck owner access plus current grant/intent/profile revisions.
  Revocation or incompatible profile changes shall block subsequent operations.
- **AC-ORCHESTRATION-ASSISTANT-009.3:** Existing workspace-scoped coordinator
  credentials shall remain unable to access foreign workspaces. Cross-workspace
  summaries shall disclose which profile receives the selected context.
- **AC-ORCHESTRATION-ASSISTANT-009.4:** Revocation shall not claim to erase text
  already delivered to a provider. Stored-copy forgetting shall be explicit, and
  another workspace's unrelated transcripts shall not be exported as context.

### REQ-ORCHESTRATION-ASSISTANT-010: Unified Orchestrator rollout

**Intent:** Keep every Orchestrator capability behind one understandable rollout gate.

#### Acceptance criteria

- **AC-ORCHESTRATION-ASSISTANT-010.1:** All Orchestrator execution, including
  owner-level objectives, context, attention, capability and native-input
  features, shall require `features.orchestration`. There is no separate
  Personal assistant product toggle.
- **AC-ORCHESTRATION-ASSISTANT-010.2:** With Orchestration off, all Orchestrator
  selection, mutation, runtime tools and background dispatch shall be unavailable
  through direct APIs as well as hidden in UI. Queued assistant work shall remain
  blocked; existing privacy protections shall remain active.
- **AC-ORCHESTRATION-ASSISTANT-010.3:** Disabling Orchestration shall preserve
  stored ownership/schema/history. An owned Orchestrator conversation shall never
  be treated as an unowned legacy conversation to bypass the disabled path.
- **AC-ORCHESTRATION-ASSISTANT-010.4:** Operators shall see the effective
  Orchestrator flag source and restart requirements through existing feature
  settings. Enabling it shall not imply that an unsupported inspect profile is
  safe.

## Exclusions and legacy mapping

No wholesale external-agent framework import, automatic account adoption,
secret storage in prompts, implicit scheduling, replacement task engine or
provider-policy bypass. Mock tests do not establish real-provider judgment quality.

Legacy scenarios map as follows: S01→007; S02–S04/S13→001/004;
S05–S06/S22→001; S07–S08/S20→002; S09–S10→005; S11–S12→006;
S14–S15→003/004; S16–S17→008; S18→009; S19→005/010; S21→the
combined validation work order. The mapping preserves meaning, not old delivery
claims. Product acceptance remains the explicit criteria above.
