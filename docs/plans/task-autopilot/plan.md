---
spec: docs/specs/tasks/requirements/autopilot-mode.md
created: 2026-08-08
status: implemented
---

# Implementation Plan: Task Autopilot Mode

## Overview

Persist autopilot as an immutable task-creation choice, derive a matching prompt
and task-mode MCP tool inventory, and add a durable non-blocking parent-question
lifecycle. Surface the identity and waiting state through shared desktop/mobile
task components, then cover the complete create → ask → answer → resume path with
backend, frontend, restart, and Playwright tests.

Issue: [#2425](https://github.com/kdlbs/kandev/issues/2425)

MCP profile decision: [ADR-2026-08-08-mcp-tool-profiles](../../decisions/2026-08-08-mcp-tool-profiles.md)

## Design constraints

- `autopilot` defaults to false, never implicitly inherits, and cannot be edited
  after creation.
- The persisted task is the source of truth. Prompts, tool discovery, APIs, boot
  payloads, and UI projection must not derive the value independently.
- Autopilot replaces the blocking operator-question capability with a structured,
  immediate-return parent-question capability.
- Question capabilities are mutually exclusive: normal tasks get the user-question
  tool, autopilot children get the parent-question tool, and autopilot roots get no
  question tool.
- Questions route to the current direct parent. A parent may relay upward through a
  separate question; top-level autopilot agents proceed without asking the user.
- A pending question gates workflow completion and normal queue draining until a
  correlated parent answer, explicit superseding input, or terminal transition.
- Existing non-autopilot clarification and peer-message behavior must remain
  unchanged.
- The first release rejects incompatible non-task/Office/passthrough runtime modes
  rather than silently providing partial autopilot semantics.
- MCP discovery uses a backend-owned profile registry. A base surface selects the
  task, Office, configuration, or external tool groups. Typed capability groups add
  or remove context-specific tools without copying a complete mode branch.

## Backend

### Persist and publish the task property

Likely files:

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/requests.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/handlers/handlers.go`

Add a non-null `autopilot_enabled` task column with a false default, carry it
through scan/insert/update/read DTO paths, and accept `autopilot` in HTTP and MCP
creation requests. Keep it out of mutation DTOs. Validate the resolved agent
profile/runtime mode before committing an autopilot task. Extend server schemas,
argument validation, handler forwarding, and API contract tests together so MCP
discovery and execution cannot drift.

### Derive the runtime prompt and tools

Likely files:

- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/internal/sysprompt/sysprompt.go`
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/agent/runtime/agentctl/control.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/tool_profiles.go`
- `apps/backend/internal/mcp/server/tool_profiles_test.go`

Carry the resolved profile context in explicit launch/runtime configuration to
agentctl. Build the autopilot prompt section at prompt-construction time, including
autonomy, last-action, direct-parent, and top-level behavior. Replace the current
mode/boolean registration branch with a declarative profile registry:

- `kanban-task` keeps the current Kanban task, plan, review, workspace, and
  diagnostics groups;
- `office-task` keeps the slimmer Office/skill/CLI surface and no Kanban task
  creation tools;
- `configuration` keeps configuration groups;
- `external` keeps configuration plus task creation and no live-session tools;
- additive groups handle `user-question`, `parent-question`, `task-title`, and
  provider automation.

Question groups remain mutually exclusive: parent question for an autopilot child,
no question group for an autopilot root, and user question for a normal task. Use
one typed profile context instead of independent enable/disable booleans or an
agent-supplied arbitrary allowlist. Keep `SetMcpMode` and `SetMcpProviders` as
compatibility paths while adding the backend-owned profile update path. Tests must
cover first launch, resume/restart, reparenting, live list replacement, and every
executor transport used by task sessions.

### Add the parent-question lifecycle

Likely files:

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/statussummary/model.go`
- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/rebuild.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification.go`
- `apps/backend/internal/orchestrator/clarification_guard.go`
- `apps/backend/internal/clarification/types.go`
- `apps/backend/internal/clarification/store.go`
- `apps/backend/internal/clarification/canceller.go`

Register and validate the structured ask-parent tool. Persist a hidden typed message
with correlation and lifecycle metadata before sending an attributed peer prompt to
the resolved parent. Extend `message_task_kandev` with
`reply_to_question_id`; authorize the recorded parent and atomically resolve and
resume the child. Use the per-session serialization boundary for answer,
superseding prompt, terminal transition, workflow completion, and queue-drain races.

Project a pending parent question to the existing clarification pending-action
category while suppressing the operator clarification overlay. Extend status-summary
rebuild and restart recovery. Test duplicate asks/replies, unavailable or reparented
parents, full queues, crash boundaries, stale answers, and the rule that no second
child turn or parent prompt is created.

## Frontend

Likely files:

- `apps/web/lib/types/http.ts`
- `apps/web/lib/api/domains/kanban-api.ts`
- `apps/web/lib/kanban/map-task.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/components/task-create-dialog-form-body.tsx`
- `apps/web/components/task-create-dialog-prop-builders.ts`
- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task/new-subtask-dialog.tsx`
- `apps/web/components/task/new-subtask-form-state.ts`
- `apps/web/components/task/new-subtask-form-parts.tsx`
- `apps/web/components/task/use-subtask-submit.ts`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/pseudo/task.json`
- `apps/web/src/locales/zh-cn/task.json`

Add `autopilot` to the shared task type and mapping. Keep top-level task creation
UI free of an Autopilot control for now; expose an off-by-default compact switch
only in the subtask creation dialog and include it in the creation request. Give
the switch a localized hover/focus help control. In the shared chat status row,
render a yellow localized badge for autopilot tasks. In the shared task item, keep
the existing primary status icon (including the question indicator) and add a
secondary autopilot icon with localized hover/focus tooltip and accessible name.

All new copy goes through `t()`. The pseudo-locale and i18n checks must cover the
chip, subtask switch label/help, and tooltip. Existing tasks whose payload omits the
field map to false during rolling compatibility.

## Mobile design contract

- **Desktop outcome:** creation dialog switch, status-row chip, and sidebar identity
  icon; a pending parent question uses the current primary question icon.
- **Mobile entry point:** the existing responsive task creation dialog and the task
  switcher sheet; no new route or desktop-only hover action.
- **Nearest exemplars:** `task-create-dialog-form-body.tsx` and
  `new-subtask-form-parts.tsx` for create-only fields, and the existing shared
  `task-item.tsx` status/question rendering used by the mobile task switcher.
- **Presentation:** keep the switch inline because it is one reversible creation
  choice; do not add a nested drawer. The informational sidebar icon has no action,
  so touch users receive its accessible/visible identity without a tooltip-only
  dependency.
- **Hierarchy and scrolling:** the primary task-state icon keeps precedence;
  autopilot is a secondary identity mark. The composer chip participates in the
  existing wrapping status row, and neither the dialog nor switcher introduces a
  horizontal scroll container.
- **Touch target:** the switch row remains at least 44 px high and the whole labeled
  row toggles the control. Disabled/incompatible profiles explain why before submit.
- **Shared state:** desktop and mobile read the same mapped task boolean and pending
  action; no viewport-specific autopilot store exists.

## Tests

- Repository/model tests cover migration defaults, round trips, and immutability.
- HTTP and MCP tests cover omitted/false/true creation, nested task creation,
  incompatible profiles, the exact short autopilot description, and schema/handler
  synchronization.
- System-prompt and MCP inventory tests prove normal and autopilot sessions expose
  the correct mutually exclusive question capability on launch and resume, including
  no question tool for an autopilot root.
- Profile registry tests prove the Kanban, Office, configuration, and external
  surfaces, additive title/provider groups, atomic replacement, and one
  `tools/list_changed` notification per effective profile change.
- Parent-question service/orchestrator tests cover direct-parent routing, turn-end
  gating, answer correlation, idempotency, races, stale/superseded state, and
  restart reconstruction.
- Frontend tests cover request serialization, DTO mapping, create-only switch
  visibility, badge styling/copy, both sidebar icons, tooltip/focus semantics, and
  missing-field compatibility.

## E2E

Add a deterministic mock-agent scenario that creates an autopilot child, invokes
the parent-question tool, ends its turn, receives a correlated parent answer, and
resumes once. A desktop Playwright spec verifies creation and the full lifecycle. A
mobile spec verifies the creation control, chat chip, autopilot identity icon,
pending question icon, answer clearing, and no horizontal overflow. Include a
backend restart while pending if the existing fixture can restart without weakening
isolation; otherwise keep restart proof in Go integration tests.

## Public documentation

Update:

- `docs/public/automation-and-mcp.md`
- `docs/public/agent-communication.md`
- `docs/public/coordination.md`

Document the creation parameter, immutable first-release semantics, profile
surfaces, context-specific capability groups, ask-parent payload, correlated reply,
direct-parent ownership, top-level behavior, and visible waiting state. Examples
must not imply that an autopilot child can call the operator clarification tool or
that an Office task can create Kanban tasks through MCP.

## Waves

Wave 1:

- [x] [task-01-persist-task-contract](task-01-persist-task-contract.md)

Wave 2:

- [x] [task-02-derive-runtime-contract](task-02-derive-runtime-contract.md)
- [x] [task-04-build-autopilot-ui](task-04-build-autopilot-ui.md)

Wave 3:

- [x] [task-03-build-parent-question-lifecycle](task-03-build-parent-question-lifecycle.md)
- [x] [task-06-document-autopilot](task-06-document-autopilot.md)

Wave 4:

- [x] [task-05-prove-autopilot-e2e](task-05-prove-autopilot-e2e.md)

## Verification

Targeted backend:

```bash
(cd apps/backend && go test ./internal/task/... ./internal/mcp/server/... ./internal/mcp/handlers/... ./internal/sysprompt/... ./internal/orchestrator/... ./internal/clarification/...)
```

Targeted frontend:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-defaults.test.ts components/task-create-dialog-form-body.test.tsx components/task/new-subtask-form-parts.test.tsx components/task/new-subtask-form-state.test.ts components/task/use-subtask-submit.test.ts components/task/task-item.test.tsx components/task/chat/chat-input-area.test.tsx lib/kanban/map-task.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

E2E:

```bash
cd apps/web && pnpm e2e:run --project chromium tests/task/autopilot-mode.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-autopilot-mode.spec.ts
```

Public docs:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Verification results

Implemented. Verification passed: 2,263 Go tests across 9 focused packages; 156
focused web tests; web typecheck and lint; i18n checks/ratchet; public docs
validation (58 tests, 41 pages); and desktop/mobile Playwright autopilot scenarios.
The fixup adds profile-capability selection tests, parent-question ownership and
claim/restore coverage, omitted-field preservation tests, and the autopilot hook
fallback test.

## Risks

- Tool discovery and prompt composition travel through several launch transports;
  missing one executor path would create a split contract after resume.
- Parent questions compose with clarification cancellation, queue admission, and
  workflow turn completion. The pending gate must share existing per-session
  serialization rather than introducing a parallel lock.
- Durable delivery and answer idempotency need stable identities; retrying after a
  crash must never generate a second parent request or child answer turn.
- Reparenting while a question is pending must fail closed. The implementation must
  either keep the recorded parent authoritative for that question or mark it stale
  before accepting a new parent; it must never silently accept both.
