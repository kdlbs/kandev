---
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-001
  - REQ-ORCHESTRATION-ASSISTANT-002
  - REQ-ORCHESTRATION-ASSISTANT-003
  - REQ-ORCHESTRATION-ASSISTANT-004
  - REQ-ORCHESTRATION-ASSISTANT-005
  - REQ-ORCHESTRATION-ASSISTANT-006
  - REQ-ORCHESTRATION-ASSISTANT-007
  - REQ-ORCHESTRATION-ASSISTANT-008
  - REQ-ORCHESTRATION-ASSISTANT-009
  - REQ-ORCHESTRATION-ASSISTANT-010
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
legacy_specs: []
created: 2026-09-16
status: in_progress
---

# Implementation plan: Personal assistant

## Overview

Extend the existing independent Orchestration runtime, not Office. First make intent and delivery durable; then add proportional routing, shared context and enforceable tools; then reconcile/resolve blockers and expose the central assistant. Add supervised workflow improvement and explicitly linked workspaces only after the single-workspace boundaries are tested.

Implementation was explicitly requested after the design-package handoff. Tasks 01 through 06 are verified. The subsequent binding-switch privacy regression was repaired after the user's explicit follow-up; the [ownership checkpoint](ownership-checkpoint.md) records the retained-owner boundary and passing checks. Later tasks remain pending. The [experiment report](experiments.md) records the pre-implementation baseline.

## Continuation context

The historical implementation started before the release rebase. The current private review branch is based on exact v0.94.0 (`bf819a0228e742d069c528293d848c985a4d1bd1`) with the preserved prototype imported above it. Use the [delivery plan](../orchestration-delivery/plan.md) for current repository, candidate and rollout status. Do not use old local baseline SHAs as new integration targets. Re-read current diffs before editing overlapping files.

Delivery task 01 adds an independent assistant flag before coordinator-only dogfooding. Preserve retained-owner/context guards even with that flag off. Continue tasks 07–11 in dependency order; these work orders now include canonical requirement/criterion references and detailed remaining checks. Tasks 01/02 below are historical completed work, not work to redo.

The existing [workspace-orchestrators plan](../workspace-orchestrators/plan.md) covers the already-built foundation. This spec extends its user-facing contract without changing old completion statuses. Runtime ownership stays in internal/orchestration; backendapp supplies adapters to core services.

## Backend

### 1. Ownership, exact intent and durable operations

Likely files: internal/orchestration/models/{conversation,assistant,operation}.go; repository/sqlite/{comments,assistant_schema,intake,operations}.go; runtime/{handler,service,recovery}.go; internal/runs/service integration.

Add orchestration_assistant_bindings (text primary ID, unique owner_user_id, orchestrator/conversation/home-workspace foreign references, integer revision); use authn.Identity and existing synthetic local identity. No grant may be inferred from a role name or task metadata.

Retain owner_user_id independently on orchestration_conversations, with additive migration/backfill and atomic claim alongside binding CAS. Default selection must not publish old history. Enforce retained ownership through native task/session access, configuration/reimport, runtime binding snapshots and legacy Office compatibility. Private automations require durable owner authorization and are currently rejected. See ADR-2026-09-16-private-conversation-ownership and scenario S22.

Add orchestration_intake (unique conversation/client_message_id, payload hash, exact comment ID, monotonic conversation sequence, intent revision, status, nullable run ID) and orchestration_operations (unique binding/operation_id, target/action digest, grant/intent/context revisions, state and safe receipt IDs). Foreign deletes remove private derived rows without deleting workers. Migrations must work without Office tables.

Use one transaction for comment plus intake/outbox acceptance. The existing queue consumes a stable idempotency key; a post-commit dispatcher repairs accepted-but-not-enqueued rows. Do not assume two repository calls using the same connection are atomic. Preserve current comment JSON fields and add receipt fields; run_id can be absent while durable intake is awaiting dispatch.

Add GetCommentByID with conversation/owner scope and cursor-based history. Remove the latest-100 scan from prompt assembly. Preserve queued source identity and cancellation/supersession; derive latest authority before side effects, never just trust a stale prompt. Unknown external delivery must reconcile against a native receipt or stay unknown.

### 2. Goals and proportional execution

Add orchestration_objectives and orchestration_objective_tasks, with bounded acceptance/evidence JSON, source comment, revision and workspace/profile policy. Separate mode from task state. Direct answer/inspect has no delivery card; execute/design creates or reuses canonical tasks only as needed.

Extend models.WorkspaceTaskSpec, runtime.Handler.createTask, backendapp.taskCreatorAdapter.CreateWorkspaceTask and cmd/agentctl/kandev.go (runOrchestrationCLI), kandev_task.go and kandev_orchestration_test.go. Keep the Orchestration-facing adapter interface Office-free.

Propagate workflow_step_id, execution_mode, context reference, operation ID and expected intent revision. Validate explicit step membership and configured entry policy; never select a step by English display name. Do not mutate existing workflow defaults. Existing requirements and repository-policy handoffs remain authoritative. Preserve profile/account assignment on adopt/start/message and use session-role links for implementation/review.

Persist completion evidence against acceptance revision. Check live background/subagent activity and required review gates; REVIEW, WAITING_FOR_INPUT or AgentCompleted alone cannot mark an objective complete.

### 3. Memory and credential descriptors

Extend existing orchestration_memory additively (owner/scope/source/revision/confirmed/priority/expiry/forgotten). Legacy entries remain workspace-scoped and unconfirmed. New context assembly selects confirmed relevant rules before recency; use a bounded budget with deterministic ordering and overflow references, not eight newest entries.

Add context packet records/DTOs and a credential-descriptor table. Initial packet limit: 12 KiB UTF-8, reserving room for current objective/account constraints; individual memory excerpts <=1 KiB. If indispensable constraints cannot fit, refuse delegation rather than silently omit them. Full scoped context remains retrievable by stable references; never include secret values.

Update runtime/memory.go and backendapp task creation/message adapters to attach versioned context. Add owner APIs for edit/forget and redaction tests using synthetic tokens. Recheck context revisions at queued dispatch; forgetting cannot promise removal from a model's already-received context. Resolver attachment and unlock use existing secret/credential integration paths, not new vault scanning.

### 4. Capability inventory and restricted invocation

Inventory adapters compose existing WorkspaceCatalog, plugin AgentTools, integration health and session MCP attachment evidence. Use safe DTOs, pagination (default 50, max 100) and generation/revision fields. Do not serialize arbitrary config/env/session metadata. Health distinguishes configured, attached, healthy, locked, missing and unavailable.

Extend backend-owned MCP conversation profile and explicit plugin-tool surface declarations. Reuse internal/plugins/manifest and registry adapters, internal/mcp/plugintools, profile and server registries; preserve the accepted per-operation MCP design. Native configured tools retain per-call resource authorization. General plugin action HTTP auth is not an assistant grant; do not spoof a user session to call it.

A backend effect-policy evaluator intersects grants with run mode. Native write handlers reject inspect runs even with a valid run token. Plugin read-only hints are insufficient: offer only trusted operations with enforced scoped host capabilities, or withhold them. Unknown external MCP/native-provider effects are blocked in inspect mode. Resolve safe execution support before starting the agent; unavailable restriction support is a visible incompatibility, not a prompt-only guarantee.

Read:
- [MCP profile ADR](../../decisions/2026-08-08-mcp-tool-profiles.md).
- [Plugin tool ADR](../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md).
- [Workspace orchestration ADR](../../decisions/2026-09-07-workspace-orchestration.md).
- integrations/AGENTS.md before touching any integration adapter.

### 5. Attention projection and wakes

Add orchestration_attention and an Office-independent AttentionReader interface. backendapp adapts task/statussummary, all relevant task sessions, canonical pending question/permission records and active errors. Task summary revisions are efficient dirty signals; they are not a substitute for request identities.

Expand runtime/events.go subscriptions to session-state and permission wildcard events plus relevant question/message resolution, worker turn and integration changes. Core event producers must include bounded source identities where missing. Reconcile after notifications and on startup; scan only managed task links every 60 seconds with bounded batches. Event handling should produce local cards within five seconds, independent of LLM latency.

Unique source/revision keys, last-notified revision and transactional outbox enqueue provide idempotency. Coalesce noise per task while preserving different questions and sessions. No-op scans use no model tokens. Paused/disabled assistants retain state and do not dispatch.

### 6. Native answer/permission resolution

Add a resolution adapter to existing clarification handlers/services and executor permission response service. Do not paste a user's answer as a generic prompt while a native request is still blocked. Pending in-memory requests that cannot survive restart become expired; a durable attention row does not recreate a provider permission grant.

Human endpoint verifies owner/workspace, task/session/request, actual options and expected revision. Runtime endpoint accepts only explicitly delegable questions and source references. Permission/authentication remain human-only. The same native record feeds both UIs. Double submission returns the prior receipt or conflict, never a second response.

If dispatch times out, persist unknown and reconcile; do not mark answered merely because a message was queued. Clarification from known context is attributed to the assistant, not falsely recorded as the human clicking approval.

### 7. Supervised improvement loop

Add redacted friction observations and candidate aggregation under internal/orchestration (new friction.go and repository helpers). Fingerprint uses workspace/account, operation family, normalized reason, origin and version; it excludes secret arguments and raw shell transcripts. Three occurrences, two distinct tasks, seven-day window, one open candidate/fingerprint.

The assistant may investigate via reads. A human-maintained grant permits an isolated normal task to produce a proposed repair, tests and local commit. Enforce changed-file/policy boundaries; changes to broad allowlists, model-provider policy and self-approval are not authorized by that grant. No automatic push/PR/deploy/restart. Collect success as resolved subsequent incidents, not merely fewer permission prompts.

### 8. Explicit additional workspace links

Add orchestration_workspace_grants with owner/binding/workspace uniqueness, operation/context-export allowlists and revisions. Require current user visibility on every grant and operation. Revocation invalidates runtime dispatch even if discovery caches or a model prompt are stale.

Use a dedicated assistant broker credential instead of widening the old workspace_coordinator scope. The broker resolves one target and calls authorized canonical services; workers keep workspace/profile-scoped credentials. Existing foreign-workspace denial tests must continue to pass. Binding/profile changes require re-confirming incompatible context-export grants.

## Frontend

### Assistant route and shell

Add /assistant to apps/web/src/spa-routes.tsx with apps/web/app/assistant/assistant-page.tsx. Reuse OrchestratorConversationPane/TaskChat and its transports, not Office page components. Selecting a default uses the human binding API; deep links to existing conversations still work.

Expose the same entry from desktop primary navigation and mobile app navigation. Goals, attention, activity and memory are progressively disclosed panels. Default chat messages stay concise; working/delivered/waiting/review/interrupted/unknown states must be distinct. Pause and stop-managed-work are separate labeled actions.

### API/client/state

Extend lib/api/domains/orchestration-conversation-api.ts with stable client-message IDs retained across retries. Add assistant-api.ts and use-assistant.ts for cursor/revision reconciliation. Follow core websocket/session synchronization patterns; if adding an event, use orchestration.assistant.updated with only binding_id and revision and re-fetch authorized DTOs. Reconnect must close gaps.

Extract/reuse native permission and clarification presenters with an injected resolution transport; do not embed an unrestricted task API client. Memory editor displays provenance, scopes and forget limitations. Capabilities show unavailable reasons and settings deep links, not raw configuration. All strings belong to orchestration locale files; preserve translated/pseudo locale parity.

## Tests

Every new non-trivial function gets focused unit coverage; repository and handler behavior uses real SQLite fixtures and production authorization paths. Prefix new tests with TestAssistant or TestOrchestrationAssistant so task filters cannot silently select unrelated tests. Verify selected test output contains the expected new names.

| Scenarios | Target file(s) | Method |
| --- | --- | --- |
| S05–S06 | runtime/intake_test.go; repository/sqlite/intake_test.go | Real DB, duplicate HTTP intake, transaction fault injection, >100 comments, stale revision |
| S02–S04, S13 | runtime/objectives_test.go; backendapp/adapters_workspace_tasks_test.go | Mode/step validation, retained profiles, two sessions, current acceptance evidence |
| S07–S08, S20 | runtime/context_test.go; repository/sqlite/memory_test.go | Deterministic bounded packets, many newer memories, synthetic resolver, forget/revision race |
| S14–S15 | runtime/capabilities_test.go; plugins/manifest/*_test.go; mcp/server/*test.go | Disconnected/revoked tools, schema/effect denial, adversarial tool content, no secrets |
| S09–S10, S19 | runtime/attention_test.go | Event duplication/out-of-order/restart, no task movement, multiple sessions, no-op token count |
| S11–S12 | runtime/attention_resolution_test.go; backendapp/adapters_assistant_input_test.go | Native questions/permissions, expired requests, double tabs, response timeout |
| S16–S17 | runtime/friction_test.go | Synthetic incidents, scope/version isolation, no grant, local-only repair receipt |
| S18 | runtime/workspace_grants_test.go; repository/sqlite/workspace_grants_test.go | Two users/workspaces/accounts, current grant check, revocation during dispatch |
| S01, all interactive states | app/assistant/*.test.tsx; hooks/domains/orchestration/use-assistant.test.ts | UI/client state and injected native resolution controls |
| S21 | backendapp/e2e_reset_test.go | Fixture-owned assistant cleanup without altering unrelated profiles |

Targeted package tests in task files are the per-change checks. Do not add an unrelated whole-repository review after every commit. The first live candidate has a separately defined full qualification gate in delivery task 02.

## E2E tests and experiments

Add personal-assistant.spec.ts, personal-assistant-attention.spec.ts, personal-assistant-capabilities.spec.ts and mobile-personal-assistant.spec.ts under apps/web/e2e/tests/orchestration.

- S01/S02/S03: open the same assistant from desktop/mobile; inspect without task/plan creation; authorized execute selects the explicit step.
- S04/S09/S11/S12/S13: two worker sessions, typed clarification and permission without board movement, one central answer, task-tab convergence, REVIEW not complete.
- S05/S06/S10/S19/S20: rapid follow-ups, response loss, duplicate events, backend restart, pause, forgotten memory and offline/reconnect preserve correct receipts.
- S07/S08: two tasks receive a fake scoped credential descriptor; no secret appears in DOM, persisted prompts or recorded tool arguments.
- S14/S15: fixture plugin read succeeds, write/unknown-effect denies; disabling mid-session revokes; malicious returned instructions do not widen access.
- S16/S17/S18: one deduplicated repair candidate, visible maintenance gate, isolated local commit receipt only; workspace-link revoke prevents a pending write.
- S21: full directory twice in one worker, then critical files in reverse order; no stale orchestrators or changed seeded profile routing. Do not fix isolation by weakening exact counts.

Use the managed e2e runner with fresh builds; --no-build only when the exact tested artifacts were just built. No arbitrary sleeps, production reset endpoint, real vault or live external writes. After deterministic coverage, run an isolated live-provider read-only trial only on a provider/executor that proves constrained tooling; if unsupported, record that blocked capability rather than claiming success.

## Verification results

Design review completed:
- Baseline internal/orchestration tests passed.
- Targeted runtime and backendapp task-adapter checks passed.
- Three temporary probes reproduced attention, memory-selection and source-message gaps; removed afterward.
- Existing automation browser test passed.
- Existing workspace browser test passed standalone (desktop/mobile included).
- The current internal/orchestration dependency graph contains no Office packages.
- At the original experiment checkpoint, the combined browser run failed due to
  an extra persisted orchestrator: 1 passed / 1 failed. The fixture cleanup was
  subsequently repaired; both coordinator browser specs passed together without
  retries at the v0.94.0 integration commit `b1cd0d2e` on 2026-09-17. This does not
  complete the pending assistant UI or read-only experiment work orders.
- Detailed commands, artifacts and trial limits: [experiments.md](experiments.md).

Implementation verification is recorded in tasks 01–03. The binding-switch privacy regression first failed with foreign history exposure, then passed after the retained-owner repair. Its focused race check passed 23 top-level tests; task 01's original filter passed 27 runtime tests and task 03's filter passed 10 tests. Native workspace-scoping, migration and redaction race checks also passed. Those historical implementation checks involved no subagents, public PR, deployment or live-provider experiment. The later private publication is recorded separately in the review packet.

## Implementation waves and task files

Execute sequentially in the primary conversation. Waves show dependencies only; none authorizes subagents. Shared schemas/runtime handlers mean these are not automatically parallel-safe.

| Wave | Work orders | Current state |
| --- | --- | --- |
| 1 | [01 Durable intake and ownership](task-01-durable-intake.md) | Done, backend |
| 2 | [02 Objectives and proportional routing](task-02-objectives-routing.md); [03 Scoped memory and handoffs](task-03-memory-context.md) | 02 and 03 done |
| 3 | [04 Native capability inventory](task-04-capability-inventory.md); [06 Attention reconciliation](task-06-attention.md) | 04 and 06 done |
| 4 | [05 Enforced inspection](task-05-tool-authority.md) | Done |
| 5 | [07 Native resolution and controls](task-07-input-resolution.md) | Done |
| 6 | [08 Assistant interface](task-08-assistant-ui.md) | Pending |
| 7 | [09 Supervised improvements](task-09-workflow-improvements.md) | Pending |
| 8 | [10 Workspace grants](task-10-workspace-grants.md) | Pending |
| 9 | [11 Combined evidence](task-11-read-only-e2e.md) | Pending |

The chosen execution order is 03 → 04 → 05 → 06 → 07 → 08 → 09 → 10 → 11.
Wave 3 shows that attention can technically follow existing task 02 independently
of capability enforcement; sequential execution avoids shared-file conflicts.
Delivery task 01 supplies the assistant rollout gate before any live candidate.

## Risks and boundaries

- The preserved implementation spans core packages; keep logical changes in focused commits and preserve mandatory privacy/queue boundaries when extracting contributions.
- Inspect safety cannot be asserted by a prompt or a readOnlyHint. Unsupported provider-native tools are a real blocker for that execution mode.
- Some permission/clarification handles expire across restart; persistence must not resurrect expired authority.
- Upstream side effects without idempotency cannot be made exactly-once; retain unknown outcomes.
- Multiworkspace summaries export context to the central profile. Require explicit permission; changing profiles must not silently retain an incompatible export grant.
- Mandatory repository planning policy is separate from workflow stage configuration. A future policy-change task needs explicit scope.
- Both features.orchestration and the planned features.personalAssistant default off. This plan authorizes no production migration/deployment or adoption of all existing work.
- Provider-specific permission failures need evidence identifying the authoritative classifier. Do not preselect a bypass or promise a provider-policy fix.

## Handoff checkpoint

The original design-package handoff and subsequent ownership escalation were completed. The user explicitly requested the ownership repair, which is now implemented and verified; see [ownership-checkpoint.md](ownership-checkpoint.md). Task 06 is complete; continue task 07 without restarting completed investigation or discarding the existing implementation.
