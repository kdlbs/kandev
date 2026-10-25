---
spec: docs/specs/integrations/requirements/provider-aware-review-automation.md
created: 2026-08-03
status: implemented
---

# Implementation Plan: Provider-Aware Review Automation

## Overview

Make task-mode MCP discovery match the providers attached to the task, carry that
capability set through every managed runtime path, and refresh it after live
source additions. In the same focused backend package, remove redundant GitLab
MR evaluation reads, bound cancellation-independent follow-up work, and replace
manual MCP handler-count arithmetic with a measured dispatcher delta.

The work changes internal contracts only. Public GitHub/GitLab automation
payloads and existing authorization checks remain intact.

## Provider-scoped MCP server contract

- Extend the task-mode MCP server with a normalized set of supported providers.
  Registration of the GitHub PR and GitLab MR automation pairs is conditional on
  that set; all other mode-specific tools retain their current behavior.
- Add `SetProviders` alongside `SetMode`. Both operations rebuild the effective
  registry under the existing synchronization, preserve the other dimension,
  and rely on one atomic mcp-go replacement to publish the tool-list-changed
  notification. Equivalent normalized updates are no-ops.
- Add optional `mcp_providers` transport fields to agentctl instance creation,
  configuration, and overrides. Add `PUT /api/v1/mcp/providers` plus the runtime
  client method used for active instances.
- Normalize once at boundaries, ignore unsupported values, and make an absent or
  invalid set expose neither provider-specific pair. Do not default to both.
- Test exact normal/task-mode membership for GitHub-only, GitLab-only, mixed,
  empty, and unknown sets; test that a live replacement preserves mode and
  changes the advertised tools.

## Launch and resume propagation

- Derive the provider union from every resolved repository entity already
  available to `buildLaunchAgentRequest`. Keep provider identity independent of
  repository ordering and primary-repository selection.
- Carry `McpProviders []string` through executor launch requests, backendapp
  adapters, lifecycle launch/executor-create requests, and standalone, Docker,
  container, Sprites, and SSH agentctl mappings.
- Use the same derivation on initial launch and resume. Do not infer the set from
  materialized Git remotes in lifecycle or agentctl.
- Add mapping tests at the derivation and lifecycle/executor seams so a newly
  added executor cannot silently drop the capability set.

## Refresh after live source attachment

- Introduce one backendapp refresher that reads the task's authoritative
  provider identities with a joined task-repository/repository query, derives
  the normalized provider union, resolves active task sessions, and asks
  lifecycle to replace each running agentctl instance's MCP providers.
- Call the refresher once after the shared workspace-source batch commits and
  materializes successfully, and once after the legacy add-branch path finalizes.
  Avoid one refresh per repository inside a batch.
- Add a lifecycle method that resolves the active execution by session ID and
  calls the agentctl client's live provider endpoint.
- Source attachment remains successful if refresh fails. Preserve the old
  runtime subset, log task/session context and the error, and rely on launch or
  resume for reconciliation.
- Cover no-active-session, multi-session, mixed-provider addition, and refresh-
  failure behavior without weakening the source transaction's compensation
  rules.

## Lean GitLab lifecycle evaluation

- Preserve the existing public `SyncTaskMR` return contract while adding an
  internal result/helper that exposes the reviewer list already returned by
  `GetMRStatus`.
- Extend `TaskMRUpdatedEvent` with an optional reviewer observation and an
  explicit validity field. Populate it for normal subscribed polls. Keep
  decoding compatible with old events and verify map/NATS JSON round-trip for
  populated and observed-empty lists.
- Add an internal evaluation snapshot query that returns the task automation
  options plus only the exact lifecycle checkpoint for repository/project/IID.
  Do not reuse the full public response loader that scans all task MR states.
- Resolve the authenticated reviewer identity from the task's workspace GitLab
  configuration `Username`. The configuration save/test/health paths continue to
  own refresh of that value; evaluation does not call `GetAuthenticatedUser` per
  MR.
- Reuse a valid reviewer observation. If the event has none, retain the strict
  `GetMR` fallback required for option-change and legacy event paths.
- Add provider-call and store-query-count assertions: the normal subscribed poll
  uses one MR status fetch, no second MR fetch, no authenticated-user lookup, and
  no task-wide lifecycle-state read.

## Bound post-side-effect contexts

- Define one short follow-up timeout for checkpoint persistence, automation error
  recording, response loading, and event publication.
- For each follow-up operation, derive a fresh timeout from
  `context.WithoutCancel(ctx)` and cancel it immediately after use. This preserves
  independence from poll cancellation without stripping every deadline.
- Keep the existing detached dispatch deadline, dispatch/checkpoint ordering, and
  at-least-once semantics.
- Use blocking fakes to prove timeout return and per-MR singleflight release for
  store and event-bus stalls.

## Derive handler inventory count

- Add a concurrency-safe `HandlerCount` operation to the WebSocket dispatcher.
- In MCP `RegisterHandlers`, record the dispatcher size before registration and
  log the delta afterward. Delete manual base counts and incremental adjustments.
- Test dispatcher counting and the full registration delta so future handler
  additions require no parallel bookkeeping.

## Documentation

- Update `docs/public/automation-and-mcp.md` when implementation lands to state
  that GitHub PR and GitLab MR task automation tools are shown from the union of
  attached repository providers and can change after source attachment.
- Keep public automation payload documentation unchanged. Link the focused spec
  and ADR from their indexes.

## Tests

- **Provider membership:** table-driven MCP server tests for single, mixed,
  empty, and unsupported provider sets, plus live replacement and mode
  preservation.
- **Transport integrity:** launch/resume and every executor mapping carry the
  normalized provider union to agentctl.
- **Live reconciliation:** source-batch and legacy-branch additions refresh once;
  refresh failure leaves attachment committed and restart derivation correct.
- **GitLab read budget:** subscribed poll/evaluation reuses reviewer observations,
  uses persisted identity, and targets one checkpoint; observation-absent events
  use the strict fallback.
- **Event compatibility:** reviewer observations, including a valid empty list,
  survive internal serialization; old events still decode.
- **Liveness:** blocking follow-up dependencies time out and release
  singleflight.
- **Handler inventory:** dispatcher and handler registration report measured
  counts without constants.

## Implementation waves

Wave 1:

- [x] [Task 01: Scope task MCP tools by provider](task-01-scope-task-mcp-tools-by-provider.md)
- [x] [Task 04: Make GitLab lifecycle evaluation lean](task-04-make-gitlab-lifecycle-evaluation-lean.md)
- [x] [Task 06: Derive MCP handler inventory](task-06-derive-mcp-handler-inventory.md)

Wave 2:

- [x] [Task 02: Propagate providers through agent launch](task-02-propagate-providers-through-agent-launch.md)
- [x] [Task 05: Bound automation follow-up contexts](task-05-bound-automation-follow-up-contexts.md)

Wave 3:

- [x] [Task 03: Refresh tools after source attachment](task-03-refresh-tools-after-source-attachment.md)

Wave 4:

- [x] [Task 07: Document provider-scoped task MCP](task-07-document-provider-scoped-task-mcp.md)

Execute sequentially by default in the delegated implementation session. The
wave grouping only identifies dependency-safe candidates if the user later
authorizes parallel work.

## Validation commands

- `cd apps/backend && go test ./internal/mcp/server ./internal/agentctl/server/api ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agent/runtime/agentctl`
- `cd apps/backend && go test ./internal/orchestrator/executor ./internal/backendapp ./internal/agent/runtime/lifecycle`
- `cd apps/backend && go test ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/task/service`
- `cd apps/backend && go test ./internal/gitlab ./internal/orchestrator`
- `cd apps/backend && go test ./pkg/websocket ./internal/mcp/handlers`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`

## Risks and non-goals

- Rebuilding MCP tools must be synchronized with mode changes. Tests should
  exercise both update orders and run relevant packages with `-race` when
  practical.
- The provider set crosses many executor mappings. Central types and seam tests
  reduce the risk of one runtime silently retaining an empty set.
- Live source refresh is deliberately best effort after persistence. Rolling back
  materialized sources because agentctl is unavailable would violate the source-
  attachment transaction's existing guarantees.
- Reviewer observations are internal and ephemeral. Avoid adding persistence or
  public response fields while refactoring the sync result.
- This plan does not add GitLab auto-fix/auto-merge, unify PR/MR tool contracts,
  make notification dispatch exactly once, alter UI surfaces, parallelize the
  poller, or handle live repository removal.
