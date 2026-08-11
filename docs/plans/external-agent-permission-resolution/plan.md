---
spec: docs/specs/agents/external-permission-resolution.md
decision: docs/decisions/2026-08-11-live-agent-permission-authority.md
created: 2026-08-11
status: complete
---

# Implementation Plan: External Agent Permission Resolution

## Overview

Add a generation-safe live permission contract at agentctl, carry it through lifecycle into an
authorized orchestrator service, and serialize resolutions with a durable transcript claim before
the provider receives an option. The external MCP tools then become thin typed adapters over that
service. The existing web response path adopts the same request identity without changing its UI,
and public/internal documentation describes the new contract and privacy boundary.

This checkout is based on upstream `kdlbs/kandev:main` commit
`66eb87ac307db76b8cb3ba5fcfae73ff6d7d3e6c` as verified on 2026-08-11. Before implementation,
fetch upstream again and fast-forward/rebase the feature branch if `main` moved.

## Existing-solution audit

- Upstream issues [#717](https://github.com/kdlbs/kandev/issues/717) and
  [#1657](https://github.com/kdlbs/kandev/issues/1657) concern stale approval cards and pending UI
  indicators, not external enumeration/resolution.
- Merged PRs [#2424](https://github.com/kdlbs/kandev/pull/2424),
  [#2445](https://github.com/kdlbs/kandev/pull/2445), and
  [#2442](https://github.com/kdlbs/kandev/pull/2442) supply pending-action cleanup, MCP profile,
  and external session-listing patterns but no permission tool.
- Merged PR [#1037](https://github.com/kdlbs/kandev/pull/1037) fixed rendering approval buttons
  for Kandev MCP tool calls inside the existing web transcript. It did not expose live requests to
  the external MCP endpoint.
- Searches of open upstream PRs and public fork code for pending-permission list/resolve tool names,
  `agent.permissions.respond` MCP adapters, and equivalent schemas found no complete competing
  implementation as of 2026-08-11.

---

## Backend

### Live agentctl request identity and presentation

- Add `RequestID` to permission events in
  `apps/backend/internal/agentctl/types/streams/agent.go` and define typed public snapshots,
  strict resolve requests/results, states, and sentinel errors beside
  `apps/backend/internal/agentctl/types/streams/permission.go`.
- In `apps/backend/internal/agentctl/server/process/manager.go`, generate a UUID request generation
  for every non-auto-approved prompt, deep-copy its options, and store `pending`/`resolving` state
  under `permissionMu`.
- Add `ListPendingPermissions` and `ResolvePendingPermission`. Listing returns immutable copies
  sorted by creation time/request ID. Resolution holds the mutex while validating request ID,
  pending ID, state, and exact option membership, transitions once, and never looks up by title or
  command.
- Keep the existing internal cancellation fallback for prompts with no reject option, but do not
  expose cancellation in the strict external contract.
- Add credential-text redaction and allowlisted action projection under
  `apps/backend/internal/common/securityutil/`. Commands/titles/URLs are sanitized and bounded;
  environment, headers, raw MCP arguments, option metadata, file contents, and diffs are omitted.
  Redaction failure drops the field.
- Add `agent.permissions.list` and strict identity fields on `agent.permissions.respond` in
  `apps/backend/internal/agentctl/server/api/agent.go`, then add matching client methods in
  `apps/backend/internal/agent/runtime/agentctl/agent.go`. Map unknown, stale, replayed, and
  unknown-option failures to distinct stable error codes.

### Durable permission audit claim

- Extend the permission-request message metadata written by
  `apps/backend/internal/backendapp/adapters.go` with `request_id` while retaining existing
  `pending_id`, options, action data, and status fields for compatibility.
- Add typed `PermissionResolutionAudit` and result constants in
  `apps/backend/internal/task/models/models.go`.
- Extend `repository.MessageRepository` and
  `apps/backend/internal/task/repository/sqlite/message.go` with atomic methods that:
  - claim a pending permission message only when task session, request ID, pending ID, and current
    audit state match;
  - distinguish claimed, already resolved, and resolution-in-progress;
  - finalize only the same `claim_id`;
  - work through the repository's SQLite/Postgres dialect abstraction.
- Add task-service wrappers in `apps/backend/internal/task/service/service_messages.go` that publish
  the existing `message.updated` event after a claim/finalization. The pre-delivery claim records
  actor, source, exact option identity, and `dispatching`; finalization records accepted, stale,
  failed, or indeterminate. A claim write failure prevents runtime delivery.
- Never write raw action details, command strings, environment values, PAT values, or token IDs
  into the resolution audit. Structured logs contain IDs, option kind, source, and result only.

### Authorized permission service and runtime path

- Carry `request_id` through
  `apps/backend/internal/agent/runtime/lifecycle/event_types.go`,
  `apps/backend/internal/agent/runtime/lifecycle/events.go`, the watcher permission payload, and
  `MessageCreator.CreatePermissionRequestMessage`.
- Add list/strict-resolve methods through lifecycle manager and executor interfaces. Every
  session lookup uses the server-owned current execution and verifies its task/session ownership;
  no MCP handler imports process-manager state.
- Move permission coordination into a focused orchestrator service file, for example
  `apps/backend/internal/orchestrator/agent_permissions.go`, with:
  - `ListPendingAgentPermissions(ctx, taskID, sessionID)`;
  - `ResolveAgentPermission(ctx, request)`;
  - stable domain errors for not-found, stale, replay, in-progress, option, audit, and delivery
    failures.
- Call `authorizeTaskSessionPair` before runtime/message access when a session is supplied. For a
  task-wide list, authorize the task, enumerate its task sessions through the task service, then
  query only their current live executions. Any enumeration failure returns an error rather than a
  misleading partial list.
- Derive the audit actor from `authn.IdentityFromContext`: browser session, PAT, synthetic identity,
  or trusted internal automation. User IDs may be stored; PAT secret material and token IDs may not.
- Refactor `Service.RespondToPermission` and the existing `permission.respond` WebSocket handler to
  use the same strict service. Add `task_id` and `request_id` to the response request; keep the
  existing reject/cancel semantics and status projection for the web UI.
- When runtime delivery succeeds but audit finalization fails, return a delivery/audit error while
  retaining the durable dispatch claim as the replay barrier. Do not retry against another live
  request.

### External MCP handlers and tools

- Define a narrow `AgentPermissionService` interface and setter in
  `apps/backend/internal/mcp/handlers/handlers.go`; wire the orchestrator implementation from
  `apps/backend/internal/backendapp/helpers.go`.
- Add backend WebSocket actions in `pkg/websocket` plus
  `apps/backend/internal/mcp/handlers/agent_permissions.go`. Handlers validate required fields,
  call only the service, preserve not-found privacy, and map domain errors to stable descriptive MCP
  failures.
- Register `list_pending_agent_permissions_kandev` and
  `resolve_agent_permission_kandev` only on `SurfaceExternal` in
  `apps/backend/internal/mcp/server/server.go`. The schemas require the exact fields from the spec;
  the mutation schema has no cancellation, command, arbitrary metadata, or free-form option field.
- Add server forwarding handlers and profile/registration tests. Discovery remains informational;
  backend authorization is mandatory on every invocation.

---

## Frontend

### Existing permission response compatibility

- Add `request_id` to `PermissionRequestMetadata` in
  `apps/web/components/task/chat/messages/use-permission-handlers.ts`.
- Send the message's `task_id`, task-session `session_id`, `request_id`, `pending_id`, and selected
  option through `permission.respond`. Keep the current allow-once, allow-always, reject, no-reject
  cancellation, loading, and error behavior unchanged.
- Update `use-permission-handlers.test.ts` to assert the complete identity tuple and retain the
  existing option-selection cases.

### External MCP catalog

- Add both tool names to the task group in
  `apps/web/lib/settings/external-mcp-tools.ts` and remove/update its known-drift note if the
  implementation reconciles the full external tool set.
- Add localized descriptions to `src/locales/en/settings.json`,
  `pseudo/settings.json`, `pt-pt/settings.json`, and `zh-cn/settings.json`; update the catalog test
  and count.
- This is copy/data-only inside the existing responsive Settings surface. It changes no layout,
  navigation, overlay, touch target, scrolling, or viewport behavior, so targeted catalog tests and
  a focused rendered Settings check satisfy mobile parity; no new mobile Playwright scenario is
  required.

---

## Tests

- **Live identity and strict option validation**
  - **File:** `internal/agentctl/server/process/manager_permission_test.go`
  - **How:** table-driven/concurrent unit tests for ordered snapshots, deep copies, exact option
    validation, request replacement with reused pending ID, duplicate response, concurrent response,
    cancellation, and cleanup.
- **Safe presentation and redaction**
  - **File:** `internal/common/securityutil/permission_test.go`
  - **How:** table-driven tests containing PATs, bearer headers, API-key flags, URL user info/query,
    env maps, MCP arguments, option metadata, file diffs, and unknown provider shapes; assert no
    canary secret occurs in marshaled output or logs.
- **Agentctl API/client contract**
  - **Files:** `internal/agentctl/server/api/agent_test.go`,
    `internal/agent/runtime/agentctl/agent_test.go`
  - **How:** request/response tests for list, strict resolve, error-code preservation, and no raw
    details in JSON.
- **Audit CAS and replay barrier**
  - **Files:** `internal/task/repository/sqlite/message_test.go`,
    `internal/task/service/service_messages_test.go`
  - **How:** real SQLite plus Postgres-dialect SQL tests for first claim, competing claim, wrong
    request/pending/session, exact-claim finalization, terminal replay, and finalization failure.
- **Authorization and service behavior**
  - **Files:** `internal/orchestrator/agent_permissions_test.go`,
    `internal/orchestrator/session_scope_matrix_test.go`
  - **How:** service fakes/real message repo covering happy list/resolve, wrong user/task/session,
    unknown option, no execution, no pending request, replaced/expired request, duplicate/replay,
    audit failure before delivery, delivery failure after claim, and actor/result audit fields.
- **MCP handler-to-service integration**
  - **Files:** `internal/mcp/handlers/agent_permissions_test.go`,
    `internal/mcp/handlers/mcp_identity_scope_test.go`,
    `internal/integration/agent_permission_mcp_test.go`
  - **How:** typed handler tests plus an authenticated external-MCP path using a real task/message
    repository and fake live runtime. Assert cross-user invisibility, task/session binding, exact
    schema, stable errors, and one provider response.
- **Tool registration and schema**
  - **Files:** `internal/mcp/server/agent_permissions_test.go`, existing profile/tool-count tests
  - **How:** assert tools occur only on external surface and mutation schema exposes exactly five
    required string fields.
- **Web compatibility and catalog**
  - **Files:** `components/task/chat/messages/use-permission-handlers.test.ts`,
    `lib/settings/external-mcp-tools.test.ts`
  - **How:** hook request-payload tests, existing allow/reject cases, catalog membership/count, i18n
    checks, and the existing `e2e/tests/chat/permission-approval.spec.ts` regression.

## E2E Tests

No new visual scenario is required: the new primary surface is an MCP contract and the Settings
change is catalog copy inside an unchanged responsive component. Re-run
`apps/web/e2e/tests/chat/permission-approval.spec.ts` to prove the existing desktop permission flow
still resolves sequential prompts and Kandev MCP tool prompts. Backend integration tests own the
external list/resolve security and race guarantees.

## Documentation

- Update `docs/public/automation-and-mcp.md` (how-to/explanation) with the two external tools, a
  list/resolve example, PAT/task scoping, stale/replay errors, and the rule that clients must display
  only the returned immutable options.
- Update `docs/public/agents-and-profiles.md` (explanation/reference) to mention that a person may
  answer structured ACP permission prompts through authorized external MCP as well as the session UI,
  without changing auto-approval defaults.
- Update `docs/backend_agentctl_connectivity.md` with `agent.permissions.list`, request generation,
  and strict resolution fields.
- Correct the permission section in `docs/WEBSOCKET_API.md` to distinguish Kandev task-session IDs
  from ACP IDs and document `request_id` on notification/response.
- MCP JSON Schemas remain generated from `mcp.NewTool` declarations at runtime; registration/schema
  tests are the checked contract. No hand-edited generated schema artifact exists.

## Verification Results

- Task 01 passed its prescribed four-package Go test suite with the repository-pinned Go 1.26.0
  toolchain.
- Task 02 repository/backendapp suites and focused task-service audit tests passed. The full
  task-service suite has unrelated container parent-chain failures because `/` is owned by
  `nobody`; details are recorded in the task file.
- Task 03 passed its prescribed lifecycle/executor/orchestrator/handler/backendapp Go suite.
- Task 04 passed its handler, MCP server, and authenticated integration suites, including PAT scope,
  strict identity, replay rejection, and audit redaction coverage.
- Task 05 passed focused web tests (16), lint, typecheck, i18n and public-doc validation, backend and
  E2E builds, and the existing Chromium permission E2E (3 tests).

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-live-permission-contract](task-01-live-permission-contract.md)

Wave 2:

- [x] [task-02-permission-audit-claim](task-02-permission-audit-claim.md)

Wave 3:

- [x] [task-03-authorized-permission-service](task-03-authorized-permission-service.md)

Wave 4 (parallel candidates after task 03; user authorization required):

- [x] [task-04-external-mcp-tools](task-04-external-mcp-tools.md)
- [x] [task-05-web-and-documentation](task-05-web-and-documentation.md)

Tasks 04 and 05 are parallel-safe only after their shared backend request contract is fixed in task
03; they own disjoint MCP-server versus frontend/docs files. The default remains sequential work in
the primary conversation.
